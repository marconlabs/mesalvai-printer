// Package agente é o coração do programa: vincula (código de 6 letras),
// mantém o stream com a Entregas (reconexão com espera crescente, vigia de
// silêncio), guarda o papel na fila em disco e imprime no setor certo
// (frente, cozinha, praças), com impressora reserva por setor e ack ao
// servidor. Tudo injetável pra teste.
package agente

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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
	Setores         []SetorEstado     `json:"setores"`
	Impressoras     []impressora.Info `json:"impressoras"`
	Procurando      bool              `json:"procurando"`
	NaFila          int               `json:"na_fila"`
	UltimaImpressao time.Time         `json:"ultima_impressao,omitempty"`
	UltimoErro      string            `json:"ultimo_erro,omitempty"`
	Impressos       int               `json:"impressos"`
	Versao          string            `json:"versao"`
}

// SetorEstado é uma linha da seção "Setores" da janela.
type SetorEstado struct {
	Key        string `json:"key"`
	Nome       string `json:"nome"`
	Tipo       string `json:"tipo"`
	Impressora string `json:"impressora"`
	Reserva    string `json:"reserva"`
}

// Opcoes ajustam tempos e injetam dependências (testes).
type Opcoes struct {
	Versao   string
	SO       string
	NomeDoPC string
	Imprimir func(ctx context.Context, nome string, dados []byte) error
	Listar   func() ([]impressora.Info, error)
	// Procurar: varredura da rede (botão "Procurar impressoras").
	Procurar   func(ctx context.Context) []impressora.Info
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
	// rede: impressoras achadas na última varredura (somam-se às do sistema).
	rede []impressora.Info
	// sincronia: um de cada vez entre buscar o config e gravar vínculo.
	sincronia sync.Mutex
	// pendentes: vínculo mudou sem rede; vai no próximo conectar.
	pendentes bool
	// naReserva: principais fora que já ganharam balão (só o laço da fila mexe).
	naReserva map[string]bool
}

