package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func NewUserID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generate user id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func Sign(userID, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(userID))
	sig := hex.EncodeToString(mac.Sum(nil))
	return userID + ":" + sig
}

func Verify(cookieValue, secret string) (string, bool) {
	idx := strings.LastIndex(cookieValue, ":")
	if idx <= 0 {
		return "", false
	}
	userID := cookieValue[:idx]
	sig := cookieValue[idx+1:]

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(userID))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return "", false
	}
	return userID, true
}
