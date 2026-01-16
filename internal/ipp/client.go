//go:build !windows
// +build !windows

package ipp

import (
    "bytes"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "regexp"
    "strings"

    goipp "github.com/OpenPrinting/goipp"
)

// SendPrintJob sends data to the printer via IPP using goipp to build the
// IPP Print-Job request. It returns a human-readable status or job identifier
// when available.
func SendPrintJob(printerURI string, r io.Reader, mime string, username string, jobName string) (string, error) {
    // Build IPP Print-Job request
    req := goipp.NewRequest(goipp.DefaultVersion, goipp.OpPrintJob, 1)
    req.Operation.Add(goipp.MakeAttribute("attributes-charset", goipp.TagCharset, goipp.String("utf-8")))
    req.Operation.Add(goipp.MakeAttribute("attributes-natural-language", goipp.TagLanguage, goipp.String("en-US")))
    req.Operation.Add(goipp.MakeAttribute("printer-uri", goipp.TagURI, goipp.String(printerURI)))
    if username != "" {
        req.Operation.Add(goipp.MakeAttribute("requesting-user-name", goipp.TagName, goipp.String(username)))
    }
    if jobName != "" {
        req.Operation.Add(goipp.MakeAttribute("job-name", goipp.TagName, goipp.String(jobName)))
    }
    if mime == "" {
        mime = "application/octet-stream"
    }
    req.Operation.Add(goipp.MakeAttribute("document-format", goipp.TagMimeType, goipp.String(mime)))

    payload, err := req.EncodeBytes()
    if err != nil {
        return "", fmt.Errorf("encode ipp request: %w", err)
    }

    // Prepare HTTP body: IPP request bytes followed by document bytes
    body := io.MultiReader(bytes.NewBuffer(payload), r)

    httpReq, err := http.NewRequest(http.MethodPost, printerURI, body)
    if err != nil {
        return "", fmt.Errorf("create http request: %w", err)
    }
    httpReq.Header.Set("Content-Type", goipp.ContentType)
    httpReq.Header.Set("Accept", goipp.ContentType)

    resp, err := http.DefaultClient.Do(httpReq)
    if resp != nil {
        defer resp.Body.Close()
    }
    if err != nil {
        return "", fmt.Errorf("http post: %w", err)
    }
    if resp.StatusCode/100 != 2 {
        return "", fmt.Errorf("http status: %s", resp.Status)
    }

    var rsp goipp.Message
    if err := rsp.Decode(resp.Body); err != nil {
        return "", fmt.Errorf("decode ipp response: %w", err)
    }
    if goipp.Status(rsp.Code) != goipp.StatusOk {
        return "", fmt.Errorf("ipp error: %s", goipp.Status(rsp.Code).String())
    }

    // Try to return job-uri or job-id if present in Job attributes
    for _, a := range rsp.Job {
        if a.Name == "job-uri" || a.Name == "job-id" {
            if len(a.Values) > 0 {
                return a.Values[0].V.String(), nil
            }
        }
    }

    return "ok", nil
}

type Printer struct {
    Name string `json:"name"`
    URI  string `json:"uri"`
}

// ListPrinters fetches the CUPS /printers HTML page on the given host (host may
// include a port or scheme) and extracts printers. Returns a slice of Printer
// with an IPP-style URI (ipp://host:631/printers/<name>).
func ListPrinters(host string) ([]Printer, error) {
    // normalize host to a http URL
    u := host
    if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
        u = "http://" + u
    }
    parsed, err := url.Parse(u)
    if err != nil {
        return nil, fmt.Errorf("invalid host: %w", err)
    }

    // if no port provided, use 631
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

    // parse anchors like <a href="/printers/Printer_Name">Printer Display</a>
    re := regexp.MustCompile(`(?i)<a[^>]+href=["']/printers/([^"'/>]+)["'][^>]*>([^<]+)</a>`)
    matches := re.FindAllSubmatch(body, -1)
    printers := make([]Printer, 0, len(matches))
    for _, m := range matches {
        name := string(m[1])
        display := string(m[2])
        // build ipp URI
        uri := fmt.Sprintf("http://%s/printers/%s", hostOnly, name)
        printers = append(printers, Printer{Name: display, URI: uri})
    }

    return printers, nil
}
