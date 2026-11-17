// Package examplepayment é o template de plugin de MEIOS DE PAGAMENTO e
// ANTIFRAUDE do Mira Commerce. Ele mostra os dois hooks que decidem, em tempo
// de checkout, *como* e *se* uma cobrança acontece:
//
//	payment.route         → escolhe/bloqueia o meio e exige 3DS (antes do gateway)
//	checkout.risk_assess  → bloqueia por risco (velocity de tentativas)
//
// As regras aqui são ilustrativas e propositalmente simples de trocar:
//
//   - Compra acima de um teto exige 3DS (o desafio protege contra chargeback).
//   - Boleto indisponível na primeira compra do cliente (risco de calote).
//   - Muitas tentativas de pagamento no mesmo pedido → bloqueia (card testing).
//
// REGRAS DO DOMÍNIO QUE VALEM PARA QUALQUER PLUGIN DE PAGAMENTO
//
//  1. FAIL-CLOSED. Estes hooks são a última porta antes do dinheiro. Se o seu
//     handler estourar 300ms ou der panic, o core NEGA a operação com
//     PLUGIN_FAILURE — melhor recusar uma compra do que aprovar às cegas.
//     Consequência prática: NUNCA faça I/O de rede aqui (ver o template de
//     frete para o padrão de cache+fallback quando precisar de API externa).
//
//  2. NUNCA TOQUE EM DADO DE CARTÃO. PAN/CVV não passam pelo core nem pelo
//     plugin — a tokenização acontece no adquirente. O que você recebe aqui é
//     o *método* (card/pix/boleto), nunca o cartão.
//
//  3. CÓDIGO DE BLOQUEIO É CONTRATO. O front e o suporte mapeiam esses códigos
//     para mensagens ao comprador; renomear quebra a UX de quem já integrou.
//
//  4. MENSAGEM AO COMPRADOR É GENÉRICA. Nunca revele a regra que bloqueou
//     ("5 tentativas") — isso é manual de bypass para o fraudador. Detalhe vai
//     no log; ao comprador, mensagem neutra.
//
// Guia do modelo de extensibilidade:
// https://github.com/mira-dev-tech/mira-commerce-plugin-example
package examplepayment

import (
	"context"
	"os"
	"strconv"
	"strings"

	commerceext "github.com/mira-dev-tech/commerce-ext"
)

// Defaults — todos sobrescrevíveis por Runtime.Config ou env EXAMPLE_PAYMENT_*.
const (
	defaultThreeDSThreshold = 1000 // R$ acima disto exige 3DS
	defaultMaxAttempts      = 5    // tentativas de pagamento por pedido
	defaultBoletoFirstBuy   = true // bloquear boleto na 1ª compra
)

// Códigos de bloqueio — CONTRATO com o front/suporte. Não renomear sem ADR.
const (
	CodeMethodUnavailable = "PAYMENT_METHOD_UNAVAILABLE"
	CodeRiskHigh          = "RISK_SCORE_HIGH"
)

// Plugin implementa commerceext.Plugin.
type Plugin struct {
	logger commerceext.Logger

	// Config imutável após Init — handlers só leem.
	threeDSThreshold float64
	maxAttempts      int
	blockBoletoFirst bool
}

// New devolve a instância do plugin.
func New() *Plugin { return &Plugin{} }

// Meta identifica o plugin.
func (p *Plugin) Meta() commerceext.Meta {
	return commerceext.Meta{
		ID:             "example-payment",
		Version:        "1.0.0",
		CompatibleCore: "^0.2.0",
		Description:    "Template — roteamento de meios de pagamento, 3DS por valor e antifraude por velocity",
	}
}

// Init lê a config uma única vez (Runtime.Config → env → default).
func (p *Plugin) Init(_ context.Context, rt *commerceext.Runtime) error {
	p.logger = commerceext.NopLogger{}
	var cfg map[string]any
	if rt != nil {
		if rt.Logger != nil {
			p.logger = rt.Logger
		}
		cfg = rt.Config
	}
	p.threeDSThreshold = configFloat(cfg, "three_ds_threshold", "EXAMPLE_PAYMENT_3DS_THRESHOLD", defaultThreeDSThreshold)
	p.maxAttempts = int(configFloat(cfg, "max_attempts", "EXAMPLE_PAYMENT_MAX_ATTEMPTS", defaultMaxAttempts))
	p.blockBoletoFirst = configBool(cfg, "block_boleto_first_purchase", "EXAMPLE_PAYMENT_BLOCK_BOLETO_FIRST", defaultBoletoFirstBuy)
	p.logger.Info("example-payment init",
		"three_ds_threshold", p.threeDSThreshold,
		"max_attempts", p.maxAttempts,
		"block_boleto_first_purchase", p.blockBoletoFirst)
	return nil
}

