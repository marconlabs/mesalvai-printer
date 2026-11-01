package impressora

import (
	"context"
	"encoding/binary"
	"regexp"
	"strconv"
	"strings"
)

// Largura do papel que o DRIVER da impressora informa (58 ou 80 mm). É só uma
// sugestão pro painel: nunca troca a largura da loja sozinha, quem confirma é
// a pessoa no papel de teste. Impressora de rede (tcp://) não tem driver aqui:
// devolve 0 e o painel sugere pelo nome/modelo.

// LerLargura devolve 58, 80 ou 0 (o driver não disse / não dá pra saber).
// Nunca imprime nada.
func LerLargura(ctx context.Context, nome string) int {
	nome = strings.TrimSpace(nome)
	if nome == "" || strings.HasPrefix(strings.ToLower(nome), "tcp://") {
		return 0
	}
	return larguraSistema(ctx, nome)
}

// ClassificarLargura leva uma medida de papel em mm para 58 ou 80.
// 48–62 mm vale 58 (rolos de 57/58 mm); 70–90 mm vale 80 (79/80/82,5 mm).
// Qualquer outra medida (A4, 100 mm de etiqueta…) é 0: não é bobina de notinha.
func ClassificarLargura(mm float64) int {
	switch {
	case mm >= 48 && mm <= 62:
		return 58
	case mm >= 70 && mm <= 90:
		return 80
	}
	return 0
}

var (
	reMM    = regexp.MustCompile(`(?i)(\d{2,3}(?:[.,]\d+)?)\s*mm`)
	reXMM   = regexp.MustCompile(`(?i)\bx(\d{2,3})mm`)
	rePts   = regexp.MustCompile(`(?i)\bw(\d{2,3})h\d+`)
	reRolo  = regexp.MustCompile(`(?i)(?:^|[^\d])(58|80)(?:[^\d]|$)`)
	reRotul = regexp.MustCompile(`(?i)\b(?:roll|rolo|receipt|recibo|rp|pos)[\s_-]*(58|80)\b`)
)

// LarguraDoTexto procura a largura no texto de um nome de papel ou de driver
// ("80mm x 297mm", "X58MMY210MM", "w226h1000" em pontos, "Roll 80", "POS-58").
// 0 = nada que pareça bobina de notinha.
func LarguraDoTexto(t string) int {
	t = strings.TrimSpace(t)
	if t == "" {
		return 0
	}
	if m := reXMM.FindStringSubmatch(t); m != nil {
		if n, _ := strconv.ParseFloat(m[1], 64); ClassificarLargura(n) != 0 {
			return ClassificarLargura(n)
		}
	}
	for _, m := range reMM.FindAllStringSubmatch(t, -1) {
		n, _ := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
		if l := ClassificarLargura(n); l != 0 {
			return l
		}
	}
	if m := rePts.FindStringSubmatch(t); m != nil {
		pts, _ := strconv.ParseFloat(m[1], 64)
		if l := ClassificarLargura(pts * 25.4 / 72); l != 0 {
			return l
		}
	}
	if m := reRotul.FindStringSubmatch(t); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	if m := reRolo.FindStringSubmatch(t); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// Campos e posições do DEVMODEW do Windows (sem cabeçalho de arquivo; vale
// pra qualquer driver). dmPaperWidth vem em décimos de milímetro.
const (
	devmodeFields     = 72
	devmodePaperWidth = 82
	devmodeMinimo     = 84
	dmPaperWidthCampo = 0x00000008
)

// LarguraDoDevMode lê a largura do papel padrão do DEVMODE do driver
// (dmPaperWidth, em décimos de mm). 0 se o driver não preencheu o campo ou
// se a medida não é de bobina de notinha.
func LarguraDoDevMode(b []byte) int {
	if len(b) < devmodeMinimo {
		return 0
	}
	campos := binary.LittleEndian.Uint32(b[devmodeFields:])
	if campos&dmPaperWidthCampo == 0 {
		return 0
	}
	decimos := binary.LittleEndian.Uint16(b[devmodePaperWidth:])
	return ClassificarLargura(float64(decimos) / 10)
}

// LarguraDoLpoptions acha o papel padrão (marcado com *) nas linhas de
// `lpoptions -p <nome> -l` (PageSize/PageRegion/media) e devolve a largura.
func LarguraDoLpoptions(saida string) int {
	for _, linha := range strings.Split(saida, "\n") {
		chave, resto, ok := strings.Cut(linha, ":")
		if !ok {
			continue
		}
		k := strings.ToLower(chave)
		if !(strings.HasPrefix(k, "pagesize") || strings.HasPrefix(k, "pageregion") || strings.HasPrefix(k, "media")) {
			continue
		}
		for _, tok := range strings.Fields(resto) {
			if strings.HasPrefix(tok, "*") {
				if l := LarguraDoTexto(strings.TrimPrefix(tok, "*")); l != 0 {
					return l
				}
			}
		}
	}
	return 0
}
