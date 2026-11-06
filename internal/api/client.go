// Package api fala com a entregas-api (rotas /api/entregas/printer-agent/*).
// Contrato: mesalvai-entregas-api/docs/arquitetura/mesalvai-printer.md.
package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Erros estáveis que o agente trata.
var (
	// ErrNaoAutorizado = 401: a chave foi revogada (desvinculado no painel).
	ErrNaoAutorizado = errors.New("computador não vinculado")
	// ErrExpirado = 410 no pair/wait: o código venceu ou já foi usado.
	ErrExpirado = errors.New("código vencido")
)

// Client é um cliente HTTP da Entregas com a chave do computador.
type Client struct {
	Base      string
	Chave     string
	Versao    string
	SO        string
	HTTP      *http.Client
	UserAgent string
}

// New cria o cliente. base sem barra no fim.
func New(base, chave, so, versao string) *Client {
	return &Client{
		Base:      strings.TrimRight(base, "/"),
		Chave:     chave,
		SO:        so,
		Versao:    versao,
		HTTP:      &http.Client{Timeout: 20 * time.Second},
		UserAgent: "mesalvai-printer/" + versao + " (" + so + ")",
	}
}

// PairStart é a resposta de POST /printer-agent/pair/start.
type PairStart struct {
	Code      string `json:"code"`
	PollToken string `json:"poll_token"`
	ExpiresAt string `json:"expires_at"`
}

// PairWait é a resposta de GET /printer-agent/pair/wait.
type PairWait struct {
	Status string `json:"status"` // waiting | paired | expired
	Key    string `json:"key"`
	Agent  struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"agent"`
	Store struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"store"`
}

// Job é um papel pronto (bytes ESC/POS em base64) vindo do stream.
type Job struct {
	ID         string `json:"id"`
	Origin     string `json:"origin"`
	RefID      string `json:"ref_id"`
	Target     string `json:"target"` // chave do setor: frente | cozinha | <uuid da praça>
	Title      string `json:"title"`
	Copies     int    `json:"copies"`
	Paper      int    `json:"paper"`
	PayloadB64 string `json:"payload_b64"`
	CreatedAt  string `json:"created_at"`
	Attempts   int    `json:"attempts"`
	// v2 (o servidor v1 não manda).
	SectorName string `json:"sector_name,omitempty"`
	Papel      string `json:"papel,omitempty"`
	Origem     string `json:"origem,omitempty"`
	// Reserva: o servidor já desviou de setor e pôs o nome no topo do papel.
	Reserva bool `json:"reserva,omitempty"`
}

// Setor da loja em GET /printer-agent/config.
type Setor struct {
	Key  string `json:"key"`
	Nome string `json:"nome"`
	Tipo string `json:"tipo"`
}

// Vinculo de um setor neste computador (bindings do config).
type Vinculo struct {
	Impressora *string `json:"impressora"`
	Reserva    *string `json:"reserva"`
	Largura    int     `json:"largura,omitempty"`
	Vias       int     `json:"vias,omitempty"`
}

// ConfigAgente é a resposta de GET /printer-agent/config. Setores/Bindings
// nil = servidor v1 (não mexe nos setores locais).
type ConfigAgente struct {
	Setores        []Setor            `json:"setores"`
	Bindings       map[string]Vinculo `json:"bindings"`
	FrontPrinter   *string            `json:"front_printer"`
	KitchenPrinter *string            `json:"kitchen_printer"`
}

// SetorStatus vai em Status.Setores; ponteiro nil no mapa = `null` = apaga.
type SetorStatus struct {
	Impressora *string `json:"impressora"`
	Reserva    *string `json:"reserva"`
}

// Caps que este programa entende (v2).
// "saude" = manda o estado de cada setor (P2/P4); "cancel" = entende o evento
// `cancel {job_ids}` (o navegador assumiu o papel). Servidor sem esses caps
// nunca recebe nem manda nada novo.
var Caps = []string{"setores", "reserva", "procurar", "saude", "cancel"}

// SaudeSetor é o estado de um setor neste computador (Status.Saude). A API
// não precisa do relógio daqui: `fora_ha_s` já diz há quanto tempo o setor
// não consegue imprimir e `degradado` é a decisão do Printer (2 min fora).
type SaudeSetor struct {
	Estado     string `json:"estado"` // pronta | sem_papel | tampa_aberta | sem_resposta
	Lido       bool   `json:"lido"`
	Detalhe    string `json:"detalhe,omitempty"`
	Impressora string `json:"impressora,omitempty"`
	NaReserva  bool   `json:"na_reserva,omitempty"`
	// Imprime: a principal OU a reserva está pronta.
	Imprime   bool `json:"imprime"`
	ForaHaS   int  `json:"fora_ha_s,omitempty"`
	Degradado bool `json:"degradado,omitempty"`
	IPMudou   bool `json:"ip_mudou,omitempty"`
	// TesteEm: último "Imprimir teste" que saiu daqui (RFC 3339); vazio = falta o teste.
	TesteEm string `json:"teste_em,omitempty"`
	// LarguraDetectada: largura do papel (58 ou 80, em mm) que o driver desta
	// impressora informa; 0/ausente = o driver não disse (impressora de rede
	// nunca diz). É só sugestão: quem decide a largura é a pessoa, no teste.
	LarguraDetectada int `json:"largura_detectada,omitempty"`
}

