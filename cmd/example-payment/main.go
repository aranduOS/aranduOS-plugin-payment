// Command example-payment empacota o plugin como BINÁRIO EXTERNO (go-plugin):
// o Extension Host do core sobe este processo e conversa com ele por RPC.
//
// Build:  go build -o bin/example-payment ./cmd/example-payment
// Teste:  ./bin/example-payment --commerce-ext-handshake   → "commerce-ext-ok"
package main

import (
	commerceext "github.com/mira-dev-tech/commerce-ext"

	examplepayment "github.com/mira-dev-tech/mira-commerce-plugin-payment"
)

func main() {
	commerceext.Serve(examplepayment.New())
}
