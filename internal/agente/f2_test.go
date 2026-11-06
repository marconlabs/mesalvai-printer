package agente

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/marconlabs/mesalvai-printer/internal/config"
	"github.com/marconlabs/mesalvai-printer/internal/impressora"
)

// "Gerar outro código": cancela a espera do código da tela e pede um novo.
func TestGerarOutroCodigo(t *testing.T) {
	srv := novoServidor(t)
	a, _, _ := montar(t, srv, &impressoraFalsa{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Run(ctx)
	ate(t, func() bool { return a.Estado().Codigo == "ABCDEF" }, "código na tela")
	srv.mu.Lock()
	antes := srv.starts
	srv.mu.Unlock()
	if err := a.NovoCodigo(); err != nil {
		t.Fatal(err)
	}
	ate(t, func() bool { srv.mu.Lock(); defer srv.mu.Unlock(); return srv.starts > antes }, "pediu outro código à API")
	ate(t, func() bool { return a.Estado().Codigo == "ABCDEF" }, "código novo na tela")
	if a.Estado().Vinculado || a.Estado().UltimoErro != "" {
		t.Fatalf("estado: %+v", a.Estado())
	}
	// A loja digita o código novo: vincula normalmente.
	srv.mu.Lock()
	srv.pareado = true
	srv.mu.Unlock()
	ate(t, func() bool { return a.Estado().Vinculado }, "vinculado depois do código novo")
	if err := a.NovoCodigo(); err == nil {
		t.Fatal("vinculado não gera outro código")
	}
}

// Evento `procurar` do painel: faz a varredura e manda o resultado no status.
func TestEventoProcurarDoPainel(t *testing.T) {
	srv := novoServidor(t)
	var varreduras atomic.Int32
	a, cfg, _ := montar(t, srv, &impressoraFalsa{})
	a.op.Procurar = func(context.Context) []impressora.Info {
		varreduras.Add(1)
		return []impressora.Info{{Nome: "tcp://192.168.0.50:9100", Rotulo: "Rede 192.168.0.50"}}
	}
	srv.pareado = true
	_ = cfg.Update(func(c *config.Config) { c.Chave, c.AgenteID = "mpk_teste", "a1" })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Run(ctx)
	ate(t, func() bool { return a.Estado().Conectado }, "conectado")

	// Para outro computador: ignora.
	srv.eventos <- "event: procurar\ndata: {\"agente_id\":\"outro\"}\n\n"
	time.Sleep(250 * time.Millisecond)
	if varreduras.Load() != 0 {
		t.Fatal("pedido de outro computador não podia varrer a rede")
	}
	// Para este (por setor) e sem corpo nenhum: varre.
	srv.eventos <- "event: procurar\ndata: {\"agente_id\":\"a1\",\"setor\":\"bar1\"}\n\n"
	ate(t, func() bool { return varreduras.Load() == 1 }, "varreu a pedido do painel")
	ate(t, func() bool { return !a.Estado().Procurando }, "terminou a procura")
	srv.eventos <- "event: procurar\ndata: \n\n"
	ate(t, func() bool { return varreduras.Load() == 2 }, "varreu sem corpo")
	ate(t, func() bool {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		for _, st := range srv.status {
			for _, p := range toStrs(st["printers"]) {
				if p == "tcp://192.168.0.50:9100" {
					return true
				}
			}
		}
		return false
	}, "achada no status")
}

// Largura que o driver informa vai no status, por setor (0 = não diz, não vai no JSON).
func TestLarguraDetectadaVaiNoStatusPorSetor(t *testing.T) {
	srv := novoServidor(t)
	var leituras atomic.Int32
	a, cfg, _ := montar(t, srv, &impressoraFalsa{})
	a.op.Largura = func(_ context.Context, nome string) int {
		leituras.Add(1)
		if nome == "EPSON" {
			return 58
		}
		return 0
	}
	srv.pareado = true
	_ = cfg.Update(func(c *config.Config) {
		c.Chave = "mpk_teste"
		c.Setores = map[string]config.SetorLocal{"frente": {Impressora: "EPSON"}, "cozinha": {Impressora: "COZ"}, "bar1": {Impressora: "tcp://192.168.0.50:9100"}}
		c.SetoresLista = []config.SetorInfo{{Key: "frente", Nome: "Caixa/Frente", Tipo: "frente"}, {Key: "cozinha", Nome: "Cozinha", Tipo: "cozinha"}, {Key: "bar1", Nome: "Bar", Tipo: "praca"}}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Run(ctx)
	ate(t, func() bool { return a.Estado().Saude["frente"].LarguraDetectada == 58 }, "frente 58 mm")
	if got := a.Estado().Saude["cozinha"].LarguraDetectada; got != 0 {
		t.Fatalf("cozinha: o driver não disse, tem que ficar 0 (veio %d)", got)
	}
	ate(t, func() bool {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		for _, st := range srv.status {
			if m, ok := st["saude"].(map[string]any); ok {
				if f, ok := m["frente"].(map[string]any); ok && f["largura_detectada"] == float64(58) {
					if c, ok := m["cozinha"].(map[string]any); ok {
						if _, tem := c["largura_detectada"]; tem {
							return false // 0 não vai no JSON
						}
					}
					return true
				}
			}
		}
		return false
	}, "status com saude.frente.largura_detectada")
	ate(t, func() bool {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		for _, st := range srv.status {
			if m, ok := st["largura_detectada"].(map[string]any); ok {
				if f, ok := m["frente"].(map[string]any); ok && f["mm"] == float64(58) && f["fonte"] == "driver" && f["impressora"] == "EPSON" {
					_, tem := m["cozinha"]
					return !tem
				}
			}
		}
		return false
	}, "status com largura_detectada.frente no topo (formato da API)")
	// Rede nunca lê driver; e não relê a cada 40 ms (cache).
	time.Sleep(300 * time.Millisecond)
	if n := leituras.Load(); n > 2 {
		t.Fatalf("leu o driver %d vezes; devia ler 1 vez por impressora (EPSON, COZ)", n)
	}
}

func TestRegistrarConferenciaVaiParaAJanela(t *testing.T) {
	srv := novoServidor(t)
	a, _, _ := montar(t, srv, &impressoraFalsa{})
	agora := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	a.agora = func() time.Time { return agora }
	a.AtualizacaoLigada(true)
	a.RegistrarConferencia(Atualizacao{Resultado: "erro", Erro: "sem resposta do GitHub"})
	u := a.Estado().Atualizacao
	if !u.Ligada || u.Resultado != "erro" || u.Erro == "" || !u.Quando.Equal(agora) {
		t.Fatalf("atualização: %+v", u)
	}
	// Conferência seguinte mantém o "ligada".
	a.RegistrarConferencia(Atualizacao{Resultado: "atual", Tag: "v2.1.0"})
	if u := a.Estado().Atualizacao; !u.Ligada || u.Resultado != "atual" || u.Erro != "" {
		t.Fatalf("depois: %+v", u)
	}
}
