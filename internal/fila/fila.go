// Package fila guarda em disco os papéis que chegaram e ainda não saíram
// (impressora desligada, sem papel, programa fechado no meio). Um arquivo
// JSON por papel; a ordem é a de chegada. Sobrevive a reiniciar o programa.
package fila

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Item é um papel esperando.
type Item struct {
	ID         string    `json:"id"`
	Target     string    `json:"target"`
	Title      string    `json:"title"`
	Copies     int       `json:"copies"`
	PayloadB64 string    `json:"payload_b64"`
	ChegouEm   time.Time `json:"chegou_em"`
	Tentativas int       `json:"tentativas"`
	UltimoErro string    `json:"ultimo_erro,omitempty"`
	// v2 (opcionais: item gravado pela v1 abre sem eles).
	SectorName string `json:"sector_name,omitempty"`
	Papel      string `json:"papel,omitempty"`
	Origem     string `json:"origem,omitempty"`
	// Reserva: o servidor já desviou e pôs o nome do setor no topo.
	Reserva bool `json:"reserva,omitempty"`
}

// Fila é a pasta com os itens.
type Fila struct {
	mu  sync.Mutex
	dir string
}

// Abrir cria a pasta se preciso.
func Abrir(dir string) (*Fila, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Fila{dir: dir}, nil
}

func (f *Fila) path(id string) string {
	return filepath.Join(f.dir, strings.ReplaceAll(id, string(filepath.Separator), "_")+".json")
}

// Por guarda o item (idempotente pelo id).
func (f *Fila) Por(it Item) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if it.ChegouEm.IsZero() {
		it.ChegouEm = time.Now()
	}
	b, err := json.Marshal(it)
	if err != nil {
		return err
	}
	tmp := f.path(it.ID) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path(it.ID))
}

// Tem diz se o id já está na fila.
func (f *Fila) Tem(id string) bool {
	_, err := os.Stat(f.path(id))
	return err == nil
}

// Tirar remove o item (impresso ou descartado).
func (f *Fila) Tirar(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	err := os.Remove(f.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Lista devolve os itens na ordem de chegada.
func (f *Fila) Lista() ([]Item, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ents, err := os.ReadDir(f.dir)
	if err != nil {
		return nil, err
	}
	var out []Item
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(f.dir, e.Name()))
		if err != nil {
			continue
		}
		var it Item
		if json.Unmarshal(b, &it) != nil || it.ID == "" {
			// Lixo: tira do caminho.
			_ = os.Remove(filepath.Join(f.dir, e.Name()))
			continue
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ChegouEm.Before(out[j].ChegouEm) })
	return out, nil
}

// Falhou anota a tentativa e o erro (o item fica).
func (f *Fila) Falhou(id, erro string) error {
	f.mu.Lock()
	b, err := os.ReadFile(f.path(id))
	f.mu.Unlock()
	if err != nil {
		return err
	}
	var it Item
	if err := json.Unmarshal(b, &it); err != nil {
		return err
	}
	it.Tentativas++
	it.UltimoErro = erro
	return f.Por(it)
}

// Tamanho conta os itens.
func (f *Fila) Tamanho() int {
	l, _ := f.Lista()
	return len(l)
}
