package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rsc.io/pdf"
)

type fileKind string

const (
	fileKindPDF    fileKind = "pdf"
	fileKindImage  fileKind = "image"
	fileKindText   fileKind = "text"
	fileKindOffice fileKind = "office"
	fileKindOther  fileKind = "other"
)

const textLinesPerPage = 60
const convertedSuffix = ".print.pdf"

func sanitizeFilename(name string) string {
	base := filepath.Base(name)
	ext := filepath.Ext(base)
	baseName := strings.TrimSuffix(base, ext)
	safeBase := sanitizeNamePart(baseName)
	if safeBase == "" {
		safeBase = "file"
	}
	safeExt := sanitizeExtPart(ext)
	return safeBase + safeExt
}

func sanitizeNamePart(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_-")
}

func sanitizeExtPart(ext string) string {
	if ext == "" {
		return ""
	}
	ext = strings.ToLower(ext)
	var b strings.Builder
	for _, r := range ext {
		if r == '.' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	safe := b.String()
	if safe == "." {
		return ""
	}
	return safe
}

func saveUploadedFile(file io.Reader, filename string, baseDir string) (string, string, error) {
	subDir := time.Now().UTC().Format("20060102")
	absDir := filepath.Join(baseDir, subDir)
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return "", "", err
	}
	safe := sanitizeFilename(filename)
	storedName := fmt.Sprintf("%s_%s_%s", time.Now().UTC().Format("20060102T150405Z"), randomToken(), safe)
	absPath := filepath.Join(absDir, storedName)
	out, err := os.Create(absPath)
	if err != nil {
		return "", "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		return "", "", err
	}
	relPath := filepath.ToSlash(filepath.Join(subDir, storedName))
	return relPath, absPath, nil
}

func convertedRelPath(storedRel string) string {
	if storedRel == "" {
		return ""
	}
	return storedRel + convertedSuffix
}

func saveConvertedPDFToUploads(tempPath string, storedRel string, baseDir string) (string, string, error) {
	convertedRel := convertedRelPath(storedRel)
	absPath := filepath.Join(baseDir, filepath.FromSlash(convertedRel))
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return "", "", err
	}
	in, err := os.Open(tempPath)
	if err != nil {
		return "", "", err
	}
	defer in.Close()
	out, err := os.Create(absPath)
	if err != nil {
		return "", "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		_ = os.Remove(absPath)
		return "", "", err
	}
	return convertedRel, absPath, nil
}

func saveTempUpload(file io.Reader, filename string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "estimate-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	absPath := filepath.Join(tmpDir, sanitizeFilename(filename))
	out, err := os.Create(absPath)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		cleanup()
		return "", nil, err
	}
	return absPath, cleanup, nil
}

func detectFileKind(path string, name string) fileKind {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".pdf" {
		return fileKindPDF
	}
	if isOfficeFile(name) {
		return fileKindOffice
	}
	f, err := os.Open(path)
	if err != nil {
		return fileKindOther
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	mime := http.DetectContentType(buf[:n])
	if mime == "application/pdf" {
		return fileKindPDF
	}
	if strings.HasPrefix(mime, "image/") {
		return fileKindImage
	}
	if strings.HasPrefix(mime, "text/") || ext == ".txt" || ext == ".md" || ext == ".html" {
		return fileKindText
	}
	return fileKindOther
}

func isOfficeFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".doc", ".docx", ".ppt", ".pptx", ".xls", ".xlsx":
		return true
	default:
		return false
	}
}

func countPages(ctx context.Context, path string, name string) (int, bool, error) {
	kind := detectFileKind(path, name)
	switch kind {
	case fileKindPDF:
		pages, err := countPDFPages(path)
		return pages, false, err
	case fileKindImage:
		return 1, false, nil
	case fileKindText:
		pages, err := estimateTextPages(path)
		return pages, true, err
	case fileKindOffice:
		outPath, cleanup, err := convertOfficeToPDF(ctx, path)
		if err != nil {
			return 0, false, err
		}
		defer cleanup()
		pages, err := countPDFPages(outPath)
		return pages, false, err
	default:
		return 1, true, nil
	}
}

func countPDFPages(path string) (int, error) {
	doc, err := pdf.Open(path)
	if err != nil {
		return 0, err
	}
	if doc.NumPage() < 1 {
		return 1, nil
	}
	return doc.NumPage(), nil
}

func estimateTextPages(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lines := 0
	for scanner.Scan() {
		lines++
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	if lines < 1 {
		lines = 1
	}
	pages := (lines + textLinesPerPage - 1) / textLinesPerPage
	if pages < 1 {
		pages = 1
	}
	return pages, nil
}
