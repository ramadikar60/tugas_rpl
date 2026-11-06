package auth

import (
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
	"time"
)

func withCost(t *testing.T, n int) {
	t.Helper()
	old := bcryptCost
	bcryptCost = n
	t.Cleanup(func() { bcryptCost = old })
}

// hashPBKDF2ForTest membuat hash format PBKDF2 (yang tidak lagi diterbitkan, hanya diverifikasi).
func hashPBKDF2ForTest(password string, iter int) string {
	salt, _ := hex.DecodeString("a1b2c3d4e5f60718293a4b5c6d7e8f90")
	dk := pbkdf2SHA256([]byte(password), salt, iter, 32)
	return pbkdf2Prefix + "$" + strconv.Itoa(iter) + "$" + hex.EncodeToString(salt) + "$" + hex.EncodeToString(dk)
}

// Vektor dihitung dengan hashlib.pbkdf2_hmac('sha256', ...) milik Python.
func TestPBKDF2SHA256Vectors(t *testing.T) {
	cases := []struct {
		pw, salt string
		iter, n  int
		want     string
	}{
		{"password", "salt", 1, 32, "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"},
		{"password", "salt", 2, 32, "ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43"},
		{"password", "salt", 4096, 32, "c5e478d59288c841aa530db6845c4c8d962893a001ce4e11a4963873aa98134a"},
		{"passwordPASSWORDpassword", "saltSALTsaltSALTsaltSALTsaltSALTsalt", 4096, 40,
			"348c89dbcbd32b2f32d814b8116e84cf2b17347ebc1800181c4e2a1fb8dd53e1c635518c7dac47e9"},
		{"pass\x00word", "sa\x00lt", 4096, 16, "89b69d0516f829893c696226650a8687"},
		{"", "salt", 1, 32, "f135c27993baf98773c5cdb40a5706ce6a345cde61b000a67858650cd6a324d7"},
		{strings.Repeat("p", 200), strings.Repeat("s", 50), 3, 70,
			"0666a9abab731bf9280750d666cfde3e3fe6e0d2f8b7921fcd4e6ce91d37c893ffa21712383fbccbe7f39898179b0b2b15b413960e6ea124cd05ffcfbf7d77204fe8487a5f3a"},
	}
	for i, c := range cases {
		got := hex.EncodeToString(pbkdf2SHA256([]byte(c.pw), []byte(c.salt), c.iter, c.n))
		if got != c.want {
			t.Errorf("vektor %d salah:\n  dapat: %s\n  mau  : %s", i, got, c.want)
		}
	}
}

func TestHashPasswordIsBcryptAndVerifies(t *testing.T) {
	withCost(t, 4)
	h1, err := HashPassword("kata-sandi-benar-123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h1, "$2a$04$") || len(h1) != 60 {
		t.Fatalf("format bcrypt salah: %s", h1)
	}
	h2, _ := HashPassword("kata-sandi-benar-123")
	if h1 == h2 {
		t.Errorf("salt harus acak: dua hash identik")
	}
	if !VerifyPassword(h1, "kata-sandi-benar-123") || !VerifyPassword(h2, "kata-sandi-benar-123") {
		t.Errorf("sandi benar harus lolos")
	}
	for _, bad := range []string{"salah", "kata-sandi-benar-124", "", "KATA-SANDI-BENAR-123", "kata-sandi-benar-123 "} {
		if VerifyPassword(h1, bad) {
			t.Errorf("sandi %q seharusnya ditolak", bad)
		}
	}
	// Sandi unicode (<= 72 byte).
	uni := "ß漢字🙂ß漢字🙂ab12" // 2+3+3+4+2+3+3+4+4 = 28 byte
	hu, err := HashPassword(uni)
	if err != nil || !VerifyPassword(hu, uni) || VerifyPassword(hu, uni+"x") {
		t.Errorf("sandi unicode gagal: %v", err)
	}
}

