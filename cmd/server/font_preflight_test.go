package main

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeFontName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SimSun", "simsun"},
		{"Microsoft YaHei", "microsoftyahei"},
		{"仿宋_GB2312", "仿宋gb2312"},
		{"FZXiaoBiaoSong-B05S", "fzxiaobiaosongb05s"},
		{"Arial Unicode MS", "arialunicodems"},
	}

	for _, tt := range tests {
		got := normalizeFontName(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeFontName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestIsFontAvailable(t *testing.T) {
	systemFonts := map[string]struct{}{
		"simsun":          {},
		"simfang":         {},
		"notosanscjksc":   {},
		"dejavusans":      {},
	}

	// 别名命中测试
	if !isFontAvailable("宋体", systemFonts) {
		t.Errorf("expected '宋体' to be available via simsun alias")
	}
	if !isFontAvailable("仿宋_GB2312", systemFonts) {
		t.Errorf("expected '仿宋_GB2312' to be available via simfang alias")
	}
	if !isFontAvailable("FangSong", systemFonts) {
		t.Errorf("expected 'FangSong' to be available via simfang alias")
	}

	// 忽略字体测试
	if !isFontAvailable("Symbol", systemFonts) {
		t.Errorf("expected 'Symbol' to be ignored and considered available")
	}
	if !isFontAvailable("Wingdings", systemFonts) {
		t.Errorf("expected 'Wingdings' to be ignored and considered available")
	}

	// 确实缺失的字体
	if isFontAvailable("方正小标宋简体", systemFonts) {
		t.Errorf("expected '方正小标宋简体' to be missing")
	}
	if isFontAvailable("SimHei", systemFonts) {
		t.Errorf("expected 'SimHei' to be missing")
	}
}

func TestExtractDocxFonts(t *testing.T) {
	tmpDir := t.TempDir()
	docxPath := filepath.Join(tmpDir, "test.docx")

	f, err := os.Create(docxPath)
	if err != nil {
		t.Fatalf("create temp file failed: %v", err)
	}

	zw := zip.NewWriter(f)
	w, err := zw.Create("word/fontTable.xml")
	if err != nil {
		t.Fatalf("create zip entry failed: %v", err)
	}

	xmlContent := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:fonts xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
    <w:font w:name="Calibri"/>
    <w:font w:name="仿宋_GB2312"/>
    <w:font w:name="方正小标宋简体"/>
</w:fonts>`

	if _, err := w.Write([]byte(xmlContent)); err != nil {
		t.Fatalf("write zip content failed: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer failed: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file failed: %v", err)
	}

	fonts, err := extractDocxFonts(docxPath)
	if err != nil {
		t.Fatalf("extractDocxFonts failed: %v", err)
	}

	if len(fonts) != 3 {
		t.Fatalf("expected 3 fonts, got %d: %v", len(fonts), fonts)
	}

	expected := map[string]bool{
		"Calibri":   true,
		"仿宋_GB2312": true,
		"方正小标宋简体":  true,
	}

	for _, font := range fonts {
		if !expected[font] {
			t.Errorf("unexpected font: %s", font)
		}
	}
}

func TestIsFontFallbackAllowed(t *testing.T) {
	// 显式传参允许
	if !isFontFallbackAllowed(context.Background(), "true") {
		t.Errorf("expected isFontFallbackAllowed with 'true' to return true")
	}
}
