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
