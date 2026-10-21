//go:build !windows && !darwin

package autostart

import (
	"os"
	"path/filepath"
)

func desktop() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "autostart", Nome+".desktop"), nil
}

// Ligar grava ~/.config/autostart/MesalvaiPrinter.desktop.
func Ligar() error {
	exe, err := executavel()
	if err != nil {
		return err
	}
	p, err := desktop()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte("[Desktop Entry]\nType=Application\nName=Mesalvaí Printer\nExec="+exe+" --bandeja\nX-GNOME-Autostart-enabled=true\n"), 0o644)
}

// Desligar apaga o .desktop.
func Desligar() error {
	p, err := desktop()
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Ligado diz se o .desktop existe.
func Ligado() bool {
	p, err := desktop()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}
