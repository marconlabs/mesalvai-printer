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

// BaseAPI é a API do GitHub (variável só pra teste apontar pra um servidor falso).
var BaseAPI = "https://api.github.com"

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
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, BaseAPI+"/repos/"+Repo+"/releases/latest", nil)
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

// Resultado de uma conferência de versão nova (vai pro log e pra janela:
// "Versão v2.0.1 · conferida há 2 h").
type Resultado struct {
	// Estado: atual | instalada | disponivel | erro.
	Estado string
	// Tag: versão achada no release (vazio se nem chegou a consultar).
	Tag string
	// Erro: motivo curto em português (só quando Estado = erro).
	Erro string
}

// Verificar confere o release mais novo: nova versão → aplica (se `aplicar`)
// e avisa; o programa troca no próximo arranque. Sempre devolve o que
// aconteceu, inclusive a falha (antes ficava muda e o suporte não sabia por
// que a loja não atualizou).
func Verificar(ctx context.Context, atual string, aplicar bool, avisar func(tag string)) Resultado {
	cl := &http.Client{Timeout: 2 * time.Minute}
	r, err := Ultima(ctx, cl)
	if err != nil {
		return Resultado{Estado: "erro", Erro: "sem resposta do GitHub: " + err.Error()}
	}
	if !Nova(atual, r.TagName) {
		return Resultado{Estado: "atual", Tag: r.TagName}
	}
	if !aplicar {
		avisar(r.TagName)
		return Resultado{Estado: "disponivel", Tag: r.TagName}
	}
	tag, err := Aplicar(ctx, cl, r)
	if err != nil {
		return Resultado{Estado: "erro", Tag: r.TagName, Erro: MotivoDaFalha(err)}
	}
	avisar(tag)
	return Resultado{Estado: "instalada", Tag: tag}
}

// MotivoDaFalha traduz a falha de Aplicar numa frase curta pra janela e pro log.
func MotivoDaFalha(err error) string {
	m := err.Error()
	switch {
	case strings.Contains(m, "sem o binário") || strings.Contains(m, "sem checksums"):
		return "o release novo ainda não tem o arquivo deste sistema"
	case strings.Contains(m, "checksum"):
		return "checksum não confere (download corrompido); tento de novo na próxima conferência"
	}
	return m
}

// Sha256 de um arquivo (usado pelo workflow local / testes).
func Sha256(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
