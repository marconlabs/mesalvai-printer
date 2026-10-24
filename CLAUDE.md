# mesalvai-printer — programa de impressão da loja (Go, bandeja Windows/Mac)
Parte do ecossistema Mesalvaí (ver `mesalvai-entregas-api/docs/ecossistema.md`).
Binário único, sem instalador. Recebe pedidos da API por SSE e manda bytes ESC/POS RAW pra impressora, mesmo com o painel fechado. Sem banco: fila em disco.

## Comandos
```bash
go build ./... && go test ./...
go run ./cmd/mesalvai-printer --sem-bandeja                       # sem ícone; Ctrl+C fecha
MESALVAI_API=http://localhost:8080 go run ./cmd/mesalvai-printer  # contra API local
MESALVAI_PRINTER_DIR=/tmp/mp go run ./cmd/mesalvai-printer        # pasta própria (não toca a config real)
GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui" ./cmd/mesalvai-printer
```
Release: `git tag vX.Y.Z && git push --tags` → `.github/workflows/release.yml` (`.exe`, `.dmg`, `checksums.txt`). Só com ordem do dono.

## Regras que não se quebram
- Nomes dos arquivos de release não mudam: o painel baixa `mesalvai-printer-windows-amd64.exe` e `mesalvai-printer-mac.dmg` de `releases/latest`.
- Nunca pedir senha no programa: vínculo é código de 6 letras; o computador ganha chave própria que a loja revoga.
- Compatibilidade v1 ↔ v2: `config.json` antigo (`frente`/`cozinha`) abre na v2 e os campos antigos continuam gravados em espelho. Fila em disco da v1 continua saindo.
- Impressora fora só segura os papéis dela; as outras seguem. Reserva sai com NOME DO SETOR grande + "SAIU NA RESERVA".
- Procurar impressoras só quando o usuário toca (porta 9100, /24 privado, teto 20 s); nunca imprime na busca.
- Publicar versão nova só depois da API com setores no ar.
- Contrato com a API: `mesalvai-entregas-api/docs/arquitetura/mesalvai-printer.md`. Sem segredo no repo.

## Onde está cada coisa
| Caminho | O quê |
|---|---|
| `cmd/mesalvai-printer` | main |
| `internal/agente` | pareamento, stream, config/setores, fila → impressora com reserva, ack; `papel.go` = cabeçalhos ESC/POS |
| `internal/api` | cliente HTTP/SSE |
| `internal/fila` | fila em disco |
| `internal/impressora` | winspool (Windows), CUPS (`lp -o raw`), TCP 9100; `rede.go` = procurar impressoras |
| `internal/ui` | página local (`127.0.0.1`) + bandeja |
| `internal/autostart`, `internal/update`, `internal/logs`, `internal/config` | abrir com o sistema, auto-update (sha256), log, config |

## Docs
Índice: `docs/README.md` · regras: `docs/negocio.md` · problemas: `docs/troubleshooting.md` · assinatura do binário: `docs/assinatura.md` · uso e instalação: `README.md`.
Mudou comportamento → atualize `docs/negocio.md` + `docs/troubleshooting.md`.
