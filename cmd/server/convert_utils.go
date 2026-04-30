package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// errBinaryNotInstalled 是一个跨模块共享的哨兵错误，表示某个**外部命令行工具**
// （gs / libreoffice 等）在本机 PATH 中找不到。
//
// 该错误**仅用于日志措辞**：PDF 标准化管线拿到它可以打友好的"已跳过"日志，
// 而不是把一大串 `exec: "xxx": executable file not found in $PATH` 噪声丢给运维/开发看。
// 降级控制流不受影响——不论是"未安装"还是"执行失败"，上层都会继续尝试下一级 fallback。
//
// 注意：返回这个错误时，error message 的前缀（"libreoffice" / "ghostscript"）由各
// 包装函数自行决定，errors.Is 只用于识别"工具未安装"这一类型。
var errBinaryNotInstalled = errors.New("not installed")

// runLibreOfficeConvert 调用 libreoffice --headless --convert-to <filter> 做通用文档转换，
// 返回生成的 PDF 绝对路径、清理函数与错误。filter 为空时默认使用 "pdf"。
//
// libreoffice 不在 PATH 中时，返回的错误可被 errors.Is(err, errBinaryNotInstalled) 识别，
// 供 PDF 标准化管线做"友好跳过"日志处理；Office 文档转换场景则正常向上报错即可。
func runLibreOfficeConvert(ctx context.Context, inputPath string, filter string) (string, func(), error) {
	if _, err := exec.LookPath("libreoffice"); err != nil {
		return "", nil, fmt.Errorf("libreoffice %w", errBinaryNotInstalled)
	}

	tmpDir, err := os.MkdirTemp("", "convert-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	convertTo := filter
	if convertTo == "" {
		convertTo = "pdf"
	}
	cmd := exec.CommandContext(ctx, "libreoffice", "--headless", "--convert-to", convertTo, "--outdir", tmpDir, inputPath)
	cmd.Env = append(os.Environ(), "LANG=zh_CN.UTF-8", "LC_ALL=zh_CN.UTF-8")
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("conversion failed: %w - %s", err, string(out))
	}

	base := filepath.Base(inputPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	outPath := filepath.Join(tmpDir, name+".pdf")
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		matches, _ := filepath.Glob(filepath.Join(tmpDir, "*.pdf"))
		if len(matches) == 0 {
			cleanup()
			return "", nil, fmt.Errorf("conversion produced no PDF")
		}
		outPath = matches[0]
	}

	return outPath, cleanup, nil
}

// convertOfficeToPDF 将 Office 文档（.doc/.docx/.xls/.xlsx/.ppt/.pptx）转成 PDF。
func convertOfficeToPDF(ctx context.Context, inputPath string) (string, func(), error) {
	return runLibreOfficeConvert(ctx, inputPath, "pdf")
}

// convertPDFViaLibreOffice 通过 LibreOffice 重新导出 PDF，用作 Ghostscript 不可用时的兜底。
// LibreOffice 对部分 Acrobat 高版本 PDF 的解析好于原生 rsc.io/pdf，但兼容性仍不如 Ghostscript，
// 因此仅在 gs 路径失败后调用。
func convertPDFViaLibreOffice(ctx context.Context, inputPath string) (string, func(), error) {
	return runLibreOfficeConvert(ctx, inputPath, "pdf")
}

func convertOFDToPDF(ctx context.Context, inputPath string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "convert-ofd-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	outPath := filepath.Join(tmpDir, "output.pdf")

	jarPath := os.Getenv("OFD_CONVERTER_JAR")
	if jarPath == "" {
		jarPath = "/ofd-converter.jar"
	}

	cmd := exec.CommandContext(ctx, "java", "-Xmx512m", "-jar", jarPath, inputPath, outPath)
	cmd.Env = append(os.Environ(), "LANG=zh_CN.UTF-8", "LC_ALL=zh_CN.UTF-8")
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("OFD to PDF conversion failed: %w - %s", err, string(out))
	}

	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		cleanup()
		return "", nil, fmt.Errorf("OFD to PDF conversion produced no output")
	}

	return outPath, cleanup, nil
}

func convertTimeoutContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 60*time.Second)
}
