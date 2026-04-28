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

	if err := auth.SetupSecureCookie(appStore.DB); err != nil {
		log.Fatal("failed to setup secure cookie: ", err)
	}

	r := mux.NewRouter()

	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/login", LoginHandler).Methods("POST")
	api.HandleFunc("/logout", LogoutHandler).Methods("POST")
	api.HandleFunc("/csrf", CSRFHandler).Methods("GET")
	// session endpoint used by frontend to detect existing session on page load
	api.HandleFunc("/session", SessionHandler).Methods("GET")

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
			http.Error(w, "failed to list printers: "+err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(printers)
	}).Methods("GET")
	protected.HandleFunc("/print", printHandler).Methods("POST")
	protected.HandleFunc("/convert", convertHandler).Methods("POST")
	protected.HandleFunc("/estimate", estimateHandler).Methods("POST")
	protected.HandleFunc("/print-records", printRecordsHandler).Methods("GET")
	protected.HandleFunc("/print-records/{id:[0-9]+}/file", printRecordFileHandler).Methods("GET")
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

	// Static files (embedded) - register after API routes so /api/* is matched first
	serverFS := server.NewEmbeddedServer(frontend.FS)
	r.PathPrefix("/").Handler(serverFS)

	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	startMaintenance(appStore, uploadDir)

	fmt.Println("listening on", addr)
	log.Fatal(srv.ListenAndServe())
}
