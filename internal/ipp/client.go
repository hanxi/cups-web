package ipp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	goipp "github.com/OpenPrinting/goipp"
)

// PrintJobOptions holds all optional parameters for a print job.
type PrintJobOptions struct {
	DriverOptions DriverOptions

	Sides        string // explicit IPP sides keyword; takes precedence over IsDuplex
	IsDuplex     bool   // legacy fallback: true = long-edge, false = one-sided
	IsColor      bool   // true = color, false = monochrome
	Copies       int    // number of copies, 0 or 1 means 1
	Orientation  string // "portrait" | "landscape"
	PaperSize    string // "A4" | "A3" | "5inch" | "6inch" | "7inch" | "8inch" | "10inch"
	PaperType    string // "plain" | "photo" | "glossy" | "matte" | "envelope" | "cardstock" | "labels" | "auto"
	PrintScaling string // "auto" | "auto-fit" | "fit" | "fill" | "none"
	MediaSource  string // input tray / paper source, e.g. "tray-1" | "main" | "manual" | "auto" (Issue #75)
	PageRange    string // e.g. "1-5 8 10-12"
	PageSet      string // "all" | "odd" | "even" – CUPS page-set filter (typical use: manual duplex)
	Mirror       bool   // mirror / horizontal flip

	// N-up (multiple document pages per physical sheet), handled natively by the
	// CUPS pdftopdf filter (Issue #78).
	NumberUp       int    // 1/2/4/6/9/16; 0 or 1 means off (one page per sheet)
	NumberUpLayout string // "lrtb" | "rltb" | "tblr" | "tbrl" (page ordering across the grid)
	PageBorder     string // "single" | "none" – border drawn around each sub-page
}

