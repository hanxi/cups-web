package main

import (
	"bytes"
	"net/http"
	"os"

	"github.com/OpenPrinting/goipp"
)

const uri = "http://localhost:631/printers/EPSON_L380_Series"

// Build IPP OpGetPrinterAttributes request
func makeRequest() ([]byte, error) {
	m := goipp.NewRequest(goipp.DefaultVersion, goipp.OpGetPrinterAttributes, 1)
	m.Operation.Add(goipp.MakeAttribute("attributes-charset",
		goipp.TagCharset, goipp.String("utf-8")))
	m.Operation.Add(goipp.MakeAttribute("attributes-natural-language",
		goipp.TagLanguage, goipp.String("en-US")))
	m.Operation.Add(goipp.MakeAttribute("printer-uri",
		goipp.TagURI, goipp.String(uri)))
	m.Operation.Add(goipp.MakeAttribute("requested-attributes",
		goipp.TagKeyword, goipp.String("all")))

	return m.EncodeBytes()
}

// Check that there is no error
func check(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	request, err := makeRequest()
	check(err)

	resp, err := http.Post(uri, goipp.ContentType, bytes.NewBuffer(request))
	check(err)

	var respMsg goipp.Message

	err = respMsg.Decode(resp.Body)
	check(err)

	respMsg.Print(os.Stdout, false)
}
