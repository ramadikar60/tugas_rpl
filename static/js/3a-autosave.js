// ============================================================
// 3A-AUTOSAVE.JS - Simpan otomatis catatan (Prompt 13c)
//
// Aturan:
//  - Kirim hanya bila isi benar-benar berubah (bandingkan dengan baseline).
//  - Satu simpan berjalan pada satu waktu; perubahan baru menyusul sesudahnya.
//  - Keberhasilan ditentukan dari respons server, bukan dari "tidak ada error".
//  - Gagal: status jelas + tombol "Coba lagi"; gagal jaringan dicoba ulang otomatis.
//  - Simpan saat editor kehilangan fokus, tab disembunyikan, dan halaman ditutup.
//  - Pindah catatan/halaman ditahan bila simpan gagal (klik sekali lagi untuk lanjut).
//
// Bergantung pada global dari 1-core.js / 3-notes.js:
//   quill, currentNoteId, saveTimer, token, csrfToken, api(), showToast()
// dan (opsional) noteSourceHTML() dari 0-sanitize.js.
// ============================================================

const AutosaveConfig = {
  debounceMs: 1500,                       // jeda sejak ketikan terakhir
  retryDelaysMs: [2000, 5000, 15000],     // coba ulang otomatis (hanya gagal jaringan)
  loadGuardMs: 400,                       // abaikan event sisa pemuatan catatan
  leaveGraceMs: 10000,                    // jendela "klik sekali lagi untuk tetap pindah"
  keepaliveMaxBytes: 60000                // batas body fetch keepalive (~64 KB)
};

let autosaveBaseline = null;      // "judul\0isi" terakhir yang diketahui sudah tersimpan
let autosaveInFlight = null;      // Promise simpan yang sedang berjalan
let autosaveFailed = false;
let autosaveRetryCount = 0;
let autosaveRetryTimer = null;
let autosaveLoadGuardUntil = 0;
let autosaveUserTouched = false;
let autosaveLeaveUntil = 0;

function autosaveIsViewer() {
  const access = window._currentNoteAccess || {};
  return !!access.isSharedViewer;
}

// Isi yang dikirim/dibandingkan: bentuk "sumber" (tanpa hasil render KaTeX/Mermaid/TOC),
// sama dengan yang akhirnya disimpan server.
function autosaveSource() {
  const html = quill.root.innerHTML;
  return (typeof noteSourceHTML === 'function') ? noteSourceHTML(html) : html;
}

function autosaveSnapshot(title, content) {
  return title + '\u0000' + content;
}

function autosaveIsDirty() {
  if (!currentNoteId || !quill || autosaveIsViewer()) return false;
  const title = document.getElementById('noteTitle').value;
  return autosaveSnapshot(title, autosaveSource()) !== autosaveBaseline;
}

// ---------- Status ----------

function setSaveStatus(state, detail) {
  const el = document.getElementById('saveStatus');
  if (!el) return;
  el.classList.toggle('save-error', state === 'error');
  el.textContent = '';

  if (state === 'saving') {
    el.textContent = 'Menyimpan...';
  } else if (state === 'saved') {
    el.textContent = 'Tersimpan · ' + new Date().toLocaleTimeString('id-ID');
  } else if (state === 'viewer') {
    el.textContent = 'Mode lihat saja, tidak bisa menyimpan';
  } else if (state === 'error') {
    const msg = document.createElement('span');
    msg.textContent = 'Gagal menyimpan' + (detail ? ' (' + detail + ')' : '') + '. ';
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'save-retry-btn';
    btn.textContent = 'Coba lagi';
    btn.addEventListener('click', () => { autosaveRetryCount = 0; saveNote(); });
    el.appendChild(msg);
    el.appendChild(btn);
  }
}

// ---------- Penjadwalan ----------

function scheduleSave() {
  if (!currentNoteId) return;
  if (autosaveIsViewer()) { setSaveStatus('viewer'); return; }
  // Event sisa dari pemuatan catatan (bukan perubahan pengguna).
  if (Date.now() < autosaveLoadGuardUntil && !autosaveUserTouched) return;

  setSaveStatus('saving');
  clearTimeout(saveTimer);
  saveTimer = setTimeout(saveNote, AutosaveConfig.debounceMs);
}

function onTitleChange() {
  autosaveUserTouched = true;
  scheduleSave();
}

// Dipanggil openNote() setelah judul dan isi dimuat ke editor.
function autosaveResetForNote() {
  clearTimeout(saveTimer);
  clearTimeout(autosaveRetryTimer);
  autosaveFailed = false;
  autosaveRetryCount = 0;
  autosaveUserTouched = false;
  autosaveLoadGuardUntil = Date.now() + AutosaveConfig.loadGuardMs;
  autosaveBaseline = autosaveSnapshot(document.getElementById('noteTitle').value, autosaveSource());

  // Post-process pemuatan (id heading, rehidrasi) mengubah DOM sedikit: ambil ulang
  // baseline bila pengguna belum menyentuh apa pun.
  const id = currentNoteId;
  setTimeout(() => {
    if (currentNoteId === id && !autosaveUserTouched && !autosaveFailed) {
      autosaveBaseline = autosaveSnapshot(document.getElementById('noteTitle').value, autosaveSource());
    }
  }, AutosaveConfig.loadGuardMs + 50);
}

// ---------- Simpan ----------

