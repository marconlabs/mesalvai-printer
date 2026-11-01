# Regras de negócio — Mesalvaí Printer

Contrato com a API: `mesalvai-entregas-api/docs/arquitetura/mesalvai-printer.md`
(seção "v2 — setores…").

## Quem imprime

- Computador vinculado e **online** (stream aberto) = o servidor gera o papel
  e manda pra ele; o painel no navegador **não** imprime (sem papel duplicado).
- Nenhum computador online = o navegador imprime como sempre.
- O servidor decide **qual computador** (setor → vínculo, cadeia de reserva do
  servidor); o programa decide **qual impressora deste computador** (abaixo).

## Setores

- Setor = chave (`target` do papel): `frente` (Caixa/Frente), `cozinha`
  (preparo geral) e as **praças do salão** (uuid: Bar, Chapa, Expedição…).
- A lista com nomes vem do servidor (`GET /printer-agent/config`) e fica
  guardada no `config.json` (`setores_lista`) pra janela abrir sem rede.
- Cada setor, em cada computador: **impressora** + **reserva** (opcional).
  Vários computadores na mesma loja e várias impressoras (USB, sistema, rede)
  por computador: cada setor aponta pra qualquer uma.
- Mudou na janela → grava aqui e manda `POST /printer-agent/status {setores}`
  (só o que mudou; `null` = apagou). Sem rede: fica gravado e sobe no próximo
  conectar, **antes** de buscar o config (senão o servidor apagaria a mudança).
- Mudou no painel → o servidor manda o evento `config` → o programa busca o
  config e troca os vínculos locais pelos do servidor.

## Para onde vai um papel (neste computador)

1. Setor com impressora aqui → ela.
2. Sem vínculo:
   - `cozinha` → impressora da frente, sem cabeçalho (igual à v1);
   - qualquer outro setor → impressora da frente **com o NOME DO SETOR grande
     no topo** (pra quem pega o papel saber de onde é);
   - papel que o servidor já desviou (`reserva: true`) já vem com o nome: não
     põe de novo.
3. Sem impressora da frente escolhida → o papel espera na fila e o programa
   avisa "Escolha a impressora da frente".

## Reserva

- A principal falhou (sem papel, desligada, fora da rede) e o setor tem
  reserva → o papel sai na reserva com: NOME DO SETOR grande, "SAIU NA
  RESERVA", o motivo curto e uma linha de traços; depois o papel inteiro.
- Ack: `{status:'impresso', via:'reserva', printer:<reserva>, error:<erro da
  principal>}` → o painel mostra o alerta.
- Balão na bandeja **uma vez por queda** (volta a avisar só depois que a
  principal imprimir de novo).
- Cada passada na fila tenta a principal de novo primeiro (voltou = volta a
  sair nela sozinho).
- Sem reserva (ou a reserva também falhou): comportamento v1 — fica na fila,
  tenta de novo a cada 15 s, balão na primeira falha, erro vai pro painel.

## Ordem

- A ordem de chegada vale **por impressora**: uma impressora fora segura só
  os papéis que iam pra ela; os das outras impressoras continuam saindo.
- Fechou o programa com papel na fila: sai no próximo arranque (papel com mais
  de 15 min não é reenviado pelo servidor, mas o que já está no disco sai).

## Procurar impressoras

- Só quando a pessoa toca em **Procurar impressoras** (nunca sozinho).
- Testa a porta 9100 nas sub-redes IPv4 **privadas** das placas de rede deste
  computador (no máximo o /24 em volta do IP de cada placa; ignora loopback,
  link-local e IP público), 300 ms por IP, até 64 ao mesmo tempo.
- Achou = entra na lista como "Rede 192.168.0.50" (`tcp://192.168.0.50:9100`)
  e vai no `printers` do status (o painel também pode escolher).
- Também dá pra digitar `tcp://IP` (ou `tcp://IP:porta`) à mão.
- **Pedido do painel**: o stream pode trazer o evento opcional
  `event: procurar` (`data: {"agente_id"?, "setor"?}`, corpo pode vir vazio).
  O programa faz a mesma varredura do botão e manda o resultado no `printers`
  do status. `agente_id` de outro computador = ignora; servidor que nunca manda
  o evento não muda nada; programa velho ignora o evento desconhecido.
- A procura **só abre e fecha a conexão** na porta 9100: nunca manda bytes,
  então nada sai impresso durante a procura.
- Teto de **20 s** por procura (várias placas de rede, VPN): o que respondeu
  até ali entra na lista; a janela nunca fica presa em "procurando".