func TestBcrypt72ByteLimitIsEnforcedNotSilentlyTruncated(t *testing.T) {
	withCost(t, 4)
	ok72 := strings.Repeat("a", 72)
	h, err := HashPassword(ok72)
	if err != nil || !VerifyPassword(h, ok72) {
		t.Fatalf("72 byte harus diterima: %v", err)
	}
	// 73 byte ditolak saat membuat hash (bukan dipotong diam-diam, yang membuat dua sandi berbeda dianggap sama).
	if _, err := HashPassword(ok72 + "b"); err == nil || err.Error() != "password maksimal 72 karakter" {
		t.Errorf("73 byte harus ditolak dengan pesan berbahasa Indonesia, dapat %v", err)
	}
	// Dan sandi 73 byte yang awalannya sama tidak boleh lolos terhadap hash 72 byte.
	if VerifyPassword(h, ok72+"b") {
		t.Errorf("sandi 73 byte tidak boleh cocok dengan hash sandi 72 byte")
	}
}

func TestHashPasswordMinimumLengthForSecondaryPasswords(t *testing.T) {
	withCost(t, 4)
	if _, err := HashPassword("12345"); err == nil {
		t.Errorf("sandi 5 karakter harus ditolak (aturan sandi catatan/berbagi)")
	}
	if _, err := HashPassword("123456"); err != nil {
		t.Errorf("sandi 6 karakter harus diterima: %v", err)
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	good := hashPBKDF2ForTest("kata-sandi-benar-123", 10)
	parts := strings.Split(good, "$")
	bad := []string{
		"", "x", ":", "zz:zz", "$2a$", "$2a$04$", "$2a$04$tidakvalid", "$2y$99$" + strings.Repeat("a", 53),
		"pbkdf2-sha256", "pbkdf2-sha256$", "pbkdf2-sha256$10$aa", "pbkdf2-sha256$10$aa$bb$cc",
		"pbkdf2-sha256$0$" + parts[2] + "$" + parts[3],
		"pbkdf2-sha256$-5$" + parts[2] + "$" + parts[3],
		"pbkdf2-sha256$abc$" + parts[2] + "$" + parts[3],
		"pbkdf2-sha256$99999999999$" + parts[2] + "$" + parts[3], // melebihi batas: harus cepat ditolak
		"pbkdf2-sha256$10$zz$" + parts[3],
		"pbkdf2-sha256$10$" + parts[2] + "$zz",
		"pbkdf2-sha256$10$" + parts[2] + "$",
	}
	start := time.Now()
	for _, s := range bad {
		if VerifyPassword(s, "kata-sandi-benar-123") {
			t.Errorf("hash rusak %q diterima", s)
		}
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("penolakan hash rusak terlalu lambat")
	}
}

func TestPBKDF2HashesStillVerifyAndAreRehashedToBcrypt(t *testing.T) {
	h := hashPBKDF2ForTest("kata-sandi-benar-123", 1000)
	if !VerifyPassword(h, "kata-sandi-benar-123") || VerifyPassword(h, "salah-banget-1") {
		t.Fatalf("hash PBKDF2 yang sudah terbit harus tetap bisa diverifikasi")
	}
	if !NeedsRehash(h) {
		t.Errorf("hash PBKDF2 harus ditandai perlu rehash ke bcrypt")
	}
}

// Hash format lama dihitung dengan algoritma lama persis (sha256(salt||sandi)).
func TestLegacyHashStillVerifiesAndNeedsRehash(t *testing.T) {
	const legacy = "00112233445566778899aabbccddeeff:1e878ec07353d3d85421d7d09a5658ccb50834b69bbd1a91f36de01f50003eee"
	if !VerifyPassword(legacy, "kataSandiLama1") {
		t.Fatalf("hash lama harus tetap bisa diverifikasi")
	}
	if VerifyPassword(legacy, "kataSandiLama2") || VerifyPassword(legacy, "") {
		t.Errorf("hash lama menerima sandi salah")
	}
	if !NeedsRehash(legacy) {
		t.Errorf("hash lama harus ditandai perlu rehash")
	}
	if VerifyPassword("00112233445566778899aabbccddeeff", "x") || VerifyPassword("zz:00", "x") {
		t.Errorf("hash lama rusak diterima")
	}
}

func TestNeedsRehashByCost(t *testing.T) {
	withCost(t, 4)
	h, _ := HashPassword("kata-sandi-benar-123")
	if NeedsRehash(h) {
		t.Errorf("hash dengan cost saat ini tidak perlu rehash")
	}
	bcryptCost = 6 // standar naik
	if !NeedsRehash(h) {
		t.Errorf("cost di bawah standar harus rehash")
	}
	if !NeedsRehash("$2a$xx$abc") || !NeedsRehash("") {
		t.Errorf("format rusak harus dianggap perlu rehash")
	}
	h2, _ := HashPassword("kata-sandi-benar-123")
	if !VerifyPassword(h2, "kata-sandi-benar-123") || NeedsRehash(h2) {
		t.Errorf("hash hasil rehash bermasalah")
	}
}

func TestDefaultCostFeasibility(t *testing.T) {
	if bcryptCost != 12 {
		t.Fatalf("cost bawaan harus 12, dapat %d", bcryptCost)
	}
	start := time.Now()
	h, err := HashPassword("kata-sandi-benar-123")
	if err != nil {
		t.Fatal(err)
	}
	dHash := time.Since(start)
	start = time.Now()
	ok := VerifyPassword(h, "kata-sandi-benar-123")
	dVerify := time.Since(start)
	DummyVerify("warmup") // inisialisasi hash dummy tidak ikut diukur
	start = time.Now()
	DummyVerify("apa-saja-123")
	dDummy := time.Since(start)
	t.Logf("bcrypt cost 12: hash=%v verifikasi=%v dummy=%v", dHash, dVerify, dDummy)
	if !ok {
		t.Errorf("verifikasi pada cost bawaan gagal")
	}
	if dVerify > 5*time.Second {
		t.Errorf("verifikasi terlalu lambat: %v", dVerify)
	}
	// DummyVerify harus sebanding dengan verifikasi sungguhan (agar waktu tidak membocorkan akun).
	if dDummy < dVerify/3 || dDummy > dVerify*3 {
		t.Errorf("DummyVerify (%v) tidak sebanding dengan VerifyPassword (%v)", dDummy, dVerify)
	}
}

func TestDummyVerifyHandlesLongInput(t *testing.T) {
	withCost(t, 4)
	DummyVerify(strings.Repeat("x", 500)) // tidak boleh panik atau galat
}

func TestValidateAccountPassword(t *testing.T) {
	ok := []string{"kuda-laut-biru-77", "Tr0ub4dor&3x!", "sangat panjang tapi mudah diingat", "ß漢字🙂ß漢字🙂ab12", "xK9#mP2$vL", strings.Repeat("abcdefghij", 7) + "kl"}
	for _, p := range ok {
		if err := ValidateAccountPassword(p, "budi@example.com", "budisantoso"); err != nil {
			t.Errorf("%q seharusnya lolos: %v", p, err)
		}
	}
	bad := map[string]string{
		"pendek1!":              "minimal 10",
		"123456789":             "minimal 10",
		strings.Repeat("a", 73): "maksimal 72",
		strings.Repeat("漢", 25): "maksimal 72", // 75 byte
		"aaaaaaaaaaaa":          "terlalu sederhana",
		"abababababab":          "terlalu sederhana",
		"password123":           "terlalu umum",
		"Password123!":          "terlalu umum",
		"PASSWORD1234567":       "terlalu umum",
		"qwertyuiop":            "terlalu umum",
		"1234567890":            "terlalu umum",
		"iloveyou2024":          "terlalu umum",
		"bismillah123":          "terlalu umum",
		"budi-rahasia-2024x":    "email atau username",
		"BUDISANTOSO-ku-99":     "email atau username",
		"salamku_budisantoso1":  "email atau username",
	}
	for p, wantSub := range bad {
		err := ValidateAccountPassword(p, "budi@example.com", "budisantoso")
		if err == nil {
			t.Errorf("%q seharusnya ditolak", p)
			continue
		}
		if !strings.Contains(strings.ToLower(err.Error()), wantSub) {
			t.Errorf("%q: pesan %q tidak memuat %q", p, err.Error(), wantSub)
		}
	}
	// Username/email pendek (< 4 karakter) tidak dipakai sebagai pola agar tidak memblokir berlebihan.
	if err := ValidateAccountPassword("kuda-laut-biru-77", "ab@example.com", "ab"); err != nil {
		t.Errorf("identitas pendek tidak boleh memblokir: %v", err)
	}
	// Sandi sama dengan email.
	if err := ValidateAccountPassword("budi@example.com", "budi@example.com", "x"); err == nil {
		t.Errorf("sandi = email harus ditolak")
	}
}