// SendPrintJob sends data to the printer via IPP using goipp to build the
// IPP Print-Job request. It returns a human-readable status or job identifier
// when available.
func SendPrintJob(printerURI string, r io.Reader, mime string, username string, jobName string, opts PrintJobOptions) (string, error) {
	sides, err := ResolveSides(opts.Sides, opts.IsDuplex)
	if err != nil {
		return "", err
	}
	printerURI, err = printerRequestURI(printerURI)
	if err != nil {
		return "", err
	}
	// IPP attribute must use ipp:// scheme; HTTP transport uses http://
	ippURI := httpToIppURI(printerURI)

	req := goipp.NewRequest(goipp.DefaultVersion, goipp.OpPrintJob, 1)
	req.Operation.Add(goipp.MakeAttribute("attributes-charset", goipp.TagCharset, goipp.String("utf-8")))
	req.Operation.Add(goipp.MakeAttribute("attributes-natural-language", goipp.TagLanguage, goipp.String("en-US")))
	req.Operation.Add(goipp.MakeAttribute("printer-uri", goipp.TagURI, goipp.String(ippURI)))
	if username != "" {
		req.Operation.Add(goipp.MakeAttribute("requesting-user-name", goipp.TagName, goipp.String(username)))
	}
	log.Printf("[ipp] SendPrintJob: uri=%q user=%q job=%q mime=%q", printerURI, username, jobName, mime)
	if jobName != "" {
		req.Operation.Add(goipp.MakeAttribute("job-name", goipp.TagName, goipp.String(jobName)))
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	req.Operation.Add(goipp.MakeAttribute("document-format", goipp.TagMimeType, goipp.String(mime)))

	// Keep manual duplex single-sided even for direct callers (issue #109).
	sides = SidesForPageSet(sides, opts.PageSet)
	req.Job.Add(goipp.MakeAttribute("sides", goipp.TagKeyword, goipp.String(sides)))

	// Color mode
	if opts.IsColor {
		req.Job.Add(goipp.MakeAttribute("print-color-mode", goipp.TagKeyword, goipp.String("color")))
	} else {
		req.Job.Add(goipp.MakeAttribute("print-color-mode", goipp.TagKeyword, goipp.String("monochrome")))
	}

	// Copies
	copies := opts.Copies
	if copies < 1 {
		copies = 1
	}
	req.Job.Add(goipp.MakeAttribute("copies", goipp.TagInteger, goipp.Integer(copies)))

	// Collate multiple copies – request collated sets (1,2,3,1,2,3) instead of
	// the uncollated default some drivers use (1,1,1,2,2,2,3,3,3), which forces
	// the user to re-sort every stack before binding (Issue #85). The CUPS
	// pdftopdf filter maps "separate-documents-collated-copies" to collated
	// output. Only meaningful when printing more than one copy.
	if copies > 1 {
		req.Job.Add(goipp.MakeAttribute("multiple-document-handling", goipp.TagKeyword, goipp.String("separate-documents-collated-copies")))
	}

	// Orientation – only override when landscape; portrait is the CUPS default
	// and explicitly sending orientation-requested=3 may cause pdftopdf to
	// re-process the document, leading to an inflated page count in CUPS.
	if opts.Orientation == "landscape" {
		req.Job.Add(goipp.MakeAttribute("orientation-requested", goipp.TagEnum, goipp.Integer(4)))
	}

	// Paper size (media)
	if opts.PaperSize != "" && opts.DriverOptions.PageSize == nil {
		mediaName := paperSizeToIPP(opts.PaperSize)
		if mediaName != "" {
			req.Job.Add(goipp.MakeAttribute("media", goipp.TagKeyword, goipp.String(mediaName)))
		}
	}

	// Paper type (media-type)
	if opts.PaperType != "" && opts.PaperType != "auto" && opts.DriverOptions.MediaType == nil {
		mediaType := paperTypeToIPP(opts.PaperType)
		if mediaType != "" {
			req.Job.Add(goipp.MakeAttribute("media-type", goipp.TagKeyword, goipp.String(mediaType)))
		}
	}

	addDriverOptions(req, opts.DriverOptions)

	// Media source (input tray). Maps to the PPD "InputSlot" option via the CUPS
	// filters. "auto" (or empty) lets the printer pick, so it is a no-op and not
	// sent on the wire. Available tray keywords are printer-specific and are
	// surfaced to the UI via media-source-supported (Issue #75).
	if opts.MediaSource != "" && opts.MediaSource != "auto" {
		req.Job.Add(goipp.MakeAttribute("media-source", goipp.TagKeyword, goipp.String(opts.MediaSource)))
	}

	// Print scaling
	if opts.PrintScaling != "" {
		req.Job.Add(goipp.MakeAttribute("print-scaling", goipp.TagKeyword, goipp.String(opts.PrintScaling)))
	}

	// Page range
	if opts.PageRange != "" {
		ranges := parsePageRange(opts.PageRange)
		if len(ranges) > 0 {
			var vals goipp.Values
			for _, rng := range ranges {
				vals.Add(goipp.TagRange, goipp.Range{Lower: rng[0], Upper: rng[1]})
			}
			req.Job.Add(goipp.Attribute{Name: "page-ranges", Values: vals})
		}
	}

	// Page set filter – CUPS-specific attribute handled by the pdftopdf filter.
	// Used together with page-ranges (range is applied first, then odd/even is
	// filtered). Typical use case: manual duplex – print odd pages, flip the
	// paper stack, then print even pages. Default "all" is a no-op and therefore
	// not sent on the wire. "even-reverse" is pre-processed by the caller (PDF
	// page reordering) so it must NOT be sent as an IPP attribute.
	if set := normalizePageSet(opts.PageSet); set != "" && set != "all" && set != "even-reverse" {
		req.Job.Add(goipp.MakeAttribute("page-set", goipp.TagKeyword, goipp.String(set)))
	}

	// Mirror (best-effort)
	if opts.Mirror {
		req.Job.Add(goipp.MakeAttribute("mirror", goipp.TagBoolean, goipp.Boolean(true)))
	}

	// N-up – place multiple document pages on a single physical sheet. Handled
	// by the CUPS pdftopdf filter. number-up=1 is the default (one page per
	// sheet) and is a no-op, so it is not sent on the wire; number-up-layout and
	// page-border only make sense when more than one page shares a sheet.
	if opts.NumberUp > 1 {
		req.Job.Add(goipp.MakeAttribute("number-up", goipp.TagInteger, goipp.Integer(opts.NumberUp)))
		if layout := normalizeNumberUpLayout(opts.NumberUpLayout); layout != "" {
			req.Job.Add(goipp.MakeAttribute("number-up-layout", goipp.TagKeyword, goipp.String(layout)))
		}
		if border := normalizePageBorder(opts.PageBorder); border != "" {
			req.Job.Add(goipp.MakeAttribute("page-border", goipp.TagKeyword, goipp.String(border)))
		}
	}

	// 不主动发 job-impressions：这个属性只是客户端提示，pdftopdf
	// 会在过滤后用真实渲染页数覆盖它；早先的 819ae20 把它作为"防 off-by-one"
	// 兜底反而让 CUPS 后台稳定多算一张（issue #20）。让 CUPS 走滤镜自己
	// 数出来的 job-impressions-completed 更准。

	payload, err := req.EncodeBytes()
	if err != nil {
		return "", fmt.Errorf("encode ipp request: %w", err)
	}

	body := io.MultiReader(bytes.NewBuffer(payload), r)

	httpReq, err := http.NewRequest(http.MethodPost, printerURI, body)
	if err != nil {
		return "", fmt.Errorf("create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", goipp.ContentType)
	httpReq.Header.Set("Accept", goipp.ContentType)

	// 打印任务可能上传较大文档，整体超时放宽到 120s；TCP 连接仍受 SSRF 校验。
	client, err := newSafeClient(120 * time.Second)
	if err != nil {
		return "", err
	}
	log.Printf("[ipp] SendPrintJob: sending HTTP POST to %q", printerURI)
	resp, err := client.Do(httpReq)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		log.Printf("[ipp] SendPrintJob: http error: %v", err)
		failure := fmt.Errorf("http post: %w", err)
		var dialErr *net.OpError
		if errors.As(err, &dialErr) && dialErr.Op == "dial" {
			return "", failure // No connection was established, so no job was sent.
		}
		return "", &SubmissionUncertainError{Err: failure}
	}
	log.Printf("[ipp] SendPrintJob: HTTP response status: %s", resp.Status)
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(limitedBody(resp.Body))
		log.Printf("[ipp] SendPrintJob: non-2xx body: %s", string(body))
		failure := fmt.Errorf("http status: %s", resp.Status)
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout {
			return "", &SubmissionUncertainError{Err: failure}
		}
		return "", failure
	}

	var rsp goipp.Message
	if err := rsp.Decode(limitedBody(resp.Body)); err != nil {
		log.Printf("[ipp] SendPrintJob: decode error: %v", err)
		return "", &SubmissionUncertainError{Err: fmt.Errorf("decode ipp response: %w", err)}
	}
	log.Printf("[ipp] SendPrintJob: IPP response code: %d (%s)", rsp.Code, goipp.Status(rsp.Code).String())
	// IPP's entire 0x00xx class is successful, including substituted/conflicting
	// attributes (RFC 8011, B.1.2). The printer has already accepted those jobs.
	if rsp.Code >= 0x0100 {
		return "", fmt.Errorf("ipp error: %s", goipp.Status(rsp.Code).String())
	}

	for _, a := range rsp.Job {
		if a.Name == "job-uri" || a.Name == "job-id" {
			if len(a.Values) > 0 {
				log.Printf("[ipp] SendPrintJob: success, %s=%s", a.Name, a.Values[0].V.String())
				return a.Values[0].V.String(), nil
			}
		}
	}

	log.Printf("[ipp] SendPrintJob: success (no job-id in response)")
	return "ok", nil
}