- As achadas ficam **lembradas no config.json** (`rede`, até 30): depois de
  reiniciar o programa continuam na lista e no `printers` do status, mesmo
  sem setor vinculado e mesmo que não respondam na próxima procura (podem
  estar desligadas).

## Computador acordado (P1)

- O programa pede ao sistema para **não entrar em repouso** enquanto roda:
  Windows `SetThreadExecutionState(CONTINUOUS|SYSTEM_REQUIRED)`; Mac
  `caffeinate -i -s -w <pid>` (`internal/energia`). A tela pode apagar.
- **Não vale para tampa fechada**: fechar o notebook ainda suspende (é do
  hardware). A janela diz "deixe na tomada e com a tampa aberta".
- Se o sistema não deixar, a janela mostra o motivo e o status manda
  `sem_sono: false`.

## Saúde da impressora (P2)

- A cada **30 s** (e antes de cada papel) o programa lê o estado das
  impressoras de cada setor, principal e reserva. **Nunca imprime** para ler.
- Rede (TCP 9100): ESC/POS em tempo real, `DLE EOT 1`, `2` e `4`. Só vale
  resposta com os bits fixos certos (`b & 0x93 == 0x12`); lixo conta como "não
  informa". Impressora que abre a porta e não responde fica **pronta, não
  lida** ("alcançável, não conta papel nem tampa"): muita impressora barata é
  assim. Uma conexão por vez por impressora: ler e imprimir nunca disputam.
- USB/sistema: Windows lê as marcas da fila do spooler (`GetPrinter`: porta
  da tampa, sem papel, fora de linha); Mac lê `lpstat -l -p` (`cover-open`,
  `media-empty`, `disabled`). **Só vale o que o driver conta**: com a USB
  desligada o Windows pode continuar dizendo "pronta" (ainda sem teste em loja).
- Situações: `pronta`, `sem_papel`, `tampa_aberta`, `sem_resposta`. "Quase
  acabando" não bloqueia. Falha de rede **avulsa não conta**: `sem_resposta`
  só depois de 2 leituras seguidas (~1 min) ou de um papel que falhou.
- Antes de mandar um papel: se a impressora diz `sem_papel` ou `tampa_aberta`
  (leitura de verdade), o papel **nem é mandado** (iria para o buffer e nunca
  sairia): vira o erro "sem papel"/"tampa aberta", a reserva assume e o
  balão diz o motivo.
- O status traz `saude` por setor com impressora: `estado`, `lido`, `detalhe`,
  `impressora`, `imprime` (principal OU reserva pronta), `na_reserva`,
  `fora_ha_s`, `degradado`, `ip_mudou`, `teste_em`; e `sem_sono`. Manda quando
  muda e, enquanto algum setor não imprime, a cada 30 s.

## "IP mudou?" (P3)

- Impressora **de rede** sem resposta (2 leituras) marca `ip_mudou`: a janela
  pergunta "O IP desta impressora pode ter mudado" com o botão **Procurar de
  novo** (é o mesmo "Procurar impressoras": só quando a pessoa toca, nunca
  imprime). O programa **não troca o IP sozinho**: não há como saber que a
  nova é a mesma impressora.

## Navegador assume o setor (P4)

- Setor sem conseguir imprimir (principal **e** reserva fora) há **2 min**
  (`LimiteDegradar`) manda `degradado: true`. A API deixa de contar o setor
  como "imprime pelo computador": o navegador volta a imprimir esse setor.
- A API manda `event: cancel` com `{job_ids}` dos papéis que o navegador
  assumiu: o programa **tira da fila local sem dar ack** (quem imprimiu foi o
  navegador; evita papel em dobro). Papel que já saiu não volta.
- Voltou (principal ou reserva pronta): `imprime: true`, o contador zera e o
  setor volta a ser do computador.
- Computador sem `caps: saude, cancel` (versão antiga) nunca degrada: o
  servidor só aplica P4 a quem declarou que entende.

## Instalação e teste obrigatório (P5)

- A janela mostra "Para não falhar na hora do pedido" até estar tudo certo:
  vinculado, computador acordado (P1), impressora da frente escolhida e
  **"Imprimir teste" feito em cada setor com impressora**.
- O teste que **saiu** grava a hora em `config.json` (`testes`) e sobe como
  `teste_em`; teste que falhou não conta. Setor sem impressora própria usa o
  teste da frente. Sem teste = o painel mostra "falta o teste".
- Dica na tela: rede é o melhor; **reserve o IP no roteador**; USB pede o
  driver do fabricante antes.

## Largura do papel que o driver informa (F2)

