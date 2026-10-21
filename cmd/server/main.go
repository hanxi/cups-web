package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"cups-web/frontend"
	"cups-web/internal/auth"
	"cups-web/internal/ipp"
	"cups-web/internal/middleware"
	"cups-web/internal/server"
	"cups-web/internal/store"

	"github.com/gorilla/mux"
)

func main() {
	// 命令行参数优先级高于环境变量。
	// 默认值留空以便区分"用户未指定"与"显式指定"，最终再回退到 :8080。
	listenFlag := flag.String("addr", "", "监听地址，如 :8080 或 0.0.0.0:8080 (优先级高于 LISTEN_ADDR 环境变量)")
	flag.Parse()
	if err := ipp.ValidateCUPSConfig(); err != nil {
		log.Fatal("invalid CUPS connection configuration: ", err)
	}

	addr := *listenFlag
	if addr == "" {
		addr = os.Getenv("LISTEN_ADDR")
	}
	if addr == "" {
		addr = ":8080"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = filepath.Join("data", "cups-web.db")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatal("failed to create data dir: ", err)
	}
	var err error
	appStore, err = store.Open(context.Background(), dbPath)
	if err != nil {
		log.Fatal("failed to open database: ", err)
	}
	if err := ensureDefaultAdmin(context.Background()); err != nil {
		log.Fatal("failed to ensure default admin: ", err)
	}

	uploadDir = os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "uploads"
	}
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Fatal("failed to create uploads dir: ", err)
	}

	// SCAN_DIR:扫描产物落盘目录(issue #111)。生产环境挂持久卷,重启不丢历史扫描件。
	// scanDir 声明在 scan_registry.go 中,scan_handlers.go 通过 os.OpenInRoot 强制约束到目录内。
	scanDir = os.Getenv("SCAN_DIR")
	if scanDir == "" {
		scanDir = "scans"
	}
	if err := os.MkdirAll(scanDir, 0755); err != nil {
		log.Fatal("failed to create scans dir: ", err)
	}

	if err := auth.SetupSecureCookie(appStore.DB); err != nil {
		log.Fatal("failed to setup secure cookie: ", err)
	}

	r := mux.NewRouter()
	// 全局安全中间件：安全响应头 + 基于 Sec-Fetch-Site 的跨源 CSRF 防护
	// （Go 1.25 http.CrossOriginProtection，作为 double-submit 之外的纵深防御）。
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CrossOriginProtection())

	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/login", LoginHandler).Methods("POST")
	api.HandleFunc("/logout", LogoutHandler).Methods("POST")
	api.HandleFunc("/csrf", CSRFHandler).Methods("GET")
	// session endpoint used by frontend to detect existing session on page load
	api.HandleFunc("/session", SessionHandler).Methods("GET")
	// 公开的版本接口：前端在登录页与主界面 footer 上展示，
	// 用户二进制覆盖升级后无需登录即可确认当前运行版本（Issue #26）。
	api.HandleFunc("/version", VersionHandler).Methods("GET")
	// 公开的前端可读设置：登录前也要用来渲染，比如管理员配置的自定义 CSS（Issue #57）。
	api.HandleFunc("/public-settings", publicSettingsHandler).Methods("GET")

	protected := api.PathPrefix("").Subrouter()
	// APIKeyAuth 位于 RequireSession 之前：识别到合法 key 时把归属 Session
	// 注入 context，让下游 RequireSession/RequireAdmin 无差别放行；
	// ValidateCSRF 会豁免 API Key 通道的 double-submit 校验（issue #113）。
	protected.Use(middleware.APIKeyAuth(appStore))
	protected.Use(middleware.RequireSession)
	protected.Use(middleware.ValidateCSRF)
	protected.HandleFunc("/me", MeHandler).Methods("GET")
	protected.HandleFunc("/printers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		cupsHost := os.Getenv("CUPS_HOST")
		if cupsHost == "" {
			cupsHost = "localhost"
		}

		printers, err := ipp.ListPrinters(cupsHost)
		if err != nil {
			log.Printf("[printers] list failed: %v", err)
			http.Error(w, "failed to list printers", http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(printers)
	}).Methods("GET")
	protected.HandleFunc("/print", printHandler).Methods("POST")
	protected.HandleFunc("/convert", convertHandler).Methods("POST")
	protected.HandleFunc("/compose", composeHandler).Methods("POST")
	protected.HandleFunc("/estimate", estimateHandler).Methods("POST")
	protected.HandleFunc("/preview-page", previewPageHandler).Methods("POST")
	protected.HandleFunc("/print-records", printRecordsHandler).Methods("GET")
	protected.HandleFunc("/print-records/{id:[0-9]+}/file", printRecordFileHandler).Methods("GET")
	protected.HandleFunc("/print-records/{id:[0-9]+}/reprint", reprintHandler).Methods("POST")
	protected.HandleFunc("/printer-info", printerInfoHandler).Methods("GET")
	protected.HandleFunc("/printer-options", printerOptionsHandler).Methods("GET")
	// 定时打印（issue #28）：用户上传文件 + 打印参数 + 触发策略，由后台调度器到点触发。
	protected.HandleFunc("/scheduled-prints", scheduledListHandler).Methods("GET")
	protected.HandleFunc("/scheduled-prints", scheduledCreateHandler).Methods("POST")
	protected.HandleFunc("/scheduled-prints/{id:[0-9]+}", scheduledUpdateHandler).Methods("PUT")
	protected.HandleFunc("/scheduled-prints/{id:[0-9]+}", scheduledDeleteHandler).Methods("DELETE")
	protected.HandleFunc("/scheduled-prints/{id:[0-9]+}/run", scheduledRunNowHandler).Methods("POST")
	protected.HandleFunc("/scheduled-prints/{id:[0-9]+}/files/{index:[0-9]+}", scheduledThumbnailHandler).Methods("GET")
	// CUPS 任务列表 / 取消:任何登录用户可用,权限由 CUPS 按 owner 校验(issue #60)。
	protected.HandleFunc("/cups-jobs", cupsJobsHandler).Methods("GET")
	protected.HandleFunc("/cups-jobs/cancel", cupsCancelJobHandler).Methods("POST")

	// 扫描(issue #111):任何登录用户可用。hp-scan(HPLIP)在容器 dbus/HPLIP daemon
	// 依赖不稳定,即使补启 dbus-daemon 仍报 SANE code=9;这里改走 scanimage 子进程,
	// 与宿主 Debian 标准 SANE 栈的验证一致。同一台扫描仪的并发由 SANE 后端处理。
	protected.HandleFunc("/scan/devices", scanListDevicesHandler).Methods("GET")
	// devices/probe:选中设备后前端异步做一次可用性探测(issue #111 复测反馈),
	// 用来提示 hpaio 之类打不开的后端。走 query 避免 SANE URI 里的 ?/&/ 撞路由。
	protected.HandleFunc("/scan/devices/probe", scanProbeDeviceHandler).Methods("GET")
	protected.HandleFunc("/scan/options", scanListOptionsHandler).Methods("GET")
	protected.HandleFunc("/scan/jobs", scanCreateJobHandler).Methods("POST")
	// jobId 是 randomToken() 生成的不透明大写 base32 串,与 driver-job id 同样只放字母数字。
	protected.HandleFunc("/scan/jobs/{id:[A-Za-z0-9]+}", scanGetJobHandler).Methods("GET")
	protected.HandleFunc("/scan/jobs/{id:[A-Za-z0-9]+}", scanCancelJobHandler).Methods("DELETE")
	protected.HandleFunc("/scan/records", scanListRecordsHandler).Methods("GET")
	protected.HandleFunc("/scan/records/{id:[0-9]+}/file", scanDownloadHandler).Methods("GET")
	protected.HandleFunc("/scan/records/{id:[0-9]+}", scanDeleteRecordHandler).Methods("DELETE")

	// 打印预置（issue #128）：按用户保存多组打印参数，guest 在 handler 内拒绝。
	protected.HandleFunc("/print-presets", printPresetListHandler).Methods("GET")
	protected.HandleFunc("/print-presets", printPresetCreateHandler).Methods("POST")
	protected.HandleFunc("/print-presets/{id:[0-9]+}", printPresetUpdateHandler).Methods("PUT")
	protected.HandleFunc("/print-presets/{id:[0-9]+}", printPresetDeleteHandler).Methods("DELETE")

	// API 密钥自服务（issue #113）：登录用户管理自己名下的密钥。创建 / 删除
	// 已在 handler 内禁止 API Key 通道调用（避免密钥自繁殖）。
	protected.HandleFunc("/api-keys", apiKeyListHandler).Methods("GET")
	protected.HandleFunc("/api-keys", apiKeyCreateHandler).Methods("POST")
	protected.HandleFunc("/api-keys/{id:[0-9]+}", apiKeyDeleteHandler).Methods("DELETE")

	admin := api.PathPrefix("/admin").Subrouter()
	// admin 子路由同样先跑 APIKeyAuth：管理员用自己的 key 也能访问管理接口。
	admin.Use(middleware.APIKeyAuth(appStore))
	admin.Use(middleware.RequireSession)
	admin.Use(middleware.RequireAdmin)
	admin.Use(middleware.ValidateCSRF)
	admin.HandleFunc("/users", adminListUsersHandler).Methods("GET")
	admin.HandleFunc("/users", adminCreateUserHandler).Methods("POST")
	admin.HandleFunc("/users/{id:[0-9]+}", adminUpdateUserHandler).Methods("PUT")
	admin.HandleFunc("/users/{id:[0-9]+}", adminDeleteUserHandler).Methods("DELETE")
	admin.HandleFunc("/print-records", adminPrintRecordsHandler).Methods("GET")
	admin.HandleFunc("/settings", adminGetSettingsHandler).Methods("GET")
	admin.HandleFunc("/settings", adminUpdateSettingsHandler).Methods("PUT")
	admin.HandleFunc("/cleanup", adminCleanupHandler).Methods("POST")
	admin.HandleFunc("/drivers", adminListDriversHandler).Methods("GET")
	admin.HandleFunc("/drivers/install", adminInstallDriverHandler).Methods("POST")
	admin.HandleFunc("/drivers/remove", adminRemoveDriverHandler).Methods("POST")
	admin.HandleFunc("/drivers/detect", adminDetectPrintersHandler).Methods("GET")
	admin.HandleFunc("/drivers/ppds", adminListPPDCandidatesHandler).Methods("GET")
	admin.HandleFunc("/drivers/upload", adminUploadDriverHandler).Methods("POST")
	admin.HandleFunc("/drivers/setup", adminSetupPrinterHandler).Methods("POST")
	// 驱动安装/卸载/一键设置改为异步任务（编译型驱动几分钟，会被全局
	// WriteTimeout=120s 掐断），这里提供任务状态与增量日志的轮询入口。
	// jobId 是 randomToken() 生成的不透明大写 base32 串，故约束放宽到字母数字。
	admin.HandleFunc("/drivers/jobs/{id:[A-Za-z0-9]+}", adminDriverJobHandler).Methods("GET")

	// Static files (embedded) - register after API routes so /api/* is matched first
	serverFS := server.NewEmbeddedServer(frontend.FS)
	r.PathPrefix("/").Handler(serverFS)

	// 超时放宽：打印 / 转换接口需要在服务端处理大图（下采样 + gofpdf 合成）+
	// 回传 PDF 到移动端，15s 在 4G 上传 10M+ 照片时很容易超时（Issue #22）。
	// 为简化起见这里统一放到 2 分钟，业务上限比 Ghostscript 标准化管线的超时还要长一些。
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	startMaintenance(appStore, uploadDir)
	startScheduler(appStore, uploadDir)
	// 扫描设备发现缓存的启动预热(issue #111 复测反馈):避免第一位打开扫描页
	// 的用户等 ~17s。缓存被 SCAN_DEVICES_CACHE_TTL<=0 关闭时自动跳过。
	startScanPrewarm()

	fmt.Println("listening on", addr)
	log.Fatal(srv.ListenAndServe())
}
