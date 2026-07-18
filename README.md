# mira-commerce-plugin-payment

Template de **plugin de meios de pagamento e antifraude** do
[Mira Commerce](https://mira-dev.tech). Clone, rode os testes, troque as
regras pelas suas.

Ele implementa as três decisões mais comuns do domínio:

| Hook | Regra do exemplo | Por que existe |
|------|------------------|----------------|
| `payment.route` | Compra > R$ 1.000 no cartão → **exige 3DS** | O desafio transfere o risco de chargeback ao emissor |
| `payment.route` | **Boleto bloqueado** na 1ª compra | Boleto de cliente novo é risco de calote (reserva estoque e some) |
| `checkout.risk_assess` | > 5 tentativas no pedido → **bloqueia** | Card testing: fraudador testa cartões roubados até um passar |

```bash
go test ./...   # funciona logo após o clone — SDK vendorado
```

> Novo no modelo de plugins? Comece pelo template genérico
> [`mira-commerce-plugin-example`](https://github.com/mira-dev-tech/mira-commerce-plugin-example)
> — ele ensina o contrato (Meta/Init/Register/Shutdown), manifest e ciclo de
> vida. Aqui focamos no que é **específico de pagamento**.

## As 4 leis do domínio de pagamento

Estas não são preferências — são as regras que separam um plugin que você
coloca em produção de um que causa incidente:

### 1. Fail-closed: seu bug vira recusa, não aprovação

`payment.route` e `checkout.risk_assess` são a última porta antes do dinheiro.
Se o handler estourar os **300ms** do hook ou der panic, o core **NEGA** com
`PLUGIN_FAILURE`. É o padrão certo (melhor recusar que aprovar às cegas) — e
tem uma consequência direta: **nunca faça I/O de rede dentro do hook**.

Precisa consultar um antifraude externo? Use o padrão de cache + timeout
próprio + fallback do template de frete
([`mira-commerce-plugin-shipping`](https://github.com/mira-dev-tech/mira-commerce-plugin-shipping)) —
com uma diferença crucial: em frete o fallback é *permitir*; em pagamento,
decida conscientemente qual é o seu fallback seguro.

O teste [`TestSlowHookFailsClosed`](integration_test.go) demonstra isso na
prática.

### 2. Você nunca toca em dado de cartão

PAN/CVV não passam pelo core nem pelo plugin — a tokenização acontece no
adquirente. O hook recebe o **método** (`card`/`pix`/`boleto`), o valor e o
contexto do pedido; nunca o cartão. Se o seu design precisa do PAN, o design
está errado (e o escopo PCI da loja inteira muda).

### 3. Mensagem ao comprador é genérica; o detalhe vai no log

Dizer "bloqueado após 5 tentativas" é entregar o manual de bypass ao
fraudador. O código de bloqueio e a razão vão para o log (para o seu time);
ao comprador, mensagem neutra. Há testes que **falham** se a mensagem vazar a
regra — leia `TestBoletoBlockedOnFirstPurchase`.

### 4. Código de bloqueio é contrato

`PAYMENT_METHOD_UNAVAILABLE`, `RISK_SCORE_HIGH` — o front e o suporte mapeiam
esses códigos para mensagens e fluxos. Renomear quebra a UX de quem já
integrou; trate como API pública (mudou? ADR + migração combinada).

## Semânticas de cadeia que você precisa conhecer

Vários plugins podem responder ao mesmo hook (o seu, os de referência do core,
o de outro parceiro). Prioridade menor roda primeiro.

| Hook | Semântica |
|------|-----------|
| `payment.route` | **Deny para a cadeia** inteira. **`Require3DS` acumula em OR**: se qualquer plugin pedir 3DS, o desafio acontece — um plugin permissivo *não desfaz* o 3DS de outro. Pedir 3DS é sempre seguro. |
| `checkout.risk_assess` | **Primeiro deny vence.** Não conte com "o outro plugin já checou". |

O teste `TestRequire3DSIsOrAcrossPlugins` prova o OR com dois plugins
instalados.

## Configuração

Tudo por `Runtime.Config` (manifest/host) com fallback a env — nada hardcoded:

| Chave (Config) | Env | Default |
|----------------|-----|---------|
| `three_ds_threshold` | `EXAMPLE_PAYMENT_3DS_THRESHOLD` | `1000` |
| `max_attempts` | `EXAMPLE_PAYMENT_MAX_ATTEMPTS` | `5` |
| `block_boleto_first_purchase` | `EXAMPLE_PAYMENT_BLOCK_BOLETO_FIRST` | `true` |

Credenciais (chave de antifraude externo, etc.) vêm de `Runtime.Secrets` —
**nunca** de Config, nunca no código, nunca logadas.

## Testando

| Nível | Arquivo | O que prova |
|-------|---------|-------------|
| Unitário | [`plugin_test.go`](plugin_test.go) | Cada regra isolada + a mensagem não vaza a política |
| Integração | [`integration_test.go`](integration_test.go) | Cadeia real via `plugintest`: 3DS em OR, deny, fail-closed no timeout |

O `plugintest` é o mini Extension Host em memória do SDK — reproduz cadeia por
prioridade, timeout de 300ms e fail-closed **na sua máquina**, sem acesso ao
core.

## Dois modos de rodar

1. **In-process** (padrão hoje): copie o pacote para `extensions/<id>` no core,
   registre no loader e use `runtime.type: in-process` no manifest.
2. **Binário externo** (este template): `go build -o bin/example-payment
   ./cmd/example-payment` e aponte `MIRA_MANIFEST_PATH` para o `manifest.yaml`.
   Valide o protocolo com `./bin/example-payment --commerce-ext-handshake`
   (deve responder `commerce-ext-ok`).

## Caminho para produção

Igual ao do template genérico — com uma exigência a mais, porque é dinheiro:

1. Unidade + integração (`plugintest`) + handshake + manifest verdes.
2. **Revisão em par das regras de bloqueio**: um falso positivo aqui não é bug,
   é receita perdida. Documente a taxa esperada de bloqueio antes de subir.
3. Ambiente Mirá (dev → staging) com cenários de aceite combinados.
4. Produção **atrás de flag**, com plano de rollback (desligar a flag) e
   monitoramento dos códigos de bloqueio nas primeiras horas.

Detalhes dos dois fluxos (dev interno / parceiro) no README do
[template genérico](https://github.com/mira-dev-tech/mira-commerce-plugin-example#colocando-em-produ%C3%A7%C3%A3o).

## Limitação conhecida

`payment.installment_resolve` (parcelamento) está no catálogo de hooks, mas
**ainda não é registrável** via `commerce-ext` — falta a superfície pública no
SDK. Está sendo tratado; até lá, regras de parcelamento não têm hook de plugin.

## Ecossistema Mirá Commerce

📚 **Documentação pública: [docs.mira-dev.tech](https://docs.mira-dev.tech)**

| Repo | O que é |
|---|---|
| [mira-commerce-sdk-js](https://github.com/mira-dev-tech/mira-commerce-sdk-js) | SDKs oficiais JS/TS — `client-sdk` (API tipada) e `checkout-sdk` (checkout headless) |
| [mira-commerce-storefront-guide](https://github.com/mira-dev-tech/mira-commerce-storefront-guide) | Guia oficial para construir um storefront (vitrine + checkout) sobre as APIs |
| [commerce-ext](https://github.com/mira-dev-tech/commerce-ext) | SDK público de plugins — Extension Protocol |
| [mira-commerce-plugin-example](https://github.com/mira-dev-tech/mira-commerce-plugin-example) | Template de plugin — exemplo didático (hooks, eventos, manifest, go-plugin) |
| [mira-commerce-plugin-payment](https://github.com/mira-dev-tech/mira-commerce-plugin-payment) | Template de plugin de pagamento e antifraude (roteamento, 3DS, velocity) |
| [mira-commerce-plugin-shipping](https://github.com/mira-dev-tech/mira-commerce-plugin-shipping) | Template de plugin de logística/frete (cotação externa, cache, circuit breaker) |