// paperSizeToIPP converts a paper size name to an IPP media keyword.
func paperSizeToIPP(size string) string {
	m := map[string]string{
		"A5":     "iso_a5_148x210mm",
		"A4":     "iso_a4_210x297mm",
		"A3":     "iso_a3_297x420mm",
		"A2":     "iso_a2_420x594mm",
		"A1":     "iso_a1_594x841mm",
		"5inch":  "oe_photo-5x7_5x7in",
		"6inch":  "oe_photo-l_3.5x5in",
		"7inch":  "oe_photo-7x5_7x5in",
		"8inch":  "oe_photo-8x10_8x10in",
		"10inch": "oe_photo-10x12_10x12in",
		"Letter": "na_letter_8.5x11in",
		"Legal":  "na_legal_8.5x14in",
	}
	return m[size]
}

// paperTypeToIPP converts a paper type name to an IPP media-type keyword.
func paperTypeToIPP(t string) string {
	m := map[string]string{
		"plain":     "stationery",
		"photo":     "photographic",
		"glossy":    "photographic-glossy",
		"matte":     "photographic-matte",
		"envelope":  "envelope",
		"cardstock": "cardstock",
		"labels":    "labels",
	}
	return m[t]
}

// normalizePageSet maps arbitrary user input to one of the recognized
// page-set values ("all" | "odd" | "even" | "even-reverse"). Returns an
// empty string for unknown values so the caller can omit the IPP attribute.
// "even-reverse" is handled by the caller (PDF page reordering) and must NOT
// be sent to CUPS as an IPP attribute.
func normalizePageSet(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "all":
		return "all"
	case "odd":
		return "odd"
	case "even":
		return "even"
	case "even-reverse":
		return "even-reverse"
	default:
		return ""
	}
}

// normalizeNumberUpLayout maps user input to a CUPS number-up-layout keyword.
// Only the four common orderings are exposed to the UI; the vertical-major
// variants (btlr/btrl/lrbt/rlbt) are rarely useful for N-up handouts. Unknown
// or empty input falls back to "lrtb" (left-to-right, top-to-bottom), which is
// the CUPS default and matches WPS's default Z ordering.
func normalizeNumberUpLayout(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "lrtb", "rltb", "tblr", "tbrl":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "lrtb"
	}
}

// normalizePageBorder maps user input to a CUPS page-border keyword. Empty input
// means "no explicit border requested" and returns an empty string so the caller
// can omit the attribute (CUPS then defaults to no border).
func normalizePageBorder(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "single", "single-thick", "double", "double-thick":
		return strings.ToLower(strings.TrimSpace(s))
	case "none":
		return "none"
	default:
		return ""
	}
}

// parsePageRange parses a page range string like "1-5 8 10-12" into [][2]int32 pairs.
func parsePageRange(s string) [][2]int {
	var result [][2]int
	parts := strings.Fields(s)
	for _, p := range parts {
		if idx := strings.Index(p, "-"); idx > 0 {
			var lo, hi int
			fmt.Sscanf(p[:idx], "%d", &lo)
			fmt.Sscanf(p[idx+1:], "%d", &hi)
			if lo > 0 && hi >= lo {
				result = append(result, [2]int{lo, hi})
			}
		} else {
			var n int
			fmt.Sscanf(p, "%d", &n)
			if n > 0 {
				result = append(result, [2]int{n, n})
			}
		}
	}
	return result
}