// Register declara os hooks. Prioridade menor roda primeiro na cadeia; 300
// deixa espaço para um plugin de cliente decidir antes (100–200) e para os
// plugins de referência do core depois (500+).
func (p *Plugin) Register(reg *commerceext.Registry) error {
	reg.OnPaymentRoute(300, p.onPaymentRoute)
	reg.OnCheckoutRiskAssess(300, p.onRiskAssess)
	return nil
}

// Shutdown libera recursos (nada aqui).
func (p *Plugin) Shutdown(context.Context) error { return nil }

// onPaymentRoute — hook payment.route, roda ANTES de chamar o gateway.
//
// Semântica da cadeia (igual à do core):
//   - Deny para a cadeia inteira e bloqueia a cobrança.
//   - Require3DS acumula em OR: se QUALQUER plugin pedir 3DS, o desafio
//     acontece. Ou seja, pedir 3DS é sempre seguro; não pedir não garante que
//     outro plugin não peça.
func (p *Plugin) onPaymentRoute(_ context.Context, in commerceext.PaymentRouteInput) commerceext.PaymentRouteDecision {
	method := strings.ToLower(strings.TrimSpace(in.PaymentMethod))

	// Regra 1: boleto na primeira compra é risco de calote (o pedido fica
	// reservado e o cliente some). Sinal vem do metadata do pedido — o core
	// carrega o que o checkout apurou.
	if p.blockBoletoFirst && method == "boleto" && isFirstPurchase(in.Order) {
		p.logger.Warn("boleto blocked on first purchase", "order_id", in.Order.ID)
		// Mensagem genérica: não explique a regra ao comprador.
		return commerceext.PaymentRouteDecision{
			Outcome: commerceext.Denied(CodeMethodUnavailable,
				"Este meio de pagamento não está disponível para esta compra."),
		}
	}

	// Regra 2: acima do teto, exigir 3DS. O desafio transfere a
	// responsabilidade do chargeback para o emissor.
	require3DS := method == "card" && in.Order.TotalAmount > p.threeDSThreshold
	if require3DS {
		p.logger.Info("3ds required by amount", "order_id", in.Order.ID, "total", in.Order.TotalAmount)
	}
	return commerceext.PaymentRouteDecision{
		Outcome:    commerceext.Allowed(),
		Require3DS: require3DS,
	}
}

// onRiskAssess — hook checkout.risk_assess. Bloqueia card testing: o fraudador
// testa cartões roubados em sequência no mesmo pedido até um passar.
//
// O core já conta as tentativas para você (AttemptCount) — em produção, some a
// isso os seus sinais (device fingerprint, geolocalização, lista de bloqueio).
func (p *Plugin) onRiskAssess(_ context.Context, in commerceext.RiskAssessInput) commerceext.Outcome {
	if in.AttemptCount >= p.maxAttempts {
		// Detalhe no log (para o time), genérico ao comprador (contra bypass).
		p.logger.Warn("risk block by velocity",
			"order_id", in.Order.ID, "attempts", in.AttemptCount, "ip", in.RequestIP)
		return commerceext.Denied(CodeRiskHigh,
			"Não foi possível concluir o pagamento. Tente novamente mais tarde.")
	}
	return commerceext.Allowed()
}

// isFirstPurchase lê o sinal do metadata do pedido. O core preenche o que o
// checkout apurou; trate a AUSÊNCIA do sinal como "não sei" e, no domínio de
// dinheiro, "não sei" nunca deve bloquear (evita falso positivo em massa).
func isFirstPurchase(order commerceext.OrderView) bool {
	if order.Metadata == nil {
		return false
	}
	switch v := order.Metadata["first_purchase"].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	}
	return false
}

func configFloat(cfg map[string]any, key, envKey string, def float64) float64 {
	if cfg != nil {
		switch v := cfg[key].(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		}
	}
	if raw := os.Getenv(envKey); raw != "" {
		if f, err := strconv.ParseFloat(raw, 64); err == nil && f >= 0 {
			return f
		}
	}
	return def
}

func configBool(cfg map[string]any, key, envKey string, def bool) bool {
	if cfg != nil {
		switch v := cfg[key].(type) {
		case bool:
			return v
		case string:
			if b, err := strconv.ParseBool(v); err == nil {
				return b
			}
		}
	}
	if raw := os.Getenv(envKey); raw != "" {
		if b, err := strconv.ParseBool(raw); err == nil {
			return b
		}
	}
	return def
}
