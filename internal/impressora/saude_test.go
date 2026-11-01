package impressora

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestInterpretarEscPos(t *testing.T) {
	ok := [3]bool{true, true, true}
	casos := []struct {
		nome                 string
		online, causa, papel byte
		lido                 [3]bool
		sit                  string
		lidoEsperado         bool
	}{
		{"tudo normal", 0x12, 0x12, 0x12, ok, Pronta, true},
		{"tampa aberta", 0x1A, 0x16, 0x12, ok, TampaAberta, true},
		{"sem papel pelo sensor", 0x12, 0x12, 0x72, ok, SemPapel, true},
		{"sem papel pela causa", 0x1A, 0x32, 0x12, ok, SemPapel, true},
		{"quase acabando não bloqueia", 0x12, 0x12, 0x1E, ok, Pronta, true},
		{"offline sem causa conhecida", 0x1A, 0x12, 0x12, ok, SemResposta, true},
		{"erro", 0x12, 0x52, 0x12, ok, SemResposta, true},
		{"resposta lixo", 0xFF, 0xFF, 0xFF, ok, Pronta, false},
		{"nada respondeu", 0, 0, 0, [3]bool{}, Pronta, false},
		{"só a causa respondeu", 0, 0x16, 0, [3]bool{false, true, false}, TampaAberta, true},
	}
	for _, c := range casos {
		l := InterpretarEscPos(c.online, c.causa, c.papel, c.lido)
		if l.Situacao != c.sit || l.Lido != c.lidoEsperado {
			t.Errorf("%s: %+v, queria %s lido=%v", c.nome, l, c.sit, c.lidoEsperado)
		}
	}
}

// impressoraFalsa responde DLE EOT n com os bytes dados.
func impressoraFalsa(t *testing.T, resp map[byte]byte, muda bool) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 3)
				for {
					if _, err := c.Read(buf); err != nil {
						return
					}
					if muda {
						continue
					}
					if b, ok := resp[buf[2]]; ok {
						c.Write([]byte{b})
					}
				}
			}(c)
		}
	}()
	return ln.Addr().String()
}

func TestLerRedeComImpressoraFalsa(t *testing.T) {
	d := (&net.Dialer{Timeout: time.Second}).DialContext
	ctx := context.Background()
	semPapel := impressoraFalsa(t, map[byte]byte{1: 0x12, 2: 0x12, 4: 0x72}, false)
	if l := lerRedeEm(ctx, semPapel, d); l.Situacao != SemPapel || !l.Lido {
		t.Fatalf("sem papel: %+v", l)
	}
	muda := impressoraFalsa(t, nil, true)
	if l := lerRedeEm(ctx, muda, d); l.Situacao != Pronta || l.Lido {
		t.Fatalf("muda: %+v", l)
	}
	l := lerRedeEm(ctx, "127.0.0.1:1", d)
	if l.Situacao != SemResposta {
		t.Fatalf("porta fechada: %+v", l)
	}
}

func TestLerRedeOcupadaNaoAbreSegundaConexao(t *testing.T) {
	host := impressoraFalsa(t, map[byte]byte{1: 0x12}, false)
	m := trancaDe(host)
	m.Lock()
	defer m.Unlock()
	if l := lerRede(context.Background(), host); !l.Ocupada {
		t.Fatalf("devia estar ocupada: %+v", l)
	}
}
