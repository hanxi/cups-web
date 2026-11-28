package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"cups-web/internal/auth"

	"github.com/gorilla/mux"
)

const (
	driversDataDir   = "/opt/cups-drivers/data"
	driversScriptDir = "/opt/cups-drivers/scripts"

	// 自定义上传件在 driversDataDir 下的子目录名。
	// custom-ppd 走 manifest.txt，能被 restore-drivers.sh 逐文件 cp -a 恢复；
	// custom-deb 只做归档记录，故意不写 manifest.txt（见 persistUploadedDeb 注释）。
	customPPDDirName = "custom-ppd"
	customDebDirName = "custom-deb"

	// 系统 PPD 安装目录：restore-drivers.sh 依赖 manifest 里的绝对路径恢复。
	customPPDInstallDir = "/usr/share/cups/model/custom"

	// 自定义驱动上传的请求体硬上限（配合 http.MaxBytesReader 生效）。
	// 厂商 .deb 驱动包通常几 MB 到几十 MB（Canon UFR II 全量包约 40MB 量级），
	// 留到 64MB 足够，同时避免管理员误传镜像/压缩包把容器磁盘打满。
	driverUploadMaxBytes = 64 << 20

	// 后台驱动任务的硬超时。canon-capt / foo2zjs-firmware 要现场编译，
	// arm 设备上十几分钟都可能，留 30 分钟余量。
	driverJobTimeout = 30 * time.Minute
	// 已完成任务在内存里的保留时长，超过后清理，避免长期运行的进程无限累积。
	driverJobRetention = time.Hour

	driverJobRunning   = "running"
	driverJobSucceeded = "succeeded"
	driverJobFailed    = "failed"
)

// ── 后台驱动任务 ───────────────────────────────────────────────────────────────
//
// 为什么必须异步：main.go 的 http.Server 是全局 WriteTimeout = 120s，
// 而编译型驱动动辄几分钟。用 exec.CommandContext(r.Context(), ...) 同步执行时，
// 连接一超时请求上下文即被取消，CommandContext 会直接 kill 掉正在 make 的进程，
// 留下半编译产物、客户端还拿不到结果。因此改成：接口立刻 202 返回 jobId，
// 真正的命令跑在 context.Background() 派生的 goroutine 里，前端轮询任务状态。

// safeBuffer 是加锁的字节缓冲。exec.Cmd 在后台 goroutine 里往里写，
// 轮询 handler 同时在读，必须用 mutex 保护（也让轮询能看到进行中的增量输出）。
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// driverJob 的字段全部小写：并发访问统一由 driverJobsMu 保护，
// 对外序列化时先在锁内快照成 driverJobView，避免 json 编码时读到撕裂状态。
type driverJob struct {
	id         string
	kind       string // install / remove / setup
	name       string
	status     string
	errMsg     string
	startedAt  time.Time
	finishedAt time.Time
	result     map[string]any
	logBuf     *safeBuffer
}

