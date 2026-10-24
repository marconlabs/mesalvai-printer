#!/usr/bin/env bash
# Qualidade local: formatação (só checa) -> vet -> testes. --fast pula os testes.
# Testes que precisam de banco pulam sozinhos sem TEST_DATABASE_URL.
set -euo pipefail
cd "$(dirname "$0")/.."

FAST=0
for a in "$@"; do
  case "$a" in
    --fast) FAST=1 ;;
    *) echo "uso: tool/check.sh [--fast]" >&2; exit 2 ;;
  esac
done

echo "→ gofmt"
UNFORMATTED="$(gofmt -l .)"
if [ -n "$UNFORMATTED" ]; then
  echo "Arquivos sem gofmt (rode: gofmt -w .):" >&2
  echo "$UNFORMATTED" >&2
  exit 1
fi

echo "→ go vet"
go vet ./...

if [ "$FAST" = 1 ]; then
  echo "→ testes (pulados: --fast)"
else
  echo "→ go test"
  go test ./...
fi

echo "CHECK OK"
