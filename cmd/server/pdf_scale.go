package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// 自定义缩放百分比的合法区间（前端也会 clamp，这里是最终防线）。
const (
	minScalePercent = 10
	maxScalePercent = 400
)

// parseScalePercent 判断 print_scaling 是否是「自定义百分比」形态。
// 前端选择「自定义」时直接把 print_scaling 填成纯数字（如 "40"），
// 其余取值（auto / fit / none ...）是 IPP keyword，原样透传给 CUPS。
func parseScalePercent(printScaling string) (int, bool) {
	p, err := strconv.Atoi(printScaling)
	if err != nil {
		return 0, false
	}
	return p, true
}

// resolveCustomScaling 落地自定义百分比缩放，返回：待打印文件路径、清理函数（可能为 nil）、
// 真正发给 IPP 的 print-scaling 取值。
//
// print-scaling 是 IPP keyword，纯数字不是合法取值，因此数字形态的 printScaling
// 绝不能透传给 CUPS，必须在这里换掉：
//   - 100 或 gs 缩放成功 → none（内容尺寸已经定死，再让 CUPS 缩放会二次变形）
//   - 非 PDF / gs 失败   → 空串（退回打印机默认，同时留日志）
func resolveCustomScaling(printPath, printScaling, printMime, logTag string) (string, func(), string) {
	percent, isCustom := parseScalePercent(printScaling)
	if !isCustom {
		return printPath, nil, printScaling
	}
	if percent < minScalePercent || percent > maxScalePercent {
		log.Printf("[%s] custom scaling %d%% out of range, ignored", logTag, percent)
		return printPath, nil, ""
	}
	// 100% 就是原尺寸打印，不必过一遍 gs，直接用 none 告诉 CUPS 别缩放。
	if percent == 100 {
		return printPath, nil, "none"
	}
	if printMime != "application/pdf" {
		log.Printf("[%s] custom scaling %d%% skipped: not a PDF (%s)", logTag, percent, printMime)
		return printPath, nil, ""
	}

	scaledPath := printPath + ".scaled.pdf"
	if err := scalePDFByPercent(context.Background(), printPath, scaledPath, percent); err != nil {
		log.Printf("[%s] custom scaling %d%% failed: %v", logTag, percent, err)
		_ = os.Remove(scaledPath)
		return printPath, nil, ""
	}
	return scaledPath, func() { _ = os.Remove(scaledPath) }, "none"
}

// scalePDFByPercent 用 Ghostscript 把 PDF 内容按 percent 百分比缩放并居中，页面尺寸保持不变。
//
// Install 过程从 currentpagedevice 读「当前这一页」的 PageSize 再算平移量，因此逐页自适应：
// 混合了纵向/横向或多种尺寸的文档不会被压成同一个纸张尺寸。
// 🚫 不要改回 -dFIXEDMEDIA + -dDEVICEWIDTHPOINTS 的固定纸张写法——横向页会被塞进纵向纸并偏移。
func scalePDFByPercent(ctx context.Context, inputPath, outputPath string, percent int) error {
	if _, err := exec.LookPath("gs"); err != nil {
		log.Printf("[scale] ghostscript not installed, skipping custom scaling")
		return err
	}

	scale := float64(percent) / 100.0
	offsetRatio := (1 - scale) / 2

	// 栈变化：w h → w*r h*r → translate → scale
	installCmd := fmt.Sprintf(
		"<</Install { currentpagedevice /PageSize get aload pop "+
			"%.6f mul exch %.6f mul exch translate %.6f %.6f scale }>> setpagedevice",
		offsetRatio, offsetRatio, scale, scale,
	)

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	// ⚠️ -sOutputFile 必须排在 -f 之前：-f 之后的参数会被 gs 当成输入文件名。
	// 曾经把它放末尾，gs 直接报 "requires an output file" 退出，缩放静默失效（Issue #98）。
	args := []string{
		"-sDEVICE=pdfwrite",
		"-dNOPAUSE", "-dBATCH", "-dQUIET", "-dSAFER",
		"-dCompatibilityLevel=1.4",
		"-sOutputFile=" + outputPath,
		"-c", installCmd,
		"-f", inputPath,
	}

	cmd := exec.CommandContext(ctx, "gs", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[scale] ghostscript failed: %v, output: %s", err, out)
		return err
	}

	log.Printf("[scale] scaled PDF to %d%% -> %s", percent, outputPath)
	return nil
}
