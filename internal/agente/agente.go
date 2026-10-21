// Package agente é o coração do programa: vincula (código de 6 letras),
// mantém o stream com a Entregas (reconexão com espera crescente, vigia de
// silêncio), guarda o papel na fila em disco e imprime no alvo certo
// (frente/cozinha), com ack ao servidor. Tudo injetável pra teste.
package agente

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/marconlabs/mesalvai-printer/internal/api"
	"github.com/marconlabs/mesalvai-printer/internal/config"
	"github.com/marconlabs/mesalvai-printer/internal/fila"
	"github.com/marconlabs/mesalvai-printer/internal/impressora"
)

// Estado é o que a bandeja e a janela mostram.
type Estado struct {
	Vinculado       bool              `json:"vinculado"`
	Conectado       bool              `json:"conectado"`
	Codigo          string            `json:"codigo,omitempty"`
	CodigoExpira    time.Time         `json:"codigo_expira,omitempty"`
	LojaNome        string            `json:"loja_nome,omitempty"`
	AgenteNome      string            `json:"agente_nome,omitempty"`
	Frente          string            `json:"frente"`
	Cozinha         string            `json:"cozinha"`
	Impressoras     []impressora.Info `json:"impressoras"`
	NaFila          int               `json:"na_fila"`
	UltimaImpressao time.Time         `json:"ultima_impressao,omitempty"`
	UltimoErro      string            `json:"ultimo_erro,omitempty"`
	Impressos       int               `json:"impressos"`
	Versao          string            `json:"versao"`
}

// Opcoes ajustam tempos e injetam dependências (testes).
type Opcoes struct {
	Versao     string
	SO         string
	NomeDoPC   string
	Imprimir   func(ctx context.Context, nome string, dados []byte) error
	Listar     func() ([]impressora.Info, error)
	BackoffMin time.Duration
	BackoffMax time.Duration
	// Silencio: sem ping por tanto tempo = conexão morta.
	Silencio time.Duration
	// RetryFila: intervalo de tentar de novo o que ficou na fila.
	RetryFila time.Duration
	// Avisar: balão da bandeja (impressora caiu, voltou, desvinculado).
	Avisar func(titulo, texto string)
	Log    *slog.Logger
}

// Agente roda o programa.
type Agente struct {
	cfg          *config.Store
	fila         *fila.Fila
	op           Opcoes
	mu           sync.Mutex
	est          Estado
	ouvintes     []func(Estado)
	acorda       chan struct{}
	cancelStream context.CancelFunc
}

// New monta o agente.
func New(cfg *config.Store, f *fila.Fila, op Opcoes) *Agente {
	if op.Imprimir == nil {
		op.Imprimir = impressora.Imprimir
	}
	if op.Listar == nil {
		op.Listar = impressora.Listar
	}
	if op.BackoffMin == 0 {
		op.BackoffMin = time.Second
	}
	if op.BackoffMax == 0 {
		op.BackoffMax = 30 * time.Second
	}
	if op.Silencio == 0 {
		op.Silencio = 45 * time.Second
	}
	if op.RetryFila == 0 {
		op.RetryFila = 15 * time.Second
	}
	if op.Log == nil {
		op.Log = slog.Default()
	}
	if op.Avisar == nil {
		op.Avisar = func(string, string) {}
	}
	c := cfg.Get()
	a := &Agente{cfg: cfg, fila: f, op: op, acorda: make(chan struct{}, 1)}
	a.est = Estado{Vinculado: c.Chave != "", LojaNome: c.LojaNome, AgenteNome: c.AgenteNome, Frente: c.Frente, Cozinha: c.Cozinha, NaFila: f.Tamanho(), Versao: op.Versao}
	return a
}

// Estado atual (cópia).
func (a *Agente) Estado() Estado {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.est
}

// Ouvir registra quem quer saber de mudança de estado.
func (a *Agente) Ouvir(f func(Estado)) {
	a.mu.Lock()
	a.ouvintes = append(a.ouvintes, f)
	a.mu.Unlock()
}

