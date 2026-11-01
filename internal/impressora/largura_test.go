package impressora

import (
	"encoding/binary"
	"testing"
)

func TestClassificarLargura(t *testing.T) {
	casos := map[float64]int{57: 58, 58: 58, 48: 58, 62: 58, 72: 80, 79: 80, 80: 80, 82.5: 80, 210: 0, 100: 0, 30: 0}
	for mm, want := range casos {
		if got := ClassificarLargura(mm); got != want {
			t.Errorf("%v mm: %d (queria %d)", mm, got, want)
		}
	}
}

func TestLarguraDoTexto(t *testing.T) {
	casos := map[string]int{
		"80mm x 297mm":   80,
		"X58MMY210MM":    58,
		"58x297mm":       58,
		"w226h1000":      80, // 226 pt = 79,7 mm
		"w164h1000":      58, // 164 pt = 57,9 mm
		"Roll 80":        80,
		"POS-58":         58,
		"XP-58 Series":   58,
		"EPSON TM-T20II": 0,
		"A4":             0,
		"210x297mm":      0,
		"":               0,
	}
	for in, want := range casos {
		if got := LarguraDoTexto(in); got != want {
			t.Errorf("%q: %d (queria %d)", in, got, want)
		}
	}
}

func devmode(campos uint32, decimos uint16) []byte {
	b := make([]byte, 220)
	binary.LittleEndian.PutUint32(b[72:], campos)
	binary.LittleEndian.PutUint16(b[82:], decimos)
	return b
}

func TestLarguraDoDevMode(t *testing.T) {
	if got := LarguraDoDevMode(devmode(dmPaperWidthCampo, 800)); got != 80 {
		t.Errorf("80 mm: %d", got)
	}
	if got := LarguraDoDevMode(devmode(dmPaperWidthCampo, 580)); got != 58 {
		t.Errorf("58 mm: %d", got)
	}
	// Driver que não preencheu o campo de largura: não chuta.
	if got := LarguraDoDevMode(devmode(0x2, 800)); got != 0 {
		t.Errorf("sem o bit de largura: %d", got)
	}
	// Folha A4 (2100 décimos) não é bobina.
	if got := LarguraDoDevMode(devmode(dmPaperWidthCampo, 2100)); got != 0 {
		t.Errorf("A4: %d", got)
	}
	if got := LarguraDoDevMode(make([]byte, 10)); got != 0 {
		t.Errorf("curto: %d", got)
	}
}

func TestLarguraDoLpoptions(t *testing.T) {
	saida := `PageSize/Media Size: 58x297mm *80x297mm Custom.WIDTHxHEIGHT
Resolution/Output Resolution: *203dpi
`
	if got := LarguraDoLpoptions(saida); got != 80 {
		t.Errorf("padrão 80: %d", got)
	}
	a4 := "PageSize/Media Size: *A4 Letter\nPageRegion/Page Region: *A4\n"
	if got := LarguraDoLpoptions(a4); got != 0 {
		t.Errorf("A4: %d", got)
	}
	if got := LarguraDoLpoptions(""); got != 0 {
		t.Errorf("vazio: %d", got)
	}
}