- Para cada impressora do sistema de um setor (USB com driver), o programa lê
  o papel padrão do driver: Windows = `dmPaperWidth` do DEVMODE (décimos de mm;
  se vazio, procura 58/80 no nome do driver); Mac/Linux = papel padrão (`*`)
  em `lpoptions -p NOME -l`. 48–62 mm vale **58**, 70–90 mm vale **80**; folha
  A4 ou etiqueta não conta (0).
- Vai no status como `saude.<setor>.largura_detectada` (58 ou 80; ausente =
  o driver não disse). Impressora de rede (`tcp://`) nunca informa: o painel
  sugere pelo nome/modelo. **É só sugestão**: o programa nunca troca a largura
  da loja; a pessoa confirma no papel de teste.
- A leitura é repetida a cada 10 min por impressora (no Mac cada leitura é um
  processo), num laço próprio que não atrasa a saúde.

## Janela: vínculo, fechar, atualização e pasta fixa (F2)

- **Código de vínculo**: botão **Copiar**, contagem "Digite no painel em 9:41" e
  **Gerar outro código** (descarta o da tela e pede um novo à API; mesma
  frase da API: "Esse código venceu. No programa, toque em Gerar outro código").
  O código novo continua aparecendo sozinho quando vence.
- **Fechar o programa** nunca é direto: a página mostra uma caixa própria
  ("Fechar o Mesalvaí Printer?", **Cancelar** é o padrão, botão vermelho
  "Fechar o programa"). O item da bandeja ("Fechar o programa…") abre a janela
  já nessa caixa (`#fechar`). SIGTERM/Ctrl+C continuam fechando direto.
- **Atualização**: cada conferência (1 min depois de abrir e a cada 6 h) e cada
  falha vai pro log (`atualizacao resultado=atual|instalada|disponivel|erro`)
  e pra janela: "Versão v2.1.0 · conferida há 2 h", ou o motivo da falha
  ("sem resposta do GitHub", "checksum não confere", "o release novo ainda não
  tem o arquivo deste sistema"). Com a atualização sozinha desligada a janela
  diz isso.
- **Pasta fixa (1ª abertura)**: se o programa roda de fora da pasta fixa
  (Windows `%LocalAppData%\Mesalvai Printer`; Mac `/Applications`, ou
  `~/Applications` se não der para escrever), a janela pergunta "Guardar o
  programa em uma pasta fixa?". **Guardar** copia (Mac: o `.app` inteiro),
  aponta o início automático para a cópia, encerra este processo e abre a cópia
  na bandeja. **Agora não** grava `pasta_fixa: "recusada"` e nunca mais
  pergunta. O arquivo baixado não é apagado. Build de desenvolvimento
  (`v0.0.0-dev`) e Linux não perguntam.

## v1 × v2

| | v1 | v2 |
|---|---|---|
| Impressoras | frente + cozinha | um vínculo por setor (frente, cozinha, praças) |
| Reserva | não | por setor, com cabeçalho e ack `via: reserva` |
| Procurar rede | não (só digitar) | botão "Procurar impressoras" |
| Impressora fora | segura a fila inteira | segura só os papéis dela |
| Evento `config` | ignora | busca o config de novo |
| `caps` no status | — | `["setores","reserva","procurar","saude","cancel"]` |

O servidor manda pra um computador sem `caps` (v1) só `frente`/`cozinha`. O
programa v1 atualiza sozinho pra v2 (releases/latest): confere 1 min depois de
abrir e a cada 6 h, troca o binário e **só vale no próximo arranque** (não
reinicia no meio do expediente). O config.json da v1 abre igual na v2
(vínculo, frente e cozinha mantidos; os campos da v1 continuam gravados, então
voltar pra v1 também abre).

**Sem configurar nada, a v2 imprime igual à v1**: setores padrão frente e
cozinha, cozinha sem impressora sai na frente. Servidor antigo (sem
`setores`/`bindings` no `GET /printer-agent/config`) não mexe no que está no
computador; os campos novos do status/ack são ignorados por ele.

### Release v2.0.0

Tag `v2.0.0` em `main` → workflow `release` monta
`mesalvai-printer-windows-amd64.exe`, `mesalvai-printer-darwin-universal`
(o auto-update do Mac usa este), `mesalvai-printer-mac.zip`,
`mesalvai-printer-mac.dmg` e `checksums.txt` (os mesmos nomes da v1.0.0; o
painel baixa por `releases/latest/download/<arquivo>`, então não muda nada lá).
A versão vem da tag (`-X main.versao=v2.0.0`). Mac exige cgo (systray): só
compila no runner macOS.
