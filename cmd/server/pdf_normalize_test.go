package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// execLookPath 是对 exec.LookPath 的别名封装，便于测试中替换或降低耦合。
var execLookPath = exec.LookPath

// minimalPDF 是一段最小可被 pdf.Open 解析的 PDF 字节流，用于 smoke test。
// 结构：%PDF-1.4 + 1 个 Pages 对象 + 1 个 Page 对象 + xref + trailer + %%EOF。
var minimalPDF = []byte(`%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Count 1 /Kids [3 0 R] >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>
endobj
xref
0 4
0000000000 65535 f 
0000000009 00000 n 
0000000058 00000 n 
0000000111 00000 n 
trailer
<< /Size 4 /Root 1 0 R >>
startxref
174
%%EOF
`)

func writeTempFile(t *testing.T, content []byte, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return path
}

func TestFileHasPDFHeader(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    bool
	}{
		{"valid PDF", minimalPDF, true},
		{"fake text", []byte("hello world this is not a pdf"), false},
		{"too short", []byte("%PD"), false},
		{"empty", []byte{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempFile(t, tt.content, "t.bin")
			got := fileHasPDFHeader(path)
			if got != tt.want {
				t.Errorf("fileHasPDFHeader(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestFileHasPDFHeader_MissingFile(t *testing.T) {
	if fileHasPDFHeader("/nonexistent/path/xxx.pdf") {
		t.Error("expected false for missing file")
	}
}

func TestDiagnosePDF_DoesNotPanic(t *testing.T) {
	// 1. 合法 PDF
	validPath := writeTempFile(t, minimalPDF, "valid.pdf")
	diagnosePDF(validPath)

	// 2. 截断的损坏 PDF
	corruptedPath := writeTempFile(t, []byte("%PDF-1.4\nthis is corrupted"), "bad.pdf")
	diagnosePDF(corruptedPath)

	// 3. 根本不存在的文件
	diagnosePDF("/this/does/not/exist.pdf")

	// 4. 空文件
	emptyPath := writeTempFile(t, nil, "empty.pdf")
	diagnosePDF(emptyPath)
}

// TestNormalizePDF_MinimalPDF 验证：normalizePDF 不会对输入文件报错，
// 且 Method ∈ {"ghostscript","libreoffice","passthrough"}——具体走哪条取决于
// 本机是否装了 gs / libreoffice。本用例在有 gs 的开发机上会走 ghostscript 路径；
// 在未装任何外部工具的环境走 passthrough。
func TestNormalizePDF_MinimalPDF(t *testing.T) {
	path := writeTempFile(t, minimalPDF, "mini.pdf")
	ctx := context.Background()
	res, err := normalizePDF(ctx, path)
	if err != nil {
		t.Fatalf("normalizePDF err: %v", err)
	}
	if res == nil {
		t.Fatal("res is nil")
	}
	switch res.Method {
	case "ghostscript", "libreoffice":
		// 真实标准化路径：输出文件必须存在
		if st, err := os.Stat(res.OutputPath); err != nil || st.Size() == 0 {
			t.Errorf("normalized output missing or empty: %v", err)
		}
		if res.Cleanup != nil {
			res.Cleanup()
		}
	case "passthrough":
		if res.OutputPath != path {
			t.Errorf("passthrough OutputPath = %q, want %q", res.OutputPath, path)
		}
	default:
		t.Errorf("unexpected Method=%q", res.Method)
	}
}

func TestNormalizePDF_MissingFile(t *testing.T) {
	_, err := normalizePDF(context.Background(), "/definitely/missing/xxx.pdf")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// TestNormalizePDF_FriendlyWarningsWhenToolsMissing 验证：当 gs 和 libreoffice
// 都没装时，Warnings 中应输出**短消息**（"ghostscript: not installed" / "libreoffice: not installed"），
// 不能再出现以前那种 `exec: "xxx": executable file not found in $PATH` 的长噪声串。
// 降级链最终会落到 passthrough，OutputPath 应等于输入原路径。
//
// 通过把 PATH 临时清空来模拟两个工具都未安装。
func TestNormalizePDF_FriendlyWarningsWhenToolsMissing(t *testing.T) {
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	emptyDir := t.TempDir()
	_ = os.Setenv("PATH", emptyDir)

	path := writeTempFile(t, minimalPDF, "mini.pdf")
	res, err := normalizePDF(context.Background(), path)
	if err != nil {
		t.Fatalf("normalizePDF err: %v", err)
	}
	if res == nil {
		t.Fatal("res is nil")
	}
	if res.Method != "passthrough" {
		t.Errorf("Method = %q, want passthrough", res.Method)
	}
	if res.OutputPath != path {
		t.Errorf("OutputPath = %q, want original path %q", res.OutputPath, path)
	}

	// 两条短消息都必须出现
	wantWarnings := []string{"ghostscript: not installed", "libreoffice: not installed"}
	for _, want := range wantWarnings {
		found := false
		for _, w := range res.Warnings {
			if w == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected warning %q in Warnings, got: %v", want, res.Warnings)
		}
	}

	// 绝不应出现之前那种长噪声串
	forbidden := "executable file not found"
	for _, w := range res.Warnings {
		if strings.Contains(w, forbidden) {
			t.Errorf("Warnings should NOT contain %q noise, got: %v", forbidden, res.Warnings)
		}
	}
}

// TestSummarizeToolError 覆盖日志摘要辅助函数：
//   - errBinaryNotInstalled → "tool: not installed"
//   - 普通错误             → "tool: <原 error 信息>"
func TestSummarizeToolError(t *testing.T) {
	gotNotInstalled := summarizeToolError("ghostscript", fmt.Errorf("ghostscript %w", errBinaryNotInstalled))
	if gotNotInstalled != "ghostscript: not installed" {
		t.Errorf("not-installed case = %q, want %q", gotNotInstalled, "ghostscript: not installed")
	}

	realErr := fmt.Errorf("gs pdfwrite(strict) failed: exit status 1 - Error: /syntaxerror")
	gotReal := summarizeToolError("ghostscript", realErr)
	if !strings.HasPrefix(gotReal, "ghostscript: ") {
		t.Errorf("real-err case should start with 'ghostscript: ', got: %q", gotReal)
	}
	if !strings.Contains(gotReal, "syntaxerror") {
		t.Errorf("real-err case should preserve original err text, got: %q", gotReal)
	}
}

// TestFirstErrorLine 覆盖 gs 错误日志的精简逻辑：从大段堆栈输出里抽出首个 "Error" 行，
// 避免 gs 报错时把 Operand/Execution/Dictionary stack 几十行全部打进日志。
func TestFirstErrorLine(t *testing.T) {
	gsBinaryTokenDump := `GPL Ghostscript 10.07.0 (2024-05-02)
Copyright (C) 2024 Artifex Software, Inc.

Error: /syntaxerror in (binary token, type=137)
Operand stack:

Execution stack:
   %interp_exit   .runexec2   --nostringval--
Dictionary stack:
   --dict:747/1123(ro)(G)--   --dict:0/20(G)--
Current allocation mode is local
GPL Ghostscript 10.07.0: Unrecoverable error, exit code 1`

	tests := []struct {
		name   string
		raw    string
		expect string
	}{
		{"binary token syntax error", gsBinaryTokenDump, "Error: /syntaxerror in (binary token, type=137)"},
		{"empty", "", "(no output)"},
		{"only whitespace", "   \n\t  \n", "(no output)"},
		{"no error keyword", "hello world\nfoo bar", "hello world"},
		{"error first line", "Error: something bad", "Error: something bad"},
		{"starred warning", "**** Warning: xxx", "**** Warning: xxx"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstErrorLine(tt.raw)
			if got != tt.expect {
				t.Errorf("firstErrorLine(%q) = %q, want %q", tt.name, got, tt.expect)
			}
		})
	}
}

func TestFirstErrorLine_Truncated(t *testing.T) {
	// 超过 200 字符的错误行应被截断并带 '…' 后缀
	long := "Error: " + strings.Repeat("x", 500)
	got := firstErrorLine(long)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected suffix '…', got: %q", got[len(got)-10:])
	}
	if !strings.HasPrefix(got, "Error: ") {
		t.Errorf("expected prefix 'Error: ', got: %q", got[:20])
	}
}

// TestRunGhostscriptNormalize_StrictOnMinimalPDF 验证本地有 gs 时，strict 档对最小
// PDF 的处理能力。无 gs 的环境会跳过。
func TestRunGhostscriptNormalize_StrictOnMinimalPDF(t *testing.T) {
	if _, err := lookPathSafe("gs"); err != nil {
		t.Skipf("gs not available: %v", err)
	}
	path := writeTempFile(t, minimalPDF, "mini.pdf")
	outPath, cleanup, mode, err := runGhostscriptNormalize(context.Background(), path)
	if err != nil {
		t.Fatalf("runGhostscriptNormalize err: %v", err)
	}
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()
	if mode != "strict" && mode != "lenient" {
		t.Errorf("unexpected mode=%q", mode)
	}
	if st, err := os.Stat(outPath); err != nil || st.Size() == 0 {
		t.Errorf("output missing or empty: path=%s err=%v", outPath, err)
	}
}

// TestRunGhostscriptNormalize_MissingBinary 验证 gs 不在 PATH 时：
//  1. 不会 panic、不会产生临时文件泄漏
//  2. 返回的 error 可被 errors.Is 识别为 errBinaryNotInstalled
//     —— normalizePDF 依赖这个契约来打"友好跳过"日志
//  3. 错误消息带有人类可读的工具名 "ghostscript"，而不是裸的 gs exec 错误
func TestRunGhostscriptNormalize_MissingBinary(t *testing.T) {
	origPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", origPath) })
	emptyDir := t.TempDir()
	_ = os.Setenv("PATH", emptyDir)

	path := writeTempFile(t, minimalPDF, "mini.pdf")
	_, _, _, err := runGhostscriptNormalize(context.Background(), path)
	if err == nil {
		t.Fatal("expected error when gs missing")
	}
	if !errors.Is(err, errBinaryNotInstalled) {
		t.Errorf("expected errors.Is(err, errBinaryNotInstalled), got: %v", err)
	}
	if !strings.Contains(err.Error(), "ghostscript") {
		t.Errorf("expected 'ghostscript' in err message, got: %v", err)
	}
}

// lookPathSafe 包装 exec.LookPath 便于在测试中按需 skip。
func lookPathSafe(name string) (string, error) {
	return execLookPath(name)
}
