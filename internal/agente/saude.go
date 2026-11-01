package agente

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/marconlabs/mesalvai-printer/internal/api"
	"github.com/marconlabs/mesalvai-printer/internal/config"
	"github.com/marconlabs/mesalvai-printer/internal/fila"
	"github.com/marconlabs/mesalvai-printer/internal/impressora"
)

// LimiteDegradar: setor que não consegue imprimir (principal e reserva fora)
// por tanto tempo deixa de ser "do computador": o navegador volta a imprimir
// e a API tira da fila daqui o que o navegador assumiu (P4).
const LimiteDegradar = 2 * time.Minute

// leitura é o que o programa sabe de uma impressora (por nome).
type leitura struct {
	// Estavel: a última leitura confiável. Uma falha de rede avulsa não vira
	// "sem resposta" (precisa de 2 seguidas, ~1 min) pra faixa não piscar.
	Estavel impressora.Leitura
	Falhas  int
	// Falhou: motivo da última impressão que não saiu (vazio = tudo certo).
	// Vale mesmo que a leitura diga "pronta" (o spooler às vezes mente).
	Falhou string
}

func ehRede(nome string) bool { return strings.HasPrefix(strings.ToLower(nome), "tcp://") }

// guardar aplica uma leitura nova.
func (a *Agente) guardar(nome string, l impressora.Leitura) {
	if l.Ocupada {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	r := a.leituras[nome]
	if r == nil {
		r = &leitura{}
		a.leituras[nome] = r
	}
	if l.Situacao == impressora.SemResposta {
		r.Falhas++
		if r.Falhas >= 2 {
			r.Estavel = l
		}
		return
	}
	r.Falhas = 0
	r.Estavel = l
	// Leitura boa de verdade (ou conexão de rede aberta) limpa a falha de impressão.
	if l.Situacao == impressora.Pronta && (l.Lido || ehRede(nome)) {
		r.Falhou = ""
	}
}

// estadoDe: o que o painel deve achar desta impressora agora.
func (a *Agente) estadoDe(nome string) impressora.Leitura {
	a.mu.Lock()
	defer a.mu.Unlock()
	r := a.leituras[nome]
	if r == nil {
		return impressora.Leitura{Situacao: impressora.Pronta}
	}
	if r.Falhou != "" && r.Estavel.Situacao != impressora.SemPapel && r.Estavel.Situacao != impressora.TampaAberta {
		return impressora.Leitura{Situacao: impressora.SemResposta, Lido: true, Detalhe: r.Falhou}
	}
	if r.Estavel.Situacao == "" {
		return impressora.Leitura{Situacao: impressora.Pronta}
	}
	return r.Estavel
}

func (a *Agente) marcarFalha(nome string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	r := a.leituras[nome]
	if r == nil {
		r = &leitura{}
		a.leituras[nome] = r
	}
	r.Falhou = err.Error()
	if len(r.Falhou) > 160 {
		r.Falhou = r.Falhou[:160]
	}
}

func (a *Agente) marcarOk(nome string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r := a.leituras[nome]; r != nil {
		r.Falhou, r.Falhas = "", 0
		if r.Estavel.Situacao == impressora.SemResposta {
			r.Estavel = impressora.Leitura{Situacao: impressora.Pronta, Lido: false}
		}
	}
}

// lerAgora pergunta à impressora e guarda a resposta.
func (a *Agente) lerAgora(ctx context.Context, nome string) impressora.Leitura {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	l := a.op.Ler(ctx, nome)
	a.guardar(nome, l)
	return l
}

// tentar imprime numa impressora: antes pergunta o estado e, se ela diz sem
// papel ou tampa aberta, nem manda (o papel iria pro buffer e nunca sairia);
// o erro "sem papel" é o que a fila, a reserva e o painel mostram.
func (a *Agente) tentar(ctx context.Context, nome string, dados []byte, copias int) error {
	if l := a.lerAgora(ctx, nome); l.Lido && (l.Situacao == impressora.SemPapel || l.Situacao == impressora.TampaAberta) {
		return l.Erro()
	}
	if err := a.imprimirVias(ctx, nome, dados, copias); err != nil {
		a.marcarFalha(nome, err)
		return err
	}
	a.marcarOk(nome)
	return nil
}

// laçoDaSaude lê o estado das impressoras dos setores a cada SaudeCada.
func (a *Agente) laçoDaSaude(ctx context.Context) {
	for {
		a.verificarSaude(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(a.op.SaudeCada):
		}
	}
}

// impressorasDosSetores: nomes únicos de principal e reserva de cada setor.
func (a *Agente) impressorasDosSetores() []string {
	c := a.cfg.Get()
	visto := map[string]bool{}
	var out []string
	for _, s := range c.Lista() {
		p, r, _ := destino(c, fila.Item{Target: s.Key})
		for _, n := range []string{p, r} {
			if n != "" && !visto[n] {
				visto[n] = true
				out = append(out, n)
			}
		}
	}
	return out
}

func (a *Agente) verificarSaude(ctx context.Context) {
	if a.cfg.Get().Chave == "" {
		return
	}
	var wg sync.WaitGroup
	for _, n := range a.impressorasDosSetores() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.lerAgora(ctx, n)
		}()
	}
	wg.Wait()
	mudou, fora := a.recalcularSaude()
	// Mudou, ou há setor fora (a API precisa do "há quanto tempo" andando):
	// manda o estado. Sem mudança e tudo bem = silêncio.
	if mudou || fora {
		_ = a.enviarStatus(ctx, nil, nil)
	}
}

