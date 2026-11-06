# Problemas comuns — Mesalvaí Printer

## Papel saiu na impressora reserva

Veio com o nome do setor grande e "SAIU NA RESERVA" no topo.

- A principal do setor falhou (o motivo está na 3ª linha do papel e no balão).
  Confira papel, tampa, energia e cabo/rede da principal.
- Quando a principal voltar, os próximos papéis saem nela sozinhos (o programa
  tenta a principal primeiro a cada papel). Não precisa fazer nada no programa.
- O painel mostra o alerta "saiu na reserva" com o erro.

## Setor sem impressora saiu na frente com o nome no topo

É de propósito: o setor (ex.: "Chapa") não tem impressora neste computador,
então o papel sai na da Caixa/Frente com o NOME DO SETOR grande pra ninguém
confundir. Pra sair no lugar certo: janela › **Setores** › escolha a
impressora do setor (ou vincule o setor em outro computador no painel).
A cozinha sem impressora sai na frente **sem** cabeçalho (igual à v1).

## Papéis de um setor pararam, os outros continuam

A impressora daquele setor está fora e o setor não tem reserva. Os papéis
dela esperam na fila (na ordem) e saem quando ela voltar; as outras
impressoras não travam. Pra não depender dela: escolha uma **reserva** no
setor.

## "Procurar impressoras" não acha a impressora de rede

- **Outra sub-rede/VLAN**: o programa só procura no /24 do computador (ex.:
  computador 192.168.0.20 procura 192.168.0.1–254). Impressora em 192.168.1.x
  ou numa VLAN separada não aparece → digite `tcp://IP` à mão.
- **Firewall** (do Windows/antivírus ou do roteador) bloqueando a porta 9100.
- **Porta ≠ 9100**: algumas impressoras usam outra porta → digite
  `tcp://IP:porta`.
- **Wi-Fi de convidados / isolamento de clientes** no roteador impede um
  aparelho de ver o outro.
- Impressora desligada ou sem IP: imprima a página de configuração dela
  (geralmente segurando o botão FEED ao ligar) pra ver o IP.

## Impressora de rede velha continua na lista

As achadas em "Procurar impressoras" ficam lembradas (até 30), mesmo sem
responder — pode ser só que estavam desligadas. Se a impressora trocou de IP
(roteador deu outro), procure de novo e vincule o IP novo no setor; o antigo
fica na lista sem uso. Pra limpar de vez: feche o programa e apague o bloco
`"rede"` do `config.json` (pasta do programa: `%AppData%\mesalvai-printer` no
Windows, `~/Library/Application Support/mesalvai-printer` no Mac).

## "Procurar impressoras" demorou e voltou com pouca coisa

A procura tem teto de 20 s. Computador com VPN ou várias placas de rede testa
um /24 por placa; o que respondeu até o teto entra. Toque de novo (as achadas
antes continuam lembradas) ou digite `tcp://IP`.

## A janela não mostra "Setores" (só frente e cozinha)

