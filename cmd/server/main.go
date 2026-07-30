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
	// Приоритет аргументов командной строки выше переменных окружения.
	// Значение по умолчанию пустое, чтобы отличить "не указано" от "указано явно",
	// в итоге откатывается к :8080.
	listenFlag := flag.String("addr", "", "Адрес прослушивания, например :8080 или 0.0.0.0:8080 (приоритет выше переменной LISTEN_ADDR)")
	flag.Parse()

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
		log.Fatal("не удалось создать директорию данных: ", err)
	}
	var err error
	appStore, err = store.Open(context.Background(), dbPath)
	if err != nil {
		log.Fatal("не удалось открыть базу данных: ", err)
	}
	if err := ensureDefaultAdmin(context.Background()); err != nil {
		log.Fatal("не удалось создать администратора по умолчанию: ", err)
	}

	uploadDir = os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "uploads"
	}
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Fatal("не удалось создать директорию загрузок: ", err)
	}

	if err := auth.SetupSecureCookie(appStore.DB); err != nil {
		log.Fatal("не удалось настроить secure cookie: ", err)
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

	protected := api.PathPrefix("").Subrouter()
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
			http.Error(w, "не удалось получить список принтеров", http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(printers)
	}).Methods("GET")
	protected.HandleFunc("/print", printHandler).Methods("POST")
	protected.HandleFunc("/convert", convertHandler).Methods("POST")
	protected.HandleFunc("/compose", composeHandler).Methods("POST")
	protected.HandleFunc("/estimate", estimateHandler).Methods("POST")
	protected.HandleFunc("/print-records", printRecordsHandler).Methods("GET")
	protected.HandleFunc("/print-records/{id:[0-9]+}/file", printRecordFileHandler).Methods("GET")
	protected.HandleFunc("/print-records/{id:[0-9]+}/reprint", reprintHandler).Methods("POST")
	protected.HandleFunc("/printer-info", printerInfoHandler).Methods("GET")

	admin := api.PathPrefix("/admin").Subrouter()
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

	fmt.Println("listening on", addr)
	log.Fatal(srv.ListenAndServe())
}
