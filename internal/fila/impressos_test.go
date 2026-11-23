package fila

import (
	"encoding/json"
	"testing"
)

func TestImpressoLembraEAckPendente(t *testing.T) {
	f, err := Abrir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if f.FoiImpresso("j1") {
		t.Fatal("não imprimiu ainda")
	}
	if err := f.MarcarImpresso("j1", json.RawMessage(`{"status":"impresso"}`)); err != nil {
		t.Fatal(err)
	}
	if !f.FoiImpresso("j1") {
		t.Fatal("deveria lembrar")
	}
	if p := f.AcksPendentes(); len(p) != 1 || p[0].ID != "j1" {
		t.Fatalf("pendentes: %+v", p)
	}
	_ = f.AckFeito("j1")
	if p := f.AcksPendentes(); len(p) != 0 {
		t.Fatalf("depois do ack: %+v", p)
	}
	if !f.FoiImpresso("j1") {
		t.Fatal("segue lembrando dentro da janela")
	}
	// O rastro não aparece como papel esperando.
	if f.Tamanho() != 0 {
		t.Fatal("rastro virou papel na fila")
	}
}
