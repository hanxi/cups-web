package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cups-web/internal/auth"
	"cups-web/internal/store"

	"github.com/gorilla/mux"
)

// scanDir 是扫描结果落盘目录,server 启动时由 main.go 从 SCAN_DIR 环境变量
// (缺省 "scans")解析并 MkdirAll。所有下载 / 删除都通过 os.OpenInRoot 收敛在
// 此目录内,阻挡路径穿越攻击(scan_registry.go 中声明为包级变量)。

// 支持的输出格式白名单——枚举后既能防止用户注入奇怪值给 scanimage / gs,
// 又能一处修改统一 UI / 后端。
var supportedScanFormats = map[string]bool{
	"png":  true,
	"jpeg": true,
	"pdf":  true,
}

// 支持的扫描模式白名单。scanimage 常见枚举值:Color / Gray / Lineart。
// 保留原样(首字母大写),透传给 scanimage。
var supportedScanModes = map[string]bool{
	"Color":   true,
	"Gray":    true,
	"Lineart": true,
}

// ------------------------------------------------------------------
// GET /api/scan/devices  —— 列出可用扫描设备
// ------------------------------------------------------------------

func scanListDevicesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	// force=1 时打穿缓存重新执行 scanimage -L(前端【刷新设备】使用);
	// 未指定时命中 TTL 缓存,避免每次进扫描页都等 ~17s(issue #111 复测反馈)。
	force := r.URL.Query().Get("force") == "1"
	devices, raw, err := scanDeviceCacheGet(ctx, force)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("列出扫描设备失败: %v\n%s", err, raw))
		return
	}
	writeJSON(w, map[string]any{"devices": devices})
}

// ------------------------------------------------------------------
// GET /api/scan/options?device=<name>  —— 列出设备的可选参数
// ------------------------------------------------------------------

func scanListOptionsHandler(w http.ResponseWriter, r *http.Request) {
	device := strings.TrimSpace(r.URL.Query().Get("device"))
	if device == "" {
		writeJSONError(w, http.StatusBadRequest, "缺少 device 参数")
		return
	}
	// scanimage 内部会主动 poll USB / network,慢设备容易 > 10s;放宽到 30s。
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// 参数抓取前先校验 device 存在,避免把任意字符串塞给驱动。
	// 走缓存:options 与 devices 都由前端在同一次进页面时发起,命中已刷新的列表即可,
	// 不需要再等一次 scanimage -L(issue #111 复测反馈)。缓存里存在但物理已拔时,
	// 下面 scanimage -A 仍会失败,由 handler 报错。
	devices, _, err := scanDeviceCacheGet(ctx, false)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("列出扫描设备失败: %v", err))
		return
	}
	if !scanDeviceExists(devices, device) {
		writeJSONError(w, http.StatusNotFound, "指定的扫描设备不存在,请刷新设备列表")
		return
	}

	opts, raw, err := listScanOptions(ctx, device)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("获取扫描参数失败: %v\n%s", err, raw))
		return
	}
	writeJSON(w, map[string]any{"options": opts})
}

func scanDeviceExists(devices []ScanDevice, name string) bool {
	for _, d := range devices {
		if d.Name == name {
			return true
		}
	}
	return false
}

// ------------------------------------------------------------------
// GET /api/scan/devices/probe?device=<name>  —— 探测某台设备是否真的能打开
// ------------------------------------------------------------------
//
// 背景(issue #111 复测反馈):hpaio: 后端在部分 HP 设备上打开就会报 SANE
// Error during device I/O,但 `scanimage -L` 仍会把它列出来,前端下拉里
// 有多台设备时用户容易选错。这里提供一个懒探测接口:前端在用户选中某
// 台设备后异步调用,给出健康/不健康提示,不阻塞设备列表本身。

