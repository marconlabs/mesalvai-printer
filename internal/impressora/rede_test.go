package impressora

import (
	"bytes"
	"context"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

// impressoraDeRedeFalsa escuta em 127.0.0.1 (porta aleatória) e entrega o
// que recebeu em cada conexão.
func impressoraDeRedeFalsa(t *testing.T) (porta int, recebido chan []byte) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	recebido = make(chan []byte, 8)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(3 * time.Second))
				b, _ := io.ReadAll(c)
				recebido <- b
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, recebido
}

func TestVarreduraAchaImpressoraEIgnoraQuemNaoResponde(t *testing.T) {
	porta, _ := impressoraDeRedeFalsa(t)
	d := &net.Dialer{}
	v := Varredura{
		Hosts:   func() []string { return []string{"192.0.2.1", "127.0.0.1"} },
		Porta:   porta,
		Timeout: 200 * time.Millisecond,
		// 192.0.2.1 (TEST-NET) "não responde": segura até o timeout.
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if host, _, _ := net.SplitHostPort(addr); host == "192.0.2.1" {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return d.DialContext(ctx, network, addr)
		},
	}
	inicio := time.Now()
	achou := v.Procurar(context.Background())
	if dur := time.Since(inicio); dur > 2*time.Second {
		t.Fatalf("varredura demorou %s (timeout por host não valeu)", dur)
	}
	want := "tcp://127.0.0.1:" + strconv.Itoa(porta)
	if len(achou) != 1 || achou[0].Nome != want || achou[0].Rotulo != "Rede 127.0.0.1" {
		t.Fatalf("achou: %+v", achou)
	}
}

func TestVarreduraPortaFechada(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	porta := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	achou := Varredura{Hosts: func() []string { return []string{"127.0.0.1"} }, Porta: porta, Timeout: 200 * time.Millisecond}.Procurar(context.Background())
	if len(achou) != 0 {
		t.Fatalf("porta fechada não é impressora: %+v", achou)
	}
}

func TestHostsDasRedesSoPrivadasAte24(t *testing.T) {
	rede := func(s string) *net.IPNet {
		ip, n, err := net.ParseCIDR(s)
		if err != nil {
			t.Fatal(err)
		}
		n.IP = ip
		return n
	}
	hosts := HostsDasRedes([]*net.IPNet{
		rede("192.168.0.10/24"), // privada: 253 (sem rede, broadcast e o próprio)
		rede("10.1.2.3/8"),      // grande: só o /24 em volta
		rede("8.8.8.8/24"),      // pública: fora
		rede("169.254.1.1/16"),  // link-local: fora
		rede("127.0.0.1/8"),     // loopback: fora
		rede("fe80::1/64"),      // IPv6: fora
	})
	if len(hosts) != 253+253 {
		t.Fatalf("hosts: %d", len(hosts))
	}
	tem := map[string]bool{}
	for _, h := range hosts {
		tem[h] = true
	}
	if !tem["192.168.0.1"] || !tem["192.168.0.254"] || tem["192.168.0.10"] || tem["192.168.0.0"] || tem["192.168.0.255"] {
		t.Fatal("/24 privada errada")
	}
	if !tem["10.1.2.1"] || tem["10.1.3.1"] || tem["8.8.8.1"] || tem["169.254.1.2"] {
		t.Fatal("filtro de rede errado")
	}
}

func TestImprimirNaRedeMandaOsBytes(t *testing.T) {
	porta, recebido := impressoraDeRedeFalsa(t)
	dados := bytes.Repeat([]byte("\x1b@PEDIDO 1\n"), 1000) // > 4096: vai em blocos
	if err := Imprimir(context.Background(), "tcp://127.0.0.1:"+strconv.Itoa(porta), dados); err != nil {
		t.Fatal(err)
	}
	select {
	case b := <-recebido:
		if !bytes.Equal(b, dados) {
			t.Fatalf("recebeu %d bytes, mandou %d", len(b), len(dados))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("impressora de rede não recebeu nada")
	}
}
