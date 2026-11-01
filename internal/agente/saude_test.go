package agente

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/marconlabs/mesalvai-printer/internal/api"
	"github.com/marconlabs/mesalvai-printer/internal/config"
	"github.com/marconlabs/mesalvai-printer/internal/impressora"
)

func semPapel() impressora.Leitura {
	return impressora.Leitura{Situacao: impressora.SemPapel, Lido: true, Detalhe: "sem papel"}
}

func fora() impressora.Leitura {
	return impressora.Leitura{Situacao: impressora.SemResposta, Detalhe: "não responde na rede"}
}

func saudeDe(a *Agente, key string) api.SaudeSetor { return a.Estado().Saude[key] }

// P2: antes de imprimir a impressora diz "sem papel": o papel nem é mandado
// e sai na reserva, com o motivo certo no ack.
func TestSemPapelNaoMandaEVaiParaAReserva(t *testing.T) {
	srv := novoServidor(t)
	imp := &impressoraFalsa{}
	imp.estado("EPSON", semPapel())
	_, _ = rodar(t, srv, imp, func(c *config.Config) {
		c.Setores = map[string]config.SetorLocal{"frente": {Impressora: "EPSON", Reserva: "COZ"}}
	})
	srv.job("j1", "frente")
	ate(t, func() bool { return srv.temAck("j1:impresso") }, "ack pela reserva")
	if n := len(imp.saidaEm("EPSON")); n != 0 {
		t.Fatalf("a principal sem papel não podia receber bytes: %d", n)
	}
	if len(imp.saidaEm("COZ")) != 1 {
		t.Fatalf("reserva: %q", imp.todas())
	}
	ack := srv.ack("j1")
	if ack["via"] != "reserva" || ack["error"] != "sem papel" {
		t.Fatalf("ack: %v", ack)
	}
}

// P2: o status que vai pra API traz a saúde de cada setor com impressora.
func TestStatusTrazSaudePorSetor(t *testing.T) {
	srv := novoServidor(t)
	imp := &impressoraFalsa{}
	imp.estado("EPSON", impressora.Leitura{Situacao: impressora.TampaAberta, Lido: true, Detalhe: "tampa aberta"})
	a, _ := rodar(t, srv, imp, func(c *config.Config) {
		c.Setores = map[string]config.SetorLocal{"frente": {Impressora: "EPSON"}}
	})
	ate(t, func() bool { return saudeDe(a, "frente").Estado == impressora.TampaAberta }, "saúde da frente")
	if s := saudeDe(a, "frente"); s.Imprime || s.Impressora != "EPSON" || !s.Lido {
		t.Fatalf("saúde: %+v", s)
	}
	ate(t, func() bool {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		for _, st := range srv.status {
			if m, ok := st["saude"].(map[string]any); ok {
				if f, ok := m["frente"].(map[string]any); ok && f["estado"] == "tampa_aberta" && f["imprime"] == false {
					return true
				}
			}
		}
		return false
	}, "status com saude.frente")
	srv.mu.Lock()
	caps := toStrs(srv.status[0]["caps"])
	srv.mu.Unlock()
	if len(caps) < 5 || caps[3] != "saude" || caps[4] != "cancel" {
		t.Fatalf("caps: %v", caps)
	}
}

// Falha de rede avulsa não acende a faixa: precisa de 2 leituras seguidas.
func TestSemRespostaPrecisaDeDuasLeituras(t *testing.T) {
	srv := novoServidor(t)
	srv.pareado = true
	imp := &impressoraFalsa{}
	a, cfg, _ := montar(t, srv, imp)
	_ = cfg.Update(func(c *config.Config) {
		c.Chave = "mpk_teste"
		c.Setores = map[string]config.SetorLocal{"frente": {Impressora: "tcp://192.168.0.50:9100"}}
	})
	ctx := context.Background()
	imp.estado("tcp://192.168.0.50:9100", fora())
	a.verificarSaude(ctx)
	if s := saudeDe(a, "frente"); s.Estado != impressora.Pronta || !s.Imprime {
		t.Fatalf("1ª falha não vale: %+v", s)
	}
	a.verificarSaude(ctx)
	s := saudeDe(a, "frente")
	if s.Estado != impressora.SemResposta || s.Imprime || !s.IPMudou {
		t.Fatalf("2ª falha vale e pergunta se o IP mudou (P3): %+v", s)
	}
	// Voltou.
	imp.estado("tcp://192.168.0.50:9100", impressora.Leitura{Situacao: impressora.Pronta})
	a.verificarSaude(ctx)
	if s := saudeDe(a, "frente"); s.Estado != impressora.Pronta || !s.Imprime || s.IPMudou {
		t.Fatalf("voltou: %+v", s)
	}
}

