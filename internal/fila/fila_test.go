package fila

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFilaEmDisco(t *testing.T) {
	f, err := Abrir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	_ = f.Por(Item{ID: "b", Target: "frente", PayloadB64: "x", ChegouEm: t0.Add(time.Second)})
	_ = f.Por(Item{ID: "a", Target: "frente", PayloadB64: "x", ChegouEm: t0})
	_ = f.Por(Item{ID: "a", Target: "frente", PayloadB64: "x", ChegouEm: t0}) // idempotente
	l, _ := f.Lista()
	if len(l) != 2 || l[0].ID != "a" || l[1].ID != "b" {
		t.Fatalf("ordem: %+v", l)
	}
	if !f.Tem("a") || f.Tem("zz") {
		t.Fatal("Tem")
	}
	_ = f.Falhou("a", "sem papel")
	l, _ = f.Lista()
	if l[0].Tentativas != 1 || l[0].UltimoErro != "sem papel" {
		t.Fatalf("falhou: %+v", l[0])
	}
	_ = f.Tirar("a")
	_ = f.Tirar("a")
	if f.Tamanho() != 1 {
		t.Fatal("tirar")
	}
}

// Item gravado pela v1 (sem os campos de setor) continua abrindo.
func TestItemDaV1AbreNaV2(t *testing.T) {
	dir := t.TempDir()
	f, _ := Abrir(dir)
	_ = os.WriteFile(filepath.Join(dir, "j1.json"), []byte(`{"id":"j1","target":"cozinha","title":"Pedido #1","copies":1,"payload_b64":"eA==","chegou_em":"2026-10-21T12:00:00Z","tentativas":2}`), 0o600)
	_ = f.Por(Item{ID: "j2", Target: "bar1", SectorName: "Bar", Papel: "preparo", Origem: "mesa", Reserva: true, PayloadB64: "eA=="})
	l, _ := f.Lista()
	if len(l) != 2 || l[0].ID != "j1" || l[0].Tentativas != 2 || l[0].SectorName != "" {
		t.Fatalf("v1: %+v", l)
	}
	if l[1].SectorName != "Bar" || !l[1].Reserva || l[1].Origem != "mesa" {
		t.Fatalf("v2: %+v", l[1])
	}
}
