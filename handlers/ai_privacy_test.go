package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"notebook-app/config"
)

func newNote(t *testing.T, uid int64, locked bool) int64 {
	t.Helper()
	res, err := config.DB.Exec("INSERT INTO notes (user_id, title, content, is_locked) VALUES (?, 'N', '<p>rahasia</p>', ?)", uid, locked)
	if err != nil {
		t.Fatalf("buat catatan: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func share(t *testing.T, noteID, ownerID, withUser int64, active bool, expiresSQL string) {
	t.Helper()
	q := "INSERT INTO shares (user_id, item_type, item_id, share_mode, permission, shared_with_user_id, is_active, expires_at) VALUES (?, 'note', ?, 'private', 'editor', ?, ?, " + expiresSQL + ")"
	if _, err := config.DB.Exec(q, ownerID, noteID, withUser, active); err != nil {
		t.Fatalf("buat share: %v", err)
	}
}

func TestIntegrationAICheckNote(t *testing.T) {
	integrationDB(t)
	owner, other, guest := makeTestUser(t), makeTestUser(t), makeTestUser(t)
	open, locked := newNote(t, owner, false), newNote(t, owner, true)
	shared := newNote(t, owner, false)
	expired, inactive, sharedLocked := newNote(t, owner, false), newNote(t, owner, false), newNote(t, owner, true)
	share(t, shared, owner, guest, true, "NULL")
	share(t, expired, owner, guest, true, "DATE_SUB(NOW(), INTERVAL 1 DAY)")
	share(t, inactive, owner, guest, false, "NULL")
	share(t, sharedLocked, owner, guest, true, "NULL")

	cases := []struct {
		name       string
		note, user int64
		want       aiNoteStatus
	}{
		{"tanpa catatan konteks", 0, owner, aiNoteOK},
		{"pemilik, tidak terkunci", open, owner, aiNoteOK},
		{"pemilik, terkunci", locked, owner, aiNoteIsLock},
		{"orang lain tanpa share", open, other, aiNoteDenied},
		{"orang lain pada catatan terkunci: tidak bocor status kunci", locked, other, aiNoteDenied},
		{"catatan tidak ada", 99999999, owner, aiNoteDenied},
		{"penerima share aktif", shared, guest, aiNoteOK},
		{"penerima share kedaluwarsa", expired, guest, aiNoteDenied},
		{"penerima share nonaktif", inactive, guest, aiNoteDenied},
		{"penerima share pada catatan terkunci", sharedLocked, guest, aiNoteIsLock},
	}
	for _, c := range cases {
		if got := aiCheckNote(c.note, c.user); got != c.want {
			t.Errorf("%s: dapat %d, mau %d", c.name, got, c.want)
		}
	}

	// GAGAL-TERTUTUP: DB bermasalah tidak boleh dianggap "tidak terkunci".
	closed, _ := sql.Open("mysql", "nbtest:nbtest@tcp(127.0.0.1:3306)/notebook_db")
	_ = closed.Close()
	orig := config.DB
	config.DB = closed
	defer func() { config.DB = orig }()
	if got := aiCheckNote(open, owner); got != aiNoteError {
		t.Errorf("DB error harus aiNoteError (gagal-tertutup), dapat %d", got)
	}
}

func TestIntegrationAIGenerateNeverSendsRestrictedNotesToProvider(t *testing.T) {
	integrationDB(t)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(chatReply("hasil")))
	}))
	defer srv.Close()
	os.Setenv("AI_API_KEY", "k")
	os.Setenv("AI_BASE_URL", srv.URL)
	os.Setenv("AI_API_STYLE", "openai")
	defer os.Unsetenv("AI_API_STYLE")

	owner, other := makeTestUser(t), makeTestUser(t)
	open, locked := newNote(t, owner, false), newNote(t, owner, true)

	run := func(uid, noteID int64) (int, int32) {
		before := atomic.LoadInt32(&hits)
		code, _ := call(t, fmt.Sprintf(`{"action":"summarize","text":"isi catatan","note_id":%d}`, noteID), uid)
		return code, atomic.LoadInt32(&hits) - before
	}

	if code, n := run(owner, open); code != 200 || n != 1 {
		t.Errorf("pemilik + tidak terkunci: harus 200 dan 1 panggilan penyedia, dapat %d dan %d", code, n)
	}
	if code, n := run(owner, locked); code != 423 || n != 0 {
		t.Errorf("terkunci: harus 423 tanpa panggilan penyedia, dapat %d dan %d", code, n)
	}
	if code, n := run(other, open); code != 404 || n != 0 {
		t.Errorf("catatan orang lain: harus 404 tanpa panggilan penyedia, dapat %d dan %d", code, n)
	}
	if code, n := run(other, locked); code != 404 || n != 0 {
		t.Errorf("catatan terkunci orang lain: harus 404 (bukan 423) tanpa panggilan penyedia, dapat %d dan %d", code, n)
	}
	if code, n := run(owner, 99999999); code != 404 || n != 0 {
		t.Errorf("catatan tak dikenal: harus 404 tanpa panggilan penyedia, dapat %d dan %d", code, n)
	}

	// DB bermasalah: gagal-tertutup 503, tidak ada kebocoran.
	closed, _ := sql.Open("mysql", "nbtest:nbtest@tcp(127.0.0.1:3306)/notebook_db")
	_ = closed.Close()
	orig := config.DB
	config.DB = closed
	code, n := run(makeUserFixedForClosedDB(), open)
	config.DB = orig
	if code != 503 || n != 0 {
		t.Errorf("DB bermasalah: harus 503 tanpa panggilan penyedia, dapat %d dan %d", code, n)
	}
}

// Saat DB ditutup, user tidak bisa dibuat; pakai ID tetap (pembatas pemakaian AI ada di memori).
func makeUserFixedForClosedDB() int64 { return 987654321 }
