package update

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNova(t *testing.T) {
	casos := []struct {
		atual, tag string
		nova       bool
	}{{"v1.0.0", "v1.0.1", true}, {"1.0.0", "v1.0.0", false}, {"v1.2.0", "v1.1.9", false}, {"v0.0.0-dev", "v0.1.0", true}, {"v1.0.0", "v2.0.0", true}}
	for _, c := range casos {
		if got := Nova(c.atual, c.tag); got != c.nova {
			t.Errorf("%s → %s: %v", c.atual, c.tag, got)
		}
	}
}

func githubFalso(t *testing.T, status int, tag string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != 200 {
			w.WriteHeader(status)
			return
		}
		_, _ = fmt.Fprintf(w, `{"tag_name":%q,"assets":[]}`, tag)
	}))
	old := BaseAPI
	BaseAPI = srv.URL
	t.Cleanup(func() { BaseAPI = old; srv.Close() })
}

// Cada conferência devolve o que aconteceu (vai pro log e pra "conferida há N h").
func TestVerificarDevolveOQueAconteceu(t *testing.T) {
	githubFalso(t, 200, "v2.0.1")
	avisos := 0
	r := Verificar(context.Background(), "v2.0.1", true, func(string) { avisos++ })
	if r.Estado != "atual" || r.Tag != "v2.0.1" || avisos != 0 {
		t.Fatalf("já na última: %+v avisos=%d", r, avisos)
	}
	githubFalso(t, 200, "v2.1.0")
	r = Verificar(context.Background(), "v2.0.1", false, func(string) { avisos++ })
	if r.Estado != "disponivel" || r.Tag != "v2.1.0" || avisos != 1 {
		t.Fatalf("só avisar: %+v avisos=%d", r, avisos)
	}
	// Release sem o binário deste sistema: erro explicado, nada de silêncio.
	r = Verificar(context.Background(), "v2.0.1", true, func(string) {})
	if r.Estado != "erro" || r.Tag != "v2.1.0" || r.Erro == "" {
		t.Fatalf("release incompleto: %+v", r)
	}
	githubFalso(t, 503, "")
	r = Verificar(context.Background(), "v2.0.1", true, func(string) {})
	if r.Estado != "erro" || r.Erro == "" {
		t.Fatalf("GitHub fora: %+v", r)
	}
}

func TestMotivoDaFalha(t *testing.T) {
	if got := MotivoDaFalha(errors.New("Updated file has wrong checksum")); got == "" || got[:8] != "checksum" {
		t.Errorf("checksum: %q", got)
	}
	if got := MotivoDaFalha(errors.New("release sem o binário deste sistema ou sem checksums.txt")); got == "" || got[:3] != "o r" {
		t.Errorf("sem binário: %q", got)
	}
}
