package main

import (
	"bufio"
	"errors"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/phpdave11/gofpdf"
	"golang.org/x/image/draw"
)

const pdfPageMarginMM = 10.0

// 大图下采样阈值：长边超过 imageDownscaleMaxEdge 时，会先缩放到该值再交给 gofpdf 嵌入，
// 避免把原始 10+MB 的手机照片整张塞进 PDF —— 移动端下载/预览链路会因此失败或超时（Issue #22）。
// 打印场景 3000px + JPEG Q85 对 A4/A3 来说分辨率已远超 300dpi，画质损失可忽略。
const (
	imageDownscaleMaxEdge = 3000
	imageDownscaleJPEGQ   = 85
)

// downscaleSeq 为每个下采样输出文件分配一个进程内单调递增序号，彻底规避同目录文件名碰撞。
// 使用 atomic 保证并发安全（不同请求可能并发处理）。
var downscaleSeq uint64

// downscaleImageIfNeeded 在必要时把图片下采样到长边 imageDownscaleMaxEdge 以内并以 JPEG 写出。
// 返回值：
//   - outPath：可供 gofpdf 读取的图片路径（当未缩放时为原 inputPath；已缩放时为 tmpDir 下的新 JPEG）
//   - cfg：最终用于布局计算的尺寸信息（Width/Height 已反映缩放结果）
//   - err：任何 I/O / 解码 / 编码错误
//
// 规则：
//  1. 只有长边严格大于阈值时才执行缩放；小图原样返回，避免二次有损编码。
//  2. 为了统一输出格式（减小 PDF 体积、避免 gofpdf 对 PNG 透明度的处理分支），缩放后一律编码为 JPEG。
//     JPEG 不支持透明通道，因此在绘制前先把目标画布整体填白，再把源图用 draw.Over 合成上去，
//     这样带 alpha 的 PNG 也能得到"白底 + 前景"的正确打印效果，而不是黑底。
//  3. 缩放算法使用 CatmullRom（质量 / 性能 折中），比 draw.ApproxBiLinear 锐利，比 draw.BiLinear 清晰。
func downscaleImageIfNeeded(inputPath string, tmpDir string) (string, image.Config, error) {
	// 先读尺寸，避免对小图做无谓的整图解码
	f, err := os.Open(inputPath)
	if err != nil {
		return "", image.Config{}, err
	}
	cfg, _, err := image.DecodeConfig(f)
	_ = f.Close()
	if err != nil {
		return "", image.Config{}, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return "", image.Config{}, errors.New("invalid image dimensions")
	}

	longEdge := cfg.Width
	if cfg.Height > longEdge {
		longEdge = cfg.Height
	}
	if longEdge <= imageDownscaleMaxEdge {
		// 尺寸够小，直接沿用原图，不做任何转码
		return inputPath, cfg, nil
	}

	// 需要缩放：先整图解码，再按比例缩到目标尺寸
	srcFile, err := os.Open(inputPath)
	if err != nil {
		return "", image.Config{}, err
	}
	srcImg, _, err := image.Decode(srcFile)
	_ = srcFile.Close()
	if err != nil {
		return "", image.Config{}, err
	}

	scale := float64(imageDownscaleMaxEdge) / float64(longEdge)
	dstW := int(math.Round(float64(cfg.Width) * scale))
	dstH := int(math.Round(float64(cfg.Height) * scale))
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}

	// 目标画布先整体填白：这样 PNG 透明区域在 JPEG 输出里会表现为白色，符合打印预期。
	dstImg := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.Draw(dstImg, dstImg.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	// 再把缩放后的源图以 Over 模式叠加到白底上，透明像素透出白色
	draw.CatmullRom.Scale(dstImg, dstImg.Bounds(), srcImg, srcImg.Bounds(), draw.Over, nil)

	seq := atomic.AddUint64(&downscaleSeq, 1)
	outPath := filepath.Join(tmpDir, "downscaled_"+itoa(int(seq))+".jpg")
	outFile, err := os.Create(outPath)
	if err != nil {
		return "", image.Config{}, err
	}
	if err := jpeg.Encode(outFile, dstImg, &jpeg.Options{Quality: imageDownscaleJPEGQ}); err != nil {
		_ = outFile.Close()
		_ = os.Remove(outPath)
		return "", image.Config{}, err
	}
	if err := outFile.Close(); err != nil {
		_ = os.Remove(outPath)
		return "", image.Config{}, err
	}

	log.Printf("downscaleImage: %s %dx%d -> %dx%d (jpeg q=%d)",
		filepath.Base(inputPath), cfg.Width, cfg.Height, dstW, dstH, imageDownscaleJPEGQ)

	// 返回更新后的尺寸，供上层布局计算使用
	newCfg := image.Config{Width: dstW, Height: dstH, ColorModel: cfg.ColorModel}
	return outPath, newCfg, nil
}

