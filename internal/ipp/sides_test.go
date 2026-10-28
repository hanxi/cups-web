package ipp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	goipp "github.com/OpenPrinting/goipp"
)

// Inspect requests produced by SendPrintJob itself, including keyword type and
// group placement; duplicating the request builder would miss issue #129.
func TestSendPrintJobSides(t *testing.T) {
	t.Setenv("CUPS_SOCKET_PATH", "")
	for _, tc := range []struct {
		name, sides, pageSet, want string
		legacy                     bool
	}{
		{name: "default", want: SidesOneSided},
		{name: "legacy-duplex", legacy: true, want: SidesLongEdge},
		{name: "long-edge", sides: SidesLongEdge, want: SidesLongEdge},
		{name: "short-edge-overrides-false", sides: SidesShortEdge, want: SidesShortEdge},
		{name: "single-overrides-true", sides: SidesOneSided, legacy: true, want: SidesOneSided},
		{name: "odd", sides: SidesShortEdge, pageSet: "odd", want: SidesOneSided},
		{name: "even", sides: SidesShortEdge, pageSet: "even", want: SidesOneSided},
		{name: "even-reverse", sides: SidesShortEdge, pageSet: "even-reverse", want: SidesOneSided},
		{name: "invalid", sides: "short-edge"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var msg goipp.Message
				if err := msg.Decode(r.Body); err != nil {
					t.Error(err)
					return
				}
				io.Copy(io.Discard, r.Body)
				count := 0
				for _, attr := range msg.Job {
					if attr.Name == "sides" {
						count++
						if len(attr.Values) != 1 || attr.Values[0].T != goipp.TagKeyword || attr.Values[0].V.String() != tc.want {
							t.Errorf("sides attribute=%v, want keyword %q", attr, tc.want)
						}
					}
				}
				if count != 1 {
					t.Errorf("sides count=%d", count)
				}
				for _, attr := range msg.Operation {
					if attr.Name == "sides" {
						t.Error("sides leaked into operation group")
					}
				}
				response := goipp.NewResponse(goipp.DefaultVersion, goipp.StatusOk, msg.RequestID)
				response.Job.Add(goipp.MakeAttribute("job-id", goipp.TagInteger, goipp.Integer(129)))
				data, err := response.EncodeBytes()
				if err != nil {
					t.Error(err)
					return
				}
				w.Write(data)
			}))
			defer server.Close()
			_, err := SendPrintJob(server.URL+"/printers/test", strings.NewReader("document"), "application/pdf", "test", "test.pdf",
				PrintJobOptions{Sides: tc.sides, IsDuplex: tc.legacy, PageSet: tc.pageSet})
			if tc.want == "" {
				if err == nil || calls != 0 {
					t.Fatalf("invalid sides: err=%v calls=%d", err, calls)
				}
			} else if err != nil || calls != 1 {
				t.Fatalf("submission: err=%v calls=%d", err, calls)
			}
		})
	}
}