// PrinterInfo holds information about a printer retrieved via IPP Get-Printer-Attributes.
type PrinterInfo struct {
	Name                 string            `json:"name"`
	URI                  string            `json:"uri"`
	State                string            `json:"state"`
	StateMessage         string            `json:"stateMessage"`
	StateReasons         []string          `json:"stateReasons"`
	QueuedJobs           int               `json:"queuedJobs"`
	FirmwareVersion      string            `json:"firmwareVersion"`
	UptimeSeconds        int               `json:"uptimeSeconds"`
	StateDurationSeconds int               `json:"stateDurationSeconds"`
	MarkerNames          []string          `json:"markerNames"`
	MarkerTypes          []string          `json:"markerTypes"`
	MarkerLevels         []int             `json:"markerLevels"`
	MarkerColors         []string          `json:"markerColors"`
	MediaReady           []string          `json:"mediaReady"`
	MediaSourceSupported []string          `json:"mediaSourceSupported"`
	Attributes           map[string]string `json:"attributes"`
}

// httpToIppURI converts an http:// URI to ipp:// for use in IPP request attributes.
// CUPS requires the printer-uri attribute to use the ipp:// scheme.
func httpToIppURI(uri string) string {
	if strings.HasPrefix(uri, "http://") {
		return "ipp://" + uri[len("http://"):]
	}
	if strings.HasPrefix(uri, "https://") {
		return "ipps://" + uri[len("https://"):]
	}
	return uri
}

