package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cups-web/internal/auth"
	"cups-web/internal/ipp"
	"cups-web/internal/store"

	"github.com/OpenPrinting/goipp"
	"github.com/gorilla/mux"
)

type bingTestTransport func(*http.Request) (*http.Response, error)

func (f bingTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const bingTestImageID = "OHR.Test_ZH-CN123_1920x1080.jpg"

func bingTestIndex(day time.Time, path string, downloadable bool) []byte {
	local := day.In(bingChinaTime)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, bingChinaTime)
	return fmt.Appendf(nil, `{"images":[{"fullstartdate":%q,"title":"测试/照片","copyright":"Bing fixture","url":%q,"wp":%t}]}`,
		midnight.UTC().Format("200601021504"), path, downloadable)
}

func bingTestJPEG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 30, 20)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestBingWallpaperPreview(t *testing.T) {
	now := time.Now()
	imageData := bingTestJPEG(t)
	index := bingTestIndex(now, "/th?id="+bingTestImageID+"&pid=hp", true)
	imageRequests := 0
	client := bingWallpaperClient{httpClient: &http.Client{Transport: bingTestTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == bingIndexURL {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(index))}, nil
		}
		if req.URL.String() != bingOrigin+"/th?id="+bingTestImageID {
			t.Fatalf("unexpected image request: %s", req.URL)
		}
		imageRequests++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(imageData))}, nil
	})}}
	preview := httptest.NewRecorder()
	client.previewHandler(preview, httptest.NewRequest(http.MethodGet, "/api/scheduled-prints/bing/preview", nil))
	var result bingWallpaper
	if err := json.Unmarshal(preview.Body.Bytes(), &result); err != nil || preview.Code != http.StatusOK ||
		result.Title != "测试/照片" || result.Copyright != "Bing fixture" || result.Date != now.In(bingChinaTime).Format(time.DateOnly) {
		t.Fatalf("unexpected preview: %d %s (%v)", preview.Code, preview.Body, err)
	}
	if result.ImageURL != "/api/scheduled-prints/bing/image/"+bingTestImageID {
		t.Fatalf("unexpected preview URL: %s", result.ImageURL)
	}
	imageResponse := httptest.NewRecorder()
	imageRequest := httptest.NewRequest(http.MethodGet, result.ImageURL, nil)
	client.imageHandler(imageResponse, mux.SetURLVars(imageRequest, map[string]string{"imageID": bingTestImageID}))
	if imageResponse.Code != http.StatusOK || imageResponse.Header().Get("Content-Type") != "image/jpeg" ||
		!bytes.Equal(imageResponse.Body.Bytes(), imageData) {
		t.Fatalf("unexpected preview image: %d", imageResponse.Code)
	}
	invalid := httptest.NewRecorder()
	client.imageHandler(invalid, mux.SetURLVars(imageRequest, map[string]string{"imageID": "https://example.invalid/photo.jpg"}))
	if invalid.Code != http.StatusBadRequest || imageRequests != 1 {
		t.Fatalf("invalid image ID made an upstream request: status=%d requests=%d", invalid.Code, imageRequests)
	}
	otherImage := httptest.NewRecorder()
	client.imageHandler(otherImage, mux.SetURLVars(imageRequest, map[string]string{"imageID": "OHR.Other.jpg"}))
	if otherImage.Code != http.StatusNotFound || imageRequests != 1 {
		t.Fatalf("an image outside today's index was downloaded: status=%d requests=%d", otherImage.Code, imageRequests)
	}
	index = bingTestIndex(now, "/th?id="+bingTestImageID, false)
	unavailable := httptest.NewRecorder()
	client.imageHandler(unavailable, mux.SetURLVars(imageRequest, map[string]string{"imageID": bingTestImageID}))
	if unavailable.Code != http.StatusBadGateway || imageRequests != 1 {
		t.Fatalf("an unavailable image was downloaded: status=%d requests=%d", unavailable.Code, imageRequests)
	}

	nextDay := now.In(bingChinaTime).AddDate(0, 0, 1)
	index = bingTestIndex(now, "/th?id="+bingTestImageID, true)
	if _, err := client.today(context.Background(), nextDay); err == nil {
		t.Fatal("yesterday's wallpaper was accepted after midnight")
	}
	const nextImageID = "OHR.Next_ZH-CN234_1920x1080.jpg"
	index = bingTestIndex(nextDay, "/th?id="+nextImageID, true)
	next, err := client.today(context.Background(), nextDay)
	if err != nil || next.Date != nextDay.Format(time.DateOnly) || next.imageID != nextImageID {
		t.Fatalf("did not refresh after midnight: %+v, %v", next, err)
	}
}

