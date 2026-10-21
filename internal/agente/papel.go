package agente

import (
	"strings"
	"time"
)

// Bytes ESC/POS que o programa monta sozinho (o resto vem pronto do servidor).
var (
	escInit      = []byte{0x1b, 0x40}       // ESC @
	escCentro    = []byte{0x1b, 0x61, 1}    // ESC a 1
	escEsquerda  = []byte{0x1b, 0x61, 0}    // ESC a 0
	gsDuplo      = []byte{0x1d, 0x21, 0x11} // GS ! 0x11 (largura e altura em dobro)
	gsNormal     = []byte{0x1d, 0x21, 0}    // GS ! 0
	linhaTracos  = "--------------------------------\n"
	semAcentoRep = strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A",
		"é", "e", "ê", "e", "è", "e", "ë", "e", "É", "E", "Ê", "E", "È", "E", "Ë", "E",
		"í", "i", "î", "i", "ì", "i", "ï", "i", "Í", "I", "Î", "I", "Ì", "I", "Ï", "I",
		"ó", "o", "ô", "o", "õ", "o", "ò", "o", "ö", "o", "Ó", "O", "Ô", "O", "Õ", "O", "Ò", "O", "Ö", "O",
		"ú", "u", "û", "u", "ù", "u", "ü", "u", "Ú", "U", "Û", "U", "Ù", "U", "Ü", "U",
		"ç", "c", "Ç", "C", "ñ", "n", "Ñ", "N", "º", "o", "ª", "a",
	)
)

// SemAcento tira acento e qualquer caractere fora do ASCII imprimível
// (impressora térmica sem code page certa imprime lixo).
func SemAcento(s string) string {
	s = semAcentoRep.Replace(s)
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r < 0x7f {
			b.WriteRune(r)
		} else if r == '\n' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func cabecalho(setor string, linhas ...string) []byte {
	var b []byte
	b = append(b, escInit...)
	b = append(b, escCentro...)
	b = append(b, gsDuplo...)
	b = append(b, strings.ToUpper(SemAcento(strings.TrimSpace(setor)))+"\n"...)
	b = append(b, gsNormal...)
	for _, l := range linhas {
		b = append(b, SemAcento(l)+"\n"...)
	}
	b = append(b, linhaTracos...)
	b = append(b, escEsquerda...)
	return b
}

// CabecalhoSetor: NOME DO SETOR grande no topo (setor sem impressora própria
// que saiu na da frente).
func CabecalhoSetor(setor string) []byte {
	return cabecalho(setor)
}

// CabecalhoReserva: NOME DO SETOR grande + "SAIU NA RESERVA" + o motivo curto.
func CabecalhoReserva(setor, motivo string) []byte {
	return cabecalho(setor, "SAIU NA RESERVA", motivoCurto(motivo))
}

func motivoCurto(m string) string {
	m = strings.TrimSpace(SemAcento(m))
	if len(m) > 42 {
		m = m[:41] + "."
	}
	return m
}

// PapelDeTesteLocal é um ESC/POS mínimo: TESTE + NOME DO SETOR grandes,
// linha, avanço e corte.
func PapelDeTesteLocal(setor string) []byte {
	b := append([]byte{}, escInit...)
	b = append(b, escCentro...)
	b = append(b, gsDuplo...)
	b = append(b, "TESTE\n"...)
	b = append(b, strings.ToUpper(SemAcento(setor))+"\n"...)
	b = append(b, gsNormal...)
	b = append(b, "Mesalvai Printer\n"...)
	b = append(b, time.Now().Format("02/01 15:04")+"\n"...)
	b = append(b, linhaTracos...)
	b = append(b, "Saiu? Entao este setor esta certo.\n"...)
	b = append(b, 0x1b, 0x64, 4, 0x1d, 0x56, 0x42, 0)
	return b
}