func (a *Agente) mudar(f func(*Estado)) {
	a.mu.Lock()
	f(&a.est)
	a.est.NaFila = a.fila.Tamanho()
	e := a.est
	ouv := append([]func(Estado){}, a.ouvintes...)
	a.mu.Unlock()
	for _, o := range ouv {
		o(e)
	}
}

func (a *Agente) client() *api.Client {
	c := a.cfg.Get()
	return api.New(c.API, c.Chave, a.op.SO, a.op.Versao)
}

// Run roda até o contexto acabar: pareia se preciso, depois stream + fila.
func (a *Agente) Run(ctx context.Context) {
	go a.laçoDaFila(ctx)
	go a.laçoDasImpressoras(ctx)
	for ctx.Err() == nil {
		if a.cfg.Get().Chave == "" {
			if err := a.parear(ctx); err != nil && ctx.Err() == nil {
				a.op.Log.Warn("pareamento", "erro", err)
				a.mudar(func(e *Estado) { e.UltimoErro = "Sem conexão com a Mesalvaí: " + err.Error() })
				esperar(ctx, a.op.BackoffMax/3)
			}
			continue
		}
		a.streamComReconexao(ctx)
	}
}

// Desvincular apaga a chave local (o painel revogou ou a pessoa pediu).
func (a *Agente) Desvincular() {
	_ = a.cfg.Update(func(c *config.Config) { c.Chave, c.LojaNome, c.LojaID, c.AgenteID, c.AgenteNome = "", "", "", "", "" })
	a.mudar(func(e *Estado) {
		e.Vinculado, e.Conectado, e.LojaNome, e.AgenteNome, e.Codigo = false, false, "", "", ""
	})
	a.mu.Lock()
	cancel := a.cancelStream
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// ── pareamento ──────────────────────────────────────────────────────────────

func (a *Agente) parear(ctx context.Context) error {
	cl := a.client()
	ps, err := cl.PairStart(ctx, a.op.NomeDoPC)
	if err != nil {
		return err
	}
	exp, _ := time.Parse(time.RFC3339, ps.ExpiresAt)
	a.mudar(func(e *Estado) { e.Codigo, e.CodigoExpira, e.UltimoErro = ps.Code, exp, "" })
	a.op.Log.Info("código de pareamento", "code", ps.Code)
	for ctx.Err() == nil {
		w, err := cl.PairWait(ctx, ps.PollToken)
		if err != nil {
			if errors.Is(err, api.ErrExpirado) {
				a.mudar(func(e *Estado) { e.Codigo = "" })
				return nil // volta e gera outro
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			esperar(ctx, 2*time.Second)
			continue
		}
		switch w.Status {
		case "paired":
			if err := a.cfg.Update(func(c *config.Config) {
				c.Chave, c.LojaNome, c.LojaID, c.AgenteID, c.AgenteNome = w.Key, w.Store.Name, w.Store.ID, w.Agent.ID, w.Agent.Name
			}); err != nil {
				return err
			}
			a.mudar(func(e *Estado) {
				e.Vinculado, e.Codigo, e.LojaNome, e.AgenteNome = true, "", w.Store.Name, w.Agent.Name
			})
			a.op.Avisar("Mesalvaí Printer", "Vinculado à loja "+w.Store.Name+".")
			return nil
		case "expired":
			a.mudar(func(e *Estado) { e.Codigo = "" })
			return nil
		}
	}
	return ctx.Err()
}

// ── stream ──────────────────────────────────────────────────────────────────

func (a *Agente) streamComReconexao(ctx context.Context) {
	espera := a.op.BackoffMin
	for ctx.Err() == nil && a.cfg.Get().Chave != "" {
		inicio := time.Now()
		err := a.umStream(ctx)
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, api.ErrNaoAutorizado) {
			a.op.Log.Warn("chave revogada: desvinculando")
			a.op.Avisar("Mesalvaí Printer", "Este computador foi desvinculado no painel. Vincule de novo com um código novo.")
			a.Desvincular()
			return
		}
		a.mudar(func(e *Estado) {
			e.Conectado = false
			if err != nil {
				e.UltimoErro = "Sem conexão com a Mesalvaí (tentando de novo)."
			}
		})
		// Ficou um bom tempo conectado: a próxima espera volta ao mínimo.
		if time.Since(inicio) > time.Minute {
			espera = a.op.BackoffMin
		}
		a.op.Log.Info("stream caiu", "erro", err, "volta_em", espera)
		esperar(ctx, espera)
		espera *= 2
		if espera > a.op.BackoffMax {
			espera = a.op.BackoffMax
		}
	}
}

func (a *Agente) umStream(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	a.mu.Lock()
	a.cancelStream = cancel
	a.mu.Unlock()
	cl := a.client()
	eventos := make(chan api.Evento, 16)
	errc := make(chan error, 1)
	go func() { errc <- cl.Stream(ctx, eventos) }()
	// Manda o estado das impressoras assim que conecta (o painel mostra).
	go a.enviarStatus(ctx, nil)
	vigia := time.NewTimer(a.op.Silencio)
	defer vigia.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errc:
			return err
		case <-vigia.C:
			return fmt.Errorf("sem ping há %s", a.op.Silencio)
		case ev := <-eventos:
			vigia.Reset(a.op.Silencio)
			switch ev.Tipo {
			case "hello":
				a.mudar(func(e *Estado) { e.Conectado, e.UltimoErro = true, "" })
				a.op.Log.Info("conectado", "agent", ev.Hello.AgentID)
			case "job":
				a.receber(*ev.Job)
			case "revoked":
				return api.ErrNaoAutorizado
			}
		}
	}
}

