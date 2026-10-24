#!/usr/bin/env bash
# Ativa os hooks versionados em .githooks/ (pre-push roda o check antes de publicar em main).
set -euo pipefail
cd "$(dirname "$0")/.."
git config core.hooksPath .githooks
chmod +x .githooks/* tool/check.sh
echo "hooks ativos: core.hooksPath=.githooks (pre-push -> tool/check.sh --fast em push para main)"
