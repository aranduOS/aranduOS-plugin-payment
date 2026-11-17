// Command example-payment é o processo do plugin de parceiro.
// Não é filho da API e não se carrega com MIRA_MANIFEST_PATH.
// No cluster a única env é COMMERCE_EXT_HOST; o host disca o RPC.
//
// Build:  go build -o bin/example-payment ./cmd/example-payment
// Prova:  ./bin/example-payment --commerce-ext-handshake   → "commerce-ext-ok"
package main

import (
	commerceext "github.com/mira-dev-tech/commerce-ext"

	examplepayment "github.com/mira-dev-tech/mira-commerce-plugin-payment"
)

func main() {
	commerceext.Serve(examplepayment.New())
}
