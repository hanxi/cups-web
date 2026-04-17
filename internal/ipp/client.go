package ipp

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	goipp "github.com/OpenPrinting/goipp"
)

// PrintJobOptions holds all optional parameters for a print job.
type PrintJobOptions struct {
	IsDuplex     bool   // true = two-sided-long-edge, false = one-sided
	IsColor      bool   // true = color, false = monochrome
	Copies       int    // number of copies, 0 or 1 means 1
	Orientation  string // "portrait" | "landscape"
	PaperSize    string // "A4" | "A3" | "5inch" | "6inch" | "7inch" | "8inch" | "10inch"
	PaperType    string // "plain" | "photo" | "glossy" | "matte" | "envelope" | "cardstock" | "labels" | "auto"
	PrintScaling string // "auto" | "auto-fit" | "fit" | "fill" | "none"
	PageRange    string // e.g. "1-5 8 10-12"
	Mirror       bool   // mirror / horizontal flip
}

// SendPrintJob sends data to the printer via IPP using goipp to build the
// IPP Print-Job request. It returns a human-readable status or job identifier
// when available.
func SendPrintJob(printerURI string, r io.Reader, mime string, username string, jobName string, opts PrintJobOptions) (string, error) {
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

	// Duplex
	if opts.IsDuplex {
		req.Job.Add(goipp.MakeAttribute("sides", goipp.TagKeyword, goipp.String("two-sided-long-edge")))
	} else {
		req.Job.Add(goipp.MakeAttribute("sides", goipp.TagKeyword, goipp.String("one-sided")))
	}

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

	// Orientation
	switch opts.Orientation {
	case "landscape":
		req.Job.Add(goipp.MakeAttribute("orientation-requested", goipp.TagEnum, goipp.Integer(4)))
	default:
		req.Job.Add(goipp.MakeAttribute("orientation-requested", goipp.TagEnum, goipp.Integer(3)))
	}

	// Paper size (media)
	if opts.PaperSize != "" {
		mediaName := paperSizeToIPP(opts.PaperSize)
		if mediaName != "" {
			req.Job.Add(goipp.MakeAttribute("media", goipp.TagKeyword, goipp.String(mediaName)))
		}
	}

	// Paper type (media-type)
	if opts.PaperType != "" && opts.PaperType != "auto" {
		mediaType := paperTypeToIPP(opts.PaperType)
		if mediaType != "" {
			req.Job.Add(goipp.MakeAttribute("media-type", goipp.TagKeyword, goipp.String(mediaType)))
		}
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

	// Mirror (best-effort)
	if opts.Mirror {
		req.Job.Add(goipp.MakeAttribute("mirror", goipp.TagBoolean, goipp.Boolean(true)))
	}

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

	client := &http.Client{Timeout: 30 * time.Second}
	log.Printf("[ipp] SendPrintJob: sending HTTP POST to %q", printerURI)
	resp, err := client.Do(httpReq)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		log.Printf("[ipp] SendPrintJob: http error: %v", err)
		return "", fmt.Errorf("http post: %w", err)
	}
	log.Printf("[ipp] SendPrintJob: HTTP response status: %s", resp.Status)
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[ipp] SendPrintJob: non-2xx body: %s", string(body))
		return "", fmt.Errorf("http status: %s", resp.Status)
	}

	var rsp goipp.Message
	if err := rsp.Decode(resp.Body); err != nil {
		log.Printf("[ipp] SendPrintJob: decode error: %v", err)
		return "", fmt.Errorf("decode ipp response: %w", err)
	}
	log.Printf("[ipp] SendPrintJob: IPP response code: %d (%s)", rsp.Code, goipp.Status(rsp.Code).String())
	if goipp.Status(rsp.Code) != goipp.StatusOk {
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
	Name            string            `json:"name"`
	URI             string            `json:"uri"`
	State           string            `json:"state"`
	StateMessage    string            `json:"stateMessage"`
	StateReasons    []string          `json:"stateReasons"`
	QueuedJobs      int               `json:"queuedJobs"`
	FirmwareVersion string            `json:"firmwareVersion"`
	UptimeSeconds   int               `json:"uptimeSeconds"`
	MarkerNames     []string          `json:"markerNames"`
	MarkerTypes     []string          `json:"markerTypes"`
	MarkerLevels    []int             `json:"markerLevels"`
	MarkerColors    []string          `json:"markerColors"`
	MediaReady      []string          `json:"mediaReady"`
	Attributes      map[string]string `json:"attributes"`
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
	resp, err := http.DefaultClient.Do(httpReq)
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
	if err := rsp.Decode(resp.Body); err != nil {
		log.Printf("[ipp] decode error: %v", err)
		return nil, fmt.Errorf("decode ipp response: %w", err)
	}
	log.Printf("[ipp] IPP response code: %d, printer attrs count: %d", rsp.Code, len(rsp.Printer))

	info := &PrinterInfo{
		URI:        printerURI,
		Attributes: make(map[string]string),
	}

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
		default:
			vals := make([]string, 0, len(a.Values))
			for _, v := range a.Values {
				vals = append(vals, v.V.String())
			}
			info.Attributes[a.Name] = strings.Join(vals, ", ")
		}
	}

	log.Printf("[ipp] GetPrinterAttributes done: name=%q state=%q jobs=%d markers=%d",
		info.Name, info.State, info.QueuedJobs, len(info.MarkerNames))
	return info, nil
}

// Printer represents a CUPS printer.
type Printer struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

// ListPrinters fetches the CUPS /printers HTML page on the given host and extracts printers.
func ListPrinters(host string) ([]Printer, error) {
	u := host
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = "http://" + u
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return nil, fmt.Errorf("invalid host: %w", err)
	}

	hostOnly := parsed.Host
	if !strings.Contains(hostOnly, ":") {
		hostOnly = hostOnly + ":631"
	}

	listURL := (&url.URL{Scheme: "http", Host: hostOnly, Path: "/printers"}).String()

	resp, err := http.Get(listURL)
	if err != nil {
		return nil, fmt.Errorf("fetch printers page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("http status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
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
