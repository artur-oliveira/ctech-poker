package v1

import (
	"testing"

	"gopkg.aoctech.app/api-commons/webpush"
)

func testVAPIDPublicKey(t *testing.T) string {
	t.Helper()
	_, k, err := webpush.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k.PublicBase64URL()
}