// LarguraSetor é a largura sugerida de um setor (só 58/80; fonte "driver").
type LarguraSetor struct {
	MM         int    `json:"mm"`
	Fonte      string `json:"fonte"`
	Impressora string `json:"impressora,omitempty"`
}

// AckInfo é o corpo do ack v2.
type AckInfo struct {
	Status  string `json:"status"`            // impresso | falhou
	Error   string `json:"error,omitempty"`   // erro (na reserva: o da principal)
	Via     string `json:"via,omitempty"`     // principal | reserva
	Printer string `json:"printer,omitempty"` // onde saiu
}

// Hello é o primeiro evento do stream.
type Hello struct {
	AgentID string `json:"agent_id"`
	StoreID string `json:"store_id"`
	PingMS  int    `json:"ping_ms"`
}

// Evento do stream SSE.
type Evento struct {
	Tipo  string // hello | job | revoked | config | cancel | procurar | ping
	Job   *Job
	Hello *Hello
	// Procurar: pedido do painel para varrer a rede atrás de impressoras.
	Procurar *ComandoProcurar
	// Cancelar: ids dos papéis que o navegador assumiu (evento cancel).
	Cancelar []string
}

// ComandoProcurar é o corpo (opcional) do evento `procurar` do stream: o painel
// pede "Procurar impressoras" neste computador. Campos vazios = qualquer um.
// Evento opcional: servidor que não manda não muda nada aqui.
type ComandoProcurar struct {
	// AgenteID: para qual computador é; vazio = quem estiver ouvindo.
	AgenteID string `json:"agente_id,omitempty"`
	// Setor: de qual setor veio o pedido (só vai pro log; a lista achada
	// serve a todos os setores).
	Setor string `json:"setor,omitempty"`
}

// Status é o que o programa manda em POST /printer-agent/status.
type Status struct {
	Printers       []string `json:"printers"`
	FrontPrinter   *string  `json:"front_printer,omitempty"`
	KitchenPrinter *string  `json:"kitchen_printer,omitempty"`
	OS             string   `json:"os,omitempty"`
	Version        string   `json:"version,omitempty"`
	LastError      *string  `json:"last_error,omitempty"`
	// v2: vínculos que mudaram (parcial; valor nil = apaga) e o que entende.
	Setores map[string]*SetorStatus `json:"setores,omitempty"`
	Caps    []string                `json:"caps,omitempty"`
	// Saude por setor (chave do setor). Sempre completo: quem não vem, saiu.
	// Ponteiro: nil = ainda não leu (não vai no JSON); mapa vazio vai como `{}`
	// (todos os setores ficaram sem impressora). Com `omitempty` num mapa, o
	// `{}` sumia e a API (COALESCE) ficava com a saúde velha para sempre.
	Saude *map[string]SaudeSetor `json:"saude,omitempty"`
	// LarguraDetectada no formato da API (`{"frente":{"mm":58,"fonte":"driver","impressora":"EPSON"}}`).
	// A API só lê o campo do topo; `saude.<setor>.largura_detectada` fica pro programa antigo.
	LarguraDetectada map[string]LarguraSetor `json:"largura_detectada,omitempty"`
	// SemSono: o programa está impedindo o repouso do computador (P1).
	SemSono *bool `json:"sem_sono,omitempty"`
}

// Me é a resposta de GET /printer-agent/me.
type Me struct {
	Agent struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"agent"`
	Store struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"store"`
}

func (c *Client) req(ctx context.Context, method, path string, body any, auth bool) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+c.Chave)
	}
	return c.HTTP.Do(req)
}

func decode(res *http.Response, out any) error {
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return ErrNaoAutorizado
	}
	if res.StatusCode == http.StatusGone {
		return ErrExpirado
	}
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		if e.Error != "" {
			return fmt.Errorf("%s (%d)", e.Error, res.StatusCode)
		}
		return fmt.Errorf("status %d", res.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
}

// PairStart pede um código de pareamento (sem chave).
func (c *Client) PairStart(ctx context.Context, nome string) (PairStart, error) {
	var out PairStart
	res, err := c.req(ctx, http.MethodPost, "/api/entregas/printer-agent/pair/start", map[string]any{"name": nome, "os": c.SO, "version": c.Versao}, false)
	if err != nil {
		return out, err
	}
	return out, decode(res, &out)
}

