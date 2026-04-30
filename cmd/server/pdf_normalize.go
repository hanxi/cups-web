package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// normalizePDFResult 描述一次 PDF 标准化的产物与元信息。
//
// OutputPath  标准化后的 PDF 绝对路径；passthrough 模式下即输入原路径
// Cleanup     临时目录清理函数（若无临时产物则为 nil）
// Method      实际走了哪条路径："ghostscript" / "libreoffice" / "passthrough"
// Warnings    降级原因、失败子步骤等诊断信息（仅用于日志，不参与控制流）
type normalizePDFResult struct {
	OutputPath string
	Cleanup    func()
	Method     string
	Warnings   []string
}

// normalizePDF 把任意 PDF 转成更兼容的版本：
//  1. 优先调用 Ghostscript `pdfwrite`，统一降级到 PDF 1.4 并强制嵌入所有字体
//     —— 解决 `UniGB-UCS2-H` 等外部 CMap + 字体未嵌入导致的打印乱码
//  2. Ghostscript 不可用或失败时，fallback 到 LibreOffice headless 重新导出
//  3. 两者都不可用时回退为 passthrough（原样返回），保证兜底打印链路不中断
//
// 对"工具未安装"和"工具执行失败"会打不同措辞的日志：
//   - 未安装：`[pdf-normalize] ghostscript not installed, skipped` —— 友好提醒
//   - 失败：`[pdf-normalize] ghostscript failed: <err>` —— 真实错误
//
// 失败不会返回 error（除非输入文件本身不可读），调用方只需关注 result.Method
// 来决定是否使用标准化后的产物。
func normalizePDF(ctx context.Context, inputPath string) (*normalizePDFResult, error) {
	if _, err := os.Stat(inputPath); err != nil {
		return nil, fmt.Errorf("normalizePDF: stat input: %w", err)
	}

	// 给标准化一个相对宽松的超时，避免个别 PDF 卡死整条打印链路。
	nctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	res := &normalizePDFResult{Method: "passthrough", OutputPath: inputPath}
	inName := filepath.Base(inputPath)

	// 1. Ghostscript 优先
	if path, cleanup, mode, err := runGhostscriptNormalize(nctx, inputPath); err == nil {
		res.Method = "ghostscript"
		res.OutputPath = path
		res.Cleanup = cleanup
		log.Printf("[pdf-normalize] method=ghostscript mode=%s in=%s out=%s", mode, inName, filepath.Base(path))
		return res, nil
	} else {
		res.Warnings = append(res.Warnings, summarizeToolError("ghostscript", err))
		if errors.Is(err, errBinaryNotInstalled) {
			log.Printf("[pdf-normalize] ghostscript not installed, skipped (install via `brew install ghostscript` or apt)")
		} else {
			log.Printf("[pdf-normalize] ghostscript failed, try libreoffice fallback: %v", err)
		}
	}

	// 2. LibreOffice fallback
	if path, cleanup, err := convertPDFViaLibreOffice(nctx, inputPath); err == nil {
		res.Method = "libreoffice"
		res.OutputPath = path
		res.Cleanup = cleanup
		log.Printf("[pdf-normalize] method=libreoffice in=%s out=%s", inName, filepath.Base(path))
		return res, nil
	} else {
		res.Warnings = append(res.Warnings, summarizeToolError("libreoffice", err))
		if errors.Is(err, errBinaryNotInstalled) {
			log.Printf("[pdf-normalize] libreoffice not installed, skipped (install via `brew install --cask libreoffice` or apt)")
		} else {
			log.Printf("[pdf-normalize] libreoffice failed: %v", err)
		}
	}

	// 3. passthrough
	log.Printf("[pdf-normalize] method=passthrough in=%s warnings=%v", inName, res.Warnings)
	return res, nil
}

// summarizeToolError 把外部工具的原始 error 转成短消息存进 Warnings：
//   - errBinaryNotInstalled → "ghostscript: not installed"
//   - 其它失败              → "ghostscript: <原始 error 首行>"
//
// 目的是让 Warnings 既能被日志友好展示，又不丢失必要的诊断信息。
func summarizeToolError(tool string, err error) string {
	if errors.Is(err, errBinaryNotInstalled) {
		return tool + ": not installed"
	}
	return tool + ": " + err.Error()
}

