// Package logs grava o log do programa em arquivo rotativo (5 MB × 3) e no
// terminal quando houver. Sem segredo no log (a chave nunca é logada).
package logs

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Abrir cria o logger em <dir>/mesalvai-printer.log.
func Abrir(dir string) (*slog.Logger, string) {
	path := filepath.Join(dir, "mesalvai-printer.log")
	rot := &lumberjack.Logger{Filename: path, MaxSize: 5, MaxBackups: 3, MaxAge: 30, Compress: false}
	var w io.Writer = rot
	if fi, err := os.Stdout.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		w = io.MultiWriter(rot, os.Stdout)
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})), path
}
