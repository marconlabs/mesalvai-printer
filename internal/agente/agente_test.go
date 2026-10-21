package agente

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/marconlabs/mesalvai-printer/internal/config"
	"github.com/marconlabs/mesalvai-printer/internal/fila"
	"github.com/marconlabs/mesalvai-printer/internal/impressora"
)

// servidorFalso imita a entregas-api: pareamento, stream SSE, ack, status.
type servidorFalso struct {
	mu       sync.Mutex
	pareado  bool
	entregue bool
	chave    string
	acks     []string
	status   []map[string]any
	jobs     chan map[string]any
	streams  int
	derrubar chan struct{}
	revogado bool
	srv      *httptest.Server
}

func novoServidor(t *testing.T) *servidorFalso {
	t.Helper()
	s := &servidorFalso{chave: "mpk_teste", jobs: make(chan map[string]any, 8), derrubar: make(chan struct{}, 1)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/entregas/printer-agent/pair/start", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		if b["name"] != "PC-TESTE" {
			t.Errorf("nome: %v", b)
		}
		w.WriteHeader(201)
		_, _ = fmt.Fprint(w, `{"code":"ABCDEF","poll_token":"tok","expires_at":"2026-10-21T12:10:00Z"}`)
	})
	mux.HandleFunc("GET /api/entregas/printer-agent/pair/wait", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("poll_token") != "tok" {
			w.WriteHeader(422)
			return
		}
		s.mu.Lock()
		p, e := s.pareado, s.entregue
		if p {
			s.entregue = true
		}
		s.mu.Unlock()
		switch {
		case p && !e:
			_, _ = fmt.Fprint(w, `{"status":"paired","key":"mpk_teste","agent":{"id":"a1","name":"PC-TESTE"},"store":{"id":"s1","name":"Loja Teste"}}`)
		case e:
			w.WriteHeader(410)
			_, _ = fmt.Fprint(w, `{"status":"expired"}`)
		default:
			time.Sleep(50 * time.Millisecond)
			_, _ = fmt.Fprint(w, `{"status":"waiting"}`)
		}
	})
	auth := func(r *http.Request) bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return !s.revogado && r.Header.Get("Authorization") == "Bearer "+s.chave
	}
	mux.HandleFunc("POST /api/entregas/printer-agent/status", func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			w.WriteHeader(401)
			return
		}
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		s.mu.Lock()
		s.status = append(s.status, b)
		s.mu.Unlock()
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /api/entregas/printer-agent/jobs/{id}/ack", func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			w.WriteHeader(401)
			return
		}
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		s.mu.Lock()
		s.acks = append(s.acks, r.PathValue("id")+":"+b["status"].(string))
		s.mu.Unlock()
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /api/entregas/printer-agent/stream", func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			w.WriteHeader(401)
			return
		}
		s.mu.Lock()
		s.streams++
		s.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		_, _ = fmt.Fprint(w, ": ok\n\n")
		_, _ = fmt.Fprint(w, "event: hello\ndata: {\"agent_id\":\"a1\",\"store_id\":\"s1\",\"ping_ms\":100}\n\n")
		fl.Flush()
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-s.derrubar:
				return
			case <-t.C:
				s.mu.Lock()
				rev := s.revogado
				s.mu.Unlock()
				if rev {
					_, _ = fmt.Fprint(w, "event: revoked\ndata: {}\n\n")
					fl.Flush()
					return
				}
				_, _ = fmt.Fprint(w, ": ping\n\n")
				fl.Flush()
			case j := <-s.jobs:
				b, _ := json.Marshal(j)
				_, _ = fmt.Fprintf(w, "event: job\ndata: %s\n\n", b)
				fl.Flush()
			}
		}
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *servidorFalso) job(id, target string) {
	s.jobs <- map[string]any{"id": id, "origin": "order", "ref_id": "o1", "target": target, "title": "Pedido #1", "copies": 1, "paper": 80, "payload_b64": base64.StdEncoding.EncodeToString([]byte("\x1b@PEDIDO 1\n")), "created_at": "2026-10-21T12:00:00Z", "attempts": 0}
}

func (s *servidorFalso) temAck(x string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.acks {
		if a == x {
			return true
		}
	}
	return false
}

// impressoraFalsa grava o que recebeu e pode "cair".
type impressoraFalsa struct {
	mu     sync.Mutex
	caida  bool
	saidas []string
}

