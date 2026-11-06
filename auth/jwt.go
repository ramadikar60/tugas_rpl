package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"notebook-app/config"
)

type Claims struct {
	UserID int64  `json:"user_id"`
	Exp    int64  `json:"exp"`
	Type   string `json:"typ"`
}

func GenerateAccessToken(userID int64) (string, error) {
	return generateJWT(userID, "access", 15*time.Minute)
}

func GenerateRefreshToken() (string, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func generateJWT(userID int64, typ string, dur time.Duration) (string, error) {
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	claims := Claims{
		UserID: userID,
		Exp:    time.Now().Add(dur).Unix(),
		Type:   typ,
	}

	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)

	h64 := base64.RawURLEncoding.EncodeToString(hb)
	c64 := base64.RawURLEncoding.EncodeToString(cb)

	signingInput := h64 + "." + c64
	sig := sign(signingInput)
	return signingInput + "." + sig, nil
}

func VerifyAccessToken(tokenStr string) (int64, error) {
	return verifyJWT(tokenStr, "access")
}

func verifyJWT(tokenStr, expectedType string) (int64, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return 0, errors.New("invalid token format")
	}
	signingInput := parts[0] + "." + parts[1]
	expected := sign(signingInput)

	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return 0, errors.New("invalid signature")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, err
	}

	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return 0, err
	}

	if time.Now().Unix() > claims.Exp {
		return 0, errors.New("token expired")
	}
	if claims.Type != expectedType {
		return 0, errors.New("invalid token type")
	}
	return claims.UserID, nil
}

var (
	jwtKeyOnce  sync.Once
	jwtKeyBytes []byte
)

// jwtKey mengambil JWT_SECRET dari lingkungan. Tidak ada nilai bawaan di kode:
// bila kosong dipakai kunci acak sementara (sesi akses hilang saat server restart,
// tetapi refresh token di database tetap bisa menerbitkan token baru).
func jwtKey() []byte {
	jwtKeyOnce.Do(func() {
		secret := config.GetEnv("JWT_SECRET", "")
		switch {
		case secret == "":
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				log.Fatalf("tidak bisa membuat kunci JWT acak: %v", err)
			}
			jwtKeyBytes = b
			log.Printf("PERINGATAN: JWT_SECRET belum diatur di .env. Memakai kunci acak sementara. Isi JWT_SECRET (minimal 32 karakter acak).")
		case len(secret) < 32:
			jwtKeyBytes = []byte(secret)
			log.Printf("PERINGATAN: JWT_SECRET terlalu pendek (%d karakter). Gunakan minimal 32 karakter acak.", len(secret))
		default:
			jwtKeyBytes = []byte(secret)
		}
	})
	return jwtKeyBytes
}

// InitJWTKey memicu pemuatan kunci saat server start, sehingga peringatan
// JWT_SECRET (kosong/terlalu pendek) langsung tampil di log, bukan menunggu login pertama.
func InitJWTKey() { jwtKey() }

func sign(input string) string {
	key := jwtKey()
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(input))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