// paperSizeToGofpdf 将纸张大小名称映射到 gofpdf 参数
// 返回：gofpdf 认识的标准名称（或空字符串表示自定义）、自定义尺寸（如果是自定义纸张）
func paperSizeToGofpdf(size string) (string, gofpdf.SizeType) {
	switch size {
	case "A5":
		return "A5", gofpdf.SizeType{}
	case "A4":
		return "A4", gofpdf.SizeType{}
	case "A3":
		return "A3", gofpdf.SizeType{}
	case "A2":
		return "A2", gofpdf.SizeType{}
	case "A1":
		return "A1", gofpdf.SizeType{}
	case "Letter":
		return "Letter", gofpdf.SizeType{}
	case "Legal":
		return "Legal", gofpdf.SizeType{}
	case "5inch":
		// 89×127 mm
		return "", gofpdf.SizeType{Wd: 89, Ht: 127}
	case "6inch":
		// 102×152 mm
		return "", gofpdf.SizeType{Wd: 102, Ht: 152}
	case "7inch":
		// 127×178 mm
		return "", gofpdf.SizeType{Wd: 127, Ht: 178}
	case "8inch":
		// 152×203 mm
		return "", gofpdf.SizeType{Wd: 152, Ht: 203}
	case "10inch":
		// 203×254 mm
		return "", gofpdf.SizeType{Wd: 203, Ht: 254}
	default:
		// 默认使用 A4
		return "A4", gofpdf.SizeType{}
	}
}

// getOrientationCode 将方向字符串转换为 gofpdf 方向代码
func getOrientationCode(orientation string) string {
	if orientation == "landscape" {
		return "L"
	}
	// 默认纵向
	return "P"
}

func convertImageToPDF(inputPath string, orientation string, paperSize string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "convert-img-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	// 大图先下采样再嵌入，避免 PDF 体积过大导致移动端预览/下载失败（Issue #22）。
	imgPath, cfg, err := downscaleImageIfNeeded(inputPath, tmpDir)
	if err != nil {
		cleanup()
		return "", nil, err
	}

	// 处理方向和纸张大小
	orientCode := getOrientationCode(orientation)
	paperName, customSize := paperSizeToGofpdf(paperSize)

	var pdf *gofpdf.Fpdf
	if paperName != "" {
		// 使用标准纸张
		pdf = gofpdf.New(orientCode, "mm", paperName, "")
	} else {
		// 使用自定义尺寸
		pdf = gofpdf.NewCustom(&gofpdf.InitType{
			UnitStr:        "mm",
			Size:           customSize,
			OrientationStr: orientCode,
		})
	}

	pdf.SetMargins(pdfPageMarginMM, pdfPageMarginMM, pdfPageMarginMM)
	pdf.SetAutoPageBreak(false, pdfPageMarginMM)
	pdf.AddPage()

	pageW, pageH := pdf.GetPageSize()
	maxW := pageW - 2*pdfPageMarginMM
	maxH := pageH - 2*pdfPageMarginMM
	scale := math.Min(maxW/float64(cfg.Width), maxH/float64(cfg.Height))
	if scale <= 0 {
		scale = 1
	}
	w := float64(cfg.Width) * scale
	h := float64(cfg.Height) * scale
	x := (pageW - w) / 2
	y := (pageH - h) / 2

	opts := gofpdf.ImageOptions{ImageType: "", ReadDpi: true}
	pdf.ImageOptions(imgPath, x, y, w, h, false, opts, 0, "")

	outPath := filepath.Join(tmpDir, "image.pdf")
	if err := pdf.OutputFileAndClose(outPath); err != nil {
		cleanup()
		return "", nil, err
	}
	return outPath, cleanup, nil
}

