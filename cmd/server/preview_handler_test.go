package main

import (
	"bytes"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"testing"

	"github.com/phpdave11/gofpdf"
)

func twoPagePDF(t *testing.T) []byte {
	t.Helper()
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 16)
	for i := 1; i <= 2; i++ {
		pdf.AddPage()
		pdf.Text(20, 30, "page "+strconv.Itoa(i))
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func servePreview(t *testing.T, data []byte, page string) *httptest.ResponseRecorder {
	t.Helper()
	s, _ := scheduledTestStore(t)
	previous := appStore
	appStore = s
	t.Cleanup(func() { appStore = previous })

	fields := map[string]string{}
	if page != "" {
		fields["page"] = page
	}
	req := photoUploadRequest(t, "/api/preview-page", "file", data, 1, fields)
	rec := httptest.NewRecorder()
	previewPageHandler(rec, req)
	return rec
}

func TestPreviewPageRendersPNG(t *testing.T) {
	if _, err := exec.LookPath("gs"); err != nil {
		t.Skipf("gs not available: %v", err)
	}
	data := twoPagePDF(t)
	for _, page := range []string{"", "2"} {
		rec := servePreview(t, data, page)
		if rec.Code != http.StatusOK {
			t.Fatalf("page %q: status %d body %s", page, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Type"); got != "image/png" {
			t.Fatalf("page %q: Content-Type %q", page, got)
		}
		if got := rec.Header().Get("X-Page-Count"); got != "2" {
			t.Fatalf("page %q: X-Page-Count %q", page, got)
		}
		img, err := png.Decode(rec.Body)
		if err != nil {
			t.Fatalf("page %q: decode png: %v", page, err)
		}
		if b := img.Bounds(); b.Dx() < 1000 || b.Dy() <= b.Dx() {
			t.Fatalf("page %q: unexpected size %v", page, b)
		}
	}
}

func TestPreviewPageRejectsInvalidInput(t *testing.T) {
	pdf := twoPagePDF(t)
	cases := []struct {
		name string
		data []byte
		page string
	}{
		{"not pdf", []byte("hello"), ""},
		{"page out of range", pdf, "3"},
		{"page zero", pdf, "0"},
		{"page not number", pdf, "x"},
	}
	for _, tc := range cases {
		rec := servePreview(t, tc.data, tc.page)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (body %s)", tc.name, rec.Code, rec.Body.String())
		}
	}
}
