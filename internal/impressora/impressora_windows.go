//go:build windows

package impressora

import (
	"context"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// RAW pelo spooler do Windows: OpenPrinterW → StartDocPrinterW (datatype
// "RAW") → WritePrinter → EndDocPrinter. É o que o Saipos Printer/QZ Tray
// fazem: nada de driver interpretando, os bytes ESC/POS vão como estão.

var (
	winspool           = windows.NewLazySystemDLL("winspool.drv")
	pOpenPrinter       = winspool.NewProc("OpenPrinterW")
	pClosePrinter      = winspool.NewProc("ClosePrinter")
	pStartDocPrinter   = winspool.NewProc("StartDocPrinterW")
	pEndDocPrinter     = winspool.NewProc("EndDocPrinter")
	pStartPagePrinter  = winspool.NewProc("StartPagePrinter")
	pEndPagePrinter    = winspool.NewProc("EndPagePrinter")
	pWritePrinter      = winspool.NewProc("WritePrinter")
	pEnumPrinters      = winspool.NewProc("EnumPrintersW")
	pGetDefaultPrinter = winspool.NewProc("GetDefaultPrinterW")
)

type docInfo1 struct {
	pDocName    *uint16
	pOutputFile *uint16
	pDatatype   *uint16
}

type printerInfo4 struct {
	pPrinterName *uint16
	pServerName  *uint16
	attributes   uint32
}

func imprimirSistema(_ context.Context, nome string, dados []byte) error {
	n, err := syscall.UTF16PtrFromString(nome)
	if err != nil {
		return err
	}
	var h windows.Handle
	r, _, e := pOpenPrinter.Call(uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(&h)), 0)
	if r == 0 {
		return fmt.Errorf("abrir impressora %q: %v", nome, e)
	}
	defer pClosePrinter.Call(uintptr(h))
	doc, _ := syscall.UTF16PtrFromString("Mesalvai Printer")
	raw, _ := syscall.UTF16PtrFromString("RAW")
	di := docInfo1{pDocName: doc, pDatatype: raw}
	r, _, e = pStartDocPrinter.Call(uintptr(h), 1, uintptr(unsafe.Pointer(&di)))
	if r == 0 {
		return fmt.Errorf("iniciar documento em %q: %v", nome, e)
	}
	defer pEndDocPrinter.Call(uintptr(h))
	if r, _, e = pStartPagePrinter.Call(uintptr(h)); r == 0 {
		return fmt.Errorf("iniciar página em %q: %v", nome, e)
	}
	defer pEndPagePrinter.Call(uintptr(h))
	var escrito uint32
	for i := 0; i < len(dados); {
		r, _, e = pWritePrinter.Call(uintptr(h), uintptr(unsafe.Pointer(&dados[i])), uintptr(len(dados)-i), uintptr(unsafe.Pointer(&escrito)))
		if r == 0 {
			return fmt.Errorf("escrever em %q: %v", nome, e)
		}
		if escrito == 0 {
			return fmt.Errorf("escrever em %q: a impressora não aceitou os dados", nome)
		}
		i += int(escrito)
	}
	return nil
}

func listarSistema() ([]Info, error) {
	const flags = 0x2 | 0x4 // PRINTER_ENUM_LOCAL | PRINTER_ENUM_CONNECTIONS
	var needed, count uint32
	pEnumPrinters.Call(flags, 0, 4, 0, 0, uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&count)))
	if needed == 0 {
		return nil, nil
	}
	buf := make([]byte, needed)
	r, _, e := pEnumPrinters.Call(flags, 0, 4, uintptr(unsafe.Pointer(&buf[0])), uintptr(needed), uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&count)))
	if r == 0 {
		return nil, fmt.Errorf("listar impressoras: %v", e)
	}
	padrao := impressoraPadrao()
	out := make([]Info, 0, count)
	sz := unsafe.Sizeof(printerInfo4{})
	for i := uint32(0); i < count; i++ {
		pi := (*printerInfo4)(unsafe.Pointer(&buf[uintptr(i)*sz]))
		nome := windows.UTF16PtrToString(pi.pPrinterName)
		out = append(out, Info{Nome: nome, Padrao: nome == padrao})
	}
	return out, nil
}

func impressoraPadrao() string {
	var n uint32
	pGetDefaultPrinter.Call(0, uintptr(unsafe.Pointer(&n)))
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n)
	if r, _, _ := pGetDefaultPrinter.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}
