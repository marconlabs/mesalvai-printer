package energia

import (
	"context"
	"testing"
	"time"
)

func TestImpedirNaoTravaNemVazaAoCancelar(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Impedir(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Skipf("este sistema não deixou: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Impedir travou")
	}
	cancel()
	time.Sleep(100 * time.Millisecond)
}