O programa instalado é a v1. Ele atualiza sozinho (confere 1 min depois de
abrir e a cada 6 h; vale no próximo arranque). Pra adiantar: feche o programa
e baixe de novo em
[releases/latest](https://github.com/marconlabs/mesalvai-printer/releases/latest).

## Setor novo do painel não aparece na janela

O servidor avisa o programa (evento `config`) quando muda. Se o computador
estava sem rede, a lista atualiza ao reconectar. A janela mostra a última
lista guardada enquanto estiver sem rede.

## Mudei a impressora na janela sem internet

Fica gravado neste computador e vale na hora pra impressão; sobe pro painel
quando a conexão voltar.

## A janela diz "Sem papel" ou "Tampa aberta" e o papel não sai

O programa leu o estado da impressora (a cada 30 s e antes de cada papel) e
**não manda** o papel enquanto ela diz isso: iria para o buffer e nunca
sairia. Ponha papel / feche a tampa; o próximo papel da fila sai sozinho (a
fila tenta a cada 15 s). Com **reserva** no setor, o papel já sai na reserva.
Se a impressora diz isso sem ser verdade, é leitura errada do modelo: anote o
modelo e teste na janela com "Teste do setor"; o programa só confia em
resposta com os bits fixos certos.

## "Alcançável (esta impressora não conta papel nem tampa)"

Normal em muita impressora de rede barata: ela abre a porta 9100 mas não
responde ao pedido de estado. O programa só sabe que ela está **ligada na
rede**; sem papel/tampa aberta só aparece no papel que não sai (e aí o erro
vai pro painel). USB com driver: o Windows só conta o que o driver conta.

## "O IP desta impressora pode ter mudado"

A impressora de rede parou de responder (2 leituras seguidas). Roteador que
reiniciou costuma dar outro IP. Toque em **Procurar de novo** e escolha a
nova no setor. Para não acontecer: **reserve o IP da impressora no
roteador** (DHCP reservado) ou ponha IP fixo nela. O programa nunca troca o
IP sozinho.

## O navegador começou a imprimir um setor sozinho

Principal e reserva do setor ficaram fora por mais de 2 min: o servidor
passou aquele setor para o navegador (é melhor sair no navegador do que
nenhum papel). Quando a impressora voltar a responder, o setor volta a ser do
computador sozinho. Se um papel aparece em dobro, confira a versão do
programa (a API só tira da fila o que o programa de versão com `cancel`
entende).

## O computador dormiu e os pedidos pararam de sair

O programa impede o repouso por **inatividade** (Windows/Mac), mas **fechar a
tampa do notebook ainda suspende**. Deixe a tampa aberta e na tomada. Se a
janela mostra "Deixe este computador acordado" com um erro, o sistema não
deixou: nas configurações de energia escolha "nunca suspender".

## O código de vínculo venceu ou digitei errado

O programa mostra a contagem ("Digite no painel em 9:41"). Venceu, ou o painel
disse "Esse código venceu / já foi usado": toque em **Gerar outro código** na
janela (o código também troca sozinho quando vence). O botão **Copiar** evita
errar as letras. Sem internet, o motivo aparece embaixo do código e o programa
tenta de novo sozinho.

## Cliquei em "Fechar o programa" na bandeja e abriu o navegador

É de propósito: a bandeja não fecha sem perguntar, então abre a janela do
programa já com a caixa "Fechar o Mesalvaí Printer?". Confirme ali ou toque em
Cancelar. Fechar = os pedidos deixam de sair por este computador.

## A loja não atualizou o Mesalvaí Printer

Abra a janela: a primeira linha mostra "Versão X · conferida há N h" e, se a
última conferência falhou, o motivo. O mesmo fica no log
(`atualizacao resultado=erro erro=...`; abra pelo botão "Ver log"). Causas: sem
acesso ao GitHub (firewall/proxy), release novo sem o arquivo do sistema ou com
`checksum não confere` (o programa tenta de novo em 6 h). "Atualização sozinha
desligada" = `auto_update` falso no `config.json`. Atualizou e continua na
versão velha: a troca só vale no **próximo arranque**.

## O programa não abre mais com o sistema (apagaram a pasta Downloads)

Na primeira abertura a janela pergunta "Guardar o programa em uma pasta fixa?".
Se foi respondido "Agora não", o início automático aponta para onde o arquivo
foi baixado. Para guardar mesmo assim: apague `pasta_fixa` do `config.json`
(fechando o programa antes) e abra de novo, ou copie o programa para
`%LocalAppData%\Mesalvai Printer` (Mac: `/Applications`) e ligue "Abrir junto
com o sistema" a partir da cópia.

## O painel sugere 58 mm mas o papel é 80 (ou o contrário)

A largura vista pelo driver (`largura_detectada`) é só sugestão. Driver
genérico ("Generic / Text Only") não informa e fica sem sugestão; impressora de
rede nunca informa. A largura que vale é a da loja: confirme no papel de teste
(a linha 1234567890 tem que chegar até a borda).

## O painel diz "Impressora: não configurada" com o Printer imprimindo

Até 06/11 o painel olhava só a impressora do navegador; hoje ele mostra
"Impressora: pelo computador (Mesalvaí Printer)". Se ainda aparecer sem
impressora no setor logo depois de escolher no programa: versão até v2.2.0 só
mandava a saúde do setor na leitura seguinte (~30 s) e, ao tirar a última
impressora, a API ficava com a saúde velha. Atualize o programa (versão nova
recalcula e manda na hora; `saude: {}` quando não sobra impressora). Conferir:
`GET /me/print-agents` → `agents[].saude` e `setores.frente.impressora`.

## O painel nunca sugere a largura do papel (58/80)

Versão até v2.2.0 mandava a largura do driver só dentro de `saude`; a API lê
`largura_detectada` no topo do status. Atualize o programa. Impressora de rede
continua sem sugestão (não tem driver).
