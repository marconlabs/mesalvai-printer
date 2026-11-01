package impressora

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// Situação da impressora (P2). São as quatro frases que o painel mostra.
const (
	Pronta      = "pronta"
	SemPapel    = "sem_papel"
	TampaAberta = "tampa_aberta"
	SemResposta = "sem_resposta"
)

// Leitura é o que se sabe da impressora agora.
type Leitura struct {
	Situacao string `json:"situacao"`
	// Lido: a impressora (ou o spooler) disse o estado de verdade. Falso =
	// a conexão abriu mas ela não respondeu ao pedido de estado (muita
	// impressora de rede barata não responde): "pronta" aqui é só "alcançável".
	Lido bool `json:"lido"`
	// Detalhe: frase curta em português, pro log e pro painel.
	Detalhe string `json:"detalhe,omitempty"`
	// Ocupada: outra conexão (impressão) usava a impressora; nada foi lido e
	// quem chamou mantém a última leitura.
	Ocupada bool `json:"-"`
}

// Frase é o texto que vai no alerta ("Impressora X: sem papel").
func (l Leitura) Frase() string {
	switch l.Situacao {
	case SemPapel:
		return "sem papel"
	case TampaAberta:
		return "tampa aberta"
	case SemResposta:
		if l.Detalhe != "" {
			return l.Detalhe
		}
		return "não responde"
	}
	return ""
}

// Ler descobre o estado da impressora `nome` (mesma regra de nomes do Imprimir).
// Nunca imprime nada.
func Ler(ctx context.Context, nome string) Leitura {
	nome = strings.TrimSpace(nome)
	if strings.HasPrefix(strings.ToLower(nome), "tcp://") {
		return lerRede(ctx, nome[len("tcp://"):])
	}
	return lerSistema(ctx, nome)
}

// ── rede: ESC/POS em tempo real (DLE EOT n) ────────────────────────────────

// Cada impressora de rede costuma aceitar uma conexão por vez: imprimir e
// ler o estado nunca abrem duas ao mesmo tempo.
var (
	trancasRede sync.Mutex
	conexoes    = map[string]*sync.Mutex{}
)

func trancaDe(host string) *sync.Mutex {
	trancasRede.Lock()
	defer trancasRede.Unlock()
	m := conexoes[host]
	if m == nil {
		m = &sync.Mutex{}
		conexoes[host] = m
	}
	return m
}

func comPorta(host string) string {
	if !strings.Contains(host, ":") {
		return host + ":9100"
	}
	return host
}

func lerRede(ctx context.Context, host string) Leitura {
	host = comPorta(host)
	m := trancaDe(host)
	if !m.TryLock() {
		return Leitura{Ocupada: true}
	}
	defer m.Unlock()
	return lerRedeEm(ctx, host, (&net.Dialer{Timeout: 3 * time.Second}).DialContext)
}

func lerRedeEm(ctx context.Context, host string, dial func(context.Context, string, string) (net.Conn, error)) Leitura {
	conn, err := dial(ctx, "tcp", host)
	if err != nil {
		return Leitura{Situacao: SemResposta, Detalhe: "não responde na rede (" + host + ")"}
	}
	defer conn.Close()
	var resp [3]byte // n=1 (online), n=2 (causa de offline), n=4 (papel)
	lido := [3]bool{}
	for i, n := range []byte{1, 2, 4} {
		_ = conn.SetDeadline(time.Now().Add(1200 * time.Millisecond))
		if _, err := conn.Write([]byte{0x10, 0x04, n}); err != nil {
			break
		}
		var b [1]byte
		if _, err := conn.Read(b[:]); err != nil {
			break
		}
		resp[i], lido[i] = b[0], true
	}
	return InterpretarEscPos(resp[0], resp[1], resp[2], lido)
}

// ErrSemPapelAgora e ErrTampaAbertaAgora: o motivo cru que a fila mostra.
var (
	ErrSemPapel    = errors.New("sem papel")
	ErrTampaAberta = errors.New("tampa aberta")
)

// InterpretarEscPos traduz as respostas de DLE EOT 1, 2 e 4 (puro, testável).
// Byte válido tem bits fixos (0x93 == 0x12): resposta que não bate é lixo e
// vale como "não respondeu ao pedido de estado".
func InterpretarEscPos(online, causa, papel byte, lido [3]bool) Leitura {
	valido := func(b byte, i int) bool { return lido[i] && b&0x93 == 0x12 }
	if !valido(online, 0) && !valido(causa, 1) && !valido(papel, 2) {
		return Leitura{Situacao: Pronta, Lido: false, Detalhe: "alcançável, mas não informa o estado"}
	}
	// A causa e o papel valem mesmo se o 1 não respondeu.
	if valido(causa, 1) && causa&0x04 != 0 {
		return Leitura{Situacao: TampaAberta, Lido: true, Detalhe: "tampa aberta"}
	}
	if (valido(papel, 2) && papel&0x60 != 0) || (valido(causa, 1) && causa&0x20 != 0) {
		return Leitura{Situacao: SemPapel, Lido: true, Detalhe: "sem papel"}
	}
	if valido(online, 0) && online&0x08 != 0 {
		return Leitura{Situacao: SemResposta, Lido: true, Detalhe: "fora de linha (erro na impressora)"}
	}
	if valido(causa, 1) && causa&0x40 != 0 {
		return Leitura{Situacao: SemResposta, Lido: true, Detalhe: "com erro (veja o visor da impressora)"}
	}
	return Leitura{Situacao: Pronta, Lido: true}
}

// Erro é o motivo que a fila mostra quando a leitura impediu a impressão.
func (l Leitura) Erro() error {
	switch l.Situacao {
	case SemPapel:
		return ErrSemPapel
	case TampaAberta:
		return ErrTampaAberta
	}
	return fmt.Errorf("%s", l.Frase())
}
