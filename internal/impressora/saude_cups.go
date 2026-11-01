//go:build !windows

package impressora

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// lerSistema lê o estado da fila do CUPS (`lpstat -l -p`, em inglês fixo).
// Só vale o que o driver/backend conta; ainda sem teste com impressora USB
// desligada no Mac.
func lerSistema(ctx context.Context, nome string) Leitura {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "lpstat", "-l", "-p", nome)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	out, err := cmd.Output()
	if err != nil {
		return Leitura{Situacao: SemResposta, Detalhe: "impressora não encontrada no sistema"}
	}
	return interpretarCups(string(out))
}

func interpretarCups(saida string) Leitura {
	s := strings.ToLower(saida)
	switch {
	case strings.Contains(s, "cover-open") || strings.Contains(s, "door-open"):
		return Leitura{Situacao: TampaAberta, Lido: true, Detalhe: "tampa aberta"}
	case strings.Contains(s, "media-empty") || strings.Contains(s, "media-needed") || strings.Contains(s, "media-jam"):
		return Leitura{Situacao: SemPapel, Lido: true, Detalhe: "sem papel"}
	case strings.Contains(s, " disabled") || strings.Contains(s, "offline-report") || strings.Contains(s, "connecting-to-device"):
		return Leitura{Situacao: SemResposta, Lido: true, Detalhe: "desligada ou desconectada (o sistema marcou parada)"}
	}
	return Leitura{Situacao: Pronta, Lido: true}
}