func scanProbeDeviceHandler(w http.ResponseWriter, r *http.Request) {
	device := strings.TrimSpace(r.URL.Query().Get("device"))
	if device == "" {
		writeJSONError(w, http.StatusBadRequest, "缺少 device 参数")
		return
	}
	// 校验 device 存在,防止把任意字符串塞给驱动。走缓存:探测通常紧跟设备列表调用,
	// 命中热数据即可,不再多等一次 scanimage -L(issue #111 复测反馈)。
	verifyCtx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	devices, _, err := scanDeviceCacheGet(verifyCtx, false)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("列出扫描设备失败: %v", err))
		return
	}
	if !scanDeviceExists(devices, device) {
		writeJSONError(w, http.StatusNotFound, "指定的扫描设备不存在,请刷新设备列表")
		return
	}
	// 探测本身允许失败——命令非零退出表示后端打不开(hpaio 典型场景),
	// 只把 healthy=false 返回给前端;仅当 scanimage 二进制或环境异常时才 500。
	healthy, detail, err := probeScanDevice(r.Context(), device)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("探测失败: %v", err))
		return
	}
	writeJSON(w, map[string]any{
		"device":  device,
		"healthy": healthy,
		"detail":  detail,
	})
}

// ------------------------------------------------------------------
// POST /api/scan/jobs  —— 提交一次扫描任务(异步)
// ------------------------------------------------------------------

type scanJobRequest struct {
	Device     string `json:"device"`
	Mode       string `json:"mode"`
	Resolution string `json:"resolution"` // 前端传字符串,后端 Atoi 后再拼回去
	Source     string `json:"source"`
	Format     string `json:"format"`
	Filename   string `json:"filename"` // 可选,用户自定义文件名(不含扩展)
}

func scanCreateJobHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req scanJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	device := strings.TrimSpace(req.Device)
	if device == "" {
		writeJSONError(w, http.StatusBadRequest, "缺少 device")
		return
	}
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "Color"
	}
	if !supportedScanModes[mode] {
		writeJSONError(w, http.StatusBadRequest, "mode 只支持 Color / Gray / Lineart")
		return
	}
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = "png"
	}
	if !supportedScanFormats[format] {
		writeJSONError(w, http.StatusBadRequest, "format 只支持 png / jpeg / pdf")
		return
	}
	dpi, err := parseResolution(req.Resolution)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("resolution 非法: %v", err))
		return
	}
	source := strings.TrimSpace(req.Source) // 允许空;空时不给 scanimage 传 --source

	// 校验 device 是否真的存在。走缓存:提交扫描时通常紧跟一次页面加载,命中热数据即可;
	// 缓存里存在但物理已拔时,scanimage 子进程会立刻报错,不影响正确性(issue #111 复测反馈)。
	verifyCtx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	devices, _, err := scanDeviceCacheGet(verifyCtx, false)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("列出扫描设备失败: %v", err))
		return
	}
	if !scanDeviceExists(devices, device) {
		writeJSONError(w, http.StatusNotFound, "指定的扫描设备不存在,请刷新设备列表")
		return
	}

	// 组装最终文件名:<base>-<yyyyMMdd-HHmmss>.<ext>
	base := sanitizeFilename(strings.TrimSpace(req.Filename))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if base == "" {
		base = "scan"
	}
	stamp := time.Now().Format("20060102-150405")
	ext := format
	if ext == "jpeg" {
		ext = "jpg"
	}
	filename := fmt.Sprintf("%s-%s.%s", base, stamp, ext)

	// 落库先建 running 记录,再启动后台 job;job 结束时更新记录。
	nowStr := time.Now().UTC().Format(time.RFC3339)
	rec := &store.ScanRecord{
		UserID:     sess.UserID,
		Device:     device,
		Mode:       mode,
		Resolution: dpi,
		Source:     source,
		Format:     format,
		Filename:   filename,
		StoredPath: filename, // 相对 scanDir
		Status:     scanStatusRunning,
		CreatedAt:  nowStr,
	}
	var recordID int64
	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		id, err := store.InsertScanRecord(r.Context(), tx, rec)
		if err != nil {
			return err
		}
		recordID = id
		return nil
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("落库失败: %v", err))
		return
	}

	outputPath := filepath.Join(scanDir, filename)

	runFn := func(runCtx context.Context, job *scanJob) (int64, error) {
		return runScanPipeline(runCtx, job.logBuf, device, mode, dpi, source, format, outputPath)
	}
	job := registerScanJob(recordID, sess.UserID, device, mode, dpi, source, format, outputPath, filename, runFn)

	writeJSONStatus(w, http.StatusAccepted, map[string]any{
		"jobId":    job.id,
		"recordId": recordID,
		"filename": filename,
	})
}

