package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cups-web/internal/ipp"
	"cups-web/internal/store"
	"golang.org/x/image/draw"
)

// 执行、编辑和删除共享同一个任务锁；使用后移除，避免长期积累任务 ID。
var scheduledActivity = struct {
	sync.Mutex
	tasks map[int64]bool
}{tasks: make(map[int64]bool)}

func acquireScheduledTask(id int64, running bool) bool {
	scheduledActivity.Lock()
	defer scheduledActivity.Unlock()
	if _, busy := scheduledActivity.tasks[id]; busy {
		return false
	}
	scheduledActivity.tasks[id] = running
	return true
}

func releaseScheduledTask(id int64) {
	scheduledActivity.Lock()
	defer scheduledActivity.Unlock()
	delete(scheduledActivity.tasks, id)
}

func scheduledTaskRunning(id int64) bool {
	scheduledActivity.Lock()
	defer scheduledActivity.Unlock()
	return scheduledActivity.tasks[id]
}

func scheduledQueueComplete(task store.ScheduledPrint) bool {
	return len(task.Files) > 0 && !task.Loop && task.NextFileIndex >= len(task.Files)
}

func selectedScheduledFile(task store.ScheduledPrint) (store.ScheduledFile, error) {
	if len(task.Files) == 0 {
		return store.ScheduledFile{Filename: task.Filename, StoredPath: task.StoredPath}, nil
	}
	if task.NextFileIndex < 0 || task.NextFileIndex >= len(task.Files) {
		return store.ScheduledFile{}, errors.New("照片队列已完成")
	}
	return task.Files[task.NextFileIndex], nil
}

// 修改打印参数或暂停不消耗/重排调度点；恢复时只重算已过期的执行时间。
func refreshScheduledNextRun(task *store.ScheduledPrint, previous store.ScheduledPrint, now time.Time) error {
	changed := task.ScheduleType != previous.ScheduleType
	if task.ScheduleType == store.ScheduleOnce {
		changed = changed || task.RunAt != previous.RunAt
	} else {
		changed = changed || task.StartDate != previous.StartDate || task.ScheduleTime != previous.ScheduleTime
		switch task.ScheduleType {
		case store.ScheduleWeekly:
			changed = changed || task.ScheduleWeekday != previous.ScheduleWeekday
		case store.ScheduleMonthly:
			changed = changed || task.ScheduleDay != previous.ScheduleDay
		}
	}
	finishedBingOnce := task.Source == store.ScheduledSourceBing && task.ScheduleType == store.ScheduleOnce && !task.Enabled
	recompute := changed || (task.NextRunAt == "" && !finishedBingOnce)
	if !previous.Enabled && task.Enabled {
		next, err := time.Parse(time.RFC3339, task.NextRunAt)
		recompute = recompute || err != nil || !next.After(now)
	}
	if !recompute {
		return nil
	}
	next, ok := computeNextRun(*task, now)
	if !ok {
		return errors.New("无法计算下一次执行时间")
	}
	task.NextRunAt = next.UTC().Format(time.RFC3339)
	if task.ScheduleType == store.ScheduleOnce {
		task.RunAt = task.NextRunAt
	} else {
		task.RunAt = ""
	}
	return nil
}

// 提交成功才消费照片；失败和错过时间保留当前位置。循环任务保存累计提交数。
func recordScheduledExecution(task *store.ScheduledPrint, now time.Time, filename, status, errMsg string) {
	task.LastRunAt = now.UTC().Format(time.RFC3339)
	task.LastStatus, task.LastError = status, errMsg
	if len(task.Files) > 0 && status == "printed" {
		task.SubmittedCount++
		task.NextFileIndex++
		if task.NextFileIndex == len(task.Files) && task.Loop {
			task.NextFileIndex = 0
		}
		if scheduledQueueComplete(*task) {
			task.Enabled = false
		}
	}
	task.History = append([]store.ScheduledRun{{
		Filename: filename, Status: status, Error: errMsg, RunAt: task.LastRunAt,
	}}, task.History...)
	if len(task.History) > 20 {
		task.History = task.History[:20]
	}
}

// executor 参数让队列行为可在测试中验证，不触发真实打印。
func executeScheduledOccurrence(s *store.Store, uploads string, task store.ScheduledPrint, now time.Time, manual bool,
	executor func(*store.Store, string, store.ScheduledPrint) (string, error)) error {
	file, err := selectedScheduledFile(task)
	if err != nil {
		return err
	}
	nextTime, parseErr := time.Parse(time.RFC3339, task.NextRunAt)
	missed := !manual && parseErr == nil && nextTime.Before(now.Add(-2*time.Minute))
	// 立即执行队列会消费下一次调度点，避免到点再次打印一张；暂停状态保持不变。
	if !manual || len(task.Files) > 0 {
		from := now
		if manual && nextTime.After(from) {
			from = nextTime
		}
		next, enabled := advanceNextRun(task, from)
		task.NextRunAt = next
		task.Enabled = enabled && (!manual || task.Enabled)
	}
	if task.Source == store.ScheduledSourceBing {
		if !manual && task.ScheduleType == store.ScheduleOnce {
			task.NextRunAt = ""
		}
		if !missed {
			return bingWallpapers.executeOccurrence(s, uploads, task, now, executor)
		}
		file.Filename = "Bing"
	}
	if len(task.Files) > 0 && !missed {
		return executeScheduledBatch(s, uploads, task, now, executor)
	}
	// Queue submissions use executeScheduledBatch's durable per-photo claim.
	// Only single-file jobs and skipped occurrences reach this branch.
	if task.Source != store.ScheduledSourceBing {
		if err := persistScheduledResult(s, task.ID, task.LastRunAt, task.LastStatus, task.LastError, task.NextRunAt, task.Enabled); err != nil {
			return err
		}
	}
	status, errMsg := "skipped", "错过执行窗口，已跳过"
	var execErr error
	if !missed {
		selected := task
		selected.Filename, selected.StoredPath = file.Filename, file.StoredPath
		status, execErr = executor(s, uploads, selected)
		errMsg = ""
		if execErr != nil {
			status, errMsg = "failed", execErr.Error()
		}
	}
	recordScheduledExecution(&task, now, file.Filename, status, errMsg)
	if err := s.WithTx(context.Background(), false, func(tx *sql.Tx) error {
		return store.SaveScheduledExecution(context.Background(), tx, &task)
	}); err != nil {
		return fmt.Errorf("保存定时执行结果失败：%w", err)
	}
	return execErr
}

