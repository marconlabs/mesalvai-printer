package instalar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRaizDoApp(t *testing.T) {
	if got := RaizDoApp("/Volumes/Mesalvaí Printer/Mesalvaí Printer.app/Contents/MacOS/mesalvai-printer"); got != "/Volumes/Mesalvaí Printer/Mesalvaí Printer.app" {
		t.Errorf("app: %q", got)
	}
	if got := RaizDoApp("/Users/ana/Downloads/mesalvai-printer"); got != "" {
		t.Errorf("solto: %q", got)
	}
}

func TestJaEstaNaPasta(t *testing.T) {
	pasta := t.TempDir()
	dentro := filepath.Join(pasta, "mesalvai-printer")
	if err := os.WriteFile(dentro, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !JaEstaNaPasta(dentro, pasta) {
		t.Error("dentro da pasta")
	}
	fora := filepath.Join(t.TempDir(), "mesalvai-printer")
	_ = os.WriteFile(fora, []byte("x"), 0o755)
	if JaEstaNaPasta(fora, pasta) {
		t.Error("fora da pasta")
	}
	if JaEstaNaPasta(fora, "") {
		t.Error("sem pasta")
	}
}

func TestCopiarArquivoSolto(t *testing.T) {
	origem := filepath.Join(t.TempDir(), "mesalvai-printer")
	if err := os.WriteFile(origem, []byte("binario"), 0o755); err != nil {
		t.Fatal(err)
	}
	pasta := filepath.Join(t.TempDir(), "fixa", "Mesalvai Printer")
	novo, err := Copiar(origem, pasta)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(novo)
	if err != nil || string(b) != "binario" {
		t.Fatalf("cópia: %q %v", b, err)
	}
	if st, _ := os.Stat(novo); st.Mode()&0o100 == 0 {
		t.Error("a cópia perdeu a permissão de executar")
	}
	if _, err := os.Stat(origem); err != nil {
		t.Error("o original tem que ficar onde estava")
	}
	// Copiar de novo por cima funciona (versão nova do mesmo programa).
	if _, err := Copiar(origem, pasta); err != nil {
		t.Fatalf("segunda cópia: %v", err)
	}
}