// PairWait espera (long-poll de até ~25 s) a loja digitar o código.
func (c *Client) PairWait(ctx context.Context, pollToken string) (PairWait, error) {
	var out PairWait
	cl := *c.HTTP
	cl.Timeout = 40 * time.Second
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/api/entregas/printer-agent/pair/wait?poll_token="+pollToken, nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	res, err := cl.Do(req)
	if err != nil {
		return out, err
	}
	return out, decode(res, &out)
}

// Me confere a chave e devolve loja/computador.
func (c *Client) Me(ctx context.Context) (Me, error) {
	var out Me
	res, err := c.req(ctx, http.MethodGet, "/api/entregas/printer-agent/me", nil, true)
	if err != nil {
		return out, err
	}
	return out, decode(res, &out)
}

// EnviarStatus manda impressoras/escolhas/erro.
func (c *Client) EnviarStatus(ctx context.Context, s Status) error {
	if s.OS == "" {
		s.OS = c.SO
	}
	if s.Version == "" {
		s.Version = c.Versao
	}
	res, err := c.req(ctx, http.MethodPost, "/api/entregas/printer-agent/status", s, true)
	if err != nil {
		return err
	}
	return decode(res, nil)
}

// Config busca setores e vínculos deste computador.
func (c *Client) Config(ctx context.Context) (ConfigAgente, error) {
	var out ConfigAgente
	res, err := c.req(ctx, http.MethodGet, "/api/entregas/printer-agent/config", nil, true)
	if err != nil {
		return out, err
	}
	return out, decode(res, &out)
}

// Ack fecha o papel: status "impresso" ou "falhou".
func (c *Client) Ack(ctx context.Context, jobID, status, erro string) error {
	return c.AckCom(ctx, jobID, AckInfo{Status: status, Error: erro})
}

// AckCom fecha o papel dizendo por onde saiu (principal/reserva).
func (c *Client) AckCom(ctx context.Context, jobID string, body AckInfo) error {
	res, err := c.req(ctx, http.MethodPost, "/api/entregas/printer-agent/jobs/"+jobID+"/ack", body, true)
	if err != nil {
		return err
	}
	err = decode(res, nil)
	// 404 = papel já fechado/desconhecido: não insiste.
	if err != nil && strings.Contains(err.Error(), "(404)") {
		return nil
	}
	return err
}

// Stream abre o SSE e entrega eventos até o contexto acabar ou a conexão cair.
// Cada `: ping` vira Evento{Tipo:"ping"} (o chamador vigia o silêncio).
func (c *Client) Stream(ctx context.Context, eventos chan<- Evento) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/api/entregas/printer-agent/stream?os="+c.SO+"&version="+c.Versao, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Authorization", "Bearer "+c.Chave)
	req.Header.Set("Accept", "text/event-stream")
	cl := &http.Client{Timeout: 0, Transport: c.HTTP.Transport}
	res, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return ErrNaoAutorizado
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("stream status %d", res.StatusCode)
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	var tipo string
	var data []string
	flush := func() error {
		if tipo == "" && len(data) == 0 {
			return nil
		}
		ev, err := parseEvento(tipo, strings.Join(data, "\n"))
		tipo, data = "", nil
		if err != nil {
			return nil // evento desconhecido: ignora
		}
		select {
		case eventos <- ev:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if err := flush(); err != nil {
				return err
			}
		case strings.HasPrefix(line, ":"):
			// comentário (`: ping`, `: ok`)
			select {
			case eventos <- Evento{Tipo: "ping"}:
			case <-ctx.Done():
				return ctx.Err()
			}
		case strings.HasPrefix(line, "event:"):
			tipo = strings.TrimSpace(line[6:])
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimSpace(line[5:]))
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return io.EOF
}

func parseEvento(tipo, data string) (Evento, error) {
	switch tipo {
	case "hello":
		var h Hello
		if err := json.Unmarshal([]byte(data), &h); err != nil {
			return Evento{}, err
		}
		return Evento{Tipo: "hello", Hello: &h}, nil
	case "job":
		var j Job
		if err := json.Unmarshal([]byte(data), &j); err != nil {
			return Evento{}, err
		}
		if j.Copies < 1 {
			j.Copies = 1
		}
		return Evento{Tipo: "job", Job: &j}, nil
	case "revoked":
		return Evento{Tipo: "revoked"}, nil
	case "config":
		// "Busque GET /printer-agent/config de novo" (o corpo não importa).
		return Evento{Tipo: "config"}, nil
	case "procurar":
		// Corpo opcional: sem JSON (ou JSON estranho) = procurar mesmo assim.
		var p ComandoProcurar
		_ = json.Unmarshal([]byte(data), &p)
		return Evento{Tipo: "procurar", Procurar: &p}, nil
	case "cancel":
		var c struct {
			JobIDs []string `json:"job_ids"`
		}
		if err := json.Unmarshal([]byte(data), &c); err != nil {
			return Evento{}, err
		}
		return Evento{Tipo: "cancel", Cancelar: c.JobIDs}, nil
	}
	return Evento{}, fmt.Errorf("evento desconhecido %q", tipo)
}