type driverJobView struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Name       string         `json:"name,omitempty"`
	Status     string         `json:"status"`
	Log        string         `json:"log"`
	Error      string         `json:"error,omitempty"`
	StartedAt  string         `json:"startedAt"`
	FinishedAt string         `json:"finishedAt,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
}

var (
	driverJobsMu sync.Mutex
	driverJobs   = map[string]*driverJob{}
)

// viewLocked 必须在持有 driverJobsMu 时调用。
func (j *driverJob) viewLocked() *driverJobView {
	v := &driverJobView{
		ID:        j.id,
		Kind:      j.kind,
		Name:      j.name,
		Status:    j.status,
		Log:       j.logBuf.String(),
		Error:     j.errMsg,
		StartedAt: j.startedAt.Format(time.RFC3339),
		Result:    j.result,
	}
	if !j.finishedAt.IsZero() {
		v.FinishedAt = j.finishedAt.Format(time.RFC3339)
	}
	return v
}

// pruneDriverJobsLocked 清理完成时间超过 driverJobRetention 的任务。
func pruneDriverJobsLocked() {
	cutoff := time.Now().Add(-driverJobRetention)
	for id, j := range driverJobs {
		if j.status != driverJobRunning && !j.finishedAt.IsZero() && j.finishedAt.Before(cutoff) {
			delete(driverJobs, id)
		}
	}
}

// startDriverJob 创建并启动一个后台驱动任务。
// apt/dpkg 自身有全局锁，并发安装只会互相失败，因此同一时刻只允许一个任务在跑；
// 已有任务运行中时返回 (nil, 正在跑的 jobId)，由 handler 回 409。
func startDriverJob(kind, name string, fn func(ctx context.Context, logBuf *safeBuffer) (map[string]any, error)) (*driverJob, string) {
	driverJobsMu.Lock()
	defer driverJobsMu.Unlock()

	pruneDriverJobsLocked()

	for _, j := range driverJobs {
		if j.status == driverJobRunning {
			return nil, j.id
		}
	}

	job := &driverJob{
		id:        randomToken(),
		kind:      kind,
		name:      name,
		status:    driverJobRunning,
		startedAt: time.Now(),
		logBuf:    &safeBuffer{},
	}
	driverJobs[job.id] = job

	go func() {
		// 用 context.Background() 派生，绝不能用 r.Context()：见文件顶部说明。
		ctx, cancel := context.WithTimeout(context.Background(), driverJobTimeout)
		defer cancel()

		result, err := fn(ctx, job.logBuf)

		driverJobsMu.Lock()
		defer driverJobsMu.Unlock()
		job.finishedAt = time.Now()
		if err != nil {
			job.status = driverJobFailed
			job.errMsg = err.Error()
			log.Printf("[driver-job] %s(%s) 失败: %v", job.kind, job.name, err)
		} else {
			job.status = driverJobSucceeded
			job.result = result
			log.Printf("[driver-job] %s(%s) 成功", job.kind, job.name)
		}
	}()

	return job, ""
}

// runningDriverJobID 返回当前正在跑的任务 ID（没有则空串）。
func runningDriverJobID() string {
	driverJobsMu.Lock()
	defer driverJobsMu.Unlock()
	for _, j := range driverJobs {
		if j.status == driverJobRunning {
			return j.id
		}
	}
	return ""
}

// writeDriverJobBusy 统一回 409，并带上正在跑的 jobId 供前端直接切过去轮询。
func writeDriverJobBusy(w http.ResponseWriter, jobID string) {
	writeJSONStatus(w, http.StatusConflict, map[string]any{
		"error": "已有驱动任务正在执行，请等待其完成后重试",
		"jobId": jobID,
	})
}

// runDriverCommand 执行一条外部命令，stdout/stderr 都写进任务日志缓冲，
// 这样轮询接口能读到进行中的编译输出，而不是等命令结束才一次性拿到。
func runDriverCommand(ctx context.Context, logBuf *safeBuffer, name string, args ...string) error {
	fmt.Fprintf(logBuf, "$ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, name, args...)
	// CUPS_AIO=1 告诉安装脚本这是"CUPS 与 Web 同容器"的形态；
	// 对 lpadmin 等命令是无害的多余环境变量。
	cmd.Env = append(os.Environ(), "CUPS_AIO=1")
	cmd.Stdout = logBuf
	cmd.Stderr = logBuf
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(logBuf, "!! 命令失败: %v\n", err)
		return err
	}
	return nil
}

// GET /api/admin/drivers/jobs/{id} — 查询后台驱动任务状态与日志。
func adminDriverJobHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	driverJobsMu.Lock()
	var view *driverJobView
	if job, ok := driverJobs[id]; ok {
		view = job.viewLocked()
	}
	driverJobsMu.Unlock()

	if view == nil {
		writeJSONError(w, http.StatusNotFound, "unknown job id")
		return
	}
	writeJSON(w, view)
}

// ── 驱动列表 ───────────────────────────────────────────────────────────────────

// GET /api/admin/drivers — 列出所有已知驱动及安装状态、当前架构、已上传的自定义 .deb。
func adminListDriversHandler(w http.ResponseWriter, r *http.Request) {
	arch := currentDebArch()

	drivers := make([]DriverStatus, 0, len(driversRegistry))
	for _, d := range driversRegistry {
		status := DriverStatus{DriverMeta: d}
		status.Supported = driverSupportsArch(d, arch)
		// driver-install 是按 install-<name>.sh 找脚本的，镜像里没有脚本就装不了。
		if _, err := os.Stat(filepath.Join(driversScriptDir, "install-"+d.Name+".sh")); err == nil {
			status.HasScript = true
		}
		if info, err := os.Stat(filepath.Join(driversDataDir, d.Name, "manifest.txt")); err == nil {
			status.Installed = true
			status.InstalledAt = info.ModTime().Format(time.RFC3339)
		}
		// metadata.txt 由 scripts/driver/driver-install.sh 写入，
		// 键名是 driver= / installed_at= / file_count= / arch=（历史代码读的 date= 永远不命中）。
		// v2 起还有 manifest_version= / restore_mode= / package_count= / package_bytes=。
		meta := readKeyValueFile(filepath.Join(driversDataDir, d.Name, "metadata.txt"))
		if v := meta["installed_at"]; v != "" {
			status.InstalledAt = v
		}
		status.InstalledArch = meta["arch"]
		status.RestoreMode = meta["restore_mode"]
		// 解析失败一律留 0（omitempty 会把字段省掉），不因为一行坏数据让整个列表 500。
		if n, err := strconv.Atoi(meta["package_count"]); err == nil {
			status.PackageCount = n
		}
		if n, err := strconv.Atoi(meta["manifest_version"]); err == nil {
			status.ManifestVersion = n
		}
		drivers = append(drivers, status)
	}

	writeJSON(w, map[string]any{
		"currentArch": arch,
		"drivers":     drivers,
		"customDebs":  listCustomDebs(),
		"hplipPlugin": getHPLIPPluginStatus(r.Context()),
		// 上传的 .deb 现在会被归档到 custom-deb/packages/ 并在容器启动时由
		// restore-drivers 用 `dpkg -i` 自动重装（幂等），不再需要用户手动重新上传。
		"customDebNotice": "上传的 .deb 包已归档，容器重启时会自动重新安装；" +
			"若某个包始终装不上，可在宿主的 ./.drivers/custom-deb/packages/ 下删除它。",
	})
}

// readKeyValueFile 解析 shell 脚本写出的 key=value 文件。
// 值可能带尾部空白（脚本里是 echo 拼的），统一 TrimSpace。
func readKeyValueFile(path string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

// listCustomDebs 列出已归档的自定义 .deb 上传件（信息性条目，供前端提示重装）。
// installedAt 用文件 mtime（每个包各自准确）；arch 取目录级 metadata.txt。
func listCustomDebs() []CustomDebPackage {
	pkgDir := filepath.Join(driversDataDir, customDebDirName, "packages")
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return []CustomDebPackage{}
	}
	meta := readKeyValueFile(filepath.Join(driversDataDir, customDebDirName, "metadata.txt"))

	result := make([]CustomDebPackage, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".deb") {
			continue
		}
		pkg := CustomDebPackage{Filename: e.Name(), Arch: meta["arch"]}
		if info, err := e.Info(); err == nil {
			pkg.InstalledAt = info.ModTime().Format(time.RFC3339)
			pkg.SizeBytes = info.Size()
		}
		result = append(result, pkg)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Filename < result[j].Filename })
	return result
}

// ── 安装 / 卸载 ────────────────────────────────────────────────────────────────

// POST /api/admin/drivers/install — 异步安装驱动，返回 202 + jobId。
func adminInstallDriverHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if payload.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "driver name is required")
		return
	}
	meta := findDriverByName(payload.Name)
	if meta == nil {
		writeJSONError(w, http.StatusNotFound, "unknown driver: "+payload.Name)
		return
	}
	arch := currentDebArch()
	if !driverSupportsArch(*meta, arch) {
		writeJSONError(w, http.StatusBadRequest,
			fmt.Sprintf("驱动 %s 不支持当前架构 %s", meta.DisplayName, arch))
		return
	}

	name := payload.Name
	job, busyID := startDriverJob("install", name, func(ctx context.Context, logBuf *safeBuffer) (map[string]any, error) {
		if err := runDriverCommand(ctx, logBuf, "/usr/local/bin/driver-install", name); err != nil {
			return nil, fmt.Errorf("driver installation failed: %w", err)
		}
		// 驱动装完后 PPD 列表变了，必须失效缓存，否则紧接着的 detect/setup 会拿到旧列表。
		invalidatePPDModels()
		return map[string]any{"name": name}, nil
	})
	if job == nil {
		writeDriverJobBusy(w, busyID)
		return
	}

	log.Printf("[driver-install] %s 任务已提交 (job=%s, user=%s)", name, job.id, sessionUsername(r))
	writeJSONStatus(w, http.StatusAccepted, map[string]any{"jobId": job.id, "name": name})
}

// POST /api/admin/drivers/remove — 异步卸载驱动，返回 202 + jobId。
// 卸载本身很快，但和安装共享 apt/dpkg 锁，统一走任务系统才能被单飞约束覆盖，
// 前端也可以复用同一套轮询逻辑。
func adminRemoveDriverHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if payload.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "driver name is required")
		return
	}

	name := payload.Name
	job, busyID := startDriverJob("remove", name, func(ctx context.Context, logBuf *safeBuffer) (map[string]any, error) {
		if err := runDriverCommand(ctx, logBuf, "/usr/local/bin/driver-remove", name); err != nil {
			return nil, fmt.Errorf("driver removal failed: %w", err)
		}
		invalidatePPDModels()
		return map[string]any{"name": name}, nil
	})
	if job == nil {
		writeDriverJobBusy(w, busyID)
		return
	}

	log.Printf("[driver-remove] %s 任务已提交 (job=%s, user=%s)", name, job.id, sessionUsername(r))
	writeJSONStatus(w, http.StatusAccepted, map[string]any{"jobId": job.id, "name": name})
}

// ── 检测打印机 ─────────────────────────────────────────────────────────────────

// GET /api/admin/drivers/detect — 检测已连接的打印机并推荐驱动。
//
// 改进点（相对历史实现）：
//   - 独立超时 context（不再挂 r.Context()，网络扫描慢时不会被浏览器取消掐断）
//   - 按 lpinfo caps 加 --timeout / --include-schemes 过滤噪声
//   - 内联 Top-1 PPD 候选（纯本地打分，零额外 fork，首屏就能显示驱动名）
//   - 四态 driverState（ready / driverless / needsVendorDriver / unmatched）
//   - lpstat -v 一次填 existingQueue / suggestedName
//
// detect 绝不调 cups-driverd 委托、绝不做 IPP 探测——那是候选接口的事。
func adminDetectPrintersHandler(w http.ResponseWriter, r *http.Request) {
	// 独立超时：lpinfo -l -v 在网络打印机多时可能要十几秒（DNS-SD/SNMP 扫描），
	// 挂 r.Context() 会被 120s WriteTimeout 或浏览器取消掐断。
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
	defer cancel()

	// 按能力探测结果组装 lpinfo 参数。
	args := []string{"-l", "-v"}
	caps := lpinfoCapabilities(ctx)
	if caps.Timeout {
		args = append(args, "--timeout", "10")
	}
	if caps.Schemes {
		args = append(args, "--include-schemes", "usb,ipp,ipps,dnssd,socket,lpd,hp,snmp")
	}

	output, err := exec.CommandContext(ctx, "lpinfo", args...).Output()
	if err != nil {
		log.Printf("[driver-detect] lpinfo %v failed: %v", args, err)
		writeJSONError(w, http.StatusInternalServerError, "failed to detect printers")
		return
	}

	printers := parseLpinfoDevices(string(output))

	// PPD 列表走缓存（不再每台设备 fork 一次 lpinfo -m）。
	entries, entriesErr := cachedPPDEntries(ctx)
	if entriesErr != nil {
		log.Printf("[driver-detect] lpinfo -m failed (降级为无 PPD 信息): %v", entriesErr)
	}

	// 队列快照：一次 lpstat -v 拿到所有已有队列的 device-uri 映射。
	queues, _ := listExistingQueues(ctx)

	for i := range printers {
		p := &printers[i]
		searchStr := strings.TrimSpace(p.Manufacturer + " " + p.Model)
		if match := matchDriverForPrinter(searchStr); match != nil {
			p.DriverMatch = match
		}

		// 本地打分 Top-1（纯内存操作，零 fork）。
		if entriesErr == nil {
			in := MatchInput{
				Manufacturer: p.Manufacturer,
				Model:        p.Model,
				DeviceID:     p.DeviceID,
				Scheme:       p.Scheme,
				PreferLang:   "zh",
			}
			cands := ScorePPDCandidates(entries, in, 5)
			p.CandidateCount = len(cands)
			if len(cands) > 0 {
				p.TopCandidate = &cands[0]
			}
		}

		// 四态 driverState。
		best := bestPPDFromCandidates(nil) // 先用空列表算
		if p.TopCandidate != nil {
			best = bestPPDFromCandidates([]PPDCandidate{*p.TopCandidate})
		}
		switch {
		case best != "":
			p.DriverState = "ready"
			p.HasDriver = true
		case p.DriverMatch != nil:
			p.DriverState = "needsVendorDriver"
		case p.Scheme != "" && driverlessSchemes[p.Scheme]:
			p.DriverState = "driverless"
			p.HasDriver = true
		default:
			p.DriverState = "unmatched"
		}

		// 已有队列与建议名。
		if q := findQueueByURI(queues, p.DeviceURI); q != "" {
			p.ExistingQueue = q
		}
		base := sanitizePrinterName(p.Model)
		if base == "" {
			base = sanitizePrinterName(p.Manufacturer)
		}
		if base == "" {
			base = "Printer"
		}
		p.SuggestedName, _ = uniquePrinterNameChecked(base, queues)
	}

	writeJSON(w, printers)
}

// ── 候选 PPD 查询 ──────────────────────────────────────────────────────────────

// ppdQuerySem 是候选查询的轻量并发闸。
// 候选查询不占 startDriverJob 的全局单飞锁（否则"看候选"和"装驱动"互斥），
// 但每次查询可能 fork cups-driverd（秒级），需要限制并发防 fork 风暴。
var ppdQuerySem = make(chan struct{}, 4)

// GET /api/admin/drivers/ppds?deviceUri=&deviceId=&manufacturer=&model=&limit=8
//
// 返回某台打印机的 PPD 候选列表（Top-N），含 driverless 可用性与排障用的 matcher 字段。
// 不走后台 job：最坏 15s（本地 <10ms + driverd ≤8s + IPP ≤5s）远低于 120s WriteTimeout，
// 而走 job 会踩全局单飞锁与安装互斥，并让前端多两次往返。
func adminListPPDCandidatesHandler(w http.ResponseWriter, r *http.Request) {
	// 并发闸：满了直接 429，防止管理员连点导致 fork 风暴。
	select {
	case ppdQuerySem <- struct{}{}:
		defer func() { <-ppdQuerySem }()
	default:
		writeJSONError(w, http.StatusTooManyRequests, "候选查询繁忙，请稍后重试")
		return
	}

	q := r.URL.Query()
	deviceURI := strings.TrimSpace(q.Get("deviceUri"))
	deviceID := strings.TrimSpace(q.Get("deviceId"))
	manufacturer := strings.TrimSpace(q.Get("manufacturer"))
	model := strings.TrimSpace(q.Get("model"))
	limit := 8
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 20 {
			limit = n
		}
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 20*time.Second)
	defer cancel()

	cands, dlInfo, driverdMatched, err := matchPPDCandidates(ctx, nil,
		manufacturer, model, deviceID, deviceURI, true, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("PPD 匹配失败: %v", err))
		return
	}

	// 队列快照。
	queues, _ := listExistingQueues(ctx)
	existingQueue := findQueueByURI(queues, deviceURI)
	base := sanitizePrinterName(model)
	if base == "" {
		base = sanitizePrinterName(manufacturer)
	}
	if base == "" {
		base = "Printer"
	}
	suggestedName, _ := uniquePrinterNameChecked(base, queues)

	// 能力探测结果（排障用）。
	caps := lpinfoCapabilities(ctx)

	writeJSON(w, map[string]any{
		"deviceUri":     deviceURI,
		"manufacturer":  manufacturer,
		"model":         model,
		"suggestedName": suggestedName,
		"existingQueue": existingQueue,
		"driverless":    dlInfo,
		"candidates":    cands,
		"allowRaw":      true,
		"matcher": map[string]any{
			"driverd":        caps.DeviceID || caps.MakeAndModel,
			"driverdMatched": driverdMatched,
			"caps": map[string]bool{
				"deviceId":     caps.DeviceID,
				"makeAndModel": caps.MakeAndModel,
				"language":     caps.Language,
				"timeout":      caps.Timeout,
			},
			"modelLines":      ppdModels.lines,
			"cacheAgeSeconds": ppdCacheAgeSeconds(),
		},
	})
}

// parseLpinfoDevices 解析 `lpinfo -l -v` 的长格式输出。
//
// CUPS（systemv/lpinfo.c 的 show_devices）在 -l 下每台设备打印一个块：
//
//	Device: uri = usb://HP/LaserJet%201020?serial=XXXX
//	        class = direct
//	        info = HP LaserJet 1020
//	        make-and-model = HP LaserJet 1020
//	        device-id = MFG:HP;MDL:LaserJet 1020;CMD:...;
//	        location =
//
// 本机没有 cups-client 可实测，故解析器按上述格式假设编写并刻意宽容：
// 以 "Device:" 开块，其余行按第一个 " = " 拆 key/value，未知 key 直接忽略；
// 万一某版本改了字段顺序或缩进也不会解析失败。
func parseLpinfoDevices(output string) []DetectedPrinter {
	// 初始化成空切片而不是 nil：JSON 序列化后是 []，前端可以直接读 .length。
	printers := []DetectedPrinter{}
	var cur map[string]string

	flush := func() {
		if cur == nil {
			return
		}
		if p, ok := buildDetectedPrinter(cur); ok {
			printers = append(printers, p)
		}
		cur = nil
	}

	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "Device:"); ok {
			flush()
			cur = map[string]string{}
			line = strings.TrimSpace(rest) // 形如 "uri = usb://..."
		}
		if cur == nil {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		cur[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	flush()

	return printers
}

// buildDetectedPrinter 把一个 lpinfo 设备块转成 DetectedPrinter，
// 返回 false 表示该块应被丢弃（裸 backend scheme 行、虚拟打印机等）。
func buildDetectedPrinter(fields map[string]string) (DetectedPrinter, bool) {
	uri := fields["uri"]

	// lpinfo 还会输出 backend 自身的行（socket / ipp / lpd / beh / http / dnssd / hp …），
	// 这类行第二列不是完整 URI（没有 "://"），必须过滤掉，否则会凭空多出 5~6 台"假打印机"。
	if uri == "" || !strings.Contains(uri, "://") {
		return DetectedPrinter{}, false
	}
	// 跳过 CUPS-PDF、盲文等虚拟设备。
	lowerURI := strings.ToLower(uri)
	if strings.Contains(lowerURI, "cups-pdf") || strings.Contains(lowerURI, "cups-brf") || uri == "file:///dev/null" {
		return DetectedPrinter{}, false
	}

	connection := normalizeDeviceClass(fields["class"], uri)

	// 型号来源按可信度排序：make-and-model → device-id 的 MFG/MDL → info → URI 路径。
	manufacturer, model := splitMakeAndModel(fields["make-and-model"])
	if model == "" {
		manufacturer, model = parseDeviceID(fields["device-id"])
	}
	if model == "" {
		manufacturer, model = splitMakeAndModel(fields["info"])
	}
	if model == "" {
		manufacturer, model = parseDeviceURI(uri)
	}

	return DetectedPrinter{
		DeviceURI:    uri,
		Manufacturer: manufacturer,
		Model:        model,
		Connection:   connection,
		DeviceID:     fields["device-id"],
		MakeAndModel: fields["make-and-model"],
		Info:         fields["info"],
		Location:     fields["location"],
		Scheme:       uriScheme(uri),
	}, true
}

// uriScheme 提取 URI 的 scheme 部分（小写），如 "usb"、"ipp"、"dnssd"。
func uriScheme(uri string) string {
	if idx := strings.Index(uri, "://"); idx > 0 {
		return strings.ToLower(uri[:idx])
	}
	return ""
}

// normalizeDeviceClass 把 lpinfo 的 class 与 URI scheme 归一成 usb / network / 原值。
func normalizeDeviceClass(class, uri string) string {
	switch {
	case strings.HasPrefix(uri, "usb://"):
		return "usb"
	case strings.HasPrefix(uri, "socket://"), strings.HasPrefix(uri, "lpd://"),
		strings.HasPrefix(uri, "ipp://"), strings.HasPrefix(uri, "ipps://"),
		strings.HasPrefix(uri, "http://"), strings.HasPrefix(uri, "https://"),
		strings.HasPrefix(uri, "dnssd://"), strings.HasPrefix(uri, "smb://"):
		return "network"
	}
	if class == "" {
		return "direct"
	}
	return class
}

// splitMakeAndModel 把 "HP LaserJet 1020" 这种整串拆成厂商 + 型号。
// CUPS 在拿不到信息时会填 "Unknown"，这种值等价于空。
func splitMakeAndModel(s string) (manufacturer, model string) {
	s = strings.TrimSpace(strings.Trim(strings.TrimSpace(s), `"`))
	if s == "" || strings.EqualFold(s, "unknown") {
		return "", ""
	}
	parts := strings.SplitN(s, " ", 2)
	manufacturer = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		model = strings.TrimSpace(parts[1])
	}
	// 只有一个词时（例如 "LBP2900"）当型号处理，厂商留空由驱动匹配自行兜底。
	if model == "" {
		return "", manufacturer
	}
	return manufacturer, model
}

