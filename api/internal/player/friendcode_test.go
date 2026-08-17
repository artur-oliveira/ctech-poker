package player

import "testing"

func TestFriendCodeForUserIDIsStableAndCanonical(t *testing.T) {
	first := FriendCodeForUserID("user-123")
	second := FriendCodeForUserID("user-123")
	if first != second {
		t.Fatalf("friend code changed: %q != %q", first, second)
	}
	if normalized, ok := NormalizeFriendCode(first); !ok || normalized != first {
		t.Fatalf("generated code is not canonical: %q normalized=%q ok=%v", first, normalized, ok)
	}
	if first == FriendCodeForUserID("user-456") {
		t.Fatal("distinct user IDs produced the same test code")
	}
}

func TestNormalizeFriendCodeIsCaseInsensitiveButExact(t *testing.T) {
	code := FriendCodeForUserID("user-123")
	if got, ok := NormalizeFriendCode("  " + lowerASCII(code) + "  "); !ok || got != code {
		t.Fatalf("got=%q ok=%v want=%q", got, ok, code)
	}
	for _, invalid := range []string{"", "PKR-ABCD-EFGH", "PKR-ABCD-EFGH-IJK0", "USR-ABCD-EFGH-IJKL"} {
		if got, ok := NormalizeFriendCode(invalid); ok {
			t.Fatalf("invalid code %q normalized to %q", invalid, got)
		}
	}
}

func lowerASCII(value string) string {
	out := []byte(value)
	for i, b := range out {
		if b >= 'A' && b <= 'Z' {
			out[i] = b + ('a' - 'A')
		}
	}
	return string(out)
}
