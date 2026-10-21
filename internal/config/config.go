// Package config guarda a configuração do programa no disco do usuário
// (chave do computador, endereço da API, impressoras escolhidas). Nunca
// senha: só a chave própria do computador que a Entregas emite no vínculo.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const (
	// APIPadrao é a Entregas em produção. Trocar só pra teste (MESALVAI_API).
	APIPadrao = "https://entregas.mesalvai.com.br"
	pasta     = "mesalvai-printer"
	arquivo   = "config.json"
)

// Config é o que fica em disco.
type Config struct {
	// API é a base da entregas-api (sem barra no fim).
	API string `json:"api"`
	// Chave do computador (mpk_…). Vazia = ainda não vinculado.
	Chave string `json:"chave,omitempty"`
	// Loja e computador, só pra mostrar na tela.
	LojaNome   string `json:"loja_nome,omitempty"`
	LojaID     string `json:"loja_id,omitempty"`
	AgenteID   string `json:"agente_id,omitempty"`
	AgenteNome string `json:"agente_nome,omitempty"`
	// Frente/Cozinha: campos da v1, mantidos em espelho de
	// Setores["frente"]/Setores["cozinha"] (config.json da v1 abre igual e
	// o status continua mandando front_printer/kitchen_printer).
	Frente  string `json:"frente,omitempty"`
	Cozinha string `json:"cozinha,omitempty"`
	// Setores (v2): chave do setor → impressora principal e reserva.
	Setores map[string]SetorLocal `json:"setores,omitempty"`
	// SetoresLista: cópia dos setores da loja (GET /printer-agent/config)
	// pra janela funcionar sem rede.
	SetoresLista []SetorInfo `json:"setores_lista,omitempty"`
	// Rede: impressoras de rede achadas em "Procurar impressoras" (ficam na
	// lista depois de reiniciar o programa, mesmo sem setor vinculado).
	Rede           []RedeAchada `json:"rede,omitempty"`
	AutoStart      bool         `json:"auto_start"`
	AutoUpdate     bool         `json:"auto_update"`
	PortaUI        int          `json:"porta_ui,omitempty"`
	AbrirAoIniciar bool         `json:"abrir_ao_iniciar"`
}

// SetorLocal é o vínculo de um setor neste computador.
type SetorLocal struct {
	Impressora string `json:"impressora,omitempty"`
	Reserva    string `json:"reserva,omitempty"`
}

// RedeAchada é uma impressora de rede que respondeu na porta 9100.
type RedeAchada struct {
	Nome   string `json:"nome"`             // tcp://IP:9100
	Rotulo string `json:"rotulo,omitempty"` // "Rede 192.168.0.50"
}

// MaxRede: quantas impressoras de rede achadas o programa lembra.
const MaxRede = 30

// JuntarRede põe as achadas agora por cima das lembradas (sem repetir; as
// lembradas que não responderam desta vez ficam — podem estar desligadas).
func JuntarRede(lembradas, agora []RedeAchada) []RedeAchada {
	visto := map[string]bool{}
	out := make([]RedeAchada, 0, len(lembradas)+len(agora))
	for _, r := range append(append([]RedeAchada(nil), agora...), lembradas...) {
		if r.Nome == "" || visto[r.Nome] {
			continue
		}
		visto[r.Nome] = true
		out = append(out, r)
	}
	if len(out) > MaxRede {
		out = out[:MaxRede]
	}
	return out
}

// SetorInfo é um setor da loja (nome e tipo vêm do servidor).
type SetorInfo struct {
	Key  string `json:"key"`
	Nome string `json:"nome"`
	Tipo string `json:"tipo,omitempty"` // frente | cozinha | praca
}

// SetoresPadrao: os dois setores que toda loja tem (v1).
func SetoresPadrao() []SetorInfo {
	return []SetorInfo{{Key: "frente", Nome: "Caixa/Frente", Tipo: "frente"}, {Key: "cozinha", Nome: "Cozinha", Tipo: "cozinha"}}
}