// parseDeviceID 从 IEEE 1284 device-id 串里取 MFG/MDL（含 MANUFACTURER/MODEL 长写法）。
func parseDeviceID(id string) (manufacturer, model string) {
	for _, kv := range strings.Split(id, ";") {
		k, v, ok := strings.Cut(kv, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.ToUpper(strings.TrimSpace(k)) {
		case "MFG", "MANUFACTURER":
			manufacturer = v
		case "MDL", "MODEL":
			model = v
		}
	}
	// 型号里常带厂商前缀（"HP LaserJet 1020"），去重以免拼出 "HP HP LaserJet 1020"。
	if manufacturer != "" && strings.HasPrefix(strings.ToLower(model), strings.ToLower(manufacturer)+" ") {
		model = strings.TrimSpace(model[len(manufacturer):])
	}
	return
}

// ── 一键设置打印机 ─────────────────────────────────────────────────────────────

// POST /api/admin/drivers/setup — 安装驱动 + lpadmin 添加打印机，异步执行返回 202 + jobId。
//
// 请求体：
//
//	{deviceUri, driverName?, manufacturer?, model?, deviceId?,
//	 ppdUri?, printerName?, allowRaw?}
//
// ppdUri 三态："" = 自动匹配；"everywhere" = IPP Everywhere；"__raw__" = 显式 raw 队列；
// 其他 = 显式指定 ppd-name（必须存在于 lpinfo -m 的输出里）。
func adminSetupPrinterHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		DeviceURI    string `json:"deviceUri"`
		DriverName   string `json:"driverName"`
		Manufacturer string `json:"manufacturer"`
		Model        string `json:"model"`
		DeviceID     string `json:"deviceId"`
		PPDURI       string `json:"ppdUri"`
		PrinterName  string `json:"printerName"`
		AllowRaw     bool   `json:"allowRaw"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	payload.DeviceURI = strings.TrimSpace(payload.DeviceURI)
	if payload.DeviceURI == "" {
		writeJSONError(w, http.StatusBadRequest, "deviceUri is required")
		return
	}
	if payload.DriverName != "" {
		meta := findDriverByName(payload.DriverName)
		if meta == nil {
			writeJSONError(w, http.StatusNotFound, "unknown driver: "+payload.DriverName)
			return
		}
		arch := currentDebArch()
		if !driverSupportsArch(*meta, arch) {
			writeJSONError(w, http.StatusBadRequest,
				fmt.Sprintf("驱动 %s 不支持当前架构 %s", meta.DisplayName, arch))
			return
		}
	}

	// handler 层预校验 ppdUri（job 内会再复核一次）。
	ppdURI := strings.TrimSpace(payload.PPDURI)
	if ppdURI != "" && ppdURI != "everywhere" && ppdURI != "__raw__" {
		if err := ValidatePPDNameSyntax(ppdURI); err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("ppdUri 不合法: %v", err))
			return
		}
	}
	if ppdURI == "__raw__" && !payload.AllowRaw {
		writeJSONError(w, http.StatusBadRequest, "建立 raw 队列需要显式设置 allowRaw=true")
		return
	}
	if ppdURI == "everywhere" {
		scheme := uriScheme(payload.DeviceURI)
		if !driverlessSchemes[scheme] {
			writeJSONError(w, http.StatusBadRequest,
				fmt.Sprintf("%s 连接不支持 IPP Everywhere（仅 IPP 连接可用）", scheme))
			return
		}
	}

	req := payload
	job, busyID := startDriverJob("setup", req.DriverName, func(ctx context.Context, logBuf *safeBuffer) (map[string]any, error) {
		driverInstalled := false

		// 第 1 步：驱动未安装时先装。
		if req.DriverName != "" {
			manifestPath := filepath.Join(driversDataDir, req.DriverName, "manifest.txt")
			if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
				if err := runDriverCommand(ctx, logBuf, "/usr/local/bin/driver-install", req.DriverName); err != nil {
					return nil, fmt.Errorf("driver installation failed: %w", err)
				}
				driverInstalled = true
				invalidatePPDModels()
				refreshPPDEntriesAfterInstall(ctx, logBuf)
			} else {
				fmt.Fprintf(logBuf, "驱动 %s 已安装，跳过安装步骤\n", req.DriverName)
			}
		}

		// 第 2 步：确定厂商/型号（来源优先级修正）。
		manufacturer, model := strings.TrimSpace(req.Manufacturer), strings.TrimSpace(req.Model)
		if model == "" {
			manufacturer, model = parseDeviceID(req.DeviceID)
		}
		if model == "" {
			manufacturer, model = parseDeviceURI(req.DeviceURI)
		}

		// 第 3 步：决定 -m 参数（三态决策树）。
		ppdURI := strings.TrimSpace(req.PPDURI)
		decision := "auto-top1"
		var cands []PPDCandidate
		var dlInfo driverlessInfo

		switch {
		case ppdURI == "__raw__":
			// a) 显式 raw：不传 -m，大字告警。
			fmt.Fprintf(logBuf, "⚠⚠⚠ 建立 raw 队列（无驱动）：将无法选择纸盒/双面，多数打印机会打出乱码 ⚠⚠⚠\n")
			decision = "raw-explicit"

		case ppdURI == "everywhere":
			// b) 显式 IPP Everywhere。
			decision = "everywhere"
			fmt.Fprintf(logBuf, "使用 IPP Everywhere（-m everywhere）\n")

		case ppdURI != "":
			// c) 显式 ppd-name：job 内复核语义白名单。
			entries, err := cachedPPDEntries(ctx)
			if err != nil {
				return nil, fmt.Errorf("获取 PPD 列表失败: %w", err)
			}
			found := false
			for i := range entries {
				if entries[i].Name == ppdURI {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("指定的驱动不存在: %s（请重新扫描后再选）", ppdURI)
			}
			decision = "explicit"
			fmt.Fprintf(logBuf, "使用管理员指定的 PPD: %s\n", ppdURI)

		default:
			// d) 自动匹配。
			var matchErr error
			cands, dlInfo, _, matchErr = matchPPDCandidates(ctx, logBuf,
				manufacturer, model, req.DeviceID, req.DeviceURI, true, 5)
			if matchErr != nil {
				fmt.Fprintf(logBuf, "PPD 匹配出错: %v\n", matchErr)
			}
			ppdURI = bestPPDFromCandidates(cands)
			if ppdURI != "" {
				decision = "auto-top1"
				fmt.Fprintf(logBuf, "自动匹配 PPD: %s\n", ppdURI)
			} else if dlInfo.Available {
				ppdURI = "everywhere"
				decision = "everywhere-fallback"
				fmt.Fprintf(logBuf, "无型号匹配，降级使用 IPP Everywhere（-m everywhere）\n")
			} else {
				// d3) 一个候选都没有 → 报错，绝不静默建 raw。
				fmt.Fprintf(logBuf, "device-id: %s\n", req.DeviceID)
				fmt.Fprintf(logBuf, "解析结果: manufacturer=%q model=%q\n", manufacturer, model)
				return nil, fmt.Errorf(
					"未能为该打印机匹配到驱动。请在候选列表中手动选择，或确认建立 raw 队列（无驱动，将无法选择纸盒/双面，多数打印机会打出乱码）")
			}
		}

		// 第 4 步：队列名（去重 + 同 URI 拒绝覆盖）。
		queues, _ := listExistingQueues(ctx)
		if existing := findQueueByURI(queues, req.DeviceURI); existing != "" {
			return nil, fmt.Errorf("该设备已添加为队列 %s，如需重新配置请先删除该队列", existing)
		}
		base := req.PrinterName
		if base == "" {
			base = sanitizePrinterName(model)
		}
		if base == "" {
			base = sanitizePrinterName(manufacturer)
		}
		if base == "" {
			base = "Printer"
		}
		printerName, nameErr := uniquePrinterNameChecked(base, queues)
		if nameErr != nil {
			return nil, nameErr
		}
		renamedFrom := ""
		if printerName != base {
			renamedFrom = base
		}

		// 第 5 步：lpadmin 添加并启用。
		args := []string{"-p", printerName, "-E", "-v", req.DeviceURI}
		expectPPD := ppdURI != "" && ppdURI != "__raw__"
		if expectPPD {
			args = append(args, "-m", ppdURI)
		}
		lpadminErr := runDriverCommand(ctx, logBuf, "lpadmin", args...)
		// -m everywhere 失败时自动降级重试一次（lpadmin 失败时队列不会被创建，重试无副作用）。
		// 不用 bestPPDFromCandidates：它在有非 PostScript 替代时会跳过 PostScript（issue #117），
		// 但 everywhere 刚失败、此处需要任何可用候选兜底，包括 PostScript——跳过逻辑只作用于
		// 「自动推荐首选」，不作用于 everywhere 失败后的降级。同时显式排除 everywhere 本身（刚失败）。
		if lpadminErr != nil && ppdURI == "everywhere" && len(cands) > 0 {
			var fallback string
			for _, c := range cands {
				if c.PPD == "everywhere" || c.Source == PPDSourceGeneric || c.Confidence == ppdConfidenceLow {
					continue
				}
				fallback = c.PPD
				break
			}
			if fallback != "" {
				fmt.Fprintf(logBuf, "everywhere 失败，降级重试 PPD: %s\n", fallback)
				args2 := []string{"-p", printerName, "-E", "-v", req.DeviceURI, "-m", fallback}
				if err2 := runDriverCommand(ctx, logBuf, "lpadmin", args2...); err2 == nil {
					ppdURI = fallback
					decision = "auto-top1"
					expectPPD = true
					lpadminErr = nil
				}
			}
		}
		if lpadminErr != nil {
			return nil, fmt.Errorf("failed to add printer: %w", lpadminErr)
		}

		// 第 6 步：默认纸张设为 A4（国内场景），失败不影响整体成功。
		if err := runDriverCommand(ctx, logBuf, "lpadmin", "-p", printerName, "-o", "media=iso_a4_210x297mm"); err != nil {
			fmt.Fprintf(logBuf, "设置 A4 默认纸张失败（不影响使用）: %v\n", err)
		}

		// 第 7 步：验证队列是否真正生效。
		optionCount, mediaSourceCount, warnings := verifyPrinterQueue(ctx, logBuf, printerName, expectPPD)
		if expectPPD && optionCount == 0 {
			// PPD 没真正生效（"假成功 raw 队列"）→ 回滚删除。
			// 走到这里的队列一定是本次 lpadmin 新建的（前面 findQueueByURI 已拒绝过重复 URI）。
			fmt.Fprintf(logBuf, "PPD 未生效，回滚删除队列 %s\n", printerName)
			runDriverCommand(ctx, logBuf, "lpadmin", "-x", printerName)
			return nil, fmt.Errorf("PPD 未真正生效（lpoptions 输出为空），队列可能是 raw 模式")
		}
		for _, w := range warnings {
			fmt.Fprintf(logBuf, "⚠ %s\n", w)
		}

		result := map[string]any{
			"printerName":     printerName,
			"driverInstalled": driverInstalled,
			"ppdUsed":         ppdURI,
			"driverless":      ppdURI == "everywhere",
			"raw":             ppdURI == "__raw__" || ppdURI == "",
			"decision":        decision,
			"verify": map[string]any{
				"ppdEffective":     optionCount > 0,
				"optionCount":      optionCount,
				"mediaSourceCount": mediaSourceCount,
				"warnings":         warnings,
			},
		}
		if renamedFrom != "" {
			result["renamedFrom"] = renamedFrom
		}
		return result, nil
	})
	if job == nil {
		writeDriverJobBusy(w, busyID)
		return
	}

	log.Printf("[driver-setup] %s 任务已提交 (job=%s, driver=%q, user=%s)",
		req.DeviceURI, job.id, req.DriverName, sessionUsername(r))
	writeJSONStatus(w, http.StatusAccepted, map[string]any{"jobId": job.id, "name": req.DriverName})
}

// ── 上传自定义驱动 ─────────────────────────────────────────────────────────────

// POST /api/admin/drivers/upload — 上传自定义 PPD 或 .deb 包。
//
// ⚠️ 安全风险面（有意保留的管理员能力，但必须知情）：
// 上传 .deb 等价于把容器内 root 代码执行权交给管理员——dpkg 会以 root 执行包里的
// maintainer script（preinst/postinst 等），可以做任何事。该接口已受
// RequireSession + RequireAdmin + ValidateCSRF 三重保护，且每次上传都会把上传者
// 用户名写进日志用于审计；部署时请把管理员账号密码视作等同于容器 root 凭据。
func adminUploadDriverHandler(w http.ResponseWriter, r *http.Request) {
	// ParseMultipartForm 的参数是 **maxMemory（内存缓冲上限）而不是请求体上限**：
	// 超出部分 Go 会静默落到临时文件，所以单靠它拦不住超大上传（原注释写的
	// "50 MB limit" 是对 Go 语义的误解）。真正的硬上限要用 MaxBytesReader 包一层
	// r.Body，超限时 ParseMultipartForm 才会返回错误。
	// maxMemory 单独给一个小值（8MB），让大包 spool 到磁盘而不是整个塞进内存。
	r.Body = http.MaxBytesReader(w, r.Body, driverUploadMaxBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeJSONError(w, http.StatusBadRequest, "文件过大或表单格式错误（单个驱动文件上限 64MB）")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	// 显式做一次 Base + 白名单校验：不要依赖 multipart 实现内部恰好做了 filepath.Base，
	// 否则一旦标准库行为变化就会变成目录穿越写任意文件。
	filename, err := safeUploadFilename(header.Filename)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	username := sessionUsername(r)
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".ppd":
		content, err := io.ReadAll(file)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to read file")
			return
		}
		// 校验 PPD 头（只看开头一小段，PPD 规范要求首行就是 *PPD-Adobe）。
		head := content
		if len(head) > 256 {
			head = head[:256]
		}
		if !strings.Contains(string(head), "*PPD-Adobe") {
			writeJSONError(w, http.StatusBadRequest, "invalid PPD file (missing *PPD-Adobe header)")
			return
		}

		if err := installCustomPPD(filename, content); err != nil {
			log.Printf("[driver-upload] PPD %s 安装失败 (user=%s): %v", filename, username, err)
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to install PPD: %v", err))
			return
		}

		log.Printf("[driver-upload] 已安装 PPD: %s (user=%s)", filename, username)
		invalidatePPDModels()
		writeJSON(w, map[string]any{"ok": true, "type": "ppd", "filename": filename})

	case ".deb":
		// dpkg 有全局锁，正在跑的驱动任务会和它抢锁，直接拒绝更清晰。
		if busyID := runningDriverJobID(); busyID != "" {
			writeDriverJobBusy(w, busyID)
			return
		}

		tmpFile, err := os.CreateTemp("", "driver-upload-*.deb")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to create temp file")
			return
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		if _, err := io.Copy(tmpFile, file); err != nil {
			tmpFile.Close()
			writeJSONError(w, http.StatusInternalServerError, "failed to save file")
			return
		}
		if err := tmpFile.Close(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to save file")
			return
		}

		log.Printf("[driver-upload] 开始安装 deb: %s (user=%s)", filename, username)
		installLog, err := installDebPackage(r.Context(), tmpPath)
		if err != nil {
			log.Printf("[driver-upload] deb %s 安装失败 (user=%s): %v\n%s", filename, username, err, installLog)
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("package installation failed: %v", err))
			return
		}
		invalidatePPDModels()

		// 归档原件 + 登记 packages.txt，容器启动时由 restore-drivers 幂等 dpkg -i 装回来。
		// 归档失败不影响本次安装成功（文件已经装进容器了），只是重启后恢复不了，降级为警告。
		warning := ""
		if err := persistUploadedDeb(filename, tmpPath, username); err != nil {
			log.Printf("[driver-upload] deb %s 归档失败 (user=%s): %v", filename, username, err)
			warning = "该 .deb 安装成功但归档失败，容器重启后需要重新上传安装（且列表中不会显示该包）。"
		}

		log.Printf("[driver-upload] 已安装 deb: %s (user=%s)", filename, username)
		writeJSON(w, map[string]any{
			"ok":       true,
			"type":     "deb",
			"filename": filename,
			"warning":  warning,
			"log":      installLog,
		})

	default:
		writeJSONError(w, http.StatusBadRequest, "unsupported file type (use .ppd or .deb)")
	}
}

// safeUploadFilename 把上传文件名收敛为安全的纯文件名。
func safeUploadFilename(raw string) (string, error) {
	// Windows 客户端可能带反斜杠路径，filepath.Base 在 Linux 上不认，先手工切一次。
	if idx := strings.LastIndexAny(raw, `\/`); idx >= 0 {
		raw = raw[idx+1:]
	}
	name := filepath.Base(strings.TrimSpace(raw))
	if name == "" || name == "." || name == ".." || name == string(filepath.Separator) ||
		strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return "", errors.New("invalid file name")
	}
	return name, nil
}

// installCustomPPD 写入系统 PPD 目录，并持久化到驱动数据目录 + 追加 manifest，
// 让 restore-drivers.sh 在容器重启后能按 manifest 逐文件恢复。
func installCustomPPD(filename string, content []byte) error {
	if err := os.MkdirAll(customPPDInstallDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", customPPDInstallDir, err)
	}
	installPath := filepath.Join(customPPDInstallDir, filename)
	if err := os.WriteFile(installPath, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", installPath, err)
	}

	// 持久化副本的目录结构必须和 manifest 里的绝对路径一一对应：
	// restore-drivers.sh 是用 "${driver_dir}${filepath}" 拼源文件路径的。
	persistDir := filepath.Join(driversDataDir, customPPDDirName, customPPDInstallDir)
	if err := os.MkdirAll(persistDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", persistDir, err)
	}
	if err := os.WriteFile(filepath.Join(persistDir, filename), content, 0o644); err != nil {
		return fmt.Errorf("persist ppd: %w", err)
	}

	manifestPath := filepath.Join(driversDataDir, customPPDDirName, "manifest.txt")
	// manifest 里存的是容器内的绝对路径（restore-drivers.sh 直接当路径用），
	// 固定用 "/" 拼接而不是 filepath.Join，语义上就是 POSIX 路径。
	entry := customPPDInstallDir + "/" + filename
	if err := appendManifestLine(manifestPath, entry); err != nil {
		return fmt.Errorf("update manifest: %w", err)
	}

	// 与 driver-install.sh 保持同一套 metadata 键名，前端/排障脚本可以统一读。
	metaPath := filepath.Join(driversDataDir, customPPDDirName, "metadata.txt")
	meta := fmt.Sprintf("driver=%s\ninstalled_at=%s\narch=%s\n",
		customPPDDirName, time.Now().Format(time.RFC3339), currentDebArch())
	if err := os.WriteFile(metaPath, []byte(meta), 0o644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	return nil
}

// appendManifestLine 追加一条 manifest 记录，已存在则跳过（重复上传同名 PPD 时避免膨胀）。
func appendManifestLine(manifestPath, entry string) error {
	if data, err := os.ReadFile(manifestPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == entry {
				return nil
			}
		}
	}
	f, err := os.OpenFile(manifestPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := fmt.Fprintf(f, "%s\n", entry)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// installDebPackage 安装 .deb：dpkg -i 失败 → apt-get -f install 补依赖 → 再 dpkg -i 一次。
// 历史实现修完依赖后没有重装，等于白跑一趟 apt。
func installDebPackage(ctx context.Context, debPath string) (string, error) {
	var logBuf safeBuffer

	err := runDriverCommand(ctx, &logBuf, "dpkg", "-i", debPath)
	if err == nil {
		return logBuf.String(), nil
	}

	fmt.Fprintf(&logBuf, "dpkg -i 失败，尝试用 apt-get -f install 修复依赖后重试\n")
	if fixErr := runDriverCommand(ctx, &logBuf, "apt-get", "install", "-y", "-f", "--no-install-recommends"); fixErr != nil {
		return logBuf.String(), fmt.Errorf("dpkg -i 失败 (%v)，apt-get -f install 也失败 (%v)", err, fixErr)
	}
	if retryErr := runDriverCommand(ctx, &logBuf, "dpkg", "-i", debPath); retryErr != nil {
		return logBuf.String(), fmt.Errorf("修复依赖后 dpkg -i 仍失败: %w", retryErr)
	}
	return logBuf.String(), nil
}

// persistUploadedDeb 把上传的 .deb 原件归档到 {driversDataDir}/custom-deb/packages/，
// 并登记到 packages.txt，让 restore-drivers 在容器启动时用 `dpkg -i` 自动重装。
//
// 仍然**不写 manifest.txt**：那是文件级恢复用的（按绝对路径逐条 cp -a），对 .deb 毫无
// 意义——真正的安装动作在 maintainer script 里，把 .deb 拷到某个路径不会让驱动生效。
// 包级恢复才是 .deb 的正确归宿：restore-drivers 读 packages.txt（或直接 glob
// packages/*.deb 收养历史遗留的归档）→ dpkg -i，postinst 会照常执行。
//
// 这也修掉了一个长期的坑：以前这里只归档不登记，用户上传的 .deb 每次容器重启都得手动
// 重新上传一遍。
func persistUploadedDeb(filename, tmpPath, username string) error {
	baseDir := filepath.Join(driversDataDir, customDebDirName)
	pkgDir := filepath.Join(baseDir, "packages")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", pkgDir, err)
	}

	src, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	defer src.Close()

	destPath := filepath.Join(pkgDir, filename)
	dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dest, src); err != nil {
		dest.Close()
		return err
	}
	if err := dest.Close(); err != nil {
		return err
	}

	// 登记到 packages.txt，供 restore-drivers 的包级恢复使用。
	// 格式与 driver-install.sh 写的一致：<pkg> <version> <arch> <相对路径>。
	// 包名/版本/架构从 .deb 控制信息里取；取不到就退回用文件名当包名 —— restore 侧
	// 对拿不到版本的条目只做"文件存在就装"，不会因此漏装。
	pkgName, pkgVer, pkgArch := debControlFields(destPath)
	if pkgName == "" {
		pkgName = strings.TrimSuffix(filename, ".deb")
	}
	if err := appendCustomDebEntry(baseDir, pkgName, pkgVer, pkgArch, filename); err != nil {
		// 归档已经成功，登记失败只影响"自动重装"，不该让整个上传报错回滚。
		log.Printf("[drivers] WARNING: 登记 custom-deb packages.txt 失败（重启后需手动重装 %s）: %v", filename, err)
	}

	meta := fmt.Sprintf("driver=%s\ninstalled_at=%s\narch=%s\nuploaded_by=%s\nlast_package=%s\nrestore_mode=package\nmanifest_version=2\n",
		customDebDirName, time.Now().Format(time.RFC3339), currentDebArch(), username, filename)
	if err := os.WriteFile(filepath.Join(baseDir, "metadata.txt"), []byte(meta), 0o644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	return nil
}

// debControlFields 用 dpkg-deb -f 读出 .deb 的包名/版本/架构。
// 任何一步失败都返回空串，由调用方决定退路（绝不让上传流程因此失败）。
func debControlFields(debPath string) (name, version, arch string) {
	out, err := exec.Command("dpkg-deb", "-f", debPath, "Package", "Version", "Architecture").Output()
	if err != nil {
		return "", "", ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch strings.TrimSpace(key) {
		case "Package":
			name = val
		case "Version":
			version = val
		case "Architecture":
			arch = val
		}
	}
	return name, version, arch
}

// appendCustomDebEntry 把一条记录写进 custom-deb/packages.txt，按包名去重
// （同一个包重复上传新版本时替换旧行，避免 restore 时同名包出现两条）。
func appendCustomDebEntry(baseDir, pkgName, pkgVer, pkgArch, filename string) error {
	listPath := filepath.Join(baseDir, "packages.txt")

	var kept []string
	if data, err := os.ReadFile(listPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if fields := strings.Fields(line); len(fields) > 0 && fields[0] == pkgName {
				continue // 旧版本条目，丢掉
			}
			kept = append(kept, line)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	kept = append(kept, strings.Join([]string{pkgName, pkgVer, pkgArch, filename}, " "))
	return os.WriteFile(listPath, []byte(strings.Join(kept, "\n")+"\n"), 0o644)
}

// ── Helper functions ──────────────────────────────────────────────────────────

// sessionUsername 取当前登录用户名，用于审计日志（会话解不出来时返回 "unknown"）。
func sessionUsername(r *http.Request) string {
	sess, err := auth.GetSession(r)
	if err != nil || sess.Username == "" {
		return "unknown"
	}
	return sess.Username
}

// parseDeviceURI extracts manufacturer and model from a CUPS device URI.
//
// 不用 url.Parse：CUPS 的 usb:// URI 在 host 段用 %20 表示空格（"usb://Canon%20Inc./LBP2900"），
// 这违反 RFC 3986（host 段只允许 IPv6 用百分号编码），url.Parse 会直接报错。
// 所以手动按 scheme 拆分，对每段做 url.PathUnescape。
//
// 支持的 scheme：
//   - usb://Vendor/Model?serial=XXX
//   - dnssd://Instance%20Name._ipp._tcp.local/?uuid=...
//
// 明确不猜的 scheme：socket:// / lpd:// / ipp://<主机名> 的 host 是 IP 或主机名，
// 不含可靠型号信息。硬猜会把 "192.168.1.50" 当型号去匹配 PPD，比空更糟。
func parseDeviceURI(uri string) (manufacturer, model string) {
	scheme, rest, ok := strings.Cut(uri, "://")
	if !ok {
		return "", ""
	}
	switch strings.ToLower(scheme) {
	case "usb":
		// 去掉 query string。
		if idx := strings.Index(rest, "?"); idx >= 0 {
			rest = rest[:idx]
		}
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) >= 1 {
			manufacturer, _ = url.PathUnescape(parts[0])
		}
		if len(parts) >= 2 {
			model, _ = url.PathUnescape(parts[1])
		}
	case "dnssd":
		// 去掉 query string 和路径。
		if idx := strings.IndexAny(rest, "/?"); idx >= 0 {
			rest = rest[:idx]
		}
		host, _ := url.PathUnescape(rest)
		// 剥掉 DNS-SD service type 尾缀。
		for _, suffix := range []string{
			"._ipp._tcp.local", "._ipps._tcp.local",
			"._printer._tcp.local", "._pdl-datastream._tcp.local",
			"._scanner._tcp.local",
		} {
			if idx := strings.Index(strings.ToLower(host), suffix); idx > 0 {
				host = host[:idx]
				break
			}
		}
		manufacturer, model = splitMakeAndModel(host)
	}
	return
}

// sanitizePrinterName converts a model string into a valid CUPS printer name.
func sanitizePrinterName(model string) string {
	name := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, model)
	// Remove consecutive underscores.
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	return strings.Trim(name, "_")
}