func convertTextToPDF(inputPath string, orientation string, paperSize string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "convert-text-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	f, err := os.Open(inputPath)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	defer f.Close()

	// 处理方向和纸张大小
	orientCode := getOrientationCode(orientation)
	paperName, customSize := paperSizeToGofpdf(paperSize)

	var pdf *gofpdf.Fpdf
	if paperName != "" {
		// 使用标准纸张
		pdf = gofpdf.New(orientCode, "mm", paperName, "")
	} else {
		// 使用自定义尺寸
		pdf = gofpdf.NewCustom(&gofpdf.InitType{
			UnitStr:        "mm",
			Size:           customSize,
			OrientationStr: orientCode,
		})
	}

	pdf.SetMargins(pdfPageMarginMM, pdfPageMarginMM, pdfPageMarginMM)
	pdf.SetAutoPageBreak(false, pdfPageMarginMM)
	pdf.AddPage()
	if err := setPdfTextFont(pdf, 10); err != nil {
		cleanup()
		return "", nil, err
	}

	_, pageH := pdf.GetPageSize()
	lineHeight := (pageH - 2*pdfPageMarginMM) / float64(textLinesPerPage)
	if lineHeight <= 0 {
		lineHeight = 4
	}

	pageW, _ := pdf.GetPageSize()
	cellW := pageW - 2*pdfPageMarginMM

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineIndex := 0
	for scanner.Scan() {
		if lineIndex >= textLinesPerPage {
			pdf.AddPage()
			lineIndex = 0
		}
		y := pdfPageMarginMM + lineHeight*float64(lineIndex)
		pdf.SetXY(pdfPageMarginMM, y)
		pdf.CellFormat(cellW, lineHeight, scanner.Text(), "", 0, "LM", false, 0, "")
		lineIndex++
	}
	if err := scanner.Err(); err != nil {
		cleanup()
		return "", nil, err
	}

	outPath := filepath.Join(tmpDir, "text.pdf")
	if err := pdf.OutputFileAndClose(outPath); err != nil {
		cleanup()
		return "", nil, err
	}
	return outPath, cleanup, nil
}

// convertImagesMultiToPDF 将多张图片合并为单个 PDF。
// 每张图片占据一页，按等比例缩放居中绘制，页面大小与方向由 orientation / paperSize 决定。
// 调用方负责在使用完输出 PDF 后调用返回的 cleanup 清理临时目录。
func convertImagesMultiToPDF(fileHeaders []*multipart.FileHeader, orientation string, paperSize string) (string, func(), error) {
	if len(fileHeaders) == 0 {
		return "", nil, errors.New("no image files provided")
	}

	tmpDir, err := os.MkdirTemp("", "convert-imgs-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	// 先把所有上传流落到临时文件，便于 gofpdf 直接读取路径
	type savedImage struct {
		path string
		cfg  image.Config
	}
	saved := make([]savedImage, 0, len(fileHeaders))
	for idx, fh := range fileHeaders {
		src, err := fh.Open()
		if err != nil {
			cleanup()
			return "", nil, err
		}
		ext := strings.ToLower(filepath.Ext(fh.Filename))
		if ext == "" {
			ext = ".img"
		}
		imgPath := filepath.Join(tmpDir, "img_"+itoa(idx)+ext)
		dst, err := os.Create(imgPath)
		if err != nil {
			src.Close()
			cleanup()
			return "", nil, err
		}
		if _, err := io.Copy(dst, src); err != nil {
			dst.Close()
			src.Close()
			cleanup()
			return "", nil, err
		}
		dst.Close()
		src.Close()

		// 大图下采样：移动端合并若干张 10M+ 原图时最容易卡在这一步
		finalPath, cfg, err := downscaleImageIfNeeded(imgPath, tmpDir)
		if err != nil {
			cleanup()
			return "", nil, err
		}
		saved = append(saved, savedImage{path: finalPath, cfg: cfg})
	}

	// 构造 PDF
	orientCode := getOrientationCode(orientation)
	paperName, customSize := paperSizeToGofpdf(paperSize)

	var pdf *gofpdf.Fpdf
	if paperName != "" {
		pdf = gofpdf.New(orientCode, "mm", paperName, "")
	} else {
		pdf = gofpdf.NewCustom(&gofpdf.InitType{
			UnitStr:        "mm",
			Size:           customSize,
			OrientationStr: orientCode,
		})
	}
	pdf.SetMargins(pdfPageMarginMM, pdfPageMarginMM, pdfPageMarginMM)
	pdf.SetAutoPageBreak(false, pdfPageMarginMM)

	for _, img := range saved {
		pdf.AddPage()
		pageW, pageH := pdf.GetPageSize()
		maxW := pageW - 2*pdfPageMarginMM
		maxH := pageH - 2*pdfPageMarginMM
		scale := math.Min(maxW/float64(img.cfg.Width), maxH/float64(img.cfg.Height))
		if scale <= 0 {
			scale = 1
		}
		w := float64(img.cfg.Width) * scale
		h := float64(img.cfg.Height) * scale
		x := (pageW - w) / 2
		y := (pageH - h) / 2

		opts := gofpdf.ImageOptions{ImageType: "", ReadDpi: true}
		pdf.ImageOptions(img.path, x, y, w, h, false, opts, 0, "")
	}

	outPath := filepath.Join(tmpDir, "images.pdf")
	if err := pdf.OutputFileAndClose(outPath); err != nil {
		cleanup()
		return "", nil, err
	}
	return outPath, cleanup, nil
}

// itoa 是一个小助手，避免再引入 strconv 仅为了格式化索引
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