func TestBingWallpaperSchedule(t *testing.T) {
	for _, tc := range []struct {
		name, schedule, failure string
		manual, paused, missed  bool
	}{
		{name: "once", schedule: store.ScheduleOnce},
		{name: "daily", schedule: store.ScheduleDaily},
		{name: "weekly", schedule: store.ScheduleWeekly},
		{name: "monthly at end of February", schedule: store.ScheduleMonthly},
		{name: "manual once", schedule: store.ScheduleOnce, manual: true},
		{name: "manual paused", schedule: store.ScheduleWeekly, manual: true, paused: true},
		{name: "missed run", schedule: store.ScheduleOnce, missed: true},
		{name: "once fetch timeout", schedule: store.ScheduleOnce, failure: "timeout"},
		{name: "old image", schedule: store.ScheduleDaily, failure: "stale"},
		{name: "no download", schedule: store.ScheduleDaily, failure: "unavailable"},
		{name: "invalid URL", schedule: store.ScheduleDaily, failure: "url"},
		{name: "bad JPEG", schedule: store.ScheduleDaily, failure: "jpeg"},
		{name: "uncertain submission", schedule: store.ScheduleDaily, failure: "uncertain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, user := scheduledTestStore(t)
			uploads := t.TempDir()
			now := time.Date(2026, time.February, 28, 9, 0, 0, 0, time.Local)
			due := now
			if tc.manual {
				due = now.Add(time.Hour)
			}
			if tc.missed {
				due = now.Add(-3 * time.Minute)
			}
			task := store.ScheduledPrint{
				Source: store.ScheduledSourceBing, UserID: user.ID, PrinterURI: "ipp://localhost/printers/test",
				ScheduleType: tc.schedule, ScheduleTime: "09:00", ScheduleWeekday: int(now.Weekday()), ScheduleDay: 31,
				RunAt: due.Format(time.RFC3339), NextRunAt: due.UTC().Format(time.RFC3339), Enabled: !tc.paused,
			}
			if err := ensureScheduleDefaults(&task); err != nil {
				t.Fatal(err)
			}
			if err := s.WithTx(context.Background(), false, func(tx *sql.Tx) error {
				var err error
				task.ID, err = store.InsertScheduledPrint(context.Background(), tx, &task)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			jpg := bingTestJPEG(t)
			day, path := now, "/th?id="+bingTestImageID
			if tc.failure == "stale" {
				day = now.AddDate(0, 0, -1)
			}
			if tc.failure == "url" {
				path = "https://example.invalid/th?id=" + bingTestImageID
			}
			index := bingTestIndex(day, path, tc.failure != "unavailable")
			upstreamRequests := 0
			previous := bingWallpapers
			t.Cleanup(func() { bingWallpapers = previous })
			bingWallpapers = bingWallpaperClient{httpClient: &http.Client{Transport: bingTestTransport(func(req *http.Request) (*http.Response, error) {
				upstreamRequests++
				switch req.URL.String() {
				case bingIndexURL:
					if tc.failure == "timeout" {
						return nil, context.DeadlineExceeded
					}
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(index))}, nil
				case bingOrigin + "/th?id=" + bingTestImageID:
					if tc.failure == "jpeg" {
						return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("not a jpeg"))}, nil
					}
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(jpg))}, nil
				default:
					t.Fatalf("unexpected upstream request: %s", req.URL)
					return nil, nil
				}
			})}}

			submitted := 0
			err := executeScheduledOccurrence(s, uploads, task, now, tc.manual, func(_ *store.Store, dir string, selected store.ScheduledPrint) (string, error) {
				submitted++
				data, err := os.ReadFile(filepath.Join(dir, selected.StoredPath))
				if err != nil || !bytes.Equal(data, jpg) || selected.Copies != 1 || selected.Orientation != "auto" {
					t.Fatalf("wrong scheduled image or settings: %+v, %v", selected, err)
				}
				claimed := readScheduledTestTask(t, s, task.ID)
				if claimed.Enabled || claimed.LastStatus != "running" {
					t.Fatalf("submission must be claimed before printing: %+v", claimed)
				}
				if tc.failure == "uncertain" {
					return "running", &ipp.SubmissionUncertainError{Err: errors.New("unknown result")}
				}
				return "printed", nil
			})
			if (err != nil) != (tc.failure != "") {
				t.Fatalf("unexpected execution error: %v", err)
			}
			got := readScheduledTestTask(t, s, task.ID)
			wantStatus, wantSubmissions := "printed", 1
			switch {
			case tc.missed:
				wantStatus, wantSubmissions = "skipped", 0
				if upstreamRequests != 0 {
					t.Fatalf("missed job contacted Bing %d times", upstreamRequests)
				}
			case tc.failure == "uncertain":
				wantStatus = "running"
			case tc.failure != "":
				wantStatus, wantSubmissions = "failed", 0
			}
			wantEnabled := !tc.paused && (tc.manual || tc.schedule != store.ScheduleOnce) && tc.failure != "uncertain"
			if got.Source != store.ScheduledSourceBing || got.Filename != "" || got.StoredPath != "" ||
				got.Enabled != wantEnabled || got.LastStatus != wantStatus || len(got.History) != 1 || submitted != wantSubmissions {
				t.Fatalf("unexpected result: submissions=%d task=%+v", submitted, got)
			}
			switch {
			case tc.manual:
				if got.NextRunAt != task.NextRunAt {
					t.Fatalf("manual execution changed next run: %s", got.NextRunAt)
				}
			case tc.schedule == store.ScheduleOnce:
				if got.NextRunAt != "" || !mapScheduledPrint(got).Completed {
					t.Fatalf("once job should be complete: %+v", got)
				}
				updated := got
				updated.Name = "Renamed"
				if err := refreshScheduledNextRun(&updated, got, now); err != nil || updated.NextRunAt != "" {
					t.Fatalf("renaming a completed once job restored its schedule: %+v, %v", updated, err)
				}
			default:
				next, err := time.Parse(time.RFC3339, got.NextRunAt)
				if err != nil || !next.After(now) {
					t.Fatalf("bad next run: %s", got.NextRunAt)
				}
				if tc.schedule == store.ScheduleMonthly && next.In(time.Local).Day() != 31 {
					t.Fatalf("monthly job did not return to its configured date: %s", next)
				}
			}
			if entries, err := os.ReadDir(uploads); err != nil || len(entries) != 0 {
				t.Fatalf("temporary image was left behind: %v %v", entries, err)
			}
		})
	}
}

