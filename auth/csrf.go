package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"notebook-app/config"
)

func GenerateCSRFToken(sessionID string) string {
	secret := []byte(config.GetEnv("CSRF_SECRET", "csrf-secret-please-change"))
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(sessionID))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func VerifyCSRFToken(sessionID, token string) bool {
	expected := GenerateCSRFToken(sessionID)
	return hmac.Equal([]byte(expected), []byte(token))
}

func RandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
