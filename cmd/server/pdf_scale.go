package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"
)

func paperDimensionsPoints(size string) (float64, float64) {
	const mmToPt = 2.83465
	switch size {
	case "A5":
		return 148 * mmToPt, 210 * mmToPt
	case "A3":
		return 297 * mmToPt, 420 * mmToPt
	case "A2":
		return 420 * mmToPt, 594 * mmToPt
	case "A1":
		return 594 * mmToPt, 841 * mmToPt
	case "5inch":
		return 89 * mmToPt, 127 * mmToPt
	case "6inch":
		return 102 * mmToPt, 152 * mmToPt
	case "7inch":
		return 127 * mmToPt, 178 * mmToPt
	case "8inch":
		return 152 * mmToPt, 203 * mmToPt
	case "10inch":
		return 203 * mmToPt, 254 * mmToPt
	case "Letter":
		return 612, 792
	case "Legal":
		return 612, 1008
	default: // A4
		return 595.28, 841.89
	}
}

func scalePDFByPercent(ctx context.Context, inputPath, outputPath string, percent int, paperSize string) error {
	if _, err := exec.LookPath("gs"); err != nil {
		log.Printf("[scale] ghostscript not installed, skipping custom scaling")
		return err
	}

	w, h := paperDimensionsPoints(paperSize)
	scale := float64(percent) / 100.0
	offsetX := w * (1 - scale) / 2
	offsetY := h * (1 - scale) / 2

	installCmd := fmt.Sprintf(
		"<</Install { %.4f %.4f translate %.4f %.4f scale }>> setpagedevice",
		offsetX, offsetY, scale, scale,
	)

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	args := []string{
		"-sDEVICE=pdfwrite",
		"-dNOPAUSE", "-dBATCH", "-dQUIET", "-dSAFER",
		"-dCompatibilityLevel=1.4",
		"-dFIXEDMEDIA",
		fmt.Sprintf("-dDEVICEWIDTHPOINTS=%.0f", w),
		fmt.Sprintf("-dDEVICEHEIGHTPOINTS=%.0f", h),
		"-c", installCmd,
		"-f", inputPath,
		"-sOutputFile=" + outputPath,
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