func (i *impressoraFalsa) imprimir(_ context.Context, nome string, dados []byte) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.caida {
		return errors.New("sem papel")
	}
	i.saidas = append(i.saidas, nome+":"+strings.TrimSpace(string(dados[2:])))
	return nil
}

func ate(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	fim := time.Now().Add(5 * time.Second)
	for time.Now().Before(fim) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("esperando: %s", msg)
}

func montar(t *testing.T, srv *servidorFalso, imp *impressoraFalsa) (*Agente, *config.Store, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("MESALVAI_PRINTER_DIR", dir)
	t.Setenv("MESALVAI_API", srv.srv.URL)
	cfg, err := config.Abrir()
	if err != nil {
		t.Fatal(err)
	}
	f, _ := fila.Abrir(filepath.Join(dir, "fila"))
	a := New(cfg, f, Opcoes{
		Versao: "0.0.0-teste", SO: "teste", NomeDoPC: "PC-TESTE",
		Imprimir: imp.imprimir,
		Listar: func() ([]impressora.Info, error) {
			return []impressora.Info{{Nome: "EPSON", Padrao: true}, {Nome: "COZ"}}, nil
		},
		BackoffMin: 20 * time.Millisecond, BackoffMax: 100 * time.Millisecond, Silencio: 600 * time.Millisecond, RetryFila: 60 * time.Millisecond,
	})
	return a, cfg, dir
}

func TestPareamentoStreamImpressaoAck(t *testing.T) {
	srv := novoServidor(t)
	imp := &impressoraFalsa{}
	a, cfg, _ := montar(t, srv, imp)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Run(ctx)

	// 1. Mostra o código e espera a loja.
	ate(t, func() bool { return a.Estado().Codigo == "ABCDEF" }, "código na tela")
	if a.Estado().Vinculado {
		t.Fatal("não podia estar vinculado")
	}
	// 2. A loja digitou: a chave chega uma vez e fica no disco.
	srv.mu.Lock()
	srv.pareado = true
	srv.mu.Unlock()
	ate(t, func() bool { return a.Estado().Vinculado && a.Estado().LojaNome == "Loja Teste" }, "vinculado")
	if cfg.Get().Chave != "mpk_teste" {
		t.Fatalf("chave: %q", cfg.Get().Chave)
	}
	// 3. Conecta o stream e manda o status com as impressoras do sistema.
	ate(t, func() bool { return a.Estado().Conectado }, "conectado")
	ate(t, func() bool { srv.mu.Lock(); defer srv.mu.Unlock(); return len(srv.status) > 0 }, "status enviado")
	srv.mu.Lock()
	st := srv.status[0]
	srv.mu.Unlock()
	if fmt.Sprint(st["printers"]) != "[EPSON COZ]" {
		t.Fatalf("printers: %v", st)
	}
	// 4. Sem impressora escolhida: o papel espera na fila e avisa.
	srv.job("j1", "frente")
	ate(t, func() bool { return a.Estado().NaFila == 1 }, "na fila")
	time.Sleep(100 * time.Millisecond)
	if len(imp.saidas) != 0 {
		t.Fatal("não podia imprimir sem impressora escolhida")
	}
	// 5. Escolheu: sai e dá ack.
	if err := a.Escolher(ctx, "EPSON", ""); err != nil {
		t.Fatal(err)
	}
	ate(t, func() bool { return srv.temAck("j1:impresso") }, "ack j1")
	imp.mu.Lock()
	s0 := imp.saidas[0]
	imp.mu.Unlock()
	if s0 != "EPSON:PEDIDO 1" || a.Estado().NaFila != 0 || a.Estado().Impressos != 1 {
		t.Fatalf("saída %q, fila %d, impressos %d", s0, a.Estado().NaFila, a.Estado().Impressos)
	}
	// 6. Cozinha sem impressora própria sai na frente; com a própria, lá.
	srv.job("j2", "cozinha")
	ate(t, func() bool { return srv.temAck("j2:impresso") }, "ack j2")
	_ = a.Escolher(ctx, "EPSON", "COZ")
	srv.job("j3", "cozinha")
	ate(t, func() bool { return srv.temAck("j3:impresso") }, "ack j3")
	imp.mu.Lock()
	ult := imp.saidas[len(imp.saidas)-1]
	imp.mu.Unlock()
	if ult != "COZ:PEDIDO 1" {
		t.Fatalf("cozinha: %q", ult)
	}
}

