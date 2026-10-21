package agente

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"
	"testing"

	"github.com/marconlabs/mesalvai-printer/internal/config"
	"github.com/marconlabs/mesalvai-printer/internal/fila"
	"github.com/marconlabs/mesalvai-printer/internal/impressora"
)

const configV2 = `{"config":{},"front_printer":"EPSON","kitchen_printer":null,
 "setores":[{"key":"frente","nome":"Caixa/Frente","tipo":"frente"},{"key":"cozinha","nome":"Cozinha","tipo":"cozinha"},{"key":"bar1","nome":"Bar","tipo":"praca"},{"key":"chapa1","nome":"Chapa","tipo":"praca"}],
 "bindings":{"frente":{"impressora":"EPSON","reserva":null},"cozinha":{"impressora":null,"reserva":null},"bar1":{"impressora":"BARIMP","reserva":null}}}`

// jobV2 manda um papel com os campos da v2.
func (s *servidorFalso) jobV2(id, target, setor string) {
	s.jobs <- map[string]any{"id": id, "origin": "salao", "ref_id": "r-" + id, "target": target, "sector_name": setor, "papel": "preparo", "origem": "mesa",
		"title": "Mesa 4", "copies": 1, "paper": 80, "payload_b64": base64.StdEncoding.EncodeToString([]byte("\x1b@PEDIDO 1\n")), "created_at": "2026-10-21T12:00:00Z", "attempts": 0}
}

func (s *servidorFalso) ack(id string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ackCorpo[id]
}

func (i *impressoraFalsa) todas() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]string(nil), i.saidas...)
}

func (i *impressoraFalsa) saidaEm(prefixo string) []string {
	var out []string
	for _, s := range i.todas() {
		if strings.HasPrefix(s, prefixo+":") {
			out = append(out, s)
		}
	}
	return out
}

