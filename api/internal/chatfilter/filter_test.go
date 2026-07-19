package chatfilter

import "testing"

func TestFilterMasksKnownWordsCaseInsensitively(t *testing.T) {
	if got := New([]string{"idiota"}).Clean("Você é um IDIOTA mesmo"); got == "Você é um IDIOTA mesmo" {
		t.Fatal("word was not masked")
	}
}
func TestFilterLeavesCleanMessagesUntouched(t *testing.T) {
	const message = "boa mão!"
	if got := New([]string{"idiota"}).Clean(message); got != message {
		t.Fatalf("got %q", got)
	}
}