// receber guarda o papel na fila e acorda quem imprime.
func (a *Agente) receber(j api.Job) {
	if a.fila.Tem(j.ID) {
		return
	}
	if err := a.fila.Por(fila.Item{ID: j.ID, Target: j.Target, Title: j.Title, Copies: j.Copies, PayloadB64: j.PayloadB64}); err != nil {
		a.op.Log.Error("fila", "erro", err)
		return
	}
	a.op.Log.Info("papel recebido", "id", j.ID, "alvo", j.Target, "titulo", j.Title)
	a.mudar(func(*Estado) {})
	select {
	case a.acorda <- struct{}{}:
	default:
	}
}

// ── fila → impressora ───────────────────────────────────────────────────────

func (a *Agente) laçoDaFila(ctx context.Context) {
	for {
		a.drenar(ctx)
		select {
		case <-ctx.Done():
			return
		case <-a.acorda:
		case <-time.After(a.op.RetryFila):
		}
	}
}

// Acordar força uma passada na fila (trocou a impressora, "tentar agora").
func (a *Agente) Acordar() {
	select {
	case a.acorda <- struct{}{}:
	default:
	}
}

func (a *Agente) impressoraDo(target string) string {
	c := a.cfg.Get()
	if target == "cozinha" {
		if c.Cozinha != "" {
			return c.Cozinha
		}
		// Sem impressora da cozinha: o preparo sai na da frente (não some).
	}
	return c.Frente
}

