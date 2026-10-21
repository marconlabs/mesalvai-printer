// Package impressora manda bytes ESC/POS (RAW) pra impressora do sistema ou
// pra rede (TCP 9100). Nada de interpretar o papel: o servidor já montou.
//
// Nome da impressora:
//   - "tcp://192.168.0.50" ou "tcp://host:9100" → rede, porta 9100;
//   - qualquer outro → impressora do sistema (winspool no Windows, CUPS `lp`
//     no Mac/Linux), pelo nome como aparece em Listar().
package impressora

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// ErrSemImpressora: nenhuma escolhida pra este alvo.
var ErrSemImpressora = errors.New("nenhuma impressora escolhida")

// Info de uma impressora do sistema.
type Info struct {
	Nome   string `json:"nome"`
	Padrao bool   `json:"padrao"`
}

// Listar as impressoras do sistema (sem as de rede digitadas à mão).
func Listar() ([]Info, error) {
	return listarSistema()
}

// Imprimir manda os bytes RAW pra impressora `nome`. Rede = TCP 9100.
func Imprimir(ctx context.Context, nome string, dados []byte) error {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return ErrSemImpressora
	}
	if strings.HasPrefix(strings.ToLower(nome), "tcp://") {
		return imprimirRede(ctx, nome[len("tcp://"):], dados)
	}
	return imprimirSistema(ctx, nome, dados)
}

func imprimirRede(ctx context.Context, host string, dados []byte) error {
	if !strings.Contains(host, ":") {
		host += ":9100"
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return fmt.Errorf("impressora de rede %s: %w", host, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	// Blocos pequenos: impressora de rede barata engasga com tudo de uma vez.
	for i := 0; i < len(dados); i += 4096 {
		fim := i + 4096
		if fim > len(dados) {
			fim = len(dados)
		}
		if _, err := conn.Write(dados[i:fim]); err != nil {
			return fmt.Errorf("impressora de rede %s: %w", host, err)
		}
	}
	return nil
}
