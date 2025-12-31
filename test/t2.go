package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/OpenPrinting/goipp"
)

const (
	PrinterURL = "http://localhost:631/printers/EPSON_L380_Series"
	TestPage   = "page.pdf"
)

// checkErr checks for an error. If err != nil, it prints error
// message and exits
func checkErr(err error, format string, args ...interface{}) {
	if err != nil {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "%s: %s\n", msg, err)
		os.Exit(1)
	}
}

// ExamplePrintPDF demo
func main() {
	// Build and encode IPP request
	req := goipp.NewRequest(goipp.DefaultVersion, goipp.OpPrintJob, 1)
	req.Operation.Add(goipp.MakeAttribute("attributes-charset",
		goipp.TagCharset, goipp.String("utf-8")))
	req.Operation.Add(goipp.MakeAttribute("attributes-natural-language",
		goipp.TagLanguage, goipp.String("en-US")))
	req.Operation.Add(goipp.MakeAttribute("printer-uri",
		goipp.TagURI, goipp.String(PrinterURL)))
	req.Operation.Add(goipp.MakeAttribute("requesting-user-name",
		goipp.TagName, goipp.String("John Doe")))
	req.Operation.Add(goipp.MakeAttribute("job-name",
		goipp.TagName, goipp.String("job name")))
	req.Operation.Add(goipp.MakeAttribute("document-format",
		goipp.TagMimeType, goipp.String("application/pdf")))

	payload, err := req.EncodeBytes()
	checkErr(err, "IPP encode")

	// Open document file
	file, err := os.Open(TestPage)
	checkErr(err, "Open document file")

	defer file.Close()

	// Build HTTP request
	body := io.MultiReader(bytes.NewBuffer(payload), file)

	httpReq, err := http.NewRequest(http.MethodPost, PrinterURL, body)
	checkErr(err, "HTTP")

	httpReq.Header.Set("content-type", goipp.ContentType)
	httpReq.Header.Set("accept", goipp.ContentType)

	// Execute HTTP request
	httpRsp, err := http.DefaultClient.Do(httpReq)
	if httpRsp != nil {
		defer httpRsp.Body.Close()
	}

	checkErr(err, "HTTP")

	if httpRsp.StatusCode/100 != 2 {
		checkErr(errors.New(httpRsp.Status), "HTTP")
	}

	// Decode IPP response
	rsp := &goipp.Message{}
	err = rsp.Decode(httpRsp.Body)
	checkErr(err, "IPP decode")

	if goipp.Status(rsp.Code) != goipp.StatusOk {
		err = errors.New(goipp.Status(rsp.Code).String())
		checkErr(err, "IPP")
	}
}