func TestImpressoraCaiuFilaEmDiscoEVolta(t *testing.T) {
	srv := novoServidor(t)
	srv.pareado = true
	imp := &impressoraFalsa{caida: true}
	a, cfg, dir := montar(t, srv, imp)
	_ = cfg.Update(func(c *config.Config) { c.Chave = "mpk_teste"; c.Frente = "EPSON" })
	ctx, cancel := context.WithCancel(context.Background())
	go a.Run(ctx)
	ate(t, func() bool { return a.Estado().Conectado }, "conectado")
	srv.job("j1", "frente")
	srv.job("j2", "frente")
	ate(t, func() bool { return a.Estado().NaFila == 2 && a.Estado().UltimoErro != "" }, "2 na fila com erro")
	// O erro vai pro painel (last_error).
	ate(t, func() bool {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		for _, s := range srv.status {
			if e, _ := s["last_error"].(string); strings.Contains(e, "sem papel") {
				return true
			}
		}
		return false
	}, "last_error no status")
	// Programa fechou com a fila cheia: os arquivos estão lá.
	cancel()
	time.Sleep(50 * time.Millisecond)
	ents, _ := os.ReadDir(filepath.Join(dir, "fila"))
	if n := len(ents); n != 2 {
		t.Fatalf("fila em disco: %d", n)
	}
	// Abriu de novo com a impressora de volta: imprime na ordem e dá os acks.
	imp.mu.Lock()
	imp.caida = false
	imp.mu.Unlock()
	f, _ := fila.Abrir(filepath.Join(dir, "fila"))
	a2 := New(cfg, f, a.op)
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go a2.Run(ctx2)
	ate(t, func() bool { return srv.temAck("j1:impresso") && srv.temAck("j2:impresso") }, "acks depois de voltar")
	imp.mu.Lock()
	ordem := strings.Join(imp.saidas, ",")
	imp.mu.Unlock()
	if ordem != "EPSON:PEDIDO 1,EPSON:PEDIDO 1" || a2.Estado().NaFila != 0 {
		t.Fatalf("ordem %q fila %d", ordem, a2.Estado().NaFila)
	}
}

func TestReconexaoERevogacao(t *testing.T) {
	srv := novoServidor(t)
	srv.pareado = true
	imp := &impressoraFalsa{}
	a, cfg, _ := montar(t, srv, imp)
	_ = cfg.Update(func(c *config.Config) { c.Chave = "mpk_teste"; c.Frente = "EPSON" })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Run(ctx)
	ate(t, func() bool { return a.Estado().Conectado }, "conectado")
	// O servidor derruba o stream: o programa volta sozinho.
	srv.derrubar <- struct{}{}
	ate(t, func() bool { return !a.Estado().Conectado }, "caiu")
	ate(t, func() bool { srv.mu.Lock(); defer srv.mu.Unlock(); return srv.streams >= 2 }, "reconectou")
	ate(t, func() bool { return a.Estado().Conectado }, "conectado de novo")
	// Papel chega depois da reconexão e sai normal.
	srv.job("j9", "frente")
	ate(t, func() bool { return srv.temAck("j9:impresso") }, "ack j9")
	// Desvinculado no painel: evento revoked → chave some, volta a pedir código.
	srv.mu.Lock()
	srv.revogado = true
	srv.mu.Unlock()
	ate(t, func() bool { return !a.Estado().Vinculado && cfg.Get().Chave == "" }, "desvinculado")
	srv.mu.Lock()
	srv.revogado, srv.entregue, srv.pareado = false, false, false
	srv.mu.Unlock()
	ate(t, func() bool { return a.Estado().Codigo == "ABCDEF" }, "pede código de novo")
}

func TestParseStreamIgnoraEventoDesconhecido(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: novidade\ndata: {}\n\nevent: hello\ndata: {\"agent_id\":\"a\",\"store_id\":\"s\",\"ping_ms\":1}\n\n")
	}))
	defer srv.Close()
	t.Setenv("MESALVAI_PRINTER_DIR", t.TempDir())
	t.Setenv("MESALVAI_API", srv.URL)
	cfg, _ := config.Abrir()
	_ = cfg.Update(func(c *config.Config) { c.Chave = "mpk_x" })
	f, _ := fila.Abrir(filepath.Join(t.TempDir(), "fila"))
	a := New(cfg, f, Opcoes{Listar: func() ([]impressora.Info, error) { return nil, nil }, BackoffMin: time.Hour, Silencio: time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = a.umStream(ctx)
	if !a.Estado().Conectado {
		t.Fatal("hello depois de evento desconhecido devia conectar")
	}
}
