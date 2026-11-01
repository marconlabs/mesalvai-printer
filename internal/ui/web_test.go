package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marconlabs/mesalvai-printer/internal/agente"
	"github.com/marconlabs/mesalvai-printer/internal/config"
	"github.com/marconlabs/mesalvai-printer/internal/fila"
)

func servir(t *testing.T, mexer func(*Web)) (*Web, string, *config.Store) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("MESALVAI_PRINTER_DIR", dir)
	cfg, err := config.Abrir()
	if err != nil {
		t.Fatal(err)
	}
	f, _ := fila.Abrir(filepath.Join(dir, "fila"))
	a := agente.New(cfg, f, agente.Opcoes{Versao: "v0.0.0-teste", SO: "teste"})
	w := &Web{Agente: a, Cfg: cfg, LogPath: filepath.Join(dir, "log.txt"), Exe: filepath.Join(dir, "baixado", "mesalvai-printer")}
	if mexer != nil {
		mexer(w)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	url, err := w.Servir(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return w, url, cfg
}

func postar(t *testing.T, url, corpo string) *http.Response {
	t.Helper()
	res, err := http.Post(url, "application/json", strings.NewReader(corpo))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func TestPaginaTemOsBotoesNovos(t *testing.T) {
	for _, quer := range []string{"Gerar outro código", `id="copiar"`, `id="contagem"`, "Guardar o programa em uma pasta fixa?", `id="fechar-caixa"`, "conferida"} {
		if !strings.Contains(string(pagina), quer) {
			t.Errorf("a página não tem %q", quer)
		}
	}
	// O confirm() do navegador saiu: a caixa é da própria página.
	if strings.Contains(string(pagina), "confirm('Fechar") {
		t.Error("Fechar o programa ainda usa o confirm() do navegador")
	}
}

func TestNovoCodigoRecusaQuemJaEstaVinculado(t *testing.T) {
	_, url, cfg := servir(t, nil)
	if res := postar(t, url+"novo-codigo", "{}"); res.StatusCode != 204 {
		t.Fatalf("sem vínculo: %d", res.StatusCode)
	}
	_ = cfg.Update(func(c *config.Config) { c.Chave = "mpk_x" })
	if res := postar(t, url+"novo-codigo", "{}"); res.StatusCode != 409 {
		t.Fatalf("vinculado: %d", res.StatusCode)
	}
}

func TestPastaFixaPerguntaUmaVezSo(t *testing.T) {
	// Dev nunca pergunta.
	_, url, _ := servir(t, func(w *Web) { w.Dev = true })
	if p := estadoPasta(t, url); p["pergunta"] != false {
		t.Fatalf("dev: %v", p)
	}
	// "Agora não" grava e a pergunta some (no Linux de teste também não há
	// pasta fixa, então só conferimos a gravação da resposta).
	_, url, cfg := servir(t, nil)
	if res := postar(t, url+"pasta-fixa", `{"aceitar":false}`); res.StatusCode != 204 {
		t.Fatalf("recusar: %d", res.StatusCode)
	}
	if cfg.Get().PastaFixa != "recusada" {
		t.Fatalf("config: %q", cfg.Get().PastaFixa)
	}
	if p := estadoPasta(t, url); p["pergunta"] != false {
		t.Fatalf("depois de recusar: %v", p)
	}
}

func estadoPasta(t *testing.T, url string) map[string]any {
	t.Helper()
	res, err := http.Get(url + "estado")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var d struct {
		PastaFixa map[string]any `json:"pasta_fixa"`
	}
	if err := json.NewDecoder(res.Body).Decode(&d); err != nil {
		t.Fatal(err)
	}
	return d.PastaFixa
}
