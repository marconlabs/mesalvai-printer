# Qualidade local (check + trava de push)

Não há GitHub Actions; o Coolify publica qualquer push em `main`. A trava é local.

- `make check` (ou `tool/check.sh`): `gofmt -l .` (falha se listar arquivos) -> `go vet ./...` -> `go test ./...`; termina com `CHECK OK`. `--fast` pula os testes.
- Testes que precisam de Postgres pulam sozinhos sem `TEST_DATABASE_URL`. Este repo não usa banco.
- `tool/install-hooks.sh` (uma vez por clone) faz `git config core.hooksPath .githooks`. O `.githooks/pre-push` roda `tool/check.sh --fast` quando o push vai para `refs/heads/main` e bloqueia se falhar.
- Bypass: `git push --no-verify` (só com ordem do dono).
