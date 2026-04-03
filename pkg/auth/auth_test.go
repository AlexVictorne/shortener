package auth

import (
	"encoding/hex"
	"testing"
)

func TestNewUserID(t *testing.T) {
	id, err := NewUserID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id) != 32 {
		t.Errorf("expected userID length 32, got %d", len(id))
	}
	_, err = hex.DecodeString(id)
	if err != nil {
		t.Errorf("userID is not valid hex: %v", err)
	}
}

func TestSignAndVerify(t *testing.T) {
	userID := "abc123"
	secret := "supersecret"
	cookie := Sign(userID, secret)
	gotID, ok := Verify(cookie, secret)
	if !ok {
		t.Error("Verify failed for valid signature")
	}
	if gotID != userID {
		t.Errorf("expected userID %q, got %q", userID, gotID)
	}
}

func TestVerify_InvalidSignature(t *testing.T) {
	userID := "abc123"
	secret := "supersecret"
	cookie := Sign(userID, secret)
	// Tamper with signature
	cookie = cookie[:len(cookie)-1] + "0"
	_, ok := Verify(cookie, secret)
	if ok {
		t.Error("Verify should fail for tampered signature")
	}
}

func TestVerify_InvalidFormat(t *testing.T) {
	secret := "supersecret"
	cases := []string{"", "no-colon", ":sigonly", "userid:"}
	for _, c := range cases {
		_, ok := Verify(c, secret)
		if ok {
			t.Errorf("Verify should fail for input: %q", c)
		}
	}
}