// recalcularSaude refaz Estado.Saude. Devolve se algo mudou (fora o contador
// de segundos) e se algum setor está sem conseguir imprimir.
func (a *Agente) recalcularSaude() (mudou, fora bool) {
	c := a.cfg.Get()
	agora := a.agora()
	novo := map[string]api.SaudeSetor{}
	a.mu.Lock()
	if a.foraDesde == nil {
		a.foraDesde = map[string]time.Time{}
	}
	a.mu.Unlock()
	for _, s := range c.Lista() {
		principal, reserva, _ := destino(c, fila.Item{Target: s.Key})
		if principal == "" {
			a.mu.Lock()
			delete(a.foraDesde, s.Key)
			a.mu.Unlock()
			continue
		}
		p := a.estadoDe(principal)
		temReserva := reserva != "" && reserva != principal
		rOK := temReserva && a.estadoDe(reserva).Situacao == impressora.Pronta
		pOK := p.Situacao == impressora.Pronta
		sh := api.SaudeSetor{Estado: p.Situacao, Lido: p.Lido, Detalhe: p.Detalhe, Impressora: principal,
			Imprime: pOK || rOK, NaReserva: !pOK && rOK, IPMudou: ehRede(principal) && p.Situacao == impressora.SemResposta}
		// Setor sem impressora própria imprime na da frente: o teste é o dela.
		teste := c.Testes[s.Key]
		if c.Setor(s.Key).Impressora == "" {
			teste = c.Testes["frente"]
		}
		sh.TesteEm = teste
		a.mu.Lock()
		if sh.Imprime {
			delete(a.foraDesde, s.Key)
		} else {
			if _, ok := a.foraDesde[s.Key]; !ok {
				a.foraDesde[s.Key] = agora
			}
			sh.ForaHaS = int(agora.Sub(a.foraDesde[s.Key]).Seconds())
			sh.Degradado = agora.Sub(a.foraDesde[s.Key]) >= LimiteDegradar
			fora = true
		}
		a.mu.Unlock()
		novo[s.Key] = sh
	}
	mudou = assinaturaSaude(novo) != assinaturaSaude(a.Estado().Saude)
	a.mudar(func(e *Estado) { e.Saude = novo })
	return mudou, fora
}

// assinaturaSaude ignora o contador de segundos (só o que muda a decisão).
func assinaturaSaude(m map[string]api.SaudeSetor) string {
	c := make(map[string]api.SaudeSetor, len(m))
	for k, v := range m {
		v.ForaHaS = 0
		c[k] = v
	}
	b, _ := json.Marshal(c)
	return string(b)
}

// cancelar tira da fila daqui os papéis que o navegador assumiu (P4). Não dá
// ack: quem imprimiu foi o navegador.
func (a *Agente) cancelar(ids []string) {
	n := 0
	for _, id := range ids {
		if a.fila.Tem(id) {
			if err := a.fila.Tirar(id); err == nil {
				n++
			}
		}
	}
	if n > 0 {
		a.op.Log.Info("papéis cancelados (o navegador assumiu)", "quantos", n)
		a.mudar(func(*Estado) {})
	}
}

// SemSono registra se o computador está sendo impedido de dormir (P1).
func (a *Agente) SemSono(ok bool, err error) {
	a.mudar(func(e *Estado) {
		e.SemSono = ok
		e.SemSonoErro = ""
		if err != nil && !errors.Is(err, context.Canceled) {
			e.SemSonoErro = err.Error()
		}
	})
}

// registrarTeste grava que o "Imprimir teste" daquele setor saiu.
func (a *Agente) registrarTeste(setor string) {
	_ = a.cfg.Update(func(c *config.Config) {
		if c.Testes == nil {
			c.Testes = map[string]string{}
		}
		c.Testes[setor] = a.agora().UTC().Format(time.RFC3339)
	})
	a.recalcularSaude()
}
