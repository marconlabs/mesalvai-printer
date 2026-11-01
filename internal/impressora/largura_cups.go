//go:build !windows

package impressora

import (
	"context"
	"os"
	"os/exec"
	"time"
)

// larguraSistema lê o papel padrão do driver no CUPS (`lpoptions -p NOME -l`).
func larguraSistema(ctx context.Context, nome string) int {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "lpoptions", "-p", nome, "-l")
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	return LarguraDoLpoptions(string(out))
}
