//go:build darwin

package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const label = "br.com.mesalvai.printer"

func plist() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "Library", "LaunchAgents", label+".plist"), nil
}

// Ligar grava o LaunchAgent (RunAtLoad) e carrega.
func Ligar() error {
	exe, err := executavel()
	if err != nil {
		return err
	}
	p, err := plist()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	conteudo := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key><array><string>%s</string><string>--bandeja</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><false/>
</dict></plist>
`, label, exe)
	if err := os.WriteFile(p, []byte(conteudo), 0o644); err != nil {
		return err
	}
	_ = exec.Command("launchctl", "load", "-w", p).Run()
	return nil
}

// Desligar descarrega e apaga o LaunchAgent.
func Desligar() error {
	p, err := plist()
	if err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", "-w", p).Run()
	err = os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Ligado diz se o plist existe.
func Ligado() bool {
	p, err := plist()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}