func TestBingWallpaperCreateAndPrintHistory(t *testing.T) {
	s, user := scheduledTestStore(t)
	previousStore, previousDir, previousBing := appStore, uploadDir, bingWallpapers
	appStore, uploadDir = s, t.TempDir()
	t.Cleanup(func() { appStore, uploadDir, bingWallpapers = previousStore, previousDir, previousBing })
	t.Setenv("CUPS_SOCKET_PATH", "")
	submissions := 0
	printer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request goipp.Message
		if err := request.Decode(r.Body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		response := goipp.NewResponse(goipp.DefaultVersion, goipp.StatusOk, request.RequestID)
		if request.Code == goipp.Code(goipp.OpPrintJob) {
			submissions++
			response.Job.Add(goipp.MakeAttribute("job-id", goipp.TagInteger, goipp.Integer(submissions)))
			if file, err := io.ReadAll(r.Body); err != nil || !bytes.HasPrefix(file, []byte("%PDF-")) {
				t.Error("expected the generated PDF to be sent to the printer")
			}
		}
		w.Header().Set("Content-Type", goipp.ContentType)
		if err := response.Encode(w); err != nil {
			t.Error(err)
		}
	}))
	defer printer.Close()

	createRequest := func(source, uploadField string) *http.Request {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		for key, value := range map[string]string{
			"printer": printer.URL + "/printers/test", "source": source,
			"schedule_type": "monthly", "schedule_day": "31", "schedule_time": "09:00",
			"copies": "3", "page_range": "2", "number_up": "2",
		} {
			if err := form.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
		if uploadField != "" {
			if _, err := form.CreateFormFile(uploadField, "test.jpg"); err != nil {
				t.Fatal(err)
			}
		}
		if err := form.Close(); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/scheduled-prints", &body)
		req.Header.Set("Content-Type", form.FormDataContentType())
		return req.WithContext(auth.WithSession(req.Context(), auth.Session{UserID: user.ID, Username: user.Username, Role: user.Role}))
	}

	for _, field := range []string{"file", "files"} {
		rejected := httptest.NewRecorder()
		scheduledCreateHandler(rejected, createRequest("bing", field))
		if rejected.Code != http.StatusBadRequest {
			t.Fatalf("Bing creation accepted %s: %d %s", field, rejected.Code, rejected.Body)
		}
	}
	created := httptest.NewRecorder()
	scheduledCreateHandler(created, createRequest("bing", ""))
	var rec scheduledPrintResponse
	if err := json.Unmarshal(created.Body.Bytes(), &rec); err != nil || created.Code != http.StatusOK ||
		rec.Source != "bing" || rec.ScheduleType != store.ScheduleMonthly || rec.ScheduleDay != 31 ||
		rec.Copies != 1 || rec.PageRange != "" || rec.NumberUp != 1 {
		t.Fatalf("unexpected creation result: %d %s %v", created.Code, created.Body, err)
	}
	task := readScheduledTestTask(t, s, rec.ID)
	update := httptest.NewRequest(http.MethodPut, "/api/scheduled-prints", strings.NewReader(`{"source":"upload"}`))
	authorized := auth.WithSession(update.Context(), auth.Session{UserID: user.ID, Username: user.Username, Role: user.Role})
	update = mux.SetURLVars(update.WithContext(authorized), map[string]string{"id": fmt.Sprint(task.ID)})
	rejected := httptest.NewRecorder()
	scheduledUpdateHandler(rejected, update)
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("source change was accepted: %d %s", rejected.Code, rejected.Body)
	}
	now := time.Now()
	imageData := bingTestJPEG(t)
	index := bingTestIndex(now, "/th?id="+bingTestImageID, true)
	bingWallpapers = bingWallpaperClient{httpClient: &http.Client{Transport: bingTestTransport(func(req *http.Request) (*http.Response, error) {
		data := imageData
		if req.URL.String() == bingIndexURL {
			data = index
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}}
	if err := executeScheduledOccurrence(s, uploadDir, task, now, true, executeScheduled); err != nil {
		t.Fatal(err)
	}
	if submissions != 1 {
		t.Fatalf("IPP submissions=%d, want 1", submissions)
	}
	var printID int64
	if err := s.WithTx(context.Background(), true, func(tx *sql.Tx) error {
		records, err := store.ListPrintRecords(context.Background(), tx, store.PrintFilter{Username: user.Username})
		if err != nil {
			return err
		}
		if len(records) != 1 || records[0].Pages != 1 || records[0].Status != "printed" || records[0].Orientation != "landscape" {
			return fmt.Errorf("wrong print records: %+v", records)
		}
		printID = records[0].ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	download := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/print-records/file", nil)
	printRecordFileHandler(download, mux.SetURLVars(request.WithContext(authorized), map[string]string{"id": fmt.Sprint(printID)}))
	if download.Code != http.StatusOK || !bytes.Equal(download.Body.Bytes(), imageData) {
		t.Fatalf("print history did not serve the original Bing image: %d", download.Code)
	}
}
