// Package ui: a janela do programa é uma página local (127.0.0.1) aberta no
// navegador — código de pareamento, status, impressoras, teste, log — e a
// bandeja do sistema. Sem toolkit gráfico pesado: um binário só.
package ui

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/marconlabs/mesalvai-printer/internal/agente"
	"github.com/marconlabs/mesalvai-printer/internal/autostart"
	"github.com/marconlabs/mesalvai-printer/internal/config"
)

//go:embed pagina.html
var pagina []byte

// Web é o servidor local da janela.
type Web struct {
	Agente  *agente.Agente
	Cfg     *config.Store
	LogPath string
	URL     string
	Sair    func()
}

// Servir sobe em 127.0.0.1 na porta gravada (ou livre) e devolve a URL.
func (w *Web) Servir(ctx context.Context) (string, error) {
	porta := w.Cfg.Get().PortaUI
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", porta))
	if err != nil {
		if ln, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			return "", err
		}
	}
	p := ln.Addr().(*net.TCPAddr).Port
	if p != porta {
		_ = w.Cfg.Update(func(c *config.Config) { c.PortaUI = p })
	}
	w.URL = fmt.Sprintf("http://127.0.0.1:%d/", p)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(rw, r)
			return
		}
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		rw.Header().Set("Cache-Control", "no-store")
		_, _ = rw.Write(pagina)
	})
	mux.HandleFunc("GET /estado", func(rw http.ResponseWriter, r *http.Request) {
		e := w.Agente.Estado()
		json_(rw, map[string]any{"estado": e, "auto_start": autostart.Ligado(), "api": w.Cfg.Get().API, "so": runtime.GOOS, "log": w.LogPath, "agora": time.Now()})
	})
	mux.HandleFunc("POST /escolher", func(rw http.ResponseWriter, r *http.Request) {
		var b struct{ Frente, Cozinha string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		if err := w.Agente.Escolher(r.Context(), strings.TrimSpace(b.Frente), strings.TrimSpace(b.Cozinha)); err != nil {
			http.Error(rw, err.Error(), 500)
			return
		}
		rw.WriteHeader(204)
	})
	mux.HandleFunc("POST /teste", func(rw http.ResponseWriter, r *http.Request) {
		var b struct{ Alvo string }
		_ = json.NewDecoder(r.Body).Decode(&b)
		if b.Alvo != "cozinha" {
			b.Alvo = "frente"
		}
		if err := w.Agente.TestePapel(r.Context(), b.Alvo); err != nil {
			http.Error(rw, err.Error(), 500)
			return
		}
		rw.WriteHeader(204)
	})
	mux.HandleFunc("POST /desvincular", func(rw http.ResponseWriter, r *http.Request) {
		w.Agente.Desvincular()
		rw.WriteHeader(204)
	})
	mux.HandleFunc("POST /autostart", func(rw http.ResponseWriter, r *http.Request) {
		var b struct{ Ligado bool }
		_ = json.NewDecoder(r.Body).Decode(&b)
		var err error
		if b.Ligado {
			err = autostart.Ligar()
		} else {
			err = autostart.Desligar()
		}
		if err != nil {
			http.Error(rw, err.Error(), 500)
			return
		}
		_ = w.Cfg.Update(func(c *config.Config) { c.AutoStart = b.Ligado })
		rw.WriteHeader(204)
	})
	mux.HandleFunc("GET /log", func(rw http.ResponseWriter, r *http.Request) {
		b, err := os.ReadFile(w.LogPath)
		if err != nil {
			b = nil
		}
		linhas := strings.Split(strings.TrimSpace(string(b)), "\n")
		if len(linhas) > 200 {
			linhas = linhas[len(linhas)-200:]
		}
		rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = rw.Write([]byte(strings.Join(linhas, "\n")))
	})
	mux.HandleFunc("POST /sair", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(204)
		if w.Sair != nil {
			go w.Sair()
		}
	})
	srv := &http.Server{Handler: semCors(mux), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	return w.URL, nil
}

// Só a própria página (mesma origem) fala com o servidor local.
func semCors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && !strings.HasPrefix(o, "http://127.0.0.1:") {
			http.Error(rw, "origem não permitida", http.StatusForbidden)
			return
		}
		h.ServeHTTP(rw, r)
	})
}

func json_(rw http.ResponseWriter, v any) {
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(rw).Encode(v)
}

// AbrirNoNavegador abre a URL no navegador padrão.
func AbrirNoNavegador(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
