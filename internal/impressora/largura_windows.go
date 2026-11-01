//go:build windows

package impressora

import (
	"context"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// larguraSistema lê o papel padrão do DEVMODE do driver (GetPrinterW nível 2)
// e, se o driver não preencheu a largura, procura "58"/"80" no nome do driver
// (ex.: "XP-58", "POS-80 Series").
func larguraSistema(_ context.Context, nome string) int {
	n, err := syscall.UTF16PtrFromString(nome)
	if err != nil {
		return 0
	}
	var h windows.Handle
	if r, _, _ := pOpenPrinter.Call(uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(&h)), 0); r == 0 {
		return 0
	}
	defer pClosePrinter.Call(uintptr(h))
	var precisa uint32
	pGetPrinter.Call(uintptr(h), 2, 0, 0, uintptr(unsafe.Pointer(&precisa)))
	if precisa == 0 {
		return 0
	}
	buf := make([]byte, precisa)
	if r, _, _ := pGetPrinter.Call(uintptr(h), 2, uintptr(unsafe.Pointer(&buf[0])), uintptr(precisa), uintptr(unsafe.Pointer(&precisa))); r == 0 {
		return 0
	}
	pi := (*printerInfo2)(unsafe.Pointer(&buf[0]))
	if pi.pDevMode != 0 {
		// O DEVMODE mora dentro de buf (GetPrinter empacota o que aponta).
		base := uintptr(unsafe.Pointer(&buf[0]))
		if off := int(pi.pDevMode - base); pi.pDevMode >= base && off >= 0 && off+devmodeMinimo <= len(buf) {
			if l := LarguraDoDevMode(buf[off:]); l != 0 {
				return l
			}
		}
	}
	if pi.pDriverName != nil {
		return LarguraDoTexto(windows.UTF16PtrToString(pi.pDriverName))
	}
	return 0
}