// runGhostscriptNormalize 调用 `gs` 把 PDF 重写为兼容性更好的 1.4 版本并嵌入所有字体。
//
// 采用两档参数，第一档失败再自动尝试第二档：
//
//	strict  —— `/prepress` 高质量模式，适用于标准合规的 PDF（Acrobat 正版、Office 导出等），
//	            强制 `EmbedAllFonts=true` 解决 CJK 外部 CMap 乱码问题
//	lenient —— 去掉 `/prepress`（避免 PDF/X 严格校验触发 syntaxerror），
//	            加上 `-dNEWPDF=false`（退回 gs 旧 PDF 解析器）和
//	            `-dPDFSTOPONERROR=false`（忽略非致命错误），
//	            专门应对 gs 10.x 对部分截图工具/老 Acrobat 生成的 PDF 报
//	            "syntaxerror in (binary token, type=N)" 这类硬失败
//
// 返回 (outputPath, cleanup, mode, err)。err 为 nil 时 mode ∈ {"strict","lenient"}。
// 若 gs 二进制不在 PATH 中，直接返回错误以便上层降级到 LibreOffice / passthrough。
func runGhostscriptNormalize(ctx context.Context, inputPath string) (string, func(), string, error) {
	gsBin, err := exec.LookPath("gs")
	if err != nil {
		// 返回共享的 errBinaryNotInstalled 哨兵错误，让 normalizePDF 能打"友好跳过"日志
		return "", nil, "", fmt.Errorf("ghostscript %w", errBinaryNotInstalled)
	}

	strictArgs := []string{
		"-dNOPAUSE", "-dBATCH", "-dQUIET", "-dSAFER",
		"-sDEVICE=pdfwrite",
		"-dCompatibilityLevel=1.4",
		"-dPDFSETTINGS=/prepress",
		"-dEmbedAllFonts=true",
		"-dSubsetFonts=true",
		"-dDetectDuplicateImages=true",
		"-dCompressFonts=true",
		"-dAutoRotatePages=/None",
	}
	// lenient：去掉 /prepress（避免 PDF/X 严格校验触发 syntaxerror），
	// 退回 gs 旧解析器，允许忽略非致命错误；嵌入字体仍然保留以解决乱码问题
	lenientArgs := []string{
    "-dNOPAUSE", "-dBATCH", "-dQUIET", "-dSAFER",
    "-sDEVICE=pdfwrite",
    "-dCompatibilityLevel=1.4",
    "-dPDFSETTINGS=/prepress",
    "-dCIDFontFallback=true",
    "-dNOPLATFONTS", // 防止选错字体
    "-dEmbedAllFonts=true",
    "-dSubsetFonts=true",
}

	if path, cleanup, err := tryGhostscriptRun(ctx, gsBin, strictArgs, inputPath, "strict"); err == nil {
		return path, cleanup, "strict", nil
	} else {
		log.Printf("[pdf-normalize] ghostscript strict failed, retry lenient: %v", err)
	}

	path, cleanup, err := tryGhostscriptRun(ctx, gsBin, lenientArgs, inputPath, "lenient")
	if err != nil {
		return "", nil, "", err
	}
	return path, cleanup, "lenient", nil
}

// tryGhostscriptRun 以给定参数集合执行一次 gs pdfwrite，成功时返回输出路径与清理函数。
// 失败时自动清理临时目录并返回"首行错误"摘要，避免 gs 堆栈几十行污染日志。
func tryGhostscriptRun(ctx context.Context, gsBin string, extraArgs []string, inputPath string, label string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "pdf-normalize-gs-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	outPath := filepath.Join(tmpDir, "normalized.pdf")

	args := append([]string{}, extraArgs...)
	args = append(args, "-sOutputFile="+outPath, inputPath)

	start := time.Now()
	cmd := exec.CommandContext(ctx, gsBin, args...)
	cmd.Env = append(os.Environ(), "LANG=C.UTF-8", "LC_ALL=C.UTF-8")
	out, err := cmd.CombinedOutput()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("gs pdfwrite(%s) failed: %w - %s", label, err, firstErrorLine(string(out)))
	}
	if st, err := os.Stat(outPath); err != nil || st.Size() == 0 {
		cleanup()
		return "", nil, fmt.Errorf("gs pdfwrite(%s) produced empty output: %v", label, err)
	}
	log.Printf("[pdf-normalize] ghostscript mode=%s elapsed=%s out=%s", label, time.Since(start).Round(time.Millisecond), filepath.Base(outPath))
	return outPath, cleanup, nil
}

