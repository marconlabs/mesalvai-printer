package impressora

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"
)

// PortaRAW é a porta padrão de impressora de rede (JetDirect/RAW).
const PortaRAW = 9100

// Varredura procura impressoras de rede com a porta RAW aberta. Só roda
// quando a pessoa toca em "Procurar impressoras" (nunca sozinha).
type Varredura struct {
	// Hosts a testar; nil = sub-redes IPv4 privadas das interfaces (HostsPrivados).
	Hosts func() []string
	// Porta (padrão 9100).
	Porta int
	// Timeout por host (padrão 300 ms).
	Timeout time.Duration
	// Concorrencia máxima de conexões abertas ao mesmo tempo (padrão 64).
	Concorrencia int
	// Dial (padrão net.Dialer); injetável pra teste.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

// ProcurarNaRede roda a varredura padrão (sub-redes privadas, porta 9100).
func ProcurarNaRede(ctx context.Context) []Info {
	return Varredura{}.Procurar(ctx)
}

// Procurar testa cada host e devolve as que atenderam, como
// "tcp://IP:porta" / "Rede IP", em ordem de IP.
func (v Varredura) Procurar(ctx context.Context) []Info {
	if v.Hosts == nil {
		v.Hosts = HostsPrivados
	}
	if v.Porta == 0 {
		v.Porta = PortaRAW
	}
	if v.Timeout == 0 {
		v.Timeout = 300 * time.Millisecond
	}
	if v.Concorrencia <= 0 {
		v.Concorrencia = 64
	}
	if v.Dial == nil {
		d := &net.Dialer{}
		v.Dial = d.DialContext
	}
	hosts := v.Hosts()
	var (
		mu     sync.Mutex
		achou  []string
		wg     sync.WaitGroup
		vagas  = make(chan struct{}, v.Concorrencia)
		testar = func(h string) {
			defer wg.Done()
			defer func() { <-vagas }()
			c, cancel := context.WithTimeout(ctx, v.Timeout)
			defer cancel()
			conn, err := v.Dial(c, "tcp", net.JoinHostPort(h, fmt.Sprint(v.Porta)))
			if err != nil {
				return
			}
			_ = conn.Close()
			mu.Lock()
			achou = append(achou, h)
			mu.Unlock()
		}
	)
	for _, h := range hosts {
		if ctx.Err() != nil {
			break
		}
		vagas <- struct{}{}
		wg.Add(1)
		go testar(h)
	}
	wg.Wait()
	sort.Slice(achou, func(i, j int) bool { return ipNum(achou[i]) < ipNum(achou[j]) })
	out := make([]Info, 0, len(achou))
	for _, h := range achou {
		nome := fmt.Sprintf("tcp://%s:%d", h, v.Porta)
		out = append(out, Info{Nome: nome, Rotulo: "Rede " + h})
	}
	return out
}

// HostsPrivados lista os IPs das sub-redes IPv4 privadas (10/8, 172.16/12,
// 192.168/16) das interfaces ligadas deste computador — no máximo um /24 por
// interface (o /24 em volta do IP dela). Ignora loopback, link-local e IP
// público, e o próprio IP.
func HostsPrivados() []string {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var redes []*net.IPNet
	for _, it := range ifs {
		if it.Flags&net.FlagUp == 0 || it.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := it.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				redes = append(redes, n)
			}
		}
	}
	return HostsDasRedes(redes)
}

// HostsDasRedes faz a conta de HostsPrivados (pura, testável).
func HostsDasRedes(redes []*net.IPNet) []string {
	visto := map[string]bool{}
	var out []string
	for _, n := range redes {
		ip := n.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || !ip.IsPrivate() {
			continue
		}
		uns, bits := n.Mask.Size()
		if bits != 32 {
			continue
		}
		if uns < 24 {
			uns = 24 // rede grande: só o /24 em volta do IP (nada de varrer 65 mil)
		}
		mask := net.CIDRMask(uns, 32)
		base := binary.BigEndian.Uint32(ip.Mask(mask))
		tam := uint32(1) << (32 - uns)
		for i := uint32(1); i+1 < tam; i++ { // sem endereço de rede e broadcast
			h := make(net.IP, 4)
			binary.BigEndian.PutUint32(h, base+i)
			s := h.String()
			if h.Equal(ip) || visto[s] {
				continue
			}
			visto[s] = true
			out = append(out, s)
		}
	}
	return out
}

func ipNum(s string) uint32 {
	ip := net.ParseIP(s).To4()
	if ip == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip)
}
