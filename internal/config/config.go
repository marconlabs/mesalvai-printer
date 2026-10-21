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
	LojaNome       string `json:"loja_nome,omitempty"`
	LojaID         string `json:"loja_id,omitempty"`
	AgenteID       string `json:"agente_id,omitempty"`
	AgenteNome     string `json:"agente_nome,omitempty"`
	Frente         string `json:"frente,omitempty"`
	Cozinha        string `json:"cozinha,omitempty"`
	AutoStart      bool   `json:"auto_start"`
	AutoUpdate     bool   `json:"auto_update"`
	PortaUI        int    `json:"porta_ui,omitempty"`
	AbrirAoIniciar bool   `json:"abrir_ao_iniciar"`
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
	s.c = c
	return s, nil
}

// Get devolve uma cópia.
func (s *Store) Get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.c
}

// Update altera e grava.
func (s *Store) Update(f func(*Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.c)
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
