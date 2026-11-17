package examplepayment_test

// Testes de INTEGRAÇÃO com o plugintest — o mini Extension Host em memória do
// SDK, que reproduz as semânticas do host real (cadeia por prioridade, timeout
// de 300ms, fail-closed). É o teste que prova que o plugin se comporta DENTRO
// da plataforma, e roda na sua máquina, sem acesso ao core.

import (
	"context"
	"testing"
	"time"

	commerceext "github.com/mira-dev-tech/commerce-ext"
	"github.com/mira-dev-tech/commerce-ext/plugintest"
	examplepayment "github.com/aranduOS/aranduOS-plugin-payment"
)

func newHost(t *testing.T, cfg map[string]any) *plugintest.Host {
	t.Helper()
	host := plugintest.NewHost()
	if err := host.Install(context.Background(), examplepayment.New(), plugintest.WithConfig(cfg)); err != nil {
		t.Fatal(err)
	}
	return host
}

func TestPaymentRouteThroughHost(t *testing.T) {
	host := newHost(t, map[string]any{"three_ds_threshold": 1000})
	dec := host.PaymentRoute(context.Background(), commerceext.PaymentRouteInput{
		Order:         commerceext.OrderView{TotalAmount: 1500},
		PaymentMethod: "card",
	})
	if !dec.Allow || !dec.Require3DS {
		t.Fatalf("expected allow + 3DS pela cadeia, got %+v", dec)
	}
}

func TestRiskAssessThroughHost(t *testing.T) {
	host := newHost(t, map[string]any{"max_attempts": 2})
	out := host.RiskAssess(context.Background(), commerceext.RiskAssessInput{AttemptCount: 5})
	if out.Allow || out.Code != examplepayment.CodeRiskHigh {
		t.Fatalf("expected deny %s, got %+v", examplepayment.CodeRiskHigh, out)
	}
}

// slowPlugin simula o erro mais comum em plugin de pagamento: chamar uma API
// externa (antifraude, adquirente) DENTRO do hook. O host tem 300ms — quando
// estoura, o core NEGA (fail-closed). Este teste existe para você ver o
// comportamento antes que ele aconteça em produção, num Black Friday.
type slowPlugin struct{ delay time.Duration }

func (slowPlugin) Meta() commerceext.Meta {
	return commerceext.Meta{ID: "slow", Version: "1.0.0", CompatibleCore: "^0.2.0"}
}
func (slowPlugin) Init(context.Context, *commerceext.Runtime) error { return nil }
func (slowPlugin) Shutdown(context.Context) error                   { return nil }
func (p slowPlugin) Register(reg *commerceext.Registry) error {
	reg.OnPaymentRoute(100, func(_ context.Context, _ commerceext.PaymentRouteInput) commerceext.PaymentRouteDecision {
		time.Sleep(p.delay) // ← I/O de rede aqui = esta lentidão
		return commerceext.PaymentRouteDecision{Outcome: commerceext.Allowed()}
	})
	return nil
}

func TestSlowHookFailsClosed(t *testing.T) {
	host := plugintest.NewHost()
	host.SetTimeout(50 * time.Millisecond) // acelera o teste; o core usa 300ms
	if err := host.Install(context.Background(), slowPlugin{delay: 200 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	dec := host.PaymentRoute(context.Background(), commerceext.PaymentRouteInput{PaymentMethod: "card"})
	if dec.Allow {
		t.Fatal("hook lento deveria NEGAR (fail-closed), não aprovar")
	}
	if dec.Code != plugintest.CodePluginFailure {
		t.Fatalf("expected %s, got %+v", plugintest.CodePluginFailure, dec)
	}
}

// Prova a semântica de OR do Require3DS: um plugin permissivo NÃO desfaz o 3DS
// pedido por outro. Em pagamento, o mais conservador vence.
func TestRequire3DSIsOrAcrossPlugins(t *testing.T) {
	host := plugintest.NewHost()
	ctx := context.Background()
	if err := host.Install(ctx, examplepayment.New(), plugintest.WithConfig(map[string]any{"three_ds_threshold": 1000})); err != nil {
		t.Fatal(err)
	}
	if err := host.Install(ctx, permissivePlugin{}); err != nil {
		t.Fatal(err)
	}
	dec := host.PaymentRoute(ctx, commerceext.PaymentRouteInput{
		Order:         commerceext.OrderView{TotalAmount: 5000},
		PaymentMethod: "card",
	})
	if !dec.Require3DS {
		t.Fatal("3DS pedido por um plugin não pode ser desfeito por outro")
	}
}

type permissivePlugin struct{}

func (permissivePlugin) Meta() commerceext.Meta {
	return commerceext.Meta{ID: "permissive", Version: "1.0.0", CompatibleCore: "^0.2.0"}
}
func (permissivePlugin) Init(context.Context, *commerceext.Runtime) error { return nil }
func (permissivePlugin) Shutdown(context.Context) error                   { return nil }
func (permissivePlugin) Register(reg *commerceext.Registry) error {
	reg.OnPaymentRoute(900, func(_ context.Context, _ commerceext.PaymentRouteInput) commerceext.PaymentRouteDecision {
		return commerceext.PaymentRouteDecision{Outcome: commerceext.Allowed(), Require3DS: false}
	})
	return nil
}