// runScanPipeline 是真正的扫描 pipeline。
//
// - png / jpeg: 直接让 scanimage 一次出图。
// - pdf:       先让 scanimage 出临时 PNG,再用 img2pdf 无损嵌入合成 PDF,最后删 PNG。
//
// 所有子进程都走 exec.CommandContext(变参),没有 shell,无注入面。
func runScanPipeline(ctx context.Context, logBuf *scanBuffer, device, mode string, dpi int, source, format, outputPath string) (int64, error) {
	switch format {
	case "png", "jpeg":
		if err := runScanimage(ctx, logBuf, device, mode, dpi, source, format, outputPath); err != nil {
			return 0, err
		}
	case "pdf":
		tmpPNG := outputPath + ".tmp.png"
		if err := runScanimage(ctx, logBuf, device, mode, dpi, source, "png", tmpPNG); err != nil {
			_ = os.Remove(tmpPNG)
			return 0, err
		}
		if err := runImg2pdfPNG2PDF(ctx, logBuf, tmpPNG, outputPath); err != nil {
			_ = os.Remove(tmpPNG)
			_ = os.Remove(outputPath)
			return 0, err
		}
		_ = os.Remove(tmpPNG)
	default:
		return 0, fmt.Errorf("不支持的 format: %s", format)
	}

	fi, err := os.Stat(outputPath)
	if err != nil {
		return 0, fmt.Errorf("stat 输出文件失败: %w", err)
	}
	return fi.Size(), nil
}

func runScanimage(ctx context.Context, logBuf *scanBuffer, device, mode string, dpi int, source, format, outputPath string) error {
	args := []string{
		"-d", device,
		"--mode", mode,
		"--resolution", strconv.Itoa(dpi),
		"--format=" + format,
		"-o", outputPath,
	}
	if source != "" {
		args = append(args, "--source", source)
	}
	// 显式补几何参数,规避 escl (eSCL/AirScan) 后端默认 br-x/br-y 被 rounded
	// 到 0 后 sane_start 报 Invalid argument 的问题(issue #112)。
	// 老式后端(如 hpaio)通常没有 br-x/br-y,此时不追加,行为与之前一致。
	args = append(args, buildScanGeometryArgs(ctx, logBuf, device)...)
	cmd := exec.CommandContext(ctx, "scanimage", args...)
	cmd.Stdout = logBuf
	cmd.Stderr = logBuf
	fmt.Fprintf(logBuf, "$ scanimage %s\n", strings.Join(args, " "))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("scanimage 失败: %w", err)
	}
	return nil
}

// buildScanGeometryArgs 探测设备的几何选项并返回需要追加给 scanimage 的参数。
//
// 优先级:
//   - 同时有 --tl-x/--tl-y/--br-x/--br-y 的 range max → 追加左上顶到 0、
//     右下顶到 max(escl 后端语义)
//   - 否则同时有 -x/-y 的 range max → 追加 -x max -y max(老式 SANE 语义,
//     -x/-y 是窗口宽/高;单靠 -x/-y 时不需要指定原点)
//   - 都没有 → 返回空切片,行为不变
//
// listScanOptions 报错、返回值缺失或抽出的 max 无法通过数字守卫时,一律降级
// 到「不追加」并把原因写进日志,保留扫描任务本身继续跑的机会。
func buildScanGeometryArgs(ctx context.Context, logBuf *scanBuffer, device string) []string {
	// -A 有时候会因为设备刚断连等原因失败;别让它拖垮扫描任务本身,
	// 给一个独立的短超时。
	optCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	opts, _, err := listScanOptions(optCtx, device)
	if err != nil {
		fmt.Fprintf(logBuf, "warn: 探测扫描区域参数失败,跳过几何设置: %v\n", err)
		return nil
	}

	maxOf := func(name string) (string, bool) {
		opt, ok := opts[name]
		if !ok || opt.Type != "range" {
			return "", false
		}
		if !isNumericScanValue(opt.Max) {
			return "", false
		}
		return opt.Max, true
	}

	if brX, okX := maxOf("br-x"); okX {
		if brY, okY := maxOf("br-y"); okY {
			return []string{
				"--tl-x", "0",
				"--tl-y", "0",
				"--br-x", brX,
				"--br-y", brY,
			}
		}
	}
	if x, okX := maxOf("x"); okX {
		if y, okY := maxOf("y"); okY {
			return []string{"-x", x, "-y", y}
		}
	}
	return nil
}

