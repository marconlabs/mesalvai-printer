//go:build windows

package energia

import (
	"context"
	"fmt"
	"runtime"

	"golang.org/x/sys/windows"
)

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

var pSetThreadExecutionState = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadExecutionState")

// SetThreadExecutionState vale por thread do sistema: a goroutine trava a
// sua thread, liga o estado e espera o contexto acabar. A tela pode apagar;
// o sistema não dorme.
func impedir(ctx context.Context) error {
	pronto := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if r, _, e := pSetThreadExecutionState.Call(esContinuous | esSystemRequired); r == 0 {
			pronto <- fmt.Errorf("SetThreadExecutionState: %v", e)
			return
		}
		pronto <- nil
		<-ctx.Done()
		pSetThreadExecutionState.Call(esContinuous)
	}()
	return <-pronto
}
