package auth

// ============================================================
// PASSWORD.GO - Hash sandi (Prompt 22A, NFR-09 SRS)
//
// Format baru   : bcrypt ($2a$<cost>$...), cost 12
// Format PBKDF2 : pbkdf2-sha256$<iterasi>$<salt hex>$<hash hex>   (hanya diverifikasi)
// Format lama   : <salt hex>:<sha256(salt||sandi) hex>              (hanya diverifikasi)
//
// Semua hash lama tetap bisa diverifikasi. Saat login berhasil, hash yang bukan
// bcrypt (atau bcrypt dengan cost di bawah standar) diganti otomatis (NeedsRehash).
//
// bcrypt hanya memproses 72 byte pertama sandi, jadi sandi dibatasi 72 byte
// (lihat MaxAccountPasswordLen). PBKDF2 tetap ditulis di sini untuk verifikasi hash
// yang sempat terbit dan divalidasi dengan vektor uji standar pada password_test.go.
// ============================================================

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

const (
	maxPasswordBytes = 72 // batas bcrypt
	pbkdf2Prefix     = "pbkdf2-sha256"
	maxIter          = 10_000_000 // batas wajar agar hash tersimpan yang rusak tidak mengunci CPU
)

// bcryptCost: tingkat kerja bcrypt (rekomendasi OWASP: minimal 10; dipakai 12).
// Variabel agar tes bisa menurunkannya.
var bcryptCost = 12

// pbkdf2SHA256 mengimplementasikan PBKDF2 (RFC 8018) dengan HMAC-SHA256.
// Hanya dipakai untuk memverifikasi hash PBKDF2 yang sudah ada.
func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen

	var buf [4]byte
	dk := make([]byte, 0, numBlocks*hashLen)
	u := make([]byte, hashLen)
	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		buf[0] = byte(block >> 24)
		buf[1] = byte(block >> 16)
		buf[2] = byte(block >> 8)
		buf[3] = byte(block)
		prf.Write(buf[:4])
		dk = prf.Sum(dk)
		t := dk[len(dk)-hashLen:]
		copy(u, t)

		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(u)
			u = u[:0]
			u = prf.Sum(u)
			for x := range u {
				t[x] ^= u[x]
			}
		}
	}
	return dk[:keyLen]
}

func isBcrypt(stored string) bool {
	return strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") || strings.HasPrefix(stored, "$2y$")
}

// HashPassword membuat hash bcrypt. Aturan panjang minimal 6 karakter berlaku untuk
// semua pemakai fungsi ini (sandi catatan terkunci, sandi berbagi). Sandi AKUN
// harus lebih dulu lolos ValidateAccountPassword.
func HashPassword(password string) (string, error) {
	if len(password) < 6 {
		return "", errors.New("password minimal 6 karakter")
	}
	if len(password) > maxPasswordBytes {
		return "", errors.New("password maksimal 72 karakter")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// VerifyPassword memeriksa sandi terhadap hash bcrypt, PBKDF2, maupun format lama.
func VerifyPassword(stored, password string) bool {
	switch {
	case isBcrypt(stored):
		if len(password) > maxPasswordBytes {
			return false // tidak mungkin cocok dengan hash yang dibuat dari sandi <= 72 byte
		}
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)) == nil
	case strings.HasPrefix(stored, pbkdf2Prefix+"$"):
		return verifyPBKDF2(stored, password)
	default:
		return verifyLegacy(stored, password)
	}
}

func verifyPBKDF2(stored, password string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 || iter > maxIter {
		return false
	}
	salt, err1 := hex.DecodeString(parts[2])
	want, err2 := hex.DecodeString(parts[3])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, iter, len(want))
	return subtle.ConstantTimeCompare(want, got) == 1
}

// verifyLegacy: format lama SHA-256 satu putaran (hanya untuk migrasi bertahap).
func verifyLegacy(stored, password string) bool {
	parts := strings.SplitN(stored, ":", 2)
	if len(parts) != 2 {
		return false
	}
	salt, err1 := hex.DecodeString(parts[0])
	want, err2 := hex.DecodeString(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(password))
	return subtle.ConstantTimeCompare(want, h.Sum(nil)) == 1
}

// NeedsRehash true bila hash bukan bcrypt, atau bcrypt dengan cost di bawah standar saat ini.
func NeedsRehash(stored string) bool {
	if !isBcrypt(stored) {
		return true
	}
	cost, err := bcrypt.Cost([]byte(stored))
	return err != nil || cost < bcryptCost
}

var (
	dummyOnce sync.Once
	dummyHash []byte
)

// DummyVerify menghabiskan waktu yang sama dengan satu verifikasi bcrypt sungguhan.
// Dipakai saat email tidak ditemukan agar waktu respons tidak membocorkan
// apakah akun ada.
func DummyVerify(password string) {
	dummyOnce.Do(func() {
		dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-untuk-menyamakan-waktu"), bcryptCost)
	})
	if len(password) > maxPasswordBytes {
		password = password[:maxPasswordBytes]
	}
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}