// runImg2pdfPNG2PDF 用 img2pdf 把 PNG 无损嵌入 PDF 容器。
//
// 此前走 gs -sDEVICE=pdfwrite,但 Ghostscript 输入只支持 PostScript / PDF,
// 读不了任何位图:PNG 二进制被按 PostScript 语法解析,必然报
// "/syntaxerror in (binary token)" 并以 exit 1 结束(issue #114)。img2pdf
// 是专为「图片 → PDF」设计的工具,不重新编码、无损嵌入。
// MVP 阶段不做 OCR,产物为无文字层的光栅 PDF,与原设计意图一致。
func runImg2pdfPNG2PDF(ctx context.Context, logBuf *scanBuffer, pngPath, pdfPath string) error {
	args := []string{pngPath, "-o", pdfPath}
	cmd := exec.CommandContext(ctx, "img2pdf", args...)
	cmd.Stdout = logBuf
	cmd.Stderr = logBuf
	fmt.Fprintf(logBuf, "$ img2pdf %s\n", strings.Join(args, " "))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("img2pdf 合成 PDF 失败: %w", err)
	}
	return nil
}

// ------------------------------------------------------------------
// GET /api/scan/jobs/{id}  —— 查询扫描任务状态(轮询)
// ------------------------------------------------------------------

func scanGetJobHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := mux.Vars(r)["id"]
	job := findScanJob(id)
	if job == nil {
		writeJSONError(w, http.StatusNotFound, "任务不存在或已过期")
		return
	}
	// 非 admin 只能看自己的任务
	if sess.Role != store.RoleAdmin && job.UserID != sess.UserID {
		writeJSONError(w, http.StatusForbidden, "无权访问该任务")
		return
	}
	writeJSON(w, job)
}

// ------------------------------------------------------------------
// DELETE /api/scan/jobs/{id}  —— 取消尚在运行的扫描任务
// ------------------------------------------------------------------

func scanCancelJobHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := mux.Vars(r)["id"]
	job := findScanJob(id)
	if job == nil {
		writeJSONError(w, http.StatusNotFound, "任务不存在或已过期")
		return
	}
	if sess.Role != store.RoleAdmin && job.UserID != sess.UserID {
		writeJSONError(w, http.StatusForbidden, "无权取消该任务")
		return
	}
	cancelScanJob(id)
	writeJSON(w, map[string]any{"ok": true})
}

// ------------------------------------------------------------------
// GET /api/scan/records  —— 列出扫描记录
// ------------------------------------------------------------------

