package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
)

func setupTestFontDir(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	originalFontDir := fontDir
	fontDir = tmpDir
	t.Cleanup(func() {
		fontDir = originalFontDir
	})
	return tmpDir
}

func TestAdminListFontsHandler(t *testing.T) {
	tmpDir := setupTestFontDir(t)

	// 创建测试文件
	if err := os.WriteFile(filepath.Join(tmpDir, "test1.ttf"), []byte("ttf content"), 0644); err != nil {
		t.Fatalf("failed to create font file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "test2.otf"), []byte("otf content"), 0644); err != nil {
		t.Fatalf("failed to create font file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "ignored.txt"), []byte("text content"), 0644); err != nil {
		t.Fatalf("failed to create non-font file: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/admin/fonts", nil)
	rr := httptest.NewRecorder()

	adminListFontsHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp fontListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	if len(resp.CustomFonts) != 2 {
		t.Fatalf("expected 2 custom fonts, got %d", len(resp.CustomFonts))
	}
	if resp.CustomFonts[0].Name != "test1.ttf" || resp.CustomFonts[1].Name != "test2.otf" {
		t.Fatalf("unexpected fonts list: %+v", resp.CustomFonts)
	}
}

func TestAdminUploadFontHandler(t *testing.T) {
	tmpDir := setupTestFontDir(t)

	// 1. 上传合法字体文件
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "uploaded.ttf")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := part.Write([]byte("fake-ttf-data")); err != nil {
		t.Fatalf("failed to write multipart data: %v", err)
	}
	writer.Close()

	req := httptest.NewRequest("POST", "/api/admin/fonts/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()

	adminUploadFontHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "uploaded.ttf")); os.IsNotExist(err) {
		t.Fatalf("uploaded file does not exist in fontDir")
	}

	// 2. 上传非法格式文件（如 .sh 或 .exe）
	badBody := &bytes.Buffer{}
	badWriter := multipart.NewWriter(badBody)
	badPart, err := badWriter.CreateFormFile("file", "malicious.sh")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	badPart.Write([]byte("#!/bin/sh\n"))
	badWriter.Close()

	badReq := httptest.NewRequest("POST", "/api/admin/fonts/upload", badBody)
	badReq.Header.Set("Content-Type", badWriter.FormDataContentType())
	badRR := httptest.NewRecorder()

	adminUploadFontHandler(badRR, badReq)

	if badRR.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid font extension, got %d", badRR.Code)
	}
}

func TestAdminDeleteFontHandler(t *testing.T) {
	tmpDir := setupTestFontDir(t)

	fontPath := filepath.Join(tmpDir, "to_delete.ttf")
	if err := os.WriteFile(fontPath, []byte("ttf"), 0644); err != nil {
		t.Fatalf("failed to create font: %v", err)
	}

	// 1. 正常删除
	r := mux.NewRouter()
	r.HandleFunc("/api/admin/fonts/{name}", adminDeleteFontHandler).Methods("DELETE")

	req := httptest.NewRequest("DELETE", "/api/admin/fonts/to_delete.ttf", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if _, err := os.Stat(fontPath); !os.IsNotExist(err) {
		t.Fatalf("expected font file to be deleted")
	}

	// 2. 重复删除已不存在的文件应返回 404
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 for deleted file, got %d", rr2.Code)
	}

	// 3. 校验路径穿越
	badReq := httptest.NewRequest("DELETE", "/api/admin/fonts/bad", nil)
	badReq = mux.SetURLVars(badReq, map[string]string{"name": "../../passwd.ttf"})
	badRR := httptest.NewRecorder()
	adminDeleteFontHandler(badRR, badReq)
	if badRR.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for path traversal attempt, got %d", badRR.Code)
	}
}

func TestAdminRefreshFontsHandler(t *testing.T) {
	_ = setupTestFontDir(t)

	req := httptest.NewRequest("POST", "/api/admin/fonts/refresh", nil)
	rr := httptest.NewRecorder()

	adminRefreshFontsHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	if res["ok"] != true {
		t.Fatalf("expected ok to be true, got %v", res["ok"])
	}
}