func rodar(t *testing.T, srv *servidorFalso, imp *impressoraFalsa, prep func(*config.Config)) (*Agente, *config.Store) {
	t.Helper()
	srv.pareado = true
	a, cfg, _ := montar(t, srv, imp)
	_ = cfg.Update(func(c *config.Config) {
		c.Chave = "mpk_teste"
		if prep != nil {
			prep(c)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.Run(ctx)
	ate(t, func() bool { return a.Estado().Conectado }, "conectado")
	return a, cfg
}

func TestRoteamentoPorSetorEFallbackComCabecalho(t *testing.T) {
	srv := novoServidor(t)
	srv.config = configV2
	imp := &impressoraFalsa{}
	a, cfg := rodar(t, srv, imp, nil)
	// Depois do hello busca o config: setores e vínculos chegam.
	ate(t, func() bool { return cfg.Get().Setor("bar1").Impressora == "BARIMP" }, "vínculo do bar")
	if cfg.Get().Frente != "EPSON" || len(a.Estado().Setores) != 4 || a.Estado().Setores[2].Nome != "Bar" {
		t.Fatalf("config: frente %q setores %+v", cfg.Get().Frente, a.Estado().Setores)
	}
	// Status com caps desde o primeiro.
	srv.mu.Lock()
	caps := srv.status[0]["caps"]
	srv.mu.Unlock()
	if !strings.Contains(strings.Join(toStrs(caps), ","), "setores,reserva,procurar") {
		t.Fatalf("caps: %v", caps)
	}

	srv.jobV2("j1", "bar1", "Bar")        // setor com impressora própria
	srv.jobV2("j2", "chapa1", "Chapa")    // setor sem impressora: frente com nome no topo
	srv.jobV2("j3", "cozinha", "Cozinha") // cozinha sem impressora: frente, como a v1
	ate(t, func() bool {
		return srv.temAck("j1:impresso") && srv.temAck("j2:impresso") && srv.temAck("j3:impresso")
	}, "3 acks")

	bar := imp.saidaEm("BARIMP")
	if len(bar) != 1 || bar[0] != "BARIMP:PEDIDO 1" {
		t.Fatalf("bar: %q", bar)
	}
	frente := imp.saidaEm("EPSON")
	if len(frente) != 2 {
		t.Fatalf("frente: %q", frente)
	}
	if !strings.Contains(frente[0], "CHAPA\n") || !strings.Contains(frente[0], "PEDIDO 1") || strings.Contains(frente[0], "RESERVA") {
		t.Fatalf("chapa na frente sem cabeçalho: %q", frente[0])
	}
	if frente[1] != "EPSON:PEDIDO 1" {
		t.Fatalf("cozinha na frente não leva cabeçalho: %q", frente[1])
	}
	if v := srv.ack("j1")["via"]; v != "principal" {
		t.Fatalf("via: %v", v)
	}
	if p := srv.ack("j1")["printer"]; p != "BARIMP" {
		t.Fatalf("printer: %v", p)
	}
}

func toStrs(v any) []string {
	l, _ := v.([]any)
	out := make([]string, 0, len(l))
	for _, x := range l {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}

func TestReservaPorSetor(t *testing.T) {
	srv := novoServidor(t)
	srv.config = `{"config":{},"setores":[{"key":"frente","nome":"Caixa/Frente","tipo":"frente"},{"key":"cozinha","nome":"Cozinha","tipo":"cozinha"}],
	 "bindings":{"frente":{"impressora":"EPSON","reserva":null},"cozinha":{"impressora":"COZ","reserva":"EPSON"}}}`
	imp := &impressoraFalsa{caidas: map[string]bool{"COZ": true}}
	var avisos []string
	var mu sync.Mutex
	srv.pareado = true
	a, cfg, _ := montar(t, srv, imp)
	a.op.Avisar = func(titulo, texto string) { mu.Lock(); avisos = append(avisos, titulo+"|"+texto); mu.Unlock() }
	_ = cfg.Update(func(c *config.Config) { c.Chave = "mpk_teste" })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Run(ctx)
	ate(t, func() bool { return cfg.Get().Setor("cozinha").Reserva == "EPSON" }, "reserva da cozinha")

	srv.jobV2("j1", "cozinha", "Cozinha")
	srv.jobV2("j2", "cozinha", "Cozinha")
	ate(t, func() bool { return srv.temAck("j1:impresso") && srv.temAck("j2:impresso") }, "acks pela reserva")
	saidas := imp.saidaEm("EPSON")
	if len(saidas) != 2 {
		t.Fatalf("reserva: %q", saidas)
	}
	for _, s := range saidas {
		if !strings.Contains(s, "COZINHA\n") || !strings.Contains(s, "SAIU NA RESERVA\n") || !strings.Contains(s, "sem papel") || !strings.Contains(s, "------\n") || !strings.HasSuffix(s, "PEDIDO 1") {
			t.Fatalf("cabeçalho da reserva: %q", s)
		}
	}
	// Cabeçalho ESC/POS: ESC @, centraliza, GS ! 0x11.
	if !strings.HasPrefix(saidas[0], "EPSON:\x1ba\x01\x1d!\x11COZINHA") {
		t.Fatalf("bytes do cabeçalho: %q", saidas[0])
	}
	ack := srv.ack("j1")
	if ack["status"] != "impresso" || ack["via"] != "reserva" || ack["printer"] != "EPSON" || !strings.Contains(ack["error"].(string), "sem papel") {
		t.Fatalf("ack: %v", ack)
	}
	if a.Estado().NaFila != 0 {
		t.Fatal("fila devia esvaziar")
	}
	mu.Lock()
	defer mu.Unlock()
	n := 0
	for _, x := range avisos {
		if strings.Contains(x, "reserva") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("um balão de reserva por queda, veio: %q", avisos)
	}
}

func TestImpressoraForaNaoTravaOutra(t *testing.T) {
	srv := novoServidor(t)
	imp := &impressoraFalsa{caidas: map[string]bool{"EPSON": true}}
	a, _ := rodar(t, srv, imp, func(c *config.Config) { c.Frente, c.Cozinha = "EPSON", "COZ" })
	srv.job("j1", "frente")
	srv.job("j2", "cozinha")
	srv.job("j3", "frente")
	ate(t, func() bool { return srv.temAck("j2:impresso") }, "cozinha sai com a frente fora")
	ate(t, func() bool { return a.Estado().NaFila == 2 }, "2 da frente esperando")
	// Frente volta: sai na ordem.
	imp.mu.Lock()
	imp.caidas = nil
	imp.mu.Unlock()
	a.Acordar()
	ate(t, func() bool { return srv.temAck("j1:impresso") && srv.temAck("j3:impresso") }, "frente voltou")
	if got := strings.Join(imp.todas(), ","); got != "COZ:PEDIDO 1,EPSON:PEDIDO 1,EPSON:PEDIDO 1" {
		t.Fatalf("ordem: %q", got)
	}
}

func TestEventoConfigRebusca(t *testing.T) {
	srv := novoServidor(t)
	srv.config = configV2
	imp := &impressoraFalsa{}
	a, cfg := rodar(t, srv, imp, nil)
	ate(t, func() bool { srv.mu.Lock(); defer srv.mu.Unlock(); return srv.configs >= 1 }, "busca no hello")
	ate(t, func() bool { return cfg.Get().Setor("bar1").Impressora == "BARIMP" }, "bar")
	// O painel trocou o bar de impressora e criou a Chapa com reserva.
	srv.mu.Lock()
	srv.config = strings.Replace(configV2, `"bar1":{"impressora":"BARIMP","reserva":null}`, `"bar1":{"impressora":"tcp://192.168.0.50:9100","reserva":null},"chapa1":{"impressora":"COZ","reserva":"EPSON"}`, 1)
	srv.mu.Unlock()
	srv.eventos <- "event: config\ndata: {}\n\n"
	ate(t, func() bool { return cfg.Get().Setor("bar1").Impressora == "tcp://192.168.0.50:9100" }, "rebuscou o config")
	if r := cfg.Get().Setor("chapa1"); r.Impressora != "COZ" || r.Reserva != "EPSON" {
		t.Fatalf("chapa: %+v", r)
	}
	ate(t, func() bool {
		for _, s := range a.Estado().Setores {
			if s.Key == "chapa1" && s.Reserva == "EPSON" {
				return true
			}
		}
		return false
	}, "janela mostra o vínculo novo")
}

func TestVincularSetorEProcurar(t *testing.T) {
	srv := novoServidor(t)
	imp := &impressoraFalsa{}
	a, cfg := rodar(t, srv, imp, nil)
	a.op.Procurar = func(context.Context) []impressora.Info {
		return []impressora.Info{{Nome: "tcp://192.168.0.50:9100", Rotulo: "Rede 192.168.0.50"}}
	}
	achou, err := a.Procurar(context.Background())
	if err != nil || len(achou) != 1 || a.Estado().Procurando {
		t.Fatalf("procurar: %v %v", achou, err)
	}
	ultimo := func() map[string]any {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		return srv.status[len(srv.status)-1]
	}
	if p := strings.Join(toStrs(ultimo()["printers"]), ","); p != "EPSON,COZ,tcp://192.168.0.50:9100" {
		t.Fatalf("printers: %q", p)
	}
	if err := a.VincularSetor(context.Background(), "bar1", "tcp://192.168.0.50:9100", "EPSON"); err != nil {
		t.Fatal(err)
	}
	st := ultimo()
	set, _ := st["setores"].(map[string]any)
	bar, _ := set["bar1"].(map[string]any)
	if bar["impressora"] != "tcp://192.168.0.50:9100" || bar["reserva"] != "EPSON" || len(set) != 1 {
		t.Fatalf("setores no status: %v", st)
	}
	// Frente pela linha de setores = campo antigo também (v1 lê front_printer).
	_ = a.VincularSetor(context.Background(), "frente", "EPSON", "")
	if ultimo()["front_printer"] != "EPSON" || cfg.Get().Frente != "EPSON" {
		t.Fatalf("espelho da frente: %v / %q", ultimo(), cfg.Get().Frente)
	}
	// Tirar o vínculo manda null.
	_ = a.VincularSetor(context.Background(), "bar1", "", "")
	set, _ = ultimo()["setores"].(map[string]any)
	if v, ok := set["bar1"]; !ok || v != nil {
		t.Fatalf("apagar devia mandar null: %v", set)
	}
	if _, ok := cfg.Get().Setores["bar1"]; ok {
		t.Fatal("vínculo local devia sumir")
	}
}

func TestTesteDoSetorComNomeGrande(t *testing.T) {
	srv := novoServidor(t)
	imp := &impressoraFalsa{}
	a, _ := rodar(t, srv, imp, func(c *config.Config) {
		c.Frente = "EPSON"
		c.Setores["cozinha"] = config.SetorLocal{Impressora: "COZ"}
		c.SetoresLista = []config.SetorInfo{{Key: "cozinha", Nome: "Cozinha Quente"}}
	})
	if err := a.TestePapel(context.Background(), "cozinha"); err != nil {
		t.Fatal(err)
	}
	s := imp.todas()
	if len(s) != 1 || !strings.HasPrefix(s[0], "COZ:") || !strings.Contains(s[0], "TESTE\nCOZINHA QUENTE\n") {
		t.Fatalf("teste: %q", s)
	}
}

// As impressoras achadas em "Procurar" ficam no config: reiniciou o programa,
// continuam na lista (e no status) mesmo sem setor vinculado.
func TestProcurarLembraDepoisDeReiniciar(t *testing.T) {
	srv := novoServidor(t)
	imp := &impressoraFalsa{}
	a, cfg := rodar(t, srv, imp, nil)
	a.op.Procurar = func(context.Context) []impressora.Info {
		return []impressora.Info{{Nome: "tcp://192.168.0.50:9100", Rotulo: "Rede 192.168.0.50"}}
	}
	if _, err := a.Procurar(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Segunda busca acha outra; a primeira (desligada agora) continua lembrada.
	a.op.Procurar = func(context.Context) []impressora.Info {
		return []impressora.Info{{Nome: "tcp://192.168.0.51:9100", Rotulo: "Rede 192.168.0.51"}}
	}
	_, _ = a.Procurar(context.Background())
	if r := cfg.Get().Rede; len(r) != 2 || r[0].Nome != "tcp://192.168.0.51:9100" || r[1].Rotulo != "Rede 192.168.0.50" {
		t.Fatalf("rede no config: %+v", r)
	}
	// "Reinicia": um agente novo com o mesmo config já lista as duas.
	f, _ := fila.Abrir(t.TempDir())
	b := New(cfg, f, Opcoes{Listar: func() ([]impressora.Info, error) { return []impressora.Info{{Nome: "EPSON"}}, nil }})
	nomes := []string{}
	for _, i := range b.impressoras() {
		nomes = append(nomes, i.Nome)
	}
	if strings.Join(nomes, ",") != "EPSON,tcp://192.168.0.51:9100,tcp://192.168.0.50:9100" {
		t.Fatalf("lista depois de reiniciar: %v", nomes)
	}
}
