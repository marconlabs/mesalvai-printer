#!/usr/bin/env bash
# Release do Mesalvaí Printer, local (sem GitHub Actions): roda num Mac.
#   scripts/release.sh v2.1.0
# Gera dist/: .exe (Windows), .zip + .dmg (Mac universal), checksums.txt e publica
# no GitHub Release (gh). O programa instalado atualiza lendo `releases/latest`
# e conferindo o sha256. Publicar só com ordem do dono.
#
# Assinatura/notarização do Mac é opcional; exporte para ativar:
#   MAC_SIGN_IDENTITY="Developer ID Application: ..."  (certificado no keychain)
#   APPLE_ID, APPLE_TEAM_ID, APPLE_APP_PASSWORD         (notarytool)
# Sem eles o Mac sai sem assinar (Gatekeeper pede "Abrir assim mesmo").
# Windows sai sem assinatura (SmartScreen avisa; docs/assinatura.md).
set -euo pipefail
cd "$(dirname "$0")/.."

VERSAO="${1:?uso: scripts/release.sh vX.Y.Z}"
[[ "$VERSAO" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "versão no formato vX.Y.Z"; exit 1; }
[ "$(uname)" = "Darwin" ] || { echo "rode num Mac (lipo, hdiutil)"; exit 1; }
[ -z "$(git status --porcelain)" ] || { echo "árvore suja: commite antes"; exit 1; }
git fetch -q origin && git merge-base --is-ancestor origin/main HEAD || { echo "HEAD não contém origin/main"; exit 1; }

go vet ./... && go test ./...

rm -rf dist && mkdir -p dist
LD="-s -w -X main.versao=${VERSAO}"

echo "→ windows-amd64"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$LD -H windowsgui" \
  -o dist/mesalvai-printer-windows-amd64.exe ./cmd/mesalvai-printer

echo "→ darwin universal"
GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -trimpath -ldflags "$LD" -o dist/arm64 ./cmd/mesalvai-printer
GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -trimpath -ldflags "$LD" -o dist/amd64 ./cmd/mesalvai-printer
lipo -create -output dist/mesalvai-printer-darwin-universal dist/arm64 dist/amd64
rm dist/arm64 dist/amd64

APP="dist/Mesalvaí Printer.app"
mkdir -p "$APP/Contents/MacOS"
cp dist/mesalvai-printer-darwin-universal "$APP/Contents/MacOS/mesalvai-printer"
cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>Mesalvaí Printer</string>
  <key>CFBundleDisplayName</key><string>Mesalvaí Printer</string>
  <key>CFBundleIdentifier</key><string>br.com.mesalvai.printer</string>
  <key>CFBundleVersion</key><string>${VERSAO#v}</string>
  <key>CFBundleShortVersionString</key><string>${VERSAO#v}</string>
  <key>CFBundleExecutable</key><string>mesalvai-printer</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSUIElement</key><true/>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
</dict></plist>
PLIST

if [ -n "${MAC_SIGN_IDENTITY:-}" ]; then
  echo "→ assinar"
  codesign --force --options runtime --timestamp --sign "$MAC_SIGN_IDENTITY" "$APP"
  codesign --force --options runtime --timestamp --sign "$MAC_SIGN_IDENTITY" dist/mesalvai-printer-darwin-universal
  if [ -n "${APPLE_ID:-}" ]; then
    echo "→ notarizar"
    ditto -c -k --keepParent "$APP" dist/notarize.zip
    xcrun notarytool submit dist/notarize.zip --apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" --password "$APPLE_APP_PASSWORD" --wait
    xcrun stapler staple "$APP"
    rm dist/notarize.zip
  fi
fi

(cd dist
 ditto -c -k --keepParent "Mesalvaí Printer.app" mesalvai-printer-mac.zip
 hdiutil create -volname "Mesalvaí Printer" -srcfolder "Mesalvaí Printer.app" -ov -format UDZO mesalvai-printer-mac.dmg
 rm -rf "Mesalvaí Printer.app"
 shasum -a 256 mesalvai-printer-* > checksums.txt && cat checksums.txt)

git tag "$VERSAO" && git push origin "$VERSAO"
gh release create "$VERSAO" dist/* --generate-notes --title "$VERSAO"
echo "release $VERSAO publicado"
