// Package update busca a versão nova nos releases do GitHub
// (marconlabs/mesalvai-printer) e troca o binário no lugar (minio/selfupdate),
// conferindo o sha256 do `checksums.txt` do release. Sem assinatura de código
// por enquanto: a conferência é o hash publicado junto do release.
package update

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/minio/selfupdate"
)

// Repo é o repositório dos releases.
const Repo = "marconlabs/mesalvai-printer"

// Release é o que importa do JSON do GitHub.
type Release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// NomeDoBinario é o nome do asset pra este SO/arquitetura.
func NomeDoBinario() string {
	switch runtime.GOOS {
	case "windows":
		return "mesalvai-printer-windows-amd64.exe"
	case "darwin":
		return "mesalvai-printer-darwin-universal"
	default:
		return "mesalvai-printer-" + runtime.GOOS + "-" + runtime.GOARCH
	}
}

// Ultima consulta o release mais novo.
func Ultima(ctx context.Context, cl *http.Client) (Release, error) {
	var r Release
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+Repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "mesalvai-printer")
	res, err := cl.Do(req)
	if err != nil {
		return r, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return r, fmt.Errorf("github status %d", res.StatusCode)
	}
	return r, json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&r)
}

// Nova compara versões "v1.2.3" (só pelo número).
func Nova(atual, tag string) bool {
	a, b := partes(atual), partes(tag)
	for i := 0; i < 3; i++ {
		if b[i] != a[i] {
			return b[i] > a[i]
		}
	}
	return false
}

func partes(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out [3]int
	for i, p := range strings.SplitN(v, ".", 3) {
		fmt.Sscanf(p, "%d", &out[i])
	}
	return out
}

// Aplicar baixa o binário do release, confere o sha256 e troca no lugar.
// Devolve a tag instalada. O programa precisa reiniciar depois.
func Aplicar(ctx context.Context, cl *http.Client, r Release) (string, error) {
	nome := NomeDoBinario()
	var urlBin, urlSum string
	for _, a := range r.Assets {
		switch a.Name {
		case nome:
			urlBin = a.URL
		case "checksums.txt":
			urlSum = a.URL
		}
	}
	if urlBin == "" || urlSum == "" {
		return "", errors.New("release sem o binário deste sistema ou sem checksums.txt")
	}
	esperado, err := hashEsperado(ctx, cl, urlSum, nome)
	if err != nil {
		return "", err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, urlBin, nil)
	res, err := cl.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("download status %d", res.StatusCode)
	}
	sum, err := hex.DecodeString(esperado)
	if err != nil {
		return "", err
	}
	if err := selfupdate.Apply(res.Body, selfupdate.Options{Hash: 0, Checksum: sum}); err != nil {
		if rerr := selfupdate.RollbackError(err); rerr != nil {
			return "", fmt.Errorf("atualização falhou e não deu pra voltar: %v", rerr)
		}
		return "", err
	}
	return r.TagName, nil
}

func hashEsperado(ctx context.Context, cl *http.Client, url, nome string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	res, err := cl.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) >= 2 && strings.TrimPrefix(f[1], "*") == nome {
			return f[0], nil
		}
	}
	return "", fmt.Errorf("checksums.txt sem %s", nome)
}

// Verificar roda de tempos em tempos: nova versão → aplica e avisa (reinicia
// no próximo arranque; `reiniciar` pode forçar).
func Verificar(ctx context.Context, atual string, aplicar bool, avisar func(tag string)) {
	cl := &http.Client{Timeout: 2 * time.Minute}
	r, err := Ultima(ctx, cl)
	if err != nil || !Nova(atual, r.TagName) {
		return
	}
	if !aplicar {
		avisar(r.TagName)
		return
	}
	if tag, err := Aplicar(ctx, cl, r); err == nil {
		avisar(tag)
	}
}

// Sha256 de um arquivo (usado pelo workflow local / testes).
func Sha256(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
