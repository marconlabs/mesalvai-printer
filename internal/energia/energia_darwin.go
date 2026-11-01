//go:build darwin

package energia

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// caffeinate -i -s -w <pid>: sem repouso por inatividade (e, na tomada, sem
// repouso do sistema) enquanto este processo existir. A tela pode apagar.
func impedir(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "caffeinate", "-i", "-s", "-w", strconv.Itoa(os.Getpid()))
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("caffeinate: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
