package admin

import "testing"

func TestBookingPath(t *testing.T) {
	if got := bookingPath(42); got != "/admin/bookings/42" {
		t.Errorf("bookingPath(42) = %q", got)
	}
}

func TestNewNonceIsRandomAndHex(t *testing.T) {
	a, err := newNonce()
	if err != nil {
		t.Fatal(err)
	}
	b, err := newNonce()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two nonces should not collide")
	}
	if len(a) != 32 { // 16 bytes, hex-encoded
		t.Errorf("nonce length = %d, want 32", len(a))
	}
}

func TestReplyErrorMessageCoversKnownCodes(t *testing.T) {
	for _, code := range []string{"empty", "expired", "1", "anything-else"} {
		if replyErrorMessage(code) == "" {
			t.Errorf("replyErrorMessage(%q) returned empty string", code)
		}
	}
}