// firstErrorLine 从外部工具（gs 等）的 CombinedOutput 中抽取首行含 "Error" 的内容
// 并截断到 200 字符。gs 失败时常吐大段 Operand/Execution/Dictionary stack，
// 全量记录噪音极大且毫无诊断价值。
func firstErrorLine(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "(no output)"
	}
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "Error") || strings.HasPrefix(trimmed, "**") {
			return truncate(trimmed, 200)
		}
	}
	// 找不到 "Error" 关键字时，返回首行前 200 字符作为兜底
	first := strings.SplitN(raw, "\n", 2)[0]
	return truncate(strings.TrimSpace(first), 200)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// fileHasPDFHeader 检查文件前若干字节是否以 "%PDF-" 开头。
// 用于在 passthrough 路径上做最基本的健全性校验。
func fileHasPDFHeader(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 8)
	n, _ := f.Read(buf)
	if n < 5 {
		return false
	}
	return strings.HasPrefix(string(buf[:n]), "%PDF-")
}

// diagnosePDF 对 PDF 做一次轻量诊断（仅日志），发现可能导致打印乱码的特征时输出警告。
// 不 panic、不返回 error；任何异常都被吞掉，保证主流程可用。
func diagnosePDF(path string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[pdf-diag] panic recovered: %v", r)
		}
	}()

	f, err := os.Open(path)
	if err != nil {
		log.Printf("[pdf-diag] open failed: %v", err)
		return
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		log.Printf("[pdf-diag] stat failed: %v", err)
		return
	}

	// 读 header（前 1KB）与 tail（后 8KB）即可覆盖版本号 / linearization / 外部 CMap 标志
	headBuf := make([]byte, 1024)
	hn, _ := f.ReadAt(headBuf, 0)
	head := string(headBuf[:hn])

	tailSize := int64(8192)
	if st.Size() < tailSize {
		tailSize = st.Size()
	}
	tailBuf := make([]byte, tailSize)
	_, _ = f.ReadAt(tailBuf, st.Size()-tailSize)
	tail := string(tailBuf)

	// 版本号
	version := "unknown"
	if m := regexp.MustCompile(`%PDF-(\d+\.\d+)`).FindStringSubmatch(head); len(m) == 2 {
		version = m[1]
	}

	linearized := strings.Contains(head, "/Linearized")

	// 粗略嗅探全文里的可疑 CMap / 未嵌入字体。scanner 只读前 512KB，避免在超大 PDF 上过度开销。
	suspiciousEncodings := map[string]bool{}
	if _, err := f.Seek(0, 0); err == nil {
		lr := &boundedReader{r: f, remaining: 512 * 1024}
		scanner := bufio.NewScanner(lr)
		scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
		patterns := []string{"UniGB-UCS2-H", "UniCNS-UCS2-H", "UniJIS-UCS2-H", "UniKS-UCS2-H", "GBK-EUC-H", "GB-EUC-H"}
		for scanner.Scan() {
			line := scanner.Text()
			for _, p := range patterns {
				if strings.Contains(line, p) {
					suspiciousEncodings[p] = true
				}
			}
		}
	}

	enc := make([]string, 0, len(suspiciousEncodings))
	for k := range suspiciousEncodings {
		enc = append(enc, k)
	}

	log.Printf("[pdf-diag] path=%s size=%d version=%s linearized=%v suspiciousCMaps=%v tailOK=%v",
		filepath.Base(path), st.Size(), version, linearized, enc, strings.Contains(tail, "%%EOF"))
}

// methodOf 安全地取 normalizePDFResult.Method，result 为 nil 时返回 "none"。
// 主要用于日志格式化，避免 nil 解引用 panic。
func methodOf(r *normalizePDFResult) string {
	if r == nil {
		return "none"
	}
	return r.Method
}

// boundedReader 限制从底层 Reader 读取的最大字节数，避免 diagnosePDF 在超大文件上耗时过长。
type boundedReader struct {
	r         interface{ Read(p []byte) (int, error) }
	remaining int64
}

func (b *boundedReader) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, fmt.Errorf("EOF-bounded")
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.r.Read(p)
	b.remaining -= int64(n)
	return n, err
}