type scanRecordResponse struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"userId"`
	Username   string `json:"username"`
	Device     string `json:"device"`
	Mode       string `json:"mode"`
	Resolution int    `json:"resolution"`
	Source     string `json:"source,omitempty"`
	Format     string `json:"format"`
	Filename   string `json:"filename"`
	SizeBytes  int64  `json:"sizeBytes"`
	Status     string `json:"status"`
	ErrMsg     string `json:"errMsg,omitempty"`
	CreatedAt  string `json:"createdAt"`
	FinishedAt string `json:"finishedAt,omitempty"`
}

func mapScanRecord(rec store.ScanRecord) scanRecordResponse {
	finished := ""
	if rec.FinishedAt.Valid {
		finished = rec.FinishedAt.String
	}
	return scanRecordResponse{
		ID:         rec.ID,
		UserID:     rec.UserID,
		Username:   rec.Username,
		Device:     rec.Device,
		Mode:       rec.Mode,
		Resolution: rec.Resolution,
		Source:     rec.Source,
		Format:     rec.Format,
		Filename:   rec.Filename,
		SizeBytes:  rec.SizeBytes,
		Status:     rec.Status,
		ErrMsg:     rec.ErrMsg,
		CreatedAt:  rec.CreatedAt,
		FinishedAt: finished,
	}
}

func scanListRecordsHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	startAt, endAt, err := parseDateRange(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid date range")
		return
	}

	q := r.URL.Query()
	filter := store.ScanFilter{
		Limit:   200,
		StartAt: startAt,
		EndAt:   endAt,
	}
	if lim := q.Get("limit"); lim != "" {
		if n, err := strconv.Atoi(lim); err == nil && n > 0 && n <= 1000 {
			filter.Limit = n
		}
	}

	// 非 admin 只能查自己;admin 可用 ?username=xxx 过滤,默认看全部
	if sess.Role != store.RoleAdmin {
		filter.Username = sess.Username
	} else if u := strings.TrimSpace(q.Get("username")); u != "" {
		filter.Username = u
	}

	var records []store.ScanRecord
	err = appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		list, err := store.ListScanRecords(r.Context(), tx, filter)
		if err != nil {
			return err
		}
		records = list
		return nil
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("查询扫描记录失败: %v", err))
		return
	}

	resp := make([]scanRecordResponse, 0, len(records))
	for _, rec := range records {
		resp = append(resp, mapScanRecord(rec))
	}
	writeJSON(w, map[string]any{"records": resp})
}

// ------------------------------------------------------------------
// GET /api/scan/records/{id}/file  —— 下载扫描文件
// ------------------------------------------------------------------

func scanDownloadHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "id 非法")
		return
	}
	var rec store.ScanRecord
	err = appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		found, err := store.GetScanRecordByID(r.Context(), tx, id)
		if err != nil {
			return err
		}
		rec = found
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "记录不存在")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sess.Role != store.RoleAdmin && rec.UserID != sess.UserID {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}
	if rec.Status != scanStatusSucceeded {
		writeJSONError(w, http.StatusConflict, "该扫描任务尚未成功,无法下载")
		return
	}

	// os.OpenInRoot 把访问限制在 scanDir 内,即便 StoredPath 被污染成 ../ 也拒绝(Go 1.24+)。
	f, err := os.OpenInRoot(scanDir, filepath.FromSlash(rec.StoredPath))
	if err != nil {
		writeJSONError(w, http.StatusNotFound, fmt.Sprintf("扫描文件已不存在: %v", err))
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", scanContentType(rec.Format))
	// mime.FormatMediaType 对非 ASCII 文件名自动输出 RFC 5987 的
	// filename*=UTF-8''… 形式,保证中文文件名下载不丢名、不乱码(issue #114),
	// 与 print_records_handlers.go 的既有写法保持一致。rec.Filename 入库前已
	// sanitize,正常不会触发空串;兜底 attachment 防御历史脏数据。
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": rec.Filename})
	if disposition == "" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", disposition)
	if _, err := io.Copy(w, f); err != nil {
		log.Printf("[scan] 下载中断: %v", err)
	}
}

func scanContentType(format string) string {
	switch strings.ToLower(format) {
	case "png":
		return "image/png"
	case "jpeg", "jpg":
		return "image/jpeg"
	case "pdf":
		return "application/pdf"
	}
	return "application/octet-stream"
}

// ------------------------------------------------------------------
// DELETE /api/scan/records/{id}  —— 删除扫描记录与对应文件
// ------------------------------------------------------------------

func scanDeleteRecordHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "id 非法")
		return
	}

	var rec store.ScanRecord
	err = appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		found, err := store.GetScanRecordByID(r.Context(), tx, id)
		if err != nil {
			return err
		}
		rec = found
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "记录不存在")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sess.Role != store.RoleAdmin && rec.UserID != sess.UserID {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}

	// 先删数据库记录,再删文件——文件删失败(比如已经被人手动清了)不阻断记录删除。
	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		return store.DeleteScanRecord(r.Context(), tx, id)
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("删除记录失败: %v", err))
		return
	}
	if rec.StoredPath != "" {
		abs := filepath.Join(scanDir, filepath.FromSlash(rec.StoredPath))
		if !strings.HasPrefix(filepath.Clean(abs), filepath.Clean(scanDir)+string(filepath.Separator)) {
			// 防御性:StoredPath 若指向 scanDir 外,拒绝删,避免误伤宿主。
			log.Printf("[scan] 拒绝删除越界文件: %s", abs)
		} else {
			_ = os.Remove(abs)
		}
	}
	writeJSON(w, map[string]any{"ok": true})
}
