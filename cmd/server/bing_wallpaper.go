package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"cups-web/internal/ipp"
	"cups-web/internal/store"

	"github.com/gorilla/mux"
)

const (
	bingOrigin       = "https://cn.bing.com"
	bingIndexURL     = bingOrigin + "/HPImageArchive.aspx?format=js&idx=0&n=1&mkt=zh-CN"
	bingMaxImageSize = 12 << 20
)

var (
	bingChinaTime  = time.FixedZone("Bing China", 8*60*60)
	bingImageID    = regexp.MustCompile(`^OHR\.[A-Za-z0-9_-]{1,180}\.jpg$`)
	bingWallpapers = bingWallpaperClient{httpClient: &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
)

type bingWallpaperClient struct {
	httpClient *http.Client
}

type bingWallpaper struct {
	Title     string `json:"title"`
	Date      string `json:"date"`
	Copyright string `json:"copyright"`
	ImageURL  string `json:"imageUrl"`
	imageID   string
}

func (b bingWallpaperClient) today(ctx context.Context, now time.Time) (bingWallpaper, error) {
	body, err := b.get(ctx, bingIndexURL, 64<<10)
	if err != nil {
		return bingWallpaper{}, fmt.Errorf("无法获取 Bing 图片信息：%w", err)
	}
	var index struct {
		Images []struct {
			URL           string `json:"url"`
			FullStartDate string `json:"fullstartdate"`
			Title         string `json:"title"`
			Copyright     string `json:"copyright"`
			Downloadable  bool   `json:"wp"`
		} `json:"images"`
	}
	if err := json.Unmarshal(body, &index); err != nil {
		return bingWallpaper{}, fmt.Errorf("无法读取 Bing 图片信息：%w", err)
	}
	if len(index.Images) == 0 {
		return bingWallpaper{}, errors.New("Bing 今日背景图尚未发布")
	}
	item := index.Images[0]
	// Bing's fullstartdate is UTC, even for the zh-CN market.
	published, err := time.Parse("200601021504", item.FullStartDate)
	if err != nil {
		return bingWallpaper{}, errors.New("Bing 背景图日期无效")
	}
	date := published.In(bingChinaTime).Format(time.DateOnly)
	if date != now.In(bingChinaTime).Format(time.DateOnly) || published.After(now) {
		return bingWallpaper{}, errors.New("Bing 今日背景图尚未发布")
	}
	if !item.Downloadable {
		return bingWallpaper{}, errors.New("Bing 今日背景图不可下载")
	}
	imageURL, err := url.Parse(item.URL)
	if err != nil || imageURL.Host != "" || imageURL.Scheme != "" || imageURL.Path != "/th" || imageURL.Fragment != "" {
		return bingWallpaper{}, errors.New("Bing 背景图地址无效")
	}
	id := imageURL.Query().Get("id")
	if !bingImageID.MatchString(id) {
		return bingWallpaper{}, errors.New("Bing 背景图地址无效")
	}
	return bingWallpaper{
		Title: item.Title, Date: date, Copyright: item.Copyright, imageID: id,
		ImageURL: "/api/scheduled-prints/bing/image/" + id,
	}, nil
}

func (b bingWallpaperClient) image(ctx context.Context, id string) ([]byte, error) {
	if !bingImageID.MatchString(id) {
		return nil, errors.New("Bing 背景图地址无效")
	}
	data, err := b.get(ctx, bingOrigin+"/th?id="+url.QueryEscape(id), bingMaxImageSize)
	if err != nil {
		return nil, fmt.Errorf("无法下载 Bing 背景图：%w", err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 {
		return nil, errors.New("Bing 背景图不是尺寸有效的 JPEG")
	}
	if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
		return nil, errors.New("Bing 背景图不是尺寸有效的 JPEG")
	}
	return data, nil
}

func (b bingWallpaperClient) get(ctx context.Context, address string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, errors.New("Bing 响应过大")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("Bing 响应过大")
	}
	return data, nil
}

func (b bingWallpaperClient) previewHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	wallpaper, err := b.today(r.Context(), time.Now())
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, wallpaper)
}

func (b bingWallpaperClient) imageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	id := mux.Vars(r)["imageID"]
	if !bingImageID.MatchString(id) {
		writeJSONError(w, http.StatusBadRequest, "Bing 背景图地址无效")
		return
	}
	wallpaper, err := b.today(r.Context(), time.Now())
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	if id != wallpaper.imageID {
		writeJSONError(w, http.StatusNotFound, "Bing 背景图地址无效")
		return
	}
	data, err := b.image(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(data)
}

// The caller holds the shared task lock and has computed the next run.
func (b bingWallpaperClient) executeOccurrence(s *store.Store, uploads string, task store.ScheduledPrint, now time.Time,
	executor func(*store.Store, string, store.ScheduledPrint) (string, error)) error {
	wallpaper, err := b.today(context.Background(), now)
	filename := "Bing"
	status := "failed"
	if err == nil {
		filename = sanitizeFilename("Bing-" + wallpaper.Date + "-" + sanitizeNamePart(wallpaper.Title) + ".jpg")
		var stagedPath string
		stagedPath, err = b.stageImage(uploads, wallpaper.imageID)
		if err == nil {
			defer func() {
				if err := os.Remove(filepath.Join(uploads, stagedPath)); err != nil {
					log.Printf("[scheduler] remove temporary Bing wallpaper: %v", err)
				}
			}()
			if err := persistScheduledResult(s, task.ID, task.LastRunAt, "running",
				"上次提交结果未确认，请核实打印机后再恢复任务", task.NextRunAt, false); err != nil {
				return err
			}
			selected := task
			selected.Filename, selected.StoredPath = filename, stagedPath
			status, err = executor(s, uploads, selected)
		}
	}
	var errMsg string
	if ipp.IsSubmissionUncertain(err) {
		task.Enabled = false
		status, errMsg = "running", "提交结果未确认，请核实打印机后再恢复任务："+err.Error()
	} else if err != nil {
		status, errMsg = "failed", err.Error()
	}
	recordScheduledExecution(&task, now, filename, status, errMsg)
	if saveErr := s.WithTx(context.Background(), false, func(tx *sql.Tx) error {
		return store.SaveScheduledExecution(context.Background(), tx, &task)
	}); saveErr != nil {
		return fmt.Errorf("保存定时执行结果失败：%w", saveErr)
	}
	return err
}

func (b bingWallpaperClient) stageImage(uploads, id string) (string, error) {
	data, err := b.image(context.Background(), id)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp(uploads, ".bing-wallpaper-*.jpg")
	if err != nil {
		return "", fmt.Errorf("无法保存 Bing 临时图片：%w", err)
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		if removeErr := os.Remove(f.Name()); removeErr != nil {
			log.Printf("[scheduler] remove temporary Bing wallpaper: %v", removeErr)
		}
		return "", fmt.Errorf("无法保存 Bing 临时图片：%w", err)
	}
	return filepath.Base(f.Name()), nil
}
