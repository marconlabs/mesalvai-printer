package ui

import (
	"context"
	"fmt"

	"fyne.io/systray"
	"github.com/gen2brain/beeep"

	"github.com/marconlabs/mesalvai-printer/internal/agente"
)

// Balao mostra um aviso do sistema (impressora caiu, voltou, desvinculado).
func Balao(titulo, texto string) {
	_ = beeep.Notify(titulo, texto, "")
}

// Bandeja roda o ícone da bandeja (bloqueia a goroutine principal; é assim
// no Mac). `abrir` abre a janela; `sair` encerra tudo.
func Bandeja(ctx context.Context, a *agente.Agente, abrir func(), teste func(), sair func()) {
	onReady := func() {
		systray.SetIcon(icone(false))
		systray.SetTooltip("Mesalvaí Printer")
		mStatus := systray.AddMenuItem("Iniciando…", "")
		mStatus.Disable()
		systray.AddSeparator()
		mAbrir := systray.AddMenuItem("Abrir", "Código, impressoras, teste e log")
		mTeste := systray.AddMenuItem("Imprimir teste", "Papel de teste na impressora da frente")
		systray.AddSeparator()
		mSair := systray.AddMenuItem("Fechar o programa", "Os pedidos param de sair por este computador")
		atualizar := func(e agente.Estado) {
			systray.SetIcon(icone(e.Conectado))
			switch {
			case !e.Vinculado && e.Codigo != "":
				mStatus.SetTitle("Código: " + e.Codigo + " (digite no painel)")
			case !e.Vinculado:
				mStatus.SetTitle("Aguardando a Mesalvaí…")
			case e.Conectado:
				mStatus.SetTitle(fmt.Sprintf("%s · conectado · %d na fila", e.LojaNome, e.NaFila))
			default:
				mStatus.SetTitle(e.LojaNome + " · reconectando…")
			}
			if e.UltimoErro != "" {
				systray.SetTooltip("Mesalvaí Printer — " + e.UltimoErro)
			} else {
				systray.SetTooltip("Mesalvaí Printer")
			}
		}
		atualizar(a.Estado())
		a.Ouvir(atualizar)
		go func() {
			for {
				select {
				case <-ctx.Done():
					systray.Quit()
					return
				case <-mAbrir.ClickedCh:
					abrir()
				case <-mTeste.ClickedCh:
					teste()
				case <-mSair.ClickedCh:
					sair()
				}
			}
		}()
	}
	systray.Run(onReady, func() {})
}
