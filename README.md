# Mesalvaí Printer

Programa de impressão da loja: fica na bandeja do notebook (Windows ou Mac),
abre junto com o sistema e **imprime os pedidos mesmo com o painel fechado**
— comanda do pedido online, papel de preparo da cozinha e etiqueta da caixa,
em impressoras distintas se a loja quiser. Um binário só, sem instalador.

## Instalar em 3 passos

1. **Baixe e abra** a versão mais nova em
   [releases/latest](https://github.com/marconlabs/mesalvai-printer/releases/latest):
   Windows `mesalvai-printer-windows-amd64.exe` · Mac `mesalvai-printer-mac.dmg`
   (arraste pra Aplicativos).
2. O programa mostra um **código de 6 letras**. No painel da loja, abra
   **Configurações › Este aparelho › Impressora › Computador de impressão** e
   digite o código em "Vincular computador".
3. Na janela do programa, escolha a **impressora da frente** (e a da cozinha,
   se tiver) e toque em **Imprimir teste**. Pronto: os pedidos passam a sair
   por aqui; o painel mostra o computador como "Online".

Nunca se digita senha no programa: o vínculo é só o código, e o computador
ganha uma chave própria que a loja revoga em "Desvincular".

### Avisos do sistema na primeira abertura

- **Windows (SmartScreen):** "O Windows protegeu o computador" → *Mais
  informações* › *Executar assim mesmo*. O binário ainda não é assinado
  (ver [docs/assinatura.md](docs/assinatura.md)).
- **Mac (Gatekeeper):** se vier "não pode ser aberto porque é de um
  desenvolvedor não identificado", clique com o botão direito › *Abrir* (uma
  vez só). Releases assinados e notarizados dispensam isso.

## Como funciona

- O servidor da Mesalvaí monta o papel (ESC/POS) e o programa só manda os
  bytes **RAW** pra impressora: Windows pelo spooler (`winspool`, datatype
  RAW), Mac/Linux pelo CUPS (`lp -o raw`), ou rede (`tcp://IP`, porta 9100).
- Conexão contínua (SSE) com reconexão automática (1 s → 30 s) e vigia de
  silêncio (45 s sem ping = reconecta).
- **Fila em disco:** impressora desligada/sem papel = o pedido espera e sai
  quando ela volta (com balão de aviso). Fechou o programa no meio? A fila
  continua no próximo arranque.
- **Quem imprime:** com este computador online, o painel no navegador não
  imprime sozinho (evita papel duplicado). Offline, o navegador volta a
  imprimir como sempre.
- Sem impressora da cozinha escolhida, o preparo sai na da frente.
- Atualiza sozinho pelo [releases/latest](https://github.com/marconlabs/mesalvai-printer/releases/latest)
  (confere o sha256 do `checksums.txt`); vale no próximo arranque.

## Onde ficam as coisas

| O quê | Windows | Mac |
|---|---|---|
| Config, fila e log | `%AppData%\mesalvai-printer\` | `~/Library/Application Support/mesalvai-printer/` |
| Abrir com o sistema | `HKCU\...\Run\MesalvaiPrinter` | `~/Library/LaunchAgents/br.com.mesalvai.printer.plist` |

Janela = página local (`http://127.0.0.1:<porta>/`, só este computador). Na
bandeja: *Abrir*, *Imprimir teste*, *Fechar o programa*.

## Desenvolver

```sh
go build ./... && go test ./...
go run ./cmd/mesalvai-printer --sem-bandeja        # sem ícone; Ctrl+C fecha
MESALVAI_API=http://localhost:8080 go run ./cmd/mesalvai-printer   # contra a API local
MESALVAI_PRINTER_DIR=/tmp/mp go run ./cmd/mesalvai-printer         # pasta própria (não mexe na config real)
GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui" ./cmd/mesalvai-printer
```

Release: `git tag v1.0.0 && git push --tags` → `.github/workflows/release.yml`
gera `.exe`, `.zip`/`.dmg` universal e `checksums.txt`. Contrato com a API:
`mesalvai-entregas-api/docs/arquitetura/mesalvai-printer.md`.

Pacotes: `internal/agente` (pareamento, stream, fila → impressora, ack),
`internal/api` (cliente HTTP/SSE), `internal/fila` (disco), `internal/impressora`
(winspool / CUPS / TCP 9100), `internal/ui` (página local + bandeja),
`internal/autostart`, `internal/update`, `internal/logs`, `internal/config`.
