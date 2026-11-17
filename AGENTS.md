# AGENTS.md — plugin de pagamento

Template de parceiro para `payment.route` e `checkout.risk_assess`. A única
dependência de plataforma é `github.com/mira-dev-tech/commerce-ext`. O
contrato de publicação é o mesmo do
[aranduOS-plugin-example](https://github.com/aranduOS/aranduOS-plugin-example/blob/main/AGENTS.md).
O que muda aqui é dinheiro em movimento: as regras de bloqueio são mais
estreitas.

## Regras deste domínio

1. Fail-closed. Timeout, panic ou erro técnico negam. A razão: deixar passar
   um pagamento que o plugin não conseguiu avaliar é pior do que recusar.

2. Sem I/O dentro do hook. 300ms é o tecto, e uma chamada a adquirente não
   cabe. A razão: o hook segura o checkout. Rede fica fora, ou o pedido
   falha com `plugin_failure` para toda a gente.

3. `Require3DS` combina em OR entre plugins. Não apague o 3DS que outro já
   pediu. A razão: um plugin de velocidade não pode desligar o 3DS de valor.

4. A mensagem para o comprador é genérica. A regra que disparou fica no
   código estável (`Denied`), não no texto. A razão: o texto aparece na
   loja; o código é o que o suporte e o front entendem. Publicado, não se
   renomeia.

5. Não clone o monorepo, não use `MIRA_MANIFEST_PATH`, não peça DSN, não
   mande preço nem chave de idempotência, não grave a release com o token
   de parceiro e não marque o pin `active`. As razões estão no AGENTS.md do
   template genérico: este processo não é a API, o digest é o da imagem que
   a plataforma construiu, e o install nasce `installing` até o role `ext_<id>`
   respeitar a RLS.

## Publicar

PR com `go test ./...` e o handshake verdes. Tag `vX.Y.Z`. O workflow da
plataforma constrói esse SHA e grava a release. O `partner_id` tem de estar
ativo no registo dinâmico do Console. Um id que já pertence a outro parceiro
é recusado na gravação. Ligar a uma empresa é outro disparo, e só numa empresa
que o registo atribua a este parceiro.