// Persist each accepted photo before submitting the next. Every in-flight batch
// stays disabled on disk until a definite result is saved for the whole occurrence.
func executeScheduledBatch(s *store.Store, uploads string, task store.ScheduledPrint, now time.Time,
	executor func(*store.Store, string, store.ScheduledPrint) (string, error)) error {
	count := task.PhotosPerRun
	if count == 0 {
		count = 1
	}
	if count < 1 || count > 100 {
		return errors.New("每次打印张数应为 1–100")
	}
	resumeEnabled := task.Enabled
	for i := 0; i < count && !scheduledQueueComplete(task); i++ {
		file, err := selectedScheduledFile(task)
		if err != nil {
			return err
		}
		if err := persistScheduledResult(s, task.ID, task.LastRunAt, "running",
			"上次提交结果未确认，请核实打印机后再恢复任务", task.NextRunAt, false); err != nil {
			return err
		}
		selected := task
		selected.Filename, selected.StoredPath = file.Filename, file.StoredPath
		status, execErr := executor(s, uploads, selected)
		errMsg := ""
		uncertain := ipp.IsSubmissionUncertain(execErr)
		if uncertain {
			status, errMsg = "running", "提交结果未确认，请核实打印机后再恢复任务："+execErr.Error()
		} else if execErr != nil {
			status, errMsg = "failed", execErr.Error()
		} else if status != "printed" {
			execErr = fmt.Errorf("照片未成功提交：%s", status)
			status, errMsg = "failed", execErr.Error()
		}
		recordScheduledExecution(&task, now, file.Filename, status, errMsg)
		finished := execErr != nil || i+1 == count || scheduledQueueComplete(task)
		snapshot := task
		snapshot.Enabled = finished && !uncertain && resumeEnabled && !scheduledQueueComplete(task)
		if !finished {
			snapshot.LastStatus = "running"
			snapshot.LastError = "批次未完成，请核实打印机后再恢复任务"
		}
		if err := s.WithTx(context.Background(), false, func(tx *sql.Tx) error {
			return store.SaveScheduledExecution(context.Background(), tx, &snapshot)
		}); err != nil {
			return fmt.Errorf("保存定时执行结果失败：%w", err)
		}
		if execErr != nil {
			return execErr
		}
	}
	return nil
}

// 真正解码并生成小缩略图，避免列表每次下载整张原图。
func createScheduledThumbnail(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	cfg, format, err := image.DecodeConfig(f)
	if err != nil || (format != "jpeg" && format != "png" && format != "gif") {
		return "", errors.New("照片队列仅支持 JPG、PNG、GIF 图片")
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "jpg" {
		ext = "jpeg"
	}
	if ext != format {
		return "", errors.New("照片扩展名与实际格式不一致，请使用正确的 JPG、PNG、GIF 文件")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 60_000_000 {
		return "", errors.New("照片尺寸超出上限（6000 万像素）")
	}
	if _, err := f.Seek(0, 0); err != nil {
		return "", err
	}
	src, _, err := image.Decode(f)
	if err != nil {
		return "", fmt.Errorf("照片无法解码：%w", err)
	}
	width, height := cfg.Width, cfg.Height
	if width > 240 || height > 240 {
		if width >= height {
			height, width = max(1, height*240/width), 240
		} else {
			width, height = max(1, width*240/height), 240
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	thumbPath := path + ".thumb.jpg"
	out, err := os.Create(thumbPath)
	if err != nil {
		return "", err
	}
	encodeErr := jpeg.Encode(out, dst, &jpeg.Options{Quality: 80})
	closeErr := out.Close()
	if err := errors.Join(encodeErr, closeErr); err != nil {
		_ = os.Remove(thumbPath)
		return "", err
	}
	return thumbPath, nil
}

func removeScheduledFiles(uploads string, paths []string) {
	root, err := os.OpenRoot(uploads)
	if err != nil {
		log.Printf("[scheduler] open cleanup root: %v", err)
		return
	}
	defer root.Close()
	for _, path := range paths {
		if path != "" {
			if err := root.Remove(filepath.FromSlash(path)); err != nil && !errors.Is(err, os.ErrNotExist) {
				log.Printf("[scheduler] remove source: %v", err)
			}
		}
	}
}
