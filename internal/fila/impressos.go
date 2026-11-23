package fila

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// JanelaImpressos: por quanto tempo o programa lembra que um papel já saiu.
// Se a rede cair entre imprimir e confirmar, o servidor reenvia o papel; sem
// essa memória ele sairia em dobro na cozinha.
const JanelaImpressos = 30 * time.Minute

// Impresso é o rastro de um papel que já saiu: o corpo do ack (JSON) e se o
// servidor já recebeu.
type Impresso struct {
	ID    string          `json:"id"`
	Ack   json.RawMessage `json:"ack"`
	Em    time.Time       `json:"em"`
	AckOk bool            `json:"ack_ok"`
}

func (f *Fila) dirImpressos() string { return filepath.Join(f.dir, "impressos") }

func (f *Fila) pathImpresso(id string) string {
	return filepath.Join(f.dirImpressos(), strings.ReplaceAll(id, string(filepath.Separator), "_")+".json")
}

// MarcarImpresso grava, ANTES de tentar o ack, que o papel saiu.
func (f *Fila) MarcarImpresso(id string, ack json.RawMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.MkdirAll(f.dirImpressos(), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(Impresso{ID: id, Ack: ack, Em: time.Now()})
	if err != nil {
		return err
	}
	tmp := f.pathImpresso(id) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.pathImpresso(id))
}

// AckFeito anota que o servidor recebeu o ack.
func (f *Fila) AckFeito(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, err := os.ReadFile(f.pathImpresso(id))
	if err != nil {
		return nil
	}
	var r Impresso
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	r.AckOk = true
	nb, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return os.WriteFile(f.pathImpresso(id), nb, 0o600)
}

// FoiImpresso diz se o papel saiu há menos de JanelaImpressos.
func (f *Fila) FoiImpresso(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, err := os.ReadFile(f.pathImpresso(id))
	if err != nil {
		return false
	}
	var r Impresso
	if json.Unmarshal(b, &r) != nil {
		return false
	}
	return time.Since(r.Em) < JanelaImpressos
}

// AcksPendentes devolve os papéis que saíram e cujo ack o servidor ainda não
// recebeu. De passagem, apaga o rastro velho (fora da janela e já confirmado).
func (f *Fila) AcksPendentes() []Impresso {
	f.mu.Lock()
	defer f.mu.Unlock()
	ents, err := os.ReadDir(f.dirImpressos())
	if err != nil {
		return nil
	}
	var out []Impresso
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p := filepath.Join(f.dirImpressos(), e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var r Impresso
		if json.Unmarshal(b, &r) != nil || r.ID == "" {
			_ = os.Remove(p)
			continue
		}
		velho := time.Since(r.Em) >= JanelaImpressos
		switch {
		case r.AckOk && velho:
			_ = os.Remove(p)
		case !r.AckOk && time.Since(r.Em) >= 24*time.Hour:
			_ = os.Remove(p) // o servidor já desistiu desse papel faz tempo
		case !r.AckOk:
			out = append(out, r)
		}
	}
	return out
}