// New monta o agente.
func New(cfg *config.Store, f *fila.Fila, op Opcoes) *Agente {
	if op.Imprimir == nil {
		op.Imprimir = impressora.Imprimir
	}
	if op.Listar == nil {
		op.Listar = impressora.Listar
	}
	if op.Procurar == nil {
		op.Procurar = impressora.ProcurarNaRede
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
	a := &Agente{cfg: cfg, fila: f, op: op, acorda: make(chan struct{}, 1), naReserva: map[string]bool{}}
	// Impressoras de rede achadas antes (Procurar) voltam pra lista.
	for _, r := range cfg.Get().Rede {
		a.rede = append(a.rede, impressora.Info{Nome: r.Nome, Rotulo: r.Rotulo})
	}
	a.est = Estado{Vinculado: c.Chave != "", LojaNome: c.LojaNome, AgenteNome: c.AgenteNome, Frente: c.Frente, Cozinha: c.Cozinha, Setores: setoresDoEstado(c), NaFila: f.Tamanho(), Versao: op.Versao}
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
	go a.enviarStatus(ctx, nil, nil)
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
				go a.buscarConfig(ctx)
			case "config":
				// Mudou setor/vínculo no painel: busca de novo.
				go a.buscarConfig(ctx)
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
	if err := a.fila.Por(fila.Item{ID: j.ID, Target: j.Target, Title: j.Title, Copies: j.Copies, PayloadB64: j.PayloadB64,
		SectorName: j.SectorName, Papel: j.Papel, Origem: j.Origem, Reserva: j.Reserva}); err != nil {
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

// destino decide onde sai um papel do setor `it.Target`:
//   - setor com impressora neste computador → ela (e a reserva dele);
//   - sem vínculo: cozinha cai na frente (como a v1); qualquer outro setor cai
//     na frente com o NOME DO SETOR grande no topo (`cabecalho`), a não ser
//     que o servidor já tenha posto (`it.Reserva`).
func destino(c config.Config, it fila.Item) (principal, reserva, cabecalho string) {
	s := c.Setor(it.Target)
	if s.Impressora == "" && s.Reserva != "" {
		s.Impressora, s.Reserva = s.Reserva, ""
	}
	if s.Impressora != "" {
		return s.Impressora, s.Reserva, ""
	}
	f := c.Setor("frente")
	if f.Impressora == "" && f.Reserva != "" {
		f.Impressora, f.Reserva = f.Reserva, ""
	}
	if it.Target == "" || it.Target == "frente" || it.Target == "cozinha" || it.Reserva {
		return f.Impressora, f.Reserva, ""
	}
	return f.Impressora, f.Reserva, nomeDoSetor(c, it)
}

func nomeDoSetor(c config.Config, it fila.Item) string {
	if it.SectorName != "" {
		return it.SectorName
	}
	return c.NomeDoSetor(it.Target)
}

func (a *Agente) imprimirVias(ctx context.Context, nome string, dados []byte, copias int) error {
	for i := 0; i < max(1, copias); i++ {
		if err := a.op.Imprimir(ctx, nome, dados); err != nil {
			return err
		}
	}
	return nil
}

// drenar imprime a fila na ordem de chegada. Impressora fora trava só os
// papéis que iam pra ela (a ordem vale por impressora); os das outras
// impressoras seguem. Com reserva no setor, o papel sai na reserva.
func (a *Agente) drenar(ctx context.Context) {
	itens, err := a.fila.Lista()
	if err != nil || len(itens) == 0 {
		return
	}
	c := a.cfg.Get()
	cl := a.client()
	travada := map[string]bool{}        // principal fora sem reserva: papéis dela esperam
	foraNaPassada := map[string]error{} // principal fora com reserva: os próximos vão direto pra reserva
	for _, it := range itens {
		if ctx.Err() != nil {
			return
		}
		principal, reserva, cabecalho := destino(c, it)
		if travada[principal] {
			continue
		}
		dados, err := base64.StdEncoding.DecodeString(it.PayloadB64)
		if err != nil {
			_ = a.fila.Tirar(it.ID)
			_ = cl.Ack(ctx, it.ID, "falhou", "papel inválido")
			continue
		}
		if principal == "" {
			if it.Tentativas == 0 {
				a.op.Avisar("Mesalvaí Printer", "Chegou um pedido e nenhuma impressora foi escolhida. Abra o programa e escolha.")
			}
			_ = a.fila.Falhou(it.ID, "nenhuma impressora escolhida")
			a.mudar(func(e *Estado) { e.UltimoErro = "Escolha a impressora da frente." })
			travada[principal] = true
			continue
		}
		papel := dados
		if cabecalho != "" {
			papel = append(CabecalhoSetor(cabecalho), dados...)
		}
		errP, jaCaiu := foraNaPassada[principal]
		if !jaCaiu {
			errP = a.imprimirVias(ctx, principal, papel, it.Copies)
		}
		if errP == nil {
			delete(a.naReserva, principal)
			a.saiu(ctx, cl, it, api.AckInfo{Status: "impresso", Via: "principal", Printer: principal})
			continue
		}
		if reserva != "" && reserva != principal {
			setor := nomeDoSetor(c, it)
			papelR := append(CabecalhoReserva(setor, errP.Error()), dados...)
			errR := a.imprimirVias(ctx, reserva, papelR, it.Copies)
			if errR == nil {
				foraNaPassada[principal] = errP
				if !a.naReserva[principal] {
					a.naReserva[principal] = true
					a.op.Avisar("Papel saiu na impressora reserva", fmt.Sprintf("%s: %s não respondeu (%s). %s saiu em %s.", strings.ToUpper(setor), principal, errP.Error(), it.Title, reserva))
				}
				a.mudar(func(e *Estado) {
					e.UltimoErro = fmt.Sprintf("Impressora %s: %s (saindo na reserva %s).", principal, errP.Error(), reserva)
				})
				a.saiu(ctx, cl, it, api.AckInfo{Status: "impresso", Via: "reserva", Printer: reserva, Error: errP.Error()})
				continue
			}
			errP = fmt.Errorf("%v; reserva %s: %v", errP, reserva, errR)
		}
		_ = a.fila.Falhou(it.ID, errP.Error())
		if it.Tentativas == 0 {
			a.op.Avisar("Impressora não respondeu", fmt.Sprintf("%s: %s. Vou tentar de novo sozinho.", it.Title, errP.Error()))
		}
		a.mudar(func(e *Estado) { e.UltimoErro = "Impressora " + principal + ": " + errP.Error() })
		_ = a.enviarStatus(ctx, ptr(errP.Error()), nil)
		travada[principal] = true // mantém a ordem desta impressora; as outras seguem
	}
}

// saiu tira o papel da fila e dá o ack.
func (a *Agente) saiu(ctx context.Context, cl *api.Client, it fila.Item, ack api.AckInfo) {
	_ = a.fila.Tirar(it.ID)
	a.op.Log.Info("impresso", "id", it.ID, "impressora", ack.Printer, "via", ack.Via, "setor", it.Target, "titulo", it.Title)
	a.mudar(func(e *Estado) {
		e.UltimaImpressao, e.Impressos = time.Now(), e.Impressos+1
		if ack.Via == "principal" {
			e.UltimoErro = ""
		}
	})
	if err := cl.AckCom(ctx, it.ID, ack); err != nil {
		a.op.Log.Warn("ack", "id", it.ID, "erro", err)
	}
	if it.Tentativas > 0 && ack.Via == "principal" {
		a.op.Avisar("Impressora voltou", it.Title+" saiu.")
	}
}

// ── impressoras, setores e status ──────────────────────────────────────────

// impressoras = as do sistema + as de rede achadas na varredura.
func (a *Agente) impressoras() []impressora.Info {
	infos, _ := a.op.Listar()
	a.mu.Lock()
	rede := append([]impressora.Info(nil), a.rede...)
	a.mu.Unlock()
	visto := map[string]bool{}
	out := make([]impressora.Info, 0, len(infos)+len(rede))
	for _, i := range append(infos, rede...) {
		if !visto[i.Nome] {
			visto[i.Nome] = true
			out = append(out, i)
		}
	}
	return out
}

func setoresDoEstado(c config.Config) []SetorEstado {
	l := c.Lista()
	out := make([]SetorEstado, 0, len(l))
	for _, s := range l {
		v := c.Setor(s.Key)
		out = append(out, SetorEstado{Key: s.Key, Nome: s.Nome, Tipo: s.Tipo, Impressora: v.Impressora, Reserva: v.Reserva})
	}
	return out
}

func (a *Agente) refletirConfig() {
	c := a.cfg.Get()
	a.mudar(func(e *Estado) { e.Frente, e.Cozinha, e.Setores = c.Frente, c.Cozinha, setoresDoEstado(c) })
}

func (a *Agente) enviarStatus(ctx context.Context, erro *string, setores map[string]*api.SetorStatus) error {
	if a.cfg.Get().Chave == "" {
		return nil
	}
	infos := a.impressoras()
	nomes := make([]string, 0, len(infos))
	for _, i := range infos {
		nomes = append(nomes, i.Nome)
	}
	c := a.cfg.Get()
	st := api.Status{Printers: nomes, FrontPrinter: ptr(c.Frente), KitchenPrinter: ptr(c.Cozinha), LastError: erro, Setores: setores, Caps: api.Caps}
	if erro == nil {
		vazio := ""
		st.LastError = &vazio
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	err := a.client().EnviarStatus(ctx, st)
	if err != nil {
		a.op.Log.Warn("status", "erro", err)
	}
	return err
}

func statusDoSetor(s config.SetorLocal) *api.SetorStatus {
	if s.Impressora == "" && s.Reserva == "" {
		return nil // null = apaga o vínculo
	}
	st := &api.SetorStatus{}
	if s.Impressora != "" {
		st.Impressora = ptr(s.Impressora)
	}
	if s.Reserva != "" {
		st.Reserva = ptr(s.Reserva)
	}
	return st
}

// buscarConfig traz setores (nomes) e vínculos do servidor. Servidor v1 (sem
// `setores`/`bindings`) não mexe no que está aqui. Vínculo mudado sem rede
// sobe antes (senão o servidor apagaria a mudança).
func (a *Agente) buscarConfig(ctx context.Context) {
	a.sincronia.Lock()
	defer a.sincronia.Unlock()
	if a.pendentes {
		c := a.cfg.Get()
		todos := map[string]*api.SetorStatus{}
		for k, s := range c.Setores {
			todos[k] = statusDoSetor(s)
		}
		if err := a.enviarStatus(ctx, nil, todos); err != nil {
			return
		}
		a.pendentes = false
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	sc, err := a.client().Config(ctx)
	if err != nil {
		a.op.Log.Warn("config", "erro", err)
		return
	}
	if sc.Setores == nil && sc.Bindings == nil {
		return
	}
	err = a.cfg.Update(func(c *config.Config) {
		if sc.Setores != nil {
			c.SetoresLista = c.SetoresLista[:0]
			for _, s := range sc.Setores {
				if s.Key != "" {
					c.SetoresLista = append(c.SetoresLista, config.SetorInfo{Key: s.Key, Nome: s.Nome, Tipo: s.Tipo})
				}
			}
		}
		if sc.Bindings != nil {
			novo := map[string]config.SetorLocal{}
			for k, b := range sc.Bindings {
				novo[k] = config.SetorLocal{Impressora: deref(b.Impressora), Reserva: deref(b.Reserva)}
			}
			c.Setores = novo
		}
	})
	if err != nil {
		a.op.Log.Error("config", "erro", err)
	}
	a.op.Log.Info("config do servidor", "setores", len(sc.Setores), "vinculos", len(sc.Bindings))
	a.refletirConfig()
	a.Acordar()
}

// laçoDasImpressoras relê a lista do sistema de tempos em tempos.
func (a *Agente) laçoDasImpressoras(ctx context.Context) {
	for {
		infos := a.impressoras()
		a.mudar(func(e *Estado) { e.Impressoras = infos })
		select {
		case <-ctx.Done():
			return
		case <-time.After(60 * time.Second):
		}
	}
}

// TetoProcurar: tempo máximo de uma procura de impressoras de rede.
var TetoProcurar = 20 * time.Second

// Procurar varre a rede atrás de impressoras (porta 9100). Só quando a
// pessoa pede (abre e fecha a conexão, nunca manda bytes). As achadas entram na lista e vão pro painel no status.
func (a *Agente) Procurar(ctx context.Context) ([]impressora.Info, error) {
	a.mu.Lock()
	if a.est.Procurando {
		a.mu.Unlock()
		return nil, errors.New("já estou procurando; espere terminar")
	}
	a.est.Procurando = true
	a.mu.Unlock()
	a.mudar(func(*Estado) {})
	// Teto da procura: mesmo com várias placas de rede (VPN, Wi-Fi e cabo)
	// a janela volta em no máximo TetoProcurar; o que respondeu até ali fica.
	pctx, cancel := context.WithTimeout(ctx, TetoProcurar)
	achou := a.op.Procurar(pctx)
	cancel()
	agora := make([]config.RedeAchada, 0, len(achou))
	for _, i := range achou {
		agora = append(agora, config.RedeAchada{Nome: i.Nome, Rotulo: i.Rotulo})
	}
	// Grava no config: depois de reiniciar, as achadas continuam na lista.
	_ = a.cfg.Update(func(c *config.Config) { c.Rede = config.JuntarRede(c.Rede, agora) })
	lembradas := a.cfg.Get().Rede
	a.mu.Lock()
	a.rede = a.rede[:0]
	for _, r := range lembradas {
		a.rede = append(a.rede, impressora.Info{Nome: r.Nome, Rotulo: r.Rotulo})
	}
	a.mu.Unlock()
	infos := a.impressoras()
	a.mudar(func(e *Estado) { e.Procurando, e.Impressoras = false, infos })
	a.op.Log.Info("procurar impressoras", "achou", len(achou))
	_ = a.enviarStatus(ctx, nil, nil)
	return achou, nil
}

// VincularSetor grava a impressora (e a reserva) de um setor neste computador
// e avisa a Entregas. Sem rede: fica gravado aqui e sobe ao reconectar.
func (a *Agente) VincularSetor(ctx context.Context, key, imp, reserva string) error {
	key, imp, reserva = strings.TrimSpace(key), strings.TrimSpace(imp), strings.TrimSpace(reserva)
	if key == "" {
		return errors.New("setor sem chave")
	}
	if reserva == imp {
		reserva = ""
	}
	a.sincronia.Lock()
	defer a.sincronia.Unlock()
	if err := a.cfg.Update(func(c *config.Config) { c.Setores[key] = config.SetorLocal{Impressora: imp, Reserva: reserva} }); err != nil {
		return err
	}
	a.refletirConfig()
	a.mudar(func(e *Estado) { e.UltimoErro = "" })
	if err := a.enviarStatus(ctx, nil, map[string]*api.SetorStatus{key: statusDoSetor(a.cfg.Get().Setor(key))}); err != nil {
		a.pendentes = true
	}
	a.Acordar()
	return nil
}

// Escolher grava a impressora da frente e da cozinha (rota da v1).
func (a *Agente) Escolher(ctx context.Context, frente, cozinha string) error {
	a.sincronia.Lock()
	defer a.sincronia.Unlock()
	if err := a.cfg.Update(func(c *config.Config) { c.Frente, c.Cozinha = frente, cozinha }); err != nil {
		return err
	}
	a.refletirConfig()
	a.mudar(func(e *Estado) { e.UltimoErro = "" })
	c := a.cfg.Get()
	if err := a.enviarStatus(ctx, nil, map[string]*api.SetorStatus{"frente": statusDoSetor(c.Setor("frente")), "cozinha": statusDoSetor(c.Setor("cozinha"))}); err != nil {
		a.pendentes = true
	}
	a.Acordar()
	return nil
}

// TestePapel manda um papel de teste local (sem servidor) pra impressora
// onde o setor imprime de verdade, com o nome do setor grande.
func (a *Agente) TestePapel(ctx context.Context, setor string) error {
	c := a.cfg.Get()
	nome, _, _ := destino(c, fila.Item{Target: setor})
	if nome == "" {
		return impressora.ErrSemImpressora
	}
	return a.op.Imprimir(ctx, nome, PapelDeTesteLocal(c.NomeDoSetor(setor)))
}

func esperar(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func ptr(s string) *string { return &s }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}
