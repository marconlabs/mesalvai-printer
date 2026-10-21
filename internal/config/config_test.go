package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// config.json gravado pela v1 abre na v2 com os setores preenchidos e os
// campos antigos mantidos (o status continua mandando front/kitchen).
func TestMigraConfigDaV1(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESALVAI_PRINTER_DIR", dir)
	t.Setenv("MESALVAI_API", "")
	v1 := `{"api":"https://entregas.mesalvai.com.br","chave":"mpk_x","loja_nome":"Loja","frente":"EPSON","cozinha":"tcp://192.168.0.9","auto_start":true,"auto_update":true,"porta_ui":5123,"abrir_ao_iniciar":true}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(v1), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Abrir()
	if err != nil {
		t.Fatal(err)
	}
	c := s.Get()
	if c.Chave != "mpk_x" || c.Frente != "EPSON" || c.Setor("frente").Impressora != "EPSON" || c.Setor("cozinha").Impressora != "tcp://192.168.0.9" {
		t.Fatalf("migração: %+v", c)
	}
	if l := c.Lista(); len(l) != 2 || l[0].Key != "frente" || l[1].Key != "cozinha" {
		t.Fatalf("lista padrão: %+v", l)
	}
	// Mexeu no campo antigo (rota /escolher da v1) → setor acompanha.
	_ = s.Update(func(c *Config) { c.Frente = "OUTRA" })
	if s.Get().Setor("frente").Impressora != "OUTRA" {
		t.Fatal("campo antigo → setor")
	}
	// Mexeu no setor → campo antigo acompanha; vínculo vazio some.
	_ = s.Update(func(c *Config) {
		c.Setores["cozinha"] = SetorLocal{}
		c.Setores["frente"] = SetorLocal{Impressora: "EPSON", Reserva: "COZ"}
		c.Setores["bar"] = SetorLocal{Impressora: "BAR"}
	})
	c = s.Get()
	if c.Frente != "EPSON" || c.Cozinha != "" || c.Setor("frente").Reserva != "COZ" {
		t.Fatalf("setor → campo antigo: %+v", c)
	}
	if _, ok := c.Setores["cozinha"]; ok {
		t.Fatal("vínculo vazio devia sumir")
	}
	// Setor vinculado fora da lista aparece (com a chave como nome).
	if l := c.Lista(); len(l) != 3 || l[2].Key != "bar" {
		t.Fatalf("lista: %+v", l)
	}
	// No disco: os campos da v1 continuam lá (v1 reinstalada abre igual).
	b, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	var disco map[string]any
	_ = json.Unmarshal(b, &disco)
	if disco["frente"] != "EPSON" || disco["setores"] == nil {
		t.Fatalf("disco: %s", b)
	}
	// Reabrir dá o mesmo.
	s2, _ := Abrir()
	if s2.Get().Setor("bar").Impressora != "BAR" || s2.Get().Setor("frente").Reserva != "COZ" {
		t.Fatalf("reabrir: %+v", s2.Get())
	}
	// Get devolve cópia: mexer no mapa não muda o guardado.
	g := s2.Get()
	g.Setores["bar"] = SetorLocal{Impressora: "X"}
	if s2.Get().Setor("bar").Impressora != "BAR" {
		t.Fatal("Get vazou o mapa")
	}
}
