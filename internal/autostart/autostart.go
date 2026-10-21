// Package autostart liga/desliga "abrir com o sistema": chave Run no
// registro (Windows) ou LaunchAgent (Mac). Linux: ~/.config/autostart.
package autostart

import "os"

// Nome do item de inicialização.
const Nome = "MesalvaiPrinter"

func executavel() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return p, nil
}