// P4: principal e reserva fora por 2 min = degradado (o navegador assume).
func TestDegradaDepoisDeDoisMinutos(t *testing.T) {
	srv := novoServidor(t)
	srv.pareado = true
	imp := &impressoraFalsa{}
	a, cfg, _ := montar(t, srv, imp)
	_ = cfg.Update(func(c *config.Config) {
		c.Chave = "mpk_teste"
		c.Setores = map[string]config.SetorLocal{"cozinha": {Impressora: "COZ", Reserva: "EPSON"}}
	})
	var mu sync.Mutex
	relogio := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	a.agora = func() time.Time { mu.Lock(); defer mu.Unlock(); return relogio }
	avancar := func(d time.Duration) { mu.Lock(); relogio = relogio.Add(d); mu.Unlock() }
	ctx := context.Background()

	// Só a principal fora: a reserva segura, o setor segue imprimindo, sem degradar.
	imp.estado("COZ", semPapel())
	a.verificarSaude(ctx)
	avancar(5 * time.Minute)
	a.verificarSaude(ctx)
	if s := saudeDe(a, "cozinha"); !s.Imprime || !s.NaReserva || s.Degradado {
		t.Fatalf("na reserva: %+v", s)
	}
	// Reserva também fora.
	imp.estado("EPSON", fora())
	a.verificarSaude(ctx)
	a.verificarSaude(ctx)
	if s := saudeDe(a, "cozinha"); s.Imprime || s.Degradado || s.ForaHaS != 0 {
		t.Fatalf("recém caiu: %+v", s)
	}
	avancar(90 * time.Second)
	a.verificarSaude(ctx)
	if s := saudeDe(a, "cozinha"); s.Degradado || s.ForaHaS != 90 {
		t.Fatalf("90 s ainda não: %+v", s)
	}
	avancar(31 * time.Second)
	a.verificarSaude(ctx)
	if s := saudeDe(a, "cozinha"); !s.Degradado || s.ForaHaS != 121 {
		t.Fatalf("passou de 2 min: %+v", s)
	}
	// Voltou: zera.
	imp.estado("EPSON", impressora.Leitura{Situacao: impressora.Pronta, Lido: true})
	a.verificarSaude(ctx)
	if s := saudeDe(a, "cozinha"); !s.Imprime || s.Degradado || s.ForaHaS != 0 {
		t.Fatalf("voltou: %+v", s)
	}
}

// P4: evento cancel tira da fila o que o navegador assumiu, sem ack.
func TestCancelTiraDaFilaSemAck(t *testing.T) {
	srv := novoServidor(t)
	imp := &impressoraFalsa{caida: true}
	a, _ := rodar(t, srv, imp, func(c *config.Config) {
		c.Setores = map[string]config.SetorLocal{"frente": {Impressora: "EPSON"}}
	})
	srv.job("j1", "frente")
	srv.job("j2", "frente")
	ate(t, func() bool { return a.Estado().NaFila == 2 }, "2 na fila (impressora fora)")
	srv.eventos <- "event: cancel\ndata: {\"job_ids\":[\"j1\"]}\n\n"
	ate(t, func() bool { return a.Estado().NaFila == 1 }, "j1 saiu da fila")
	imp.mu.Lock()
	imp.caida = false
	imp.mu.Unlock()
	ate(t, func() bool { return srv.temAck("j2:impresso") }, "j2 sai quando a impressora volta")
	if srv.temAck("j1:impresso") || srv.temAck("j1:falhou") {
		t.Fatal("j1 foi cancelado: nada de ack")
	}
	if len(imp.saidaEm("EPSON")) != 1 {
		t.Fatalf("só o j2 sai: %q", imp.todas())
	}
}

// P5: o teste que saiu fica gravado e sobe no status (falta o teste = vazio).
func TestTesteGravaQuandoSai(t *testing.T) {
	srv := novoServidor(t)
	imp := &impressoraFalsa{}
	a, cfg := rodar(t, srv, imp, func(c *config.Config) {
		c.Setores = map[string]config.SetorLocal{"frente": {Impressora: "EPSON"}}
	})
	ate(t, func() bool { return saudeDe(a, "frente").Impressora == "EPSON" }, "saúde")
	if saudeDe(a, "frente").TesteEm != "" {
		t.Fatal("sem teste ainda")
	}
	if err := a.TestePapel(context.Background(), "frente"); err != nil {
		t.Fatal(err)
	}
	if cfg.Get().Testes["frente"] == "" || saudeDe(a, "frente").TesteEm == "" {
		t.Fatalf("teste: %v / %+v", cfg.Get().Testes, saudeDe(a, "frente"))
	}
	// Teste que falha não conta.
	imp.mu.Lock()
	imp.caida = true
	imp.mu.Unlock()
	_ = cfg.Update(func(c *config.Config) { delete(c.Testes, "frente") })
	if err := a.TestePapel(context.Background(), "frente"); err == nil {
		t.Fatal("devia falhar")
	}
	if cfg.Get().Testes["frente"] != "" {
		t.Fatal("teste que falhou não pode contar")
	}
}

// P1: o main avisa o agente; vai pro status.
func TestSemSonoVaiNoStatus(t *testing.T) {
	srv := novoServidor(t)
	a, _ := rodar(t, srv, &impressoraFalsa{}, func(c *config.Config) {
		c.Setores = map[string]config.SetorLocal{"frente": {Impressora: "EPSON"}}
	})
	a.SemSono(true, nil)
	ate(t, func() bool { return a.Estado().SemSono }, "sem sono")
	_ = a.enviarStatus(context.Background(), nil, nil)
	srv.mu.Lock()
	defer srv.mu.Unlock()
	ult := srv.status[len(srv.status)-1]
	if v, ok := ult["sem_sono"].(bool); !ok || !v {
		t.Fatalf("status: %v", ult["sem_sono"])
	}
}
