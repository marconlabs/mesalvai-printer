# Assinatura dos binários

## Hoje
- **Windows:** sem assinatura. O SmartScreen mostra "editor desconhecido" na
  1ª execução (*Mais informações › Executar assim mesmo*). O download e a
  atualização conferem o sha256 publicado no `checksums.txt` do release.
- **Mac:** assina e notariza **só se** os segredos existirem no repositório
  (`MAC_CERT_P12`, `MAC_CERT_PASSWORD`, `APPLE_ID`, `APPLE_TEAM_ID`,
  `APPLE_APP_PASSWORD`). Sem eles, o `.app` sai sem assinatura (botão direito
  › Abrir). Nunca coloque o certificado no repo: só em *Settings › Secrets*.

## Windows depois: Azure Trusted Signing
1. Conta Azure → recurso **Trusted Signing** (região East US/West Europe),
   validação de identidade da marconlabs (CNPJ), perfil de certificado
   *Public Trust*.
2. Service principal com papel *Trusted Signing Certificate Profile Signer*;
   guardar `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET` como
   segredos do Actions.
3. No `release.yml`, job `windows` passa a rodar em `windows-latest` e, depois
   do build, a action `azure/trusted-signing-action@v0`:
   ```yaml
   - uses: azure/trusted-signing-action@v0
     with:
       azure-tenant-id: ${{ secrets.AZURE_TENANT_ID }}
       azure-client-id: ${{ secrets.AZURE_CLIENT_ID }}
       azure-client-secret: ${{ secrets.AZURE_CLIENT_SECRET }}
       endpoint: https://eus.codesigning.azure.net/
       trusted-signing-account-name: <conta>
       certificate-profile-name: <perfil>
       files-folder: dist
       files-folder-filter: exe
   ```
4. Assinado + reputação acumulada = SmartScreen para de avisar.

Alternativa: certificado EV/OV em token (Certum, SSL.com) com `signtool`
no runner Windows — mais caro e com hardware.

## Mac: o que é preciso
Apple Developer Program (US$ 99/ano) → certificado *Developer ID
Application* exportado em `.p12` (base64 no segredo) + senha de app do
Apple ID para o `notarytool`. O workflow já faz `codesign --options runtime`,
`notarytool submit --wait` e `stapler`.
