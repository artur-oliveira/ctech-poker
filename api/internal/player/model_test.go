package player

import "testing"

func TestTermsAcceptedIsComputedFromCurrentVersion(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    bool
	}{
		{"", false}, {"0.9", false}, {CurrentPokerTermsVersion, true},
	} {
		p := &PlayerProfile{PokerTermsVersion: tc.version}
		if got := p.TermsAccepted(); got != tc.want {
			t.Fatalf("version %q: got %v, want %v", tc.version, got, tc.want)
		}
	}
	if (*PlayerProfile)(nil).TermsAccepted() {
		t.Fatal("nil profile must not have accepted terms")
	}
}

func TestAvatarURL(t *testing.T) {
	profile := &PlayerProfile{AvatarKey: "av/user/3.jpg", AvatarVersion: 3}
	if got := AvatarURL(profile, "https://poker.aoctech.app/avatars/"); got != "https://poker.aoctech.app/avatars/user/3.jpg" {
		t.Fatalf("got %q", got)
	}
	profile.AvatarBlocked = true
	if got := AvatarURL(profile, "https://poker.aoctech.app/avatars"); got != "" {
		t.Fatalf("blocked avatar leaked: %q", got)
	}
}
