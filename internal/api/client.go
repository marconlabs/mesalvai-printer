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
	Target     string `json:"target"` // frente | cozinha
	Title      string `json:"title"`
	Copies     int    `json:"copies"`
	Paper      int    `json:"paper"`
	PayloadB64 string `json:"payload_b64"`
	CreatedAt  string `json:"created_at"`
	Attempts   int    `json:"attempts"`
}

// Hello é o primeiro evento do stream.
type Hello struct {
	AgentID string `json:"agent_id"`
	StoreID string `json:"store_id"`
	PingMS  int    `json:"ping_ms"`
}

// Evento do stream SSE.
type Evento struct {
	Tipo  string // hello | job | revoked | ping
	Job   *Job
	Hello *Hello
}

// Status é o que o programa manda em POST /printer-agent/status.
type Status struct {
	Printers       []string `json:"printers"`
	FrontPrinter   *string  `json:"front_printer,omitempty"`
	KitchenPrinter *string  `json:"kitchen_printer,omitempty"`
	OS             string   `json:"os,omitempty"`
	Version        string   `json:"version,omitempty"`
	LastError      *string  `json:"last_error,omitempty"`
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

// Ack fecha o papel: status "impresso" ou "falhou".
func (c *Client) Ack(ctx context.Context, jobID, status, erro string) error {
	body := map[string]any{"status": status}
	if erro != "" {
		body["error"] = erro
	}
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
	}
	return Evento{}, fmt.Errorf("evento desconhecido %q", tipo)
}
