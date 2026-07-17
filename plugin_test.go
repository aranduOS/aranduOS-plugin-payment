package examplepayment

// Testes UNITÁRIOS — chamam os handlers direto, sem host.
// Os testes de integração (cadeia real, timeout, fail-closed) estão em
// integration_test.go, com o plugintest.

import (
	"context"
	"testing"

	commerceext "github.com/mira-dev-tech/commerce-ext"
)

func newInited(t *testing.T, cfg map[string]any) *Plugin {
	t.Helper()
	p := New()
	if err := p.Init(context.Background(), &commerceext.Runtime{Config: cfg}); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestThreeDSAboveThreshold(t *testing.T) {
	p := newInited(t, map[string]any{"three_ds_threshold": 1000})
	dec := p.onPaymentRoute(context.Background(), commerceext.PaymentRouteInput{
		Order:         commerceext.OrderView{ID: "o1", TotalAmount: 1500},
		PaymentMethod: "card",
	})
	if !dec.Allow || !dec.Require3DS {
		t.Fatalf("expected allow com 3DS, got %+v", dec)
	}
}

func TestNoThreeDSBelowThreshold(t *testing.T) {
	p := newInited(t, map[string]any{"three_ds_threshold": 1000})
	dec := p.onPaymentRoute(context.Background(), commerceext.PaymentRouteInput{
		Order:         commerceext.OrderView{ID: "o2", TotalAmount: 500},
		PaymentMethod: "card",
	})
	if !dec.Allow || dec.Require3DS {
		t.Fatalf("expected allow sem 3DS, got %+v", dec)
	}
}

func TestThreeDSOnlyForCard(t *testing.T) {
	// Pix acima do teto não tem 3DS — o desafio é do mundo do cartão.
	p := newInited(t, map[string]any{"three_ds_threshold": 1000})
	dec := p.onPaymentRoute(context.Background(), commerceext.PaymentRouteInput{
		Order:         commerceext.OrderView{TotalAmount: 5000},
		PaymentMethod: "pix",
	})
	if !dec.Allow || dec.Require3DS {
		t.Fatalf("pix não deve exigir 3DS, got %+v", dec)
	}
}

func TestBoletoBlockedOnFirstPurchase(t *testing.T) {
	p := newInited(t, nil)
	dec := p.onPaymentRoute(context.Background(), commerceext.PaymentRouteInput{
		Order: commerceext.OrderView{
			ID:       "o3",
			Metadata: map[string]any{"first_purchase": true},
		},
		PaymentMethod: "boleto",
	})
	if dec.Allow || dec.Code != CodeMethodUnavailable {
		t.Fatalf("expected %s deny, got %+v", CodeMethodUnavailable, dec)
	}
	// A mensagem ao comprador não pode revelar a regra.
	if containsAny(dec.Message, "primeira", "first", "boleto") {
		t.Fatalf("mensagem vaza a regra ao comprador: %q", dec.Message)
	}
}

func TestBoletoAllowedForReturningCustomer(t *testing.T) {
	p := newInited(t, nil)
	dec := p.onPaymentRoute(context.Background(), commerceext.PaymentRouteInput{
		Order:         commerceext.OrderView{Metadata: map[string]any{"first_purchase": false}},
		PaymentMethod: "boleto",
	})
	if !dec.Allow {
		t.Fatalf("cliente recorrente pode boleto, got %+v", dec)
	}
}

func TestUnknownFirstPurchaseSignalDoesNotBlock(t *testing.T) {
	// Sem o sinal no metadata, "não sei" NÃO bloqueia (evita falso positivo).
	p := newInited(t, nil)
	dec := p.onPaymentRoute(context.Background(), commerceext.PaymentRouteInput{
		Order:         commerceext.OrderView{},
		PaymentMethod: "boleto",
	})
	if !dec.Allow {
		t.Fatalf("ausência de sinal não pode bloquear, got %+v", dec)
	}
}

func TestRiskBlocksAfterMaxAttempts(t *testing.T) {
	p := newInited(t, map[string]any{"max_attempts": 3})
	out := p.onRiskAssess(context.Background(), commerceext.RiskAssessInput{
		Order:        commerceext.OrderView{ID: "o4"},
		AttemptCount: 3,
	})
	if out.Allow || out.Code != CodeRiskHigh {
		t.Fatalf("expected %s deny, got %+v", CodeRiskHigh, out)
	}
	if containsAny(out.Message, "tentativa", "attempt", "3") {
		t.Fatalf("mensagem vaza a regra ao fraudador: %q", out.Message)
	}
}

func TestRiskAllowsBelowMaxAttempts(t *testing.T) {
	p := newInited(t, map[string]any{"max_attempts": 3})
	out := p.onRiskAssess(context.Background(), commerceext.RiskAssessInput{AttemptCount: 2})
	if !out.Allow {
		t.Fatalf("expected allow, got %+v", out)
	}
}

func TestRegisterMatchesManifest(t *testing.T) {
	// O que Register declara tem de bater com capabilities do manifest.yaml.
	p := newInited(t, nil)
	reg := commerceext.NewRegistry(p.Meta().ID)
	if err := p.Register(reg); err != nil {
		t.Fatal(err)
	}
	hooks := map[string]bool{}
	for _, h := range reg.Hooks() {
		hooks[h.HookID] = true
	}
	if !hooks[commerceext.HookPaymentRoute] || !hooks[commerceext.HookCheckoutRiskAssess] {
		t.Fatalf("hooks divergem do manifest: %v", hooks)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 && len(s) >= len(sub) && indexFold(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func indexFold(s, sub string) int {
	ls, lsub := toLower(s), toLower(sub)
	for i := 0; i+len(lsub) <= len(ls); i++ {
		if ls[i:i+len(lsub)] == lsub {
			return i
		}
	}
	return -1
}

func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
