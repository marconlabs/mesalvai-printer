//go:build windows

package autostart

import "golang.org/x/sys/windows/registry"

const chave = `Software\Microsoft\Windows\CurrentVersion\Run`

// Ligar grava HKCU\...\Run\MesalvaiPrinter = "<exe>" --bandeja.
func Ligar() error {
	exe, err := executavel()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, chave, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(Nome, `"`+exe+`" --bandeja`)
}

// Desligar apaga a chave.
func Desligar() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, chave, registry.SET_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	err = k.DeleteValue(Nome)
	if err == registry.ErrNotExist {
		return nil
	}
	return err
}

// Ligado diz se a chave existe.
func Ligado() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, chave, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(Nome)
	return err == nil
}