// Mengembalikan Promise<boolean>: true = tidak ada yang perlu disimpan / berhasil.
function saveNote() {
  if (autosaveInFlight) return autosaveInFlight.then(() => saveNote());
  autosaveInFlight = autosaveRun().finally(() => { autosaveInFlight = null; });
  return autosaveInFlight;
}

async function autosaveRun() {
  const noteId = currentNoteId;
  if (!noteId || !quill) return true;
  if (autosaveIsViewer()) { setSaveStatus('viewer'); return true; }
  clearTimeout(saveTimer);

  const title = document.getElementById('noteTitle').value;
  const content = autosaveSource();
  const snap = autosaveSnapshot(title, content);
  if (snap === autosaveBaseline) return true; // tidak ada perubahan nyata

  setSaveStatus('saving');
  let res;
  try {
    res = await api('/notes/' + noteId, {
      method: 'PUT',
      body: JSON.stringify({ title, content })
    });
  } catch (e) {
    return autosaveFail(noteId, navigator.onLine === false ? 'tidak ada koneksi' : 'koneksi bermasalah', true);
  }
  if (!res || res.error) {
    return autosaveFail(noteId, res && res.error ? String(res.error) : 'server tidak merespons', false);
  }

  if (noteId !== currentNoteId) return true; // pengguna sudah pindah catatan
  autosaveBaseline = snap;
  autosaveFailed = false;
  autosaveRetryCount = 0;
  clearTimeout(autosaveRetryTimer);

  // Ada perubahan baru selagi menyimpan?
  const nowTitle = document.getElementById('noteTitle').value;
  if (autosaveSnapshot(nowTitle, autosaveSource()) !== autosaveBaseline) scheduleSave();
  else setSaveStatus('saved');
  return true;
}

function autosaveFail(noteId, detail, retryable) {
  if (noteId !== currentNoteId) return false;
  autosaveFailed = true;
  setSaveStatus('error', detail);
  if (retryable && autosaveRetryCount < AutosaveConfig.retryDelaysMs.length) {
    const delay = AutosaveConfig.retryDelaysMs[autosaveRetryCount++];
    clearTimeout(autosaveRetryTimer);
    autosaveRetryTimer = setTimeout(() => { if (currentNoteId === noteId) saveNote(); }, delay);
  }
  return false;
}

// ---------- Pindah catatan / halaman ----------

// true = boleh pindah. Bila simpan gagal, pindah ditahan sekali; klik kedua dalam
// leaveGraceMs berarti pengguna memilih tetap pindah (perubahan dilepas).
async function ensureSavedBeforeLeave() {
  if (!currentNoteId || !quill || autosaveIsViewer()) return true;
  const ok = await saveNote();
  if (ok) { autosaveLeaveUntil = 0; return true; }

  const now = Date.now();
  if (autosaveLeaveUntil && now < autosaveLeaveUntil) {
    autosaveLeaveUntil = 0;
    return true;
  }
  autosaveLeaveUntil = now + AutosaveConfig.leaveGraceMs;
  showToast('Perubahan belum tersimpan. Klik sekali lagi untuk tetap pindah.', 'error', 4000);
  return false;
}

// ---------- Simpan saat keluar ----------

// Dipakai saat halaman ditutup: fetch keepalive tetap berjalan setelah halaman hilang.
function autosaveKeepaliveFlush() {
  if (!autosaveIsDirty()) return;
  const title = document.getElementById('noteTitle').value;
  const content = autosaveSource();
  const body = JSON.stringify({ title, content });
  if (body.length > AutosaveConfig.keepaliveMaxBytes) return; // terlalu besar untuk keepalive

  const headers = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = 'Bearer ' + token;
  if (csrfToken) headers['X-CSRF-Token'] = csrfToken;
  try {
    fetch('/api/notes/' + currentNoteId, {
      method: 'PUT', headers, body, keepalive: true, credentials: 'same-origin'
    }).catch(() => {});
  } catch (e) { /* diabaikan: halaman sedang ditutup */ }
}

function autosaveIsEditorTarget(t) {
  if (!t) return false;
  if (t.id === 'noteTitle') return true;
  return !!(t.classList && t.classList.contains('ql-editor'));
}

// Tanda bahwa pengguna sendiri yang berinteraksi (bukan event sisa pemuatan).
['keydown', 'beforeinput', 'paste', 'cut', 'drop'].forEach(evt => {
  document.addEventListener(evt, (e) => {
    if (autosaveIsEditorTarget(e.target)) autosaveUserTouched = true;
  }, true);
});

// Kehilangan fokus (ke luar editor) = simpan sekarang.
document.addEventListener('focusout', (e) => {
  if (!autosaveIsEditorTarget(e.target)) return;
  const editorMode = document.getElementById('editorMode');
  if (e.relatedTarget && editorMode && editorMode.contains(e.relatedTarget)) return;
  if (autosaveIsDirty()) saveNote();
});

document.addEventListener('visibilitychange', () => {
  if (document.visibilityState === 'hidden' && autosaveIsDirty()) saveNote();
});

window.addEventListener('pagehide', autosaveKeepaliveFlush);

window.addEventListener('beforeunload', (e) => {
  if (autosaveFailed || autosaveInFlight || autosaveIsDirty()) {
    e.preventDefault();
    e.returnValue = ''; // dialog bawaan browser; hanya muncul bila ada yang belum tersimpan
  }
});

// Koneksi kembali: coba lagi simpan yang gagal.
window.addEventListener('online', () => {
  if (autosaveFailed) { autosaveRetryCount = 0; saveNote(); }
});
