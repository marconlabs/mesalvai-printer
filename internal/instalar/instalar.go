// Package instalar guarda o programa numa pasta fixa na primeira abertura.
// Quem baixa o Mesalvaí Printer costuma rodá-lo de "Downloads" ou direto do
// .dmg; o "abrir com o sistema" aponta para esse caminho e quebra no dia em
// que a pessoa limpa a pasta. A pergunta "Guardar o programa em uma pasta
// fixa?" copia o programa para:
//
//	Windows  %LocalAppData%\Mesalvai Printer
//	Mac      /Applications (se der para escrever) ou ~/Applications
//
// e o início automático passa a apontar para a cópia. Nada é apagado: o
// arquivo baixado fica onde está.
package instalar

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrSemSuporte: neste sistema o programa não se copia (Linux, servidor).
var ErrSemSuporte = errors.New("este sistema não guarda o programa em pasta fixa")

// Pasta é a pasta fixa deste sistema.
func Pasta() (string, error) {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("LocalAppData")
		if base == "" {
			return "", errors.New("não achei a pasta LocalAppData")
		}
		return filepath.Join(base, "Mesalvai Printer"), nil
	case "darwin":
		return pastaDoMac(), nil
	}
	return "", ErrSemSuporte
}

// pastaDoMac: /Applications se o usuário consegue escrever lá (administrador),
// senão a pasta Aplicativos do próprio usuário.
func pastaDoMac() string {
	if escrevivel("/Applications") {
		return "/Applications"
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "/Applications"
	}
	return filepath.Join(h, "Applications")
}

func escrevivel(dir string) bool {
	f, err := os.CreateTemp(dir, ".mesalvai-teste-*")
	if err != nil {
		return false
	}
	nome := f.Name()
	_ = f.Close()
	_ = os.Remove(nome)
	return true
}

// RaizDoApp devolve o "X.app" que contém o executável (Mac), ou "".
func RaizDoApp(exe string) string {
	p := filepath.Clean(exe)
	for d := p; d != "/" && d != "." && d != filepath.Dir(d); d = filepath.Dir(d) {
		if strings.HasSuffix(strings.ToLower(d), ".app") {
			return d
		}
	}
	return ""
}

// JaEstaNaPasta diz se o executável já mora dentro da pasta fixa.
func JaEstaNaPasta(exe, pasta string) bool {
	if pasta == "" {
		return false
	}
	e, err1 := filepath.EvalSymlinks(exe)
	p, err2 := filepath.EvalSymlinks(pasta)
	if err1 != nil || err2 != nil {
		e, p = filepath.Clean(exe), filepath.Clean(pasta)
	}
	rel, err := filepath.Rel(p, e)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Oferecer diz se vale perguntar: sistema com suporte, programa fora da pasta fixa.
func Oferecer(exe string) (destino string, ok bool) {
	pasta, err := Pasta()
	if err != nil || exe == "" || JaEstaNaPasta(exe, pasta) {
		return "", false
	}
	return pasta, true
}

// Copiar guarda o programa em `pasta` e devolve o caminho do executável novo.
// No Mac, quem roda de dentro de um .app copia o .app inteiro (ditto).
func Copiar(exe, pasta string) (string, error) {
	if err := os.MkdirAll(pasta, 0o755); err != nil {
		return "", fmt.Errorf("criar a pasta %s: %w", pasta, err)
	}
	if app := RaizDoApp(exe); app != "" {
		destino := filepath.Join(pasta, filepath.Base(app))
		_ = os.RemoveAll(destino)
		if out, err := exec.Command("ditto", app, destino).CombinedOutput(); err != nil {
			return "", fmt.Errorf("copiar o aplicativo: %v %s", err, strings.TrimSpace(string(out)))
		}
		rel, _ := filepath.Rel(app, exe)
		return filepath.Join(destino, rel), nil
	}
	destino := filepath.Join(pasta, nomeDoArquivo(exe))
	if err := copiarArquivo(exe, destino); err != nil {
		return "", err
	}
	return destino, nil
}

func nomeDoArquivo(exe string) string {
	if runtime.GOOS == "windows" {
		return "mesalvai-printer.exe"
	}
	return "mesalvai-printer"
}

func copiarArquivo(origem, destino string) error {
	in, err := os.Open(origem)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := destino + ".novo"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, destino); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("já existe uma cópia em uso em %s: feche-a e tente de novo (%v)", destino, err)
	}
	return nil
}
