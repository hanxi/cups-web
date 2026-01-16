//go:build windows

package ipp

import (
	"errors"
	"fmt"
	"io"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Printer struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

const (
	printerEnumLocal       = 0x00000002
	printerEnumConnections = 0x00000004
)

var (
	modWinspool          = windows.NewLazySystemDLL("winspool.drv")
	procEnumPrintersW    = modWinspool.NewProc("EnumPrintersW")
	procOpenPrinterW     = modWinspool.NewProc("OpenPrinterW")
	procClosePrinter     = modWinspool.NewProc("ClosePrinter")
	procStartDocPrinterW = modWinspool.NewProc("StartDocPrinterW")
	procEndDocPrinter    = modWinspool.NewProc("EndDocPrinter")
	procStartPagePrinter = modWinspool.NewProc("StartPagePrinter")
	procEndPagePrinter   = modWinspool.NewProc("EndPagePrinter")
	procWritePrinter     = modWinspool.NewProc("WritePrinter")
)

type printerInfo4 struct {
	pPrinterName *uint16
	pServerName  *uint16
	attributes   uint32
}

type docInfo1 struct {
	pDocName    *uint16
	pOutputFile *uint16
	pDatatype   *uint16
}

// ListPrinters returns printers installed on the Windows system.
// The host argument is ignored on Windows.
func ListPrinters(_ string) ([]Printer, error) {
	flags := uintptr(printerEnumLocal | printerEnumConnections)
	level := uintptr(4)
	var needed uint32
	var returned uint32

	r1, _, err := procEnumPrintersW.Call(
		flags,
		0,
		level,
		0,
		0,
		uintptr(unsafe.Pointer(&needed)),
		uintptr(unsafe.Pointer(&returned)),
	)
	if r1 == 0 && needed == 0 {
		if err != nil && err != windows.ERROR_SUCCESS {
			return nil, fmt.Errorf("enum printers: %w", err)
		}
		return []Printer{}, nil
	}

	if needed == 0 {
		return []Printer{}, nil
	}

	buf := make([]byte, needed)
	r1, _, err = procEnumPrintersW.Call(
		flags,
		0,
		level,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(needed),
		uintptr(unsafe.Pointer(&needed)),
		uintptr(unsafe.Pointer(&returned)),
	)
	if r1 == 0 {
		return nil, fmt.Errorf("enum printers: %w", err)
	}

	infos := unsafe.Slice((*printerInfo4)(unsafe.Pointer(&buf[0])), returned)
	printers := make([]Printer, 0, len(infos))
	for _, info := range infos {
		name := windows.UTF16PtrToString(info.pPrinterName)
		if name == "" {
			continue
		}
		printers = append(printers, Printer{Name: name, URI: name})
	}
	return printers, nil
}

// SendPrintJob sends the document bytes directly to a Windows printer queue.
// The printerURI is treated as a Windows printer name.
func SendPrintJob(printerURI string, r io.Reader, _ string, _ string, jobName string) (string, error) {
	if printerURI == "" {
		return "", errors.New("missing printer name")
	}
	namePtr, err := windows.UTF16PtrFromString(printerURI)
	if err != nil {
		return "", fmt.Errorf("invalid printer name: %w", err)
	}

	var handle windows.Handle
	r1, _, err := procOpenPrinterW.Call(
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(&handle)),
		0,
	)
	if r1 == 0 {
		return "", fmt.Errorf("open printer: %w", err)
	}
	defer procClosePrinter.Call(uintptr(handle))

	if jobName == "" {
		jobName = "print-job"
	}
	docNamePtr, _ := windows.UTF16PtrFromString(jobName)
	dataTypePtr, _ := windows.UTF16PtrFromString("RAW")
	docInfo := docInfo1{
		pDocName:  docNamePtr,
		pDatatype: dataTypePtr,
	}

	jobID, _, err := procStartDocPrinterW.Call(uintptr(handle), 1, uintptr(unsafe.Pointer(&docInfo)))
	if jobID == 0 {
		return "", fmt.Errorf("start doc: %w", err)
	}

	r1, _, err = procStartPagePrinter.Call(uintptr(handle))
	if r1 == 0 {
		_, _, _ = procEndDocPrinter.Call(uintptr(handle))
		return "", fmt.Errorf("start page: %w", err)
	}

	buf := make([]byte, 32*1024)
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			var written uint32
			r1, _, err = procWritePrinter.Call(
				uintptr(handle),
				uintptr(unsafe.Pointer(&buf[0])),
				uintptr(n),
				uintptr(unsafe.Pointer(&written)),
			)
			if r1 == 0 {
				_, _, _ = procEndPagePrinter.Call(uintptr(handle))
				_, _, _ = procEndDocPrinter.Call(uintptr(handle))
				return "", fmt.Errorf("write printer: %w", err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_, _, _ = procEndPagePrinter.Call(uintptr(handle))
			_, _, _ = procEndDocPrinter.Call(uintptr(handle))
			return "", fmt.Errorf("read input: %w", readErr)
		}
	}

	r1, _, err = procEndPagePrinter.Call(uintptr(handle))
	if r1 == 0 {
		_, _, _ = procEndDocPrinter.Call(uintptr(handle))
		return "", fmt.Errorf("end page: %w", err)
	}

	r1, _, err = procEndDocPrinter.Call(uintptr(handle))
	if r1 == 0 {
		return "", fmt.Errorf("end doc: %w", err)
	}

	return fmt.Sprintf("%d", jobID), nil
}
