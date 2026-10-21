# Mesalvaí Printer

Programa de impressão da loja: fica na bandeja do notebook (Windows ou Mac),
abre junto com o sistema e **imprime os pedidos mesmo com o painel fechado**
— comanda do pedido online, papel de preparo, mesa/salão e balcão, cada
**setor** (Caixa/Frente, Cozinha, Bar, Chapa…) na impressora que a loja
quiser, com **impressora reserva** por setor. Um binário só, sem instalador.

## Instalar em 3 passos

1. **Baixe e abra** a versão mais nova em
   [releases/latest](https://github.com/marconlabs/mesalvai-printer/releases/latest):
   Windows `mesalvai-printer-windows-amd64.exe` · Mac `mesalvai-printer-mac.dmg`
   (arraste pra Aplicativos).
2. O programa mostra um **código de 6 letras**. No painel da loja, abra
   **Configurações › Este aparelho › Impressora › Computador de impressão** e
   digite o código em "Vincular computador".
3. Na janela do programa, em **Setores**, escolha a impressora de cada setor
   (no mínimo a **Caixa/Frente**) e toque em **Teste <SETOR>**. Pronto: os
   pedidos passam a sair por aqui; o painel mostra o computador como "Online".
   Impressora de rede? Toque em **Procurar impressoras** (ou digite `tcp://IP`).

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
- **Setores (v2):** a lista vem do painel (`GET /printer-agent/config`;
  sempre tem `frente` e `cozinha`, mais as praças do salão). Cada setor tem
  impressora e **reserva** (opcional). Muda na janela = vale na hora e sobe
  pro painel; muda no painel = o servidor manda o evento `config` e o
  programa busca de novo. A lista fica guardada pra janela abrir sem rede.
- **Para onde vai o papel:** setor com impressora aqui → ela. Sem vínculo, a
  cozinha cai na frente (como a v1) e qualquer outro setor cai na frente
  **com o nome do setor grande no topo**.
- **Reserva:** a principal falhou e o setor tem reserva → sai na reserva com
  o NOME DO SETOR grande + "SAIU NA RESERVA" + o motivo, balão na bandeja, e o
  painel fica sabendo (ack `via: reserva`). Sem reserva: espera na fila e
  tenta de novo (v1). Uma impressora fora **só segura os papéis dela**; os
  das outras impressoras continuam saindo, cada impressora na sua ordem.
- **Procurar impressoras** (só quando você toca): testa a porta 9100 nas
  sub-redes privadas deste computador (no máximo um /24 por placa de rede,
  300 ms por IP, 64 por vez). As achadas aparecem como "Rede 192.168.0.50"
  (`tcp://192.168.0.50:9100`) e vão pro painel.
- Vários computadores na mesma loja e várias impressoras por computador: cada
  setor aponta pra qualquer impressora de qualquer computador vinculado.
- Atualiza sozinho pelo [releases/latest](https://github.com/marconlabs/mesalvai-printer/releases/latest)
  (confere o sha256 do `checksums.txt`); vale no próximo arranque.

## Onde ficam as coisas

| O quê | Windows | Mac |
|---|---|---|
| Config, fila e log | `%AppData%\mesalvai-printer\` | `~/Library/Application Support/mesalvai-printer/` |
| Abrir com o sistema | `HKCU\...\Run\MesalvaiPrinter` | `~/Library/LaunchAgents/br.com.mesalvai.printer.plist` |

Janela = página local (`http://127.0.0.1:<porta>/`, só este computador). Na
bandeja: *Abrir*, *Imprimir teste* (Caixa/Frente), *Fechar o programa*.

## Compatibilidade v1 ↔ v2

- `config.json` da v1 (`frente`/`cozinha`) abre na v2: vira
  `setores.frente`/`setores.cozinha` e os campos antigos continuam gravados
  em espelho (dá pra voltar pra v1 sem perder a escolha).
- O status continua mandando `front_printer`/`kitchen_printer`; a v2 soma
  `setores` (parcial, `null` apaga) e `caps: ["setores","reserva","procurar"]`.
- Servidor sem setores (v1) = o programa não mexe nos vínculos locais.
- Programa v1 ignora o evento `config` e só recebe `frente`/`cozinha`; atualiza
  sozinho pro v2 pelo releases/latest.
- Fila em disco da v1 continua saindo (os campos novos são opcionais).

Regras de negócio: [docs/negocio.md](docs/negocio.md) · problemas comuns:
[docs/troubleshooting.md](docs/troubleshooting.md).

## Desenvolver

```sh
go build ./... && go test ./...
go run ./cmd/mesalvai-printer --sem-bandeja        # sem ícone; Ctrl+C fecha
MESALVAI_API=http://localhost:8080 go run ./cmd/mesalvai-printer   # contra a API local
MESALVAI_PRINTER_DIR=/tmp/mp go run ./cmd/mesalvai-printer         # pasta própria (não mexe na config real)
GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui" ./cmd/mesalvai-printer
```

Release: `git tag v2.0.0 && git push --tags` → `.github/workflows/release.yml`
gera `.exe`, `.zip`/`.dmg` universal e `checksums.txt` (os nomes dos
arquivos não mudam: o painel baixa `mesalvai-printer-windows-amd64.exe` e
`mesalvai-printer-mac.dmg` de `releases/latest`). Publicar a v2 só depois da
API com setores no ar (a v2 fala com API v1, mas sem setores). Contrato com a API:
`mesalvai-entregas-api/docs/arquitetura/mesalvai-printer.md`.

Pacotes: `internal/agente` (pareamento, stream, config/setores, fila →
impressora com reserva, ack; `papel.go` = cabeçalhos ESC/POS),
`internal/api` (cliente HTTP/SSE), `internal/fila` (disco), `internal/impressora`
(winspool / CUPS / TCP 9100; `rede.go` = Procurar impressoras), `internal/ui` (página local + bandeja),
`internal/autostart`, `internal/update`, `internal/logs`, `internal/config`.
