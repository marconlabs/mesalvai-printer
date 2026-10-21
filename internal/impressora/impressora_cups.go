//go:build !windows

package impressora

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Mac/Linux: CUPS. `lp -d <fila> -o raw` manda os bytes sem o filtro
// (ESC/POS como está). A impressora precisa estar instalada no sistema
// (Ajustes › Impressoras; térmica USB costuma entrar como "Generic").

func imprimirSistema(ctx context.Context, nome string, dados []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "lp", "-d", nome, "-o", "raw", "-t", "Mesalvai Printer", "-s")
	cmd.Stdin = bytes.NewReader(dados)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(out.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("lp em %q: %s", nome, msg)
	}
	return nil
}

func listarSistema() ([]Info, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "lpstat", "-p").Output()
	if err != nil {
		// Sem CUPS/sem impressora: lista vazia não é erro.
		if _, ok := err.(*exec.ExitError); ok {
			return nil, nil
		}
		return nil, err
	}
	padrao := ""
	if d, err := exec.CommandContext(ctx, "lpstat", "-d").Output(); err == nil {
		s := strings.TrimSpace(string(d))
		if i := strings.LastIndex(s, ": "); i >= 0 {
			padrao = s[i+2:]
		}
	}
	var infos []Info
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		// "printer <nome> is idle. ..." (inglês) / "impressora <nome> ..." (pt)
		if len(f) >= 2 && (f[0] == "printer" || f[0] == "impressora") {
			infos = append(infos, Info{Nome: f[1], Padrao: f[1] == padrao})
		}
	}
	_ = os.Getenv
	return infos, nil
}