func (a *Agente) drenar(ctx context.Context) {
	itens, err := a.fila.Lista()
	if err != nil || len(itens) == 0 {
		return
	}
	cl := a.client()
	caiu := false
	for _, it := range itens {
		if ctx.Err() != nil {
			return
		}
		nome := a.impressoraDo(it.Target)
		dados, err := base64.StdEncoding.DecodeString(it.PayloadB64)
		if err != nil {
			_ = a.fila.Tirar(it.ID)
			_ = cl.Ack(ctx, it.ID, "falhou", "papel inválido")
			continue
		}
		if nome == "" {
			if it.Tentativas == 0 {
				a.op.Avisar("Mesalvaí Printer", "Chegou um pedido e nenhuma impressora foi escolhida. Abra o programa e escolha.")
			}
			_ = a.fila.Falhou(it.ID, "nenhuma impressora escolhida")
			a.mudar(func(e *Estado) { e.UltimoErro = "Escolha a impressora da frente." })
			break
		}
		var errImp error
		for i := 0; i < max(1, it.Copies); i++ {
			if errImp = a.op.Imprimir(ctx, nome, dados); errImp != nil {
				break
			}
		}
		if errImp != nil {
			_ = a.fila.Falhou(it.ID, errImp.Error())
			if it.Tentativas == 0 {
				a.op.Avisar("Impressora não respondeu", fmt.Sprintf("%s: %s. Vou tentar de novo sozinho.", it.Title, errImp.Error()))
			}
			a.mudar(func(e *Estado) { e.UltimoErro = "Impressora " + nome + ": " + errImp.Error() })
			a.enviarStatus(ctx, ptr(errImp.Error()))
			caiu = true
			break // mantém a ordem: não pula pro próximo com a impressora fora
		}
		_ = a.fila.Tirar(it.ID)
		a.op.Log.Info("impresso", "id", it.ID, "impressora", nome, "titulo", it.Title)
		a.mudar(func(e *Estado) { e.UltimaImpressao, e.Impressos, e.UltimoErro = time.Now(), e.Impressos+1, "" })
		if err := cl.Ack(ctx, it.ID, "impresso", ""); err != nil {
			a.op.Log.Warn("ack", "id", it.ID, "erro", err)
		}
		if it.Tentativas > 0 && !caiu {
			a.op.Avisar("Impressora voltou", it.Title+" saiu.")
		}
	}
}

func (a *Agente) enviarStatus(ctx context.Context, erro *string) {
	if a.cfg.Get().Chave == "" {
		return
	}
	infos, _ := a.op.Listar()
	nomes := make([]string, 0, len(infos))
	for _, i := range infos {
		nomes = append(nomes, i.Nome)
	}
	c := a.cfg.Get()
	st := api.Status{Printers: nomes, FrontPrinter: ptr(c.Frente), KitchenPrinter: ptr(c.Cozinha), LastError: erro}
	if erro == nil {
		vazio := ""
		st.LastError = &vazio
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := a.client().EnviarStatus(ctx, st); err != nil {
		a.op.Log.Warn("status", "erro", err)
	}
}

// laçoDasImpressoras relê a lista do sistema de tempos em tempos.
func (a *Agente) laçoDasImpressoras(ctx context.Context) {
	for {
		infos, _ := a.op.Listar()
		a.mudar(func(e *Estado) { e.Impressoras = infos })
		select {
		case <-ctx.Done():
			return
		case <-time.After(60 * time.Second):
		}
	}
}

// Escolher grava a impressora da frente e da cozinha e avisa a Entregas.
func (a *Agente) Escolher(ctx context.Context, frente, cozinha string) error {
	if err := a.cfg.Update(func(c *config.Config) { c.Frente, c.Cozinha = frente, cozinha }); err != nil {
		return err
	}
	a.mudar(func(e *Estado) { e.Frente, e.Cozinha, e.UltimoErro = frente, cozinha, "" })
	a.enviarStatus(ctx, nil)
	a.Acordar()
	return nil
}

// TestePapel manda um papel de teste local (sem servidor) pro alvo.
func (a *Agente) TestePapel(ctx context.Context, target string) error {
	nome := a.impressoraDo(target)
	if nome == "" {
		return impressora.ErrSemImpressora
	}
	return a.op.Imprimir(ctx, nome, PapelDeTesteLocal(target))
}

// PapelDeTesteLocal é um ESC/POS mínimo: título, linha, avanço e corte.
func PapelDeTesteLocal(target string) []byte {
	b := []byte{0x1b, 0x40, 0x1b, 0x61, 1, 0x1d, 0x21, 0x11}
	b = append(b, []byte("MESALVAI PRINTER\n")...)
	b = append(b, 0x1d, 0x21, 0)
	b = append(b, []byte("Teste da impressora ("+target+")\n")...)
	b = append(b, []byte(time.Now().Format("02/01 15:04")+"\n")...)
	b = append(b, []byte("--------------------------------\n")...)
	b = append(b, []byte("Saiu? Entao esta tudo certo.\n")...)
	b = append(b, 0x1b, 0x64, 4, 0x1d, 0x56, 0x42, 0)
	return b
}

func esperar(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func ptr(s string) *string { return &s }
