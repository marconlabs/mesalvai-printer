package agente

import (
	"strings"
	"testing"
)

// O cabeçalho que o programa monta sozinho cabe em 32 colunas (48 e 58 mm) e,
// por tabela, em 80 mm (48 colunas).
func TestCabecalhoCabeNoPapelEstreito(t *testing.T) {
	b := string(CabecalhoReserva("Cozinha", "A impressora da cozinha ficou sem papel e sem resposta há mais de dois minutos"))
	for _, l := range strings.Split(b, "\n") {
		limpa := strings.Map(func(r rune) rune {
			if r < 0x20 || r > 0x7e {
				return -1
			}
			return r
		}, l)
		// tira os parâmetros dos comandos ESC/GS (bytes imprimíveis após 0x1b/0x1d)
		if len(limpa) > colunasMinimas+3 {
			t.Errorf("linha passa de 32 colunas: %q (%d)", limpa, len(limpa))
		}
	}
	if got := motivoCurto(strings.Repeat("a", 50)); len(got) != colunasMinimas {
		t.Errorf("motivo curto: %d", len(got))
	}
	if len(strings.TrimSuffix(linhaTracos, "\n")) != colunasMinimas {
		t.Errorf("linha de traços deve ter 32 colunas")
	}
}
