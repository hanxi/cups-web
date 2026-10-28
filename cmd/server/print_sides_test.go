package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"cups-web/internal/auth"
	"cups-web/internal/ipp"
	"cups-web/internal/store"
	goipp "github.com/OpenPrinting/goipp"
	"github.com/gorilla/mux"
)

func TestPrintSidesPipeline(t *testing.T) {
	t.Setenv("CUPS_SOCKET_PATH", "")
	for _, tc := range []struct {
		name, sides, pageSet, want string
		legacy                     bool
	}{
		{name: "default", want: ipp.SidesOneSided},
		{name: "legacy-duplex", legacy: true, want: ipp.SidesLongEdge},
		{name: "long-edge", sides: ipp.SidesLongEdge, want: ipp.SidesLongEdge},
		{name: "short-edge", sides: ipp.SidesShortEdge, want: ipp.SidesShortEdge},
		{name: "explicit-single", sides: ipp.SidesOneSided, legacy: true, want: ipp.SidesOneSided},
		{name: "manual-odd", sides: ipp.SidesShortEdge, pageSet: "odd", want: ipp.SidesOneSided},
		{name: "manual-even", sides: ipp.SidesShortEdge, pageSet: "even", want: ipp.SidesOneSided},
		{name: "manual-even-reverse", sides: ipp.SidesShortEdge, pageSet: "even-reverse", want: ipp.SidesOneSided},
		{name: "invalid", sides: "short-edge"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, user := scheduledTestStore(t)
			previousStore, previousDir := appStore, uploadDir
			appStore, uploadDir = s, t.TempDir()
			t.Cleanup(func() { appStore, uploadDir = previousStore, previousDir })
			session := auth.Session{UserID: user.ID, Username: user.Username, Role: user.Role}
			var submitted []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req goipp.Message
				if err := req.Decode(r.Body); err != nil {
					t.Error(err)
					return
				}
				io.Copy(io.Discard, r.Body)
				if req.Code == goipp.Code(goipp.OpPrintJob) {
					var sides string
					for _, attr := range req.Job {
						if attr.Name == "sides" {
							sides = attr.Values[0].V.String()
						}
					}
					submitted = append(submitted, sides)
				}
				resp := goipp.NewResponse(goipp.DefaultVersion, goipp.StatusOk, req.RequestID)
				resp.Job.Add(goipp.MakeAttribute("job-id", goipp.TagInteger, goipp.Integer(len(submitted))))
				data, err := resp.EncodeBytes()
				if err != nil {
					t.Error(err)
					return
				}
				w.Write(data)
			}))
			defer server.Close()
			printer := server.URL + "/printers/test"
			var photo bytes.Buffer
			if err := png.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 10, 10))); err != nil {
				t.Fatal(err)
			}
			fields := map[string]string{"printer": printer, "sides": tc.sides, "duplex": strconv.FormatBool(tc.legacy), "page_set": tc.pageSet, "paper_size": "A4"}
			checkResponse := func(w *httptest.ResponseRecorder) {
				t.Helper()
				want := http.StatusOK
				if tc.want == "" {
					want = http.StatusBadRequest
				}
				if w.Code != want {
					t.Fatalf("status=%d, want %d: %s", w.Code, want, w.Body.String())
				}
			}
			// Normal single and multi-photo submissions exercise both upload shapes.
			for _, field := range []string{"file", "files"} {
				count := 1
				if field == "files" {
					count = 2
				}
				req := photoUploadRequest(t, "/api/print", field, photo.Bytes(), count, fields)
				req = req.WithContext(auth.WithSession(req.Context(), session))
				w := httptest.NewRecorder()
				printHandler(w, req)
				checkResponse(w)
			}
			// Reprint both a photo history and a document history through the API.
			if err := os.WriteFile(filepath.Join(uploadDir, "source.pdf"), twoPagePDF(t), 0600); err != nil {
				t.Fatal(err)
			}
			var documentID int64
			if err := s.WithTx(context.Background(), false, func(tx *sql.Tx) error {
				var err error
				documentID, err = store.InsertPrintRecord(context.Background(), tx, &store.PrintRecord{
					UserID: user.ID, Filename: "source.pdf", StoredPath: "source.pdf", CreatedAt: "now",
				})
				return err
			}); err != nil {
				t.Fatal(err)
			}
			for _, id := range []int64{1, documentID} {
				data, err := json.Marshal(reprintRequest{Printer: printer, Sides: tc.sides, Duplex: tc.legacy, PageSet: tc.pageSet, PaperSize: "A4"})
				if err != nil {
					t.Fatal(err)
				}
				req := httptest.NewRequest("POST", "/api/print-records/reprint", bytes.NewReader(data))
				req = mux.SetURLVars(req, map[string]string{"id": strconv.FormatInt(id, 10)})
				req = req.WithContext(auth.WithSession(req.Context(), session))
				w := httptest.NewRecorder()
				reprintHandler(w, req)
				checkResponse(w)
			}
			// Scheduled creation, edit/pause/resume, then real execution.
			fields["schedule_type"], fields["schedule_time"] = "daily", "09:00"
			req := photoUploadRequest(t, "/api/scheduled-prints", "file", photo.Bytes(), 1, fields)
			req = req.WithContext(auth.WithSession(req.Context(), session))
			w := httptest.NewRecorder()
			scheduledCreateHandler(w, req)
			checkResponse(w)
			if tc.want != "" {
				var created scheduledPrintResponse
				if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
					t.Fatal(err)
				}
				for _, enabled := range []bool{false, true} {
					data, err := json.Marshal(scheduledUpdatePayload{
						PrinterURI: printer, Sides: tc.sides, IsDuplex: tc.legacy, Enabled: &enabled,
						PaperSize: "A4", PageSet: tc.pageSet, ScheduleType: "daily", ScheduleTime: "09:00",
					})
					if err != nil {
						t.Fatal(err)
					}
					req := httptest.NewRequest("PUT", "/api/scheduled-prints", bytes.NewReader(data))
					req = mux.SetURLVars(req, map[string]string{"id": strconv.FormatInt(created.ID, 10)})
					req = req.WithContext(auth.WithSession(req.Context(), session))
					w := httptest.NewRecorder()
					scheduledUpdateHandler(w, req)
					checkResponse(w)
				}
				task := readScheduledTestTask(t, s, created.ID)
				if _, err := executeScheduled(s, uploadDir, task); err != nil {
					t.Fatal(err)
				}
			} else {
				data := []byte(`{"sides":"short-edge"}`)
				req := httptest.NewRequest("PUT", "/api/scheduled-prints/1", bytes.NewReader(data))
				req = mux.SetURLVars(req, map[string]string{"id": "1"})
				req = req.WithContext(auth.WithSession(req.Context(), session))
				w := httptest.NewRecorder()
				scheduledUpdateHandler(w, req)
				checkResponse(w)
			}
			wantCount := 5
			if tc.want == "" {
				wantCount = 0
			}
			if len(submitted) != wantCount {
				t.Fatalf("submissions=%v, want %d", submitted, wantCount)
			}
			for _, sides := range submitted {
				if sides != tc.want {
					t.Errorf("IPP sides=%q, want %q", sides, tc.want)
				}
			}
			if err := s.WithTx(context.Background(), true, func(tx *sql.Tx) error {
				records, err := store.ListPrintRecords(context.Background(), tx, store.PrintFilter{})
				if err != nil {
					return err
				}
				for _, record := range records {
					if record.ID != documentID && (record.Sides != tc.want || record.IsDuplex != (tc.want != ipp.SidesOneSided)) {
						t.Errorf("lost history sides: %+v", record)
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
