package fila

import (
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