// Lista devolve os setores conhecidos (cache do servidor ou o padrão), sempre
// com frente e cozinha e com os setores vinculados que não estão na lista.
func (c Config) Lista() []SetorInfo {
	l := append([]SetorInfo{}, c.SetoresLista...)
	tem := map[string]bool{}
	for _, s := range l {
		tem[s.Key] = true
	}
	for _, p := range SetoresPadrao() {
		if !tem[p.Key] {
			l = append(l, p)
			tem[p.Key] = true
		}
	}
	for k := range c.Setores {
		if !tem[k] {
			l = append(l, SetorInfo{Key: k, Nome: k, Tipo: "praca"})
			tem[k] = true
		}
	}
	// frente e cozinha primeiro, o resto na ordem que veio.
	out := make([]SetorInfo, 0, len(l))
	for _, k := range []string{"frente", "cozinha"} {
		for _, s := range l {
			if s.Key == k {
				out = append(out, s)
			}
		}
	}
	for _, s := range l {
		if s.Key != "frente" && s.Key != "cozinha" {
			out = append(out, s)
		}
	}
	return out
}

// NomeDoSetor: nome conhecido do setor (ou a própria chave).
func (c Config) NomeDoSetor(key string) string {
	for _, s := range c.Lista() {
		if s.Key == key && s.Nome != "" {
			return s.Nome
		}
	}
	return key
}

// Setor devolve o vínculo do setor (vazio se não houver).
func (c Config) Setor(key string) SetorLocal {
	return c.Setores[key]
}

// migrar leva a config da v1 (frente/cozinha) pros setores e mantém o
// espelho. `antes` é a config antes da mudança (nil = abriu do disco):
// mexeram no campo antigo → vale o antigo; senão vale o setor.
func migrar(antes *Config, c *Config) {
	if c.Setores == nil {
		c.Setores = map[string]SetorLocal{}
	}
	for _, x := range []struct {
		key   string
		campo *string
		velho string
	}{{"frente", &c.Frente, campoDe(antes, "frente")}, {"cozinha", &c.Cozinha, campoDe(antes, "cozinha")}} {
		s := c.Setores[x.key]
		if antes == nil {
			// Abriu do disco: config v1 (sem setor) herda o campo antigo.
			if s.Impressora == "" && *x.campo != "" {
				s.Impressora = *x.campo
			}
		} else if *x.campo != x.velho {
			s.Impressora = *x.campo
		}
		*x.campo = s.Impressora
		c.Setores[x.key] = s
	}
	for k, s := range c.Setores {
		if s.Impressora == "" && s.Reserva == "" {
			delete(c.Setores, k)
		}
	}
}

func campoDe(c *Config, key string) string {
	if c == nil {
		return ""
	}
	if key == "frente" {
		return c.Frente
	}
	return c.Cozinha
}

// Padrao é a configuração de fábrica.
func Padrao() Config {
	api := os.Getenv("MESALVAI_API")
	if api == "" {
		api = APIPadrao
	}
	return Config{API: api, AutoStart: true, AutoUpdate: true, AbrirAoIniciar: true}
}

// Dir é a pasta do programa (config, fila, logs): %AppData%\mesalvai-printer
// no Windows, ~/Library/Application Support/mesalvai-printer no Mac.
func Dir() (string, error) {
	if d := os.Getenv("MESALVAI_PRINTER_DIR"); d != "" {
		return d, os.MkdirAll(d, 0o700)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(base, pasta)
	return d, os.MkdirAll(d, 0o700)
}

// Store lê e grava a config com trava.
type Store struct {
	mu   sync.Mutex
	path string
	c    Config
}

// Abrir carrega (ou cria) a config.
func Abrir() (*Store, error) {
	d, err := Dir()
	if err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(d, arquivo), c: Padrao()}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, s.salvar()
		}
		return nil, err
	}
	c := Padrao()
	if err := json.Unmarshal(b, &c); err != nil {
		// Arquivo corrompido: começa de novo (o vínculo é refeito em 1 minuto).
		return s, s.salvar()
	}
	if c.API == "" {
		c.API = Padrao().API
	}
	migrar(nil, &c)
	s.c = c
	return s, nil
}

// Get devolve uma cópia.
func (s *Store) Get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.c
	c.Setores = make(map[string]SetorLocal, len(s.c.Setores))
	for k, v := range s.c.Setores {
		c.Setores[k] = v
	}
	c.SetoresLista = append([]SetorInfo(nil), s.c.SetoresLista...)
	c.Rede = append([]RedeAchada(nil), s.c.Rede...)
	return c
}

// Update altera e grava.
func (s *Store) Update(f func(*Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	antes := s.c
	antes.Setores = nil
	c := s.c
	c.Setores = make(map[string]SetorLocal, len(s.c.Setores))
	for k, v := range s.c.Setores {
		c.Setores[k] = v
	}
	f(&c)
	migrar(&antes, &c)
	s.c = c
	return s.salvar()
}

func (s *Store) salvar() error {
	b, err := json.MarshalIndent(s.c, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