// GetPrinterAttributes queries a printer via IPP Get-Printer-Attributes and returns structured info.
func GetPrinterAttributes(printerURI string) (*PrinterInfo, error) {
	log.Printf("[ipp] GetPrinterAttributes start, uri=%q", printerURI)
	printerURI, err := printerRequestURI(printerURI)
	if err != nil {
		return nil, err
	}

	// IPP attribute must use ipp:// scheme; HTTP transport uses http://
	ippURI := httpToIppURI(printerURI)
	log.Printf("[ipp] using ipp-scheme uri for attribute: %q", ippURI)

	req := goipp.NewRequest(goipp.DefaultVersion, goipp.OpGetPrinterAttributes, 1)
	req.Operation.Add(goipp.MakeAttribute("attributes-charset", goipp.TagCharset, goipp.String("utf-8")))
	req.Operation.Add(goipp.MakeAttribute("attributes-natural-language", goipp.TagLanguage, goipp.String("en-US")))
	req.Operation.Add(goipp.MakeAttribute("printer-uri", goipp.TagURI, goipp.String(ippURI)))
	req.Operation.Add(goipp.MakeAttribute("requested-attributes", goipp.TagKeyword, goipp.String("all")))

	payload, err := req.EncodeBytes()
	if err != nil {
		log.Printf("[ipp] encode error: %v", err)
		return nil, fmt.Errorf("encode ipp request: %w", err)
	}
	log.Printf("[ipp] encoded payload, %d bytes", len(payload))

	httpReq, err := http.NewRequest(http.MethodPost, printerURI, bytes.NewBuffer(payload))
	if err != nil {
		log.Printf("[ipp] create http request error: %v", err)
		return nil, fmt.Errorf("create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", goipp.ContentType)
	httpReq.Header.Set("Accept", goipp.ContentType)

	log.Printf("[ipp] sending HTTP POST to %q", printerURI)
	client, err := newSafeClient(dialTimeout)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(httpReq)
	if resp != nil {
		defer resp.Body.Close()
		log.Printf("[ipp] HTTP response status: %s", resp.Status)
	}
	if err != nil {
		log.Printf("[ipp] http post error: %v", err)
		return nil, fmt.Errorf("http post: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		log.Printf("[ipp] non-2xx status: %s", resp.Status)
		return nil, fmt.Errorf("http status: %s", resp.Status)
	}

	log.Printf("[ipp] decoding IPP response")
	var rsp goipp.Message
	if err := rsp.Decode(limitedBody(resp.Body)); err != nil {
		log.Printf("[ipp] decode error: %v", err)
		return nil, fmt.Errorf("decode ipp response: %w", err)
	}
	log.Printf("[ipp] IPP response code: %d, printer attrs count: %d", rsp.Code, len(rsp.Printer))

	info := &PrinterInfo{
		URI:        printerURI,
		Attributes: make(map[string]string),
	}

	var stateChangeTime int
	for _, a := range rsp.Printer {
		if len(a.Values) == 0 {
			continue
		}
		switch a.Name {
		case "printer-name":
			info.Name = a.Values[0].V.String()
			log.Printf("[ipp] printer-name=%q", info.Name)
		case "printer-state":
			raw := a.Values[0].V.String()
			log.Printf("[ipp] printer-state raw=%q (tag=%v)", raw, a.Values[0].T)
			switch raw {
			case "3":
				info.State = "idle"
			case "4":
				info.State = "processing"
			case "5":
				info.State = "stopped"
			default:
				info.State = raw
			}
		case "printer-state-message":
			info.StateMessage = a.Values[0].V.String()
		case "printer-state-reasons":
			for _, v := range a.Values {
				info.StateReasons = append(info.StateReasons, v.V.String())
			}
		case "queued-job-count":
			fmt.Sscanf(a.Values[0].V.String(), "%d", &info.QueuedJobs)
		case "printer-firmware-string-version":
			info.FirmwareVersion = a.Values[0].V.String()
		case "printer-up-time":
			fmt.Sscanf(a.Values[0].V.String(), "%d", &info.UptimeSeconds)
		case "printer-state-change-time":
			fmt.Sscanf(a.Values[0].V.String(), "%d", &stateChangeTime)
		case "marker-names":
			for _, v := range a.Values {
				info.MarkerNames = append(info.MarkerNames, v.V.String())
			}
		case "marker-types":
			for _, v := range a.Values {
				info.MarkerTypes = append(info.MarkerTypes, v.V.String())
			}
		case "marker-levels":
			for _, v := range a.Values {
				var lvl int
				fmt.Sscanf(v.V.String(), "%d", &lvl)
				info.MarkerLevels = append(info.MarkerLevels, lvl)
			}
		case "marker-colors":
			for _, v := range a.Values {
				info.MarkerColors = append(info.MarkerColors, v.V.String())
			}
		case "media-ready":
			for _, v := range a.Values {
				info.MediaReady = append(info.MediaReady, v.V.String())
			}
		case "media-source-supported":
			for _, v := range a.Values {
				info.MediaSourceSupported = append(info.MediaSourceSupported, v.V.String())
			}
		default:
			vals := make([]string, 0, len(a.Values))
			for _, v := range a.Values {
				vals = append(vals, v.V.String())
			}
			info.Attributes[a.Name] = strings.Join(vals, ", ")
		}
	}

	// 计算状态持续时间：uptime - stateChangeTime
	if info.UptimeSeconds > 0 && stateChangeTime > 0 && info.UptimeSeconds >= stateChangeTime {
		info.StateDurationSeconds = info.UptimeSeconds - stateChangeTime
	}

	log.Printf("[ipp] GetPrinterAttributes done: name=%q state=%q jobs=%d markers=%d",
		info.Name, info.State, info.QueuedJobs, len(info.MarkerNames))
	return info, nil
}

// Printer represents a CUPS printer.
//
// Info / Location / MakeAndModel 分别来自 CUPS 的 printer-info（Web 界面上的
// “描述”）、printer-location、printer-make-and-model，用于在打印机多的时候把
// 同型号队列区分开（issue #101）。HTML 兜底路径拿不到这些属性，会留空。
type Printer struct {
	Name         string `json:"name"`
	URI          string `json:"uri"`
	Info         string `json:"info"`
	Location     string `json:"location"`
	MakeAndModel string `json:"makeAndModel"`
}

// cupsHostPort 在 Socket 模式返回固定占位主机，否则归一化 CUPS_HOST。
func cupsHostPort(host string) (string, error) {
	socketPath, err := cupsSocketPath()
	if err != nil {
		return "", err
	}
	if socketPath != "" {
		return socketHost, nil
	}
	u := host
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = "http://" + u
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("invalid host: %w", err)
	}
	hostOnly := parsed.Host
	if hostOnly == "" {
		return "", fmt.Errorf("invalid host: %q", host)
	}
	if !strings.Contains(hostOnly, ":") {
		hostOnly = hostOnly + ":631"
	}
	return hostOnly, nil
}

// ListPrinters 列出 CUPS 上的打印机队列。
//
// 优先走 IPP 的 CUPS-Get-Printers：一次请求就能拿到描述/位置/型号，比抓 HTML
// 页面稳。失败时退回抓 /printers 页面 —— 远端 CUPS 禁掉了该操作、或者对端压根
// 不是 CUPS 时，至少还能列出队列（只是没有描述）。
func ListPrinters(host string) ([]Printer, error) {
	hostOnly, err := cupsHostPort(host)
	if err != nil {
		return nil, err
	}
	printers, ippErr := listPrintersIPP(hostOnly)
	if ippErr == nil {
		return printers, nil
	}
	log.Printf("[ipp] CUPS-Get-Printers failed (%v), falling back to /printers page", ippErr)
	return listPrintersHTML(hostOnly)
}

// listPrintersIPP 用 CUPS-Get-Printers 拉队列列表及其描述属性。
func listPrintersIPP(hostOnly string) ([]Printer, error) {
	reqURL := (&url.URL{Scheme: "http", Host: hostOnly, Path: "/"}).String()
	if err := validatePrinterURI(reqURL); err != nil {
		return nil, err
	}

	req := goipp.NewRequest(goipp.DefaultVersion, goipp.OpCupsGetPrinters, 1)
	req.Operation.Add(goipp.MakeAttribute("attributes-charset", goipp.TagCharset, goipp.String("utf-8")))
	req.Operation.Add(goipp.MakeAttribute("attributes-natural-language", goipp.TagLanguage, goipp.String("en-US")))
	// 只点名列表要用的属性：不写 requested-attributes 时 CUPS 会给每台打印机
	// 回上百个属性，队列多了纯属浪费带宽和解析时间。
	var wanted goipp.Values
	for _, name := range []string{"printer-name", "printer-info", "printer-location", "printer-make-and-model"} {
		wanted.Add(goipp.TagKeyword, goipp.String(name))
	}
	req.Operation.Add(goipp.Attribute{Name: "requested-attributes", Values: wanted})

	payload, err := req.EncodeBytes()
	if err != nil {
		return nil, fmt.Errorf("encode ipp request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", goipp.ContentType)
	httpReq.Header.Set("Accept", goipp.ContentType)

	client, err := newSafeClient(dialTimeout)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("http status: %s", resp.Status)
	}

	var rsp goipp.Message
	if err := rsp.Decode(limitedBody(resp.Body)); err != nil {
		return nil, fmt.Errorf("decode ipp response: %w", err)
	}
	if status := goipp.Status(rsp.Code); status != goipp.StatusOk {
		return nil, fmt.Errorf("ipp error: %s", status.String())
	}

	// CUPS-Get-Printers 每台打印机回一个 printer-attributes group，所以必须遍历
	// rsp.Groups —— rsp.Printer 只是被压平的那一组，多队列时会丢数据。
	printers := make([]Printer, 0, len(rsp.Groups))
	for _, grp := range rsp.Groups {
		if grp.Tag != goipp.TagPrinterGroup {
			continue
		}
		var p Printer
		for _, a := range grp.Attrs {
			if len(a.Values) == 0 {
				continue
			}
			v := strings.TrimSpace(a.Values[0].V.String())
			switch a.Name {
			case "printer-name":
				p.Name = v
			case "printer-info":
				p.Info = v
			case "printer-location":
				p.Location = v
			case "printer-make-and-model":
				p.MakeAndModel = v
			}
		}
		if p.Name == "" {
			continue
		}
		// URI 仍按 host + 队列名拼，不用 printer-uri-supported：那里面是 cupsd
		// 自报的主机名，容器/跨网段场景下客户端未必解析得到。
		p.URI = fmt.Sprintf("http://%s/printers/%s", hostOnly, p.Name)
		printers = append(printers, p)
	}

	log.Printf("[ipp] CUPS-Get-Printers: %d printer(s) from %s", len(printers), hostOnly)
	return printers, nil
}

// listPrintersHTML 抓 CUPS /printers 页面并从超链接里提取队列名（兜底路径）。
func listPrintersHTML(hostOnly string) ([]Printer, error) {
	listURL := (&url.URL{Scheme: "http", Host: hostOnly, Path: "/printers"}).String()

	if err := validatePrinterURI(listURL); err != nil {
		return nil, err
	}
	client, err := newSafeClient(dialTimeout)
	if err != nil {
		return nil, err
	}
	resp, err := client.Get(listURL)
	if err != nil {
		return nil, fmt.Errorf("fetch printers page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("http status: %s", resp.Status)
	}

	body, err := io.ReadAll(limitedBody(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("read printers page: %w", err)
	}

	re := regexp.MustCompile(`(?i)<a[^>]+href=["']/printers/([^"'/>]+)["'][^>]*>([^<]+)</a>`)
	matches := re.FindAllSubmatch(body, -1)
	printers := make([]Printer, 0, len(matches))
	for _, m := range matches {
		name := string(m[1])
		display := string(m[2])
		uri := fmt.Sprintf("http://%s/printers/%s", hostOnly, name)
		printers = append(printers, Printer{Name: display, URI: uri})
	}

	return printers, nil
}

// Job 描述 CUPS 上的一个打印任务的核心状态字段。字段命名与 IPP 属性一一对应,
// 便于前端做筛选/取消操作时准确定位到远端队列里的具体任务(issue #60)。
type Job struct {
	// ID 是 CUPS 分配的 job-id(整数),取消操作必须用它作为 IPP 属性。
	ID int `json:"id"`
	// PrinterURI 是 job-printer-uri，Socket 模式归一化为固定占位主机。前端取消时原样回传。
	// 注意:CUPS 回给我们的是 ipp:// scheme + cupsd 自报的主机名,某些场景下
	// 客户端拿到后连不上,所以我们再暴露一个 PrinterName + 由前端与 host 拼出
	// 可用 URI 的能力。这里两条都留。
	PrinterURI  string `json:"printerUri"`
	PrinterName string `json:"printerName"`
	// Name 是 job-name(通常就是文件名),用户识别用。
	Name string `json:"name"`
	// User 是 job-originating-user-name,便于用户认出"这是不是我发的任务"。
	User string `json:"user"`
	// State 是 CUPS 侧的原始状态整数(3=pending, 4=held, 5=processing,
	// 6=stopped, 7=canceled, 8=aborted, 9=completed)。ListActiveJobs 只返
	// which-jobs=not-completed,所以基本只会看到 3/4/5/6。
	State int `json:"state"`
	// StateText 已把 State 翻译为可读字串,前端直接展示。
	StateText string `json:"stateText"`
	// StateReasons 是 job-state-reasons,值形如 "job-printer-not-reachable"、
	// "job-outgoing"、"processing-to-stop-point" 等,是排查"卡住"最关键的信号。
	StateReasons []string `json:"stateReasons"`
	// SizeKB 是 job-k-octets(KB),CUPS 报的是 KB 单位;缺失则为 0。
	SizeKB int `json:"sizeKB"`
	// TimeAtCreation 是 job 创建时 CUPS 内部计时(相对开机秒数,非绝对时间)。
	// 前端主要用它做排序,不做绝对时间显示。
	TimeAtCreation int `json:"timeAtCreation"`
	// SheetsCompleted 是 job-media-sheets-completed,已经打完的纸张数。
	// "任务卡在打印中"时用户很关心这个 —— 是完全没动还是只是最后一张卡纸。
	SheetsCompleted int `json:"sheetsCompleted"`
}

// jobStateText 把 IPP job-state 整数翻成人可读文本。
// RFC 8011 §5.3.7 定义了 3~9 这些值。
func jobStateText(s int) string {
	switch s {
	case 3:
		return "pending"
	case 4:
		return "held"
	case 5:
		return "processing"
	case 6:
		return "stopped"
	case 7:
		return "canceled"
	case 8:
		return "aborted"
	case 9:
		return "completed"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// ListActiveJobs 查询 CUPS 上所有未完成的打印任务(issue #60)。
//
// 传统上 IPP Get-Jobs 是"某台打印机上的任务",但 CUPS 扩展允许把 printer-uri
// 指向 ipp://host/(而不是具体队列),这样能一次拿到所有队列的任务列表 —— 对
// "任务卡住"排查场景更省事(不用先枚举打印机)。搭配 which-jobs=not-completed
// 只拿活动任务,my-jobs=false 拿所有用户的任务(默认只返回请求者自己的)。
func ListActiveJobs(host string) ([]Job, error) {
	hostOnly, err := cupsHostPort(host)
	if err != nil {
		return nil, err
	}
	socketPath, err := cupsSocketPath()
	if err != nil {
		return nil, err
	}
	reqURL := (&url.URL{Scheme: "http", Host: hostOnly, Path: "/"}).String()
	if err := validatePrinterURI(reqURL); err != nil {
		return nil, err
	}
	// IPP 侧的 printer-uri 需要 ipp:// scheme。
	ippURI := httpToIppURI(reqURL)

	req := goipp.NewRequest(goipp.DefaultVersion, goipp.OpGetJobs, 1)
	req.Operation.Add(goipp.MakeAttribute("attributes-charset", goipp.TagCharset, goipp.String("utf-8")))
	req.Operation.Add(goipp.MakeAttribute("attributes-natural-language", goipp.TagLanguage, goipp.String("en-US")))
	req.Operation.Add(goipp.MakeAttribute("printer-uri", goipp.TagURI, goipp.String(ippURI)))
	req.Operation.Add(goipp.MakeAttribute("which-jobs", goipp.TagKeyword, goipp.String("not-completed")))
	req.Operation.Add(goipp.MakeAttribute("my-jobs", goipp.TagBoolean, goipp.Boolean(false)))
	// 精确点名要用的属性,避免 CUPS 回一大堆无关字段。
	var wanted goipp.Values
	for _, name := range []string{
		"job-id", "job-name", "job-state", "job-state-reasons",
		"job-printer-uri", "job-originating-user-name",
		"job-k-octets", "time-at-creation", "job-media-sheets-completed",
	} {
		wanted.Add(goipp.TagKeyword, goipp.String(name))
	}
	req.Operation.Add(goipp.Attribute{Name: "requested-attributes", Values: wanted})

	payload, err := req.EncodeBytes()
	if err != nil {
		return nil, fmt.Errorf("encode ipp request: %w", err)
	}
	httpReq, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", goipp.ContentType)
	httpReq.Header.Set("Accept", goipp.ContentType)

	client, err := newSafeClient(dialTimeout)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("http status: %s", resp.Status)
	}

	var rsp goipp.Message
	if err := rsp.Decode(limitedBody(resp.Body)); err != nil {
		return nil, fmt.Errorf("decode ipp response: %w", err)
	}
	if status := goipp.Status(rsp.Code); status != goipp.StatusOk {
		return nil, fmt.Errorf("ipp error: %s", status.String())
	}

	// Get-Jobs 每个任务是一个 job-attributes group。跟 CUPS-Get-Printers 一样,
	// 只看 rsp.Job 会漏 —— 必须遍历 rsp.Groups。
	jobs := make([]Job, 0, len(rsp.Groups))
	for _, grp := range rsp.Groups {
		if grp.Tag != goipp.TagJobGroup {
			continue
		}
		var j Job
		for _, a := range grp.Attrs {
			if len(a.Values) == 0 {
				continue
			}
			switch a.Name {
			case "job-id":
				fmt.Sscanf(a.Values[0].V.String(), "%d", &j.ID)
			case "job-name":
				j.Name = a.Values[0].V.String()
			case "job-originating-user-name":
				j.User = a.Values[0].V.String()
			case "job-state":
				fmt.Sscanf(a.Values[0].V.String(), "%d", &j.State)
			case "job-state-reasons":
				for _, v := range a.Values {
					s := v.V.String()
					if s != "" && s != "none" {
						j.StateReasons = append(j.StateReasons, s)
					}
				}
			case "job-printer-uri":
				j.PrinterURI = a.Values[0].V.String()
				// 从 ipp://host/printers/NAME 里拆出 NAME,前端展示更友好。
				if idx := strings.LastIndex(j.PrinterURI, "/"); idx > 0 && idx < len(j.PrinterURI)-1 {
					j.PrinterName = j.PrinterURI[idx+1:]
				}
			case "job-k-octets":
				fmt.Sscanf(a.Values[0].V.String(), "%d", &j.SizeKB)
			case "time-at-creation":
				fmt.Sscanf(a.Values[0].V.String(), "%d", &j.TimeAtCreation)
			case "job-media-sheets-completed":
				fmt.Sscanf(a.Values[0].V.String(), "%d", &j.SheetsCompleted)
			}
		}
		if j.ID == 0 {
			continue
		}
		if socketPath != "" && j.PrinterURI != "" {
			normalized, err := printerRequestURI(j.PrinterURI)
			if err != nil {
				return nil, fmt.Errorf("invalid job printer uri: %w", err)
			}
			j.PrinterURI = httpToIppURI(normalized)
		}
		j.StateText = jobStateText(j.State)
		jobs = append(jobs, j)
	}
	log.Printf("[ipp] ListActiveJobs: %d job(s) from %s", len(jobs), hostOnly)
	return jobs, nil
}

// CancelJob 通过 IPP Cancel-Job 取消 CUPS 上一个仍在活动状态的任务(issue #60)。
//
// jobURI 应当来自 ListActiveJobs 返回的 Job.PrinterURI —— 它是 job-printer-uri,
// 值形如 ipp://host/printers/NAME。CUPS 允许拿"打印机的 printer-uri + job-id"
// 组合定位任务,这是 RFC 8011 推荐的方式(另一种是 job-uri,但 CUPS 拼给我们
// 的 hostname 在跨网络场景常常不好用)。
//
// username 建议传当前登录用户名,便于 CUPS 在开启 owner 校验时能通过 —— 默认
// 策略允许 owner 取消自己发出的任务;若 CUPS 配置成 admin-only,普通用户会
// 拿到 client-error-forbidden,交由上层把 IPP status 直接透传给前端。
func CancelJob(printerURI string, jobID int, username string) error {
	if jobID <= 0 {
		return fmt.Errorf("invalid job id: %d", jobID)
	}
	httpTarget, err := printerRequestURI(printerURI)
	if err != nil {
		return err
	}
	ippURI := httpToIppURI(httpTarget)

	req := goipp.NewRequest(goipp.DefaultVersion, goipp.OpCancelJob, 1)
	req.Operation.Add(goipp.MakeAttribute("attributes-charset", goipp.TagCharset, goipp.String("utf-8")))
	req.Operation.Add(goipp.MakeAttribute("attributes-natural-language", goipp.TagLanguage, goipp.String("en-US")))
	req.Operation.Add(goipp.MakeAttribute("printer-uri", goipp.TagURI, goipp.String(ippURI)))
	req.Operation.Add(goipp.MakeAttribute("job-id", goipp.TagInteger, goipp.Integer(jobID)))
	if username != "" {
		req.Operation.Add(goipp.MakeAttribute("requesting-user-name", goipp.TagName, goipp.String(username)))
	}

	payload, err := req.EncodeBytes()
	if err != nil {
		return fmt.Errorf("encode ipp request: %w", err)
	}
	httpReq, err := http.NewRequest(http.MethodPost, httpTarget, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", goipp.ContentType)
	httpReq.Header.Set("Accept", goipp.ContentType)

	client, err := newSafeClient(dialTimeout)
	if err != nil {
		return err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("http status: %s", resp.Status)
	}
	var rsp goipp.Message
	if err := rsp.Decode(limitedBody(resp.Body)); err != nil {
		return fmt.Errorf("decode ipp response: %w", err)
	}
	status := goipp.Status(rsp.Code)
	// CUPS 对已经不存在或已完成的任务会回 client-error-not-found / not-possible。
	// 语义上"已经不在了"其实等价于取消成功,但为了让前端能提示"任务已消失/已完成",
	// 这里如实透传 IPP status 字符串。
	if status != goipp.StatusOk {
		return fmt.Errorf("ipp error: %s", status.String())
	}
	log.Printf("[ipp] CancelJob: printer=%q job=%d user=%q ok", printerURI, jobID, username)
	return nil
}
