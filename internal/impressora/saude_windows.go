//go:build windows

package impressora

import (
	"context"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var pGetPrinter = winspool.NewProc("GetPrinterW")

// PRINTER_INFO_2: só Status e Attributes importam; o resto mantém o layout.
type printerInfo2 struct {
	pServerName, pPrinterName, pShareName, pPortName, pDriverName, pComment, pLocation     *uint16
	pDevMode                                                                               uintptr
	pSepFile, pPrintProcessor, pDatatype, pParameters                                      *uint16
	pSecurityDescriptor                                                                    uintptr
	attributes, priority, defaultPriority, startTime, untilTime, status, cJobs, averagePPM uint32
}

const (
	psError             = 0x00000002
	psPaperJam          = 0x00000008
	psPaperOut          = 0x00000010
	psPaperProblem      = 0x00000040
	psOffline           = 0x00000080
	psUserIntervention  = 0x00100000
	psNotAvailable      = 0x00001000
	psDoorOpen          = 0x00400000
	atributoWorkOffline = 0x00000400
)

// lerSistema lê as marcas da fila do spooler (USB com driver). O Windows só
// sabe o que o driver/porta contam: com a impressora USB desligada a marca
// "offline" depende do driver (ainda sem teste em loja).
func lerSistema(_ context.Context, nome string) Leitura {
	n, err := syscall.UTF16PtrFromString(nome)
	if err != nil {
		return Leitura{Situacao: SemResposta, Detalhe: "nome de impressora inválido"}
	}
	var h windows.Handle
	if r, _, _ := pOpenPrinter.Call(uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(&h)), 0); r == 0 {
		return Leitura{Situacao: SemResposta, Detalhe: "impressora não encontrada no Windows"}
	}
	defer pClosePrinter.Call(uintptr(h))
	var precisa uint32
	pGetPrinter.Call(uintptr(h), 2, 0, 0, uintptr(unsafe.Pointer(&precisa)))
	if precisa == 0 {
		return Leitura{Situacao: Pronta, Lido: false, Detalhe: "o Windows não informou o estado"}
	}
	buf := make([]byte, precisa)
	if r, _, _ := pGetPrinter.Call(uintptr(h), 2, uintptr(unsafe.Pointer(&buf[0])), uintptr(precisa), uintptr(unsafe.Pointer(&precisa))); r == 0 {
		return Leitura{Situacao: Pronta, Lido: false, Detalhe: "o Windows não informou o estado"}
	}
	pi := (*printerInfo2)(unsafe.Pointer(&buf[0]))
	return interpretarSpooler(pi.status, pi.attributes)
}

func interpretarSpooler(status, atributos uint32) Leitura {
	switch {
	case status&psDoorOpen != 0:
		return Leitura{Situacao: TampaAberta, Lido: true, Detalhe: "tampa aberta"}
	case status&(psPaperOut|psPaperJam|psPaperProblem) != 0:
		return Leitura{Situacao: SemPapel, Lido: true, Detalhe: "sem papel"}
	case status&(psOffline|psNotAvailable|psUserIntervention|psError) != 0 || atributos&atributoWorkOffline != 0:
		return Leitura{Situacao: SemResposta, Lido: true, Detalhe: "desligada ou desconectada (o Windows marcou fora de linha)"}
	}
	return Leitura{Situacao: Pronta, Lido: true}
}
