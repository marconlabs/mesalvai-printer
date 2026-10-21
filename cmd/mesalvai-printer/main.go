// Mesalvaí Printer — programa de impressão da loja (Windows/Mac).
//
//	mesalvai-printer              abre a bandeja e a janela no navegador
//	mesalvai-printer --bandeja    só a bandeja (é como abre com o sistema)
//	mesalvai-printer --sem-bandeja  sem ícone (servidor/teste); Ctrl+C fecha
//	mesalvai-printer --versao
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/marconlabs/mesalvai-printer/internal/agente"
	"github.com/marconlabs/mesalvai-printer/internal/autostart"
	"github.com/marconlabs/mesalvai-printer/internal/config"
	"github.com/marconlabs/mesalvai-printer/internal/fila"
	"github.com/marconlabs/mesalvai-printer/internal/logs"
	"github.com/marconlabs/mesalvai-printer/internal/ui"
	"github.com/marconlabs/mesalvai-printer/internal/update"
)

// versao vem do build (-ldflags "-X main.versao=v1.2.3").
var versao = "v0.0.0-dev"

func main() {
	bandeja := flag.Bool("bandeja", false, "abre só na bandeja, sem a janela")
	semBandeja := flag.Bool("sem-bandeja", false, "sem ícone na bandeja (servidor/teste)")
	mostrarVersao := flag.Bool("versao", false, "mostra a versão e sai")
	flag.Parse()
	if *mostrarVersao {
		fmt.Println(versao)
		return
	}
	cfg, err := config.Abrir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	dir, _ := config.Dir()
	log, logPath := logs.Abrir(dir)
	log.Info("mesalvai-printer", "versao", versao, "so", runtime.GOOS, "pasta", dir)
	f, err := fila.Abrir(filepath.Join(dir, "fila"))
	if err != nil {
		log.Error("fila", "erro", err)
		os.Exit(1)
	}
	host, _ := os.Hostname()
	ctx, cancel := context.WithCancel(context.Background())
	avisar := func(t, x string) { log.Info("aviso", "titulo", t, "texto", x); ui.Balao(t, x) }
	a := agente.New(cfg, f, agente.Opcoes{Versao: versao, SO: runtime.GOOS, NomeDoPC: host, Avisar: avisar, Log: log})
	go a.Run(ctx)

	// Primeira vez: liga "abrir com o sistema" (a pessoa desliga na janela).
	c := cfg.Get()
	if c.AutoStart && !autostart.Ligado() {
		if err := autostart.Ligar(); err != nil {
			log.Warn("autostart", "erro", err)
		}
	}
	// Atualização sozinha: confere 1 min depois de abrir e a cada 6 h.
	if c.AutoUpdate && versao != "v0.0.0-dev" {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Minute):
				}
				update.Verificar(ctx, versao, true, func(tag string) {
					avisar("Mesalvaí Printer atualizado", "Versão "+tag+" instalada. Vale no próximo arranque.")
				})
				select {
				case <-ctx.Done():
					return
				case <-time.After(6 * time.Hour):
				}
			}
		}()
	}

	sair := func() {
		log.Info("fechando")
		cancel()
		time.Sleep(300 * time.Millisecond)
		os.Exit(0)
	}
	w := &ui.Web{Agente: a, Cfg: cfg, LogPath: logPath, Sair: sair}
	url, err := w.Servir(ctx)
	if err != nil {
		log.Error("janela", "erro", err)
		os.Exit(1)
	}
	log.Info("janela", "url", url)
	abrir := func() { ui.AbrirNoNavegador(url) }
	// Sem vínculo ainda (ou pediram): mostra a janela com o código.
	if !*bandeja || cfg.Get().Chave == "" {
		abrir()
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		sair()
	}()
	if *semBandeja {
		<-ctx.Done()
		return
	}
	ui.Bandeja(ctx, a, abrir, func() {
		if err := a.TestePapel(ctx, "frente"); err != nil {
			avisar("Teste não saiu", err.Error())
		}
	}, sair)
}
