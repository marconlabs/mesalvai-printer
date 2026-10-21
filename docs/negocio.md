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

## v1 × v2

| | v1 | v2 |
|---|---|---|
| Impressoras | frente + cozinha | um vínculo por setor (frente, cozinha, praças) |
| Reserva | não | por setor, com cabeçalho e ack `via: reserva` |
| Procurar rede | não (só digitar) | botão "Procurar impressoras" |
| Impressora fora | segura a fila inteira | segura só os papéis dela |
| Evento `config` | ignora | busca o config de novo |
| `caps` no status | — | `["setores","reserva","procurar"]` |

O servidor manda pra um computador sem `caps` (v1) só `frente`/`cozinha`. O
programa v1 atualiza sozinho pra v2 (releases/latest).
