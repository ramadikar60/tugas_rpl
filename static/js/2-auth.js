// ============================================================
// 2-AUTH.JS — Authentication, Profile, Preferences
// ============================================================

// ============================================================
// AUTH ENTER HANDLERS
// ============================================================
function setupAuthEnterHandlers() {
  const loginEmail = document.getElementById('loginEmail');
  const loginPassword = document.getElementById('loginPassword');

  if (loginEmail) {
    loginEmail.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        login();
      }
    });
  }

  if (loginPassword) {
    loginPassword.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        login();
      }
    });
  }

  const regUsername = document.getElementById('regUsername');
  const regEmail = document.getElementById('regEmail');
  const regPassword = document.getElementById('regPassword');

  if (regUsername) {
    regUsername.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        register();
      }
    });
  }

  if (regEmail) {
    regEmail.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        register();
      }
    });
  }

  if (regPassword) {
    regPassword.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        register();
      }
    });
  }
}

// ============================================================
// 6. AUTH
// ============================================================
async function tryRefresh() {
  if (refreshInFlight) return refreshInFlight;
  refreshInFlight = (async () => {
    try {
      const res = await fetch('/api/auth/refresh', {
        method: 'POST',
        credentials: 'same-origin'
      });
      if (!res.ok) return false;
      const data = await res.json();
      token = data.token;
      csrfToken = data.csrf_token;
      localStorage.setItem('token', token);
      localStorage.setItem('csrf', csrfToken);
      return true;
    } catch {
      return false;
    } finally {
      refreshInFlight = null;
    }
  })();
  return refreshInFlight;
}

async function login() {
  const email = document.getElementById('loginEmail').value.trim();
  const password = document.getElementById('loginPassword').value;

  if (!email || !password) {
    showToast('Email dan password wajib diisi.', 'error');
    return;
  }

  const res = await fetch('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ email, password })
  });
  const data = await res.json();

  if (data.token) {
    token = data.token;
    csrfToken = data.csrf_token;
    localStorage.setItem('token', token);
    localStorage.setItem('csrf', csrfToken);

    if (data.user && data.user.role) {
      localStorage.setItem('user_role', data.user.role);
    }

    showApp();
    if (typeof initQuill === 'function') initQuill();
    loadAll();
    if (typeof startNotifPolling === 'function') startNotifPolling();
    setTimeout(() => {
      if (typeof checkAdminAccess === 'function') checkAdminAccess();
    }, 800);
  } else {
    showToast(data.error || 'Login gagal', 'error');
  }
}

async function register() {
  const username = document.getElementById('regUsername').value.trim();
  const email = document.getElementById('regEmail').value.trim();
  const password = document.getElementById('regPassword').value;

  if (!username || !email || !password) {
    showToast('Semua field wajib diisi.', 'error');
    return;
  }
  if (password.length < 10) {
    showToast('Password minimal 10 karakter.', 'error');
    return;
  }

  const res = await fetch('/api/auth/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ username, email, password })
  });

  const data = await res.json();

  if (res.ok) {
    showToast('Registrasi berhasil! Silakan masuk.', 'success');
    showLogin();
    document.getElementById('loginEmail').value = email;
    document.getElementById('loginPassword').value = '';
    document.getElementById('loginPassword').focus();
    document.getElementById('regUsername').value = '';
    document.getElementById('regEmail').value = '';
    document.getElementById('regPassword').value = '';
  } else {
    showToast(data.error || 'Registrasi gagal', 'error');
  }
}

async function logout() {
  try {
    await fetch('/api/auth/logout', { method: 'POST', credentials: 'same-origin' });
  } catch {}
  localStorage.removeItem('token');
  localStorage.removeItem('csrf');
  localStorage.removeItem('user_role');
  if (typeof notifPollInterval !== 'undefined' && notifPollInterval) {
    clearInterval(notifPollInterval);
  }
  location.reload();
}

function showAuthForm(id) {
  ['loginForm', 'registerForm', 'forgotForm', 'resetForm'].forEach(f => {
    const el = document.getElementById(f);
    if (el) el.style.display = (f === id) ? 'block' : 'none';
  });
}

function showRegister() { showAuthForm('registerForm'); }

function showLogin() { showAuthForm('loginForm'); }

// ---------- Lupa password / reset (Prompt 22A) ----------
// Server tidak pernah mengembalikan token reset. Token hanya sampai lewat email,
// berupa tautan dengan token di fragmen URL: /#reset=<token>.

function showForgot() {
  showAuthForm('forgotForm');
  const loginEmail = document.getElementById('loginEmail');
  const forgotEmail = document.getElementById('forgotEmail');
  if (forgotEmail) {
    if (loginEmail && loginEmail.value && !forgotEmail.value) forgotEmail.value = loginEmail.value;
    forgotEmail.focus();
  }
}

async function submitForgot() {
  const email = document.getElementById('forgotEmail').value.trim();
  const btn = document.getElementById('forgotSubmitBtn');
  if (!email) {
    showToast('Masukkan email Anda.', 'error');
    return;
  }
  btn.disabled = true;
  try {
    const res = await fetch('/api/auth/forgot-password', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'same-origin',
      body: JSON.stringify({ email })
    });
    const data = await res.json().catch(() => ({}));
    if (res.ok) {
      showToast(data.message || 'Jika email terdaftar, tautan reset telah dikirim.', 'success', 6000);
      showLogin();
    } else {
      showToast(data.error || 'Gagal mengirim permintaan.', 'error');
    }
  } catch (e) {
    showToast('Tidak bisa terhubung ke server.', 'error');
  } finally {
    btn.disabled = false;
  }
}

// Dipanggil saat halaman dimuat: bila URL berisi #reset=<token>, tampilkan form reset.
function initResetLink() {
  const m = /^#reset=([0-9a-fA-F]{64})$/.exec(location.hash);
  if (!m) return;
  window.__pendingResetToken = m[1].toLowerCase();
  // Hapus token dari URL dan riwayat agar tidak tertinggal di address bar.
  history.replaceState(null, '', location.pathname + location.search);
  showAuthForm('resetForm');
  const p = document.getElementById('resetPassword');
  if (p) p.focus();
}

async function submitReset() {
  const token = window.__pendingResetToken;
  if (!token) {
    showToast('Tautan reset tidak valid. Minta tautan baru.', 'error');
    showLogin();
    return;
  }
  const p1 = document.getElementById('resetPassword').value;
  const p2 = document.getElementById('resetPassword2').value;
  if (p1.length < 10) {
    showToast('Password minimal 10 karakter.', 'error');
    return;
  }
  if (p1 !== p2) {
    showToast('Kedua password harus sama.', 'error');
    return;
  }

  const btn = document.getElementById('resetSubmitBtn');
  btn.disabled = true;
  try {
    const res = await fetch('/api/auth/reset-password', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'same-origin',
      body: JSON.stringify({ token, new_password: p1 })
    });
    const data = await res.json().catch(() => ({}));
    if (res.ok) {
      window.__pendingResetToken = null;
      document.getElementById('resetPassword').value = '';
      document.getElementById('resetPassword2').value = '';
      // Semua sesi lama dicabut server; bersihkan sisa di browser lalu masuk ulang.
      localStorage.removeItem('token');
      localStorage.removeItem('csrf');
      showToast(data.message || 'Password berhasil direset. Silakan masuk.', 'success', 5000);
      showLogin();
    } else {
      showToast(data.error || 'Gagal mereset password.', 'error', 5000);
      // Token salah/kedaluwarsa: tidak ada gunanya mencoba lagi dengan token yang sama.
      if (/token/i.test(data.error || '')) {
        window.__pendingResetToken = null;
        showLogin();
      }
    }
  } catch (e) {
    showToast('Tidak bisa terhubung ke server.', 'error');
  } finally {
    btn.disabled = false;
  }
}

function cancelReset() {
  window.__pendingResetToken = null;
  location.reload(); // kembali ke aplikasi bila masih ada sesi, atau ke halaman masuk
}

function showApp() {
  document.getElementById('authView').style.display = 'none';
  document.getElementById('appView').style.display = 'flex';
  showListMode();
}

// ============================================================
// 27. USER PREFERENCES
// ============================================================
async function loadPreferences() {
  const prefs = await api('/preferences');
  if (!prefs) return;

  if (prefs.theme && prefs.theme !== 'system') {
    document.documentElement.setAttribute('data-theme', prefs.theme);
    localStorage.setItem('theme', prefs.theme);
    updateThemeButtons(prefs.theme);
  }

  if (prefs.accent_color) applyAccentColor(prefs.accent_color);

  if (prefs.font_size) {
    document.documentElement.style.setProperty('--editor-font-size', prefs.font_size + 'px');
  }

  if (prefs.compact_mode) document.body.classList.add('compact-mode');
}

async function savePreferences(changes) {
  await api('/preferences', {
    method: 'PUT',
    body: JSON.stringify(changes)
  });
}

function applyAccentColor(color) {
  const root = document.documentElement;
  root.style.setProperty('--accent', color);
  root.style.setProperty('--accent-hover', shadeColor(color, -10));
  root.style.setProperty('--accent-soft', hexToRgba(color, 0.1));
}

function shadeColor(color, percent) {
  const num = parseInt(color.replace('#', ''), 16);
  const amt = Math.round(2.55 * percent);
  const R = Math.max(0, Math.min(255, (num >> 16) + amt));
  const G = Math.max(0, Math.min(255, ((num >> 8) & 0x00FF) + amt));
  const B = Math.max(0, Math.min(255, (num & 0x0000FF) + amt));
  return '#' + (0x1000000 + R * 0x10000 + G * 0x100 + B).toString(16).slice(1);
}

function hexToRgba(hex, alpha) {
  const num = parseInt(hex.replace('#', ''), 16);
  const R = num >> 16;
  const G = (num >> 8) & 0x00FF;
  const B = num & 0x0000FF;
  return `rgba(${R},${G},${B},${alpha})`;
}

// ============================================================
// PROFILE MANAGEMENT
// ============================================================

async function openProfileModal() {
  const modal = document.getElementById('profileModal');
  modal.style.display = 'flex';
  await loadProfileIntoModal();
  loadSessions();
  loadLoginHistory();
}

// ---------- Keamanan akun: sesi aktif dan riwayat login (SRS FR-092, FR-093) ----------

// Ringkasan perangkat dari user agent (hanya untuk tampilan).
function describeDevice(ua) {
  ua = String(ua || '');
  let browser = 'Browser';
  if (/Edg\//.test(ua)) browser = 'Edge';
  else if (/OPR\/|Opera/.test(ua)) browser = 'Opera';
  else if (/Firefox\//.test(ua)) browser = 'Firefox';
  else if (/Chrome\//.test(ua)) browser = 'Chrome';
  else if (/Safari\//.test(ua)) browser = 'Safari';
  let os = '';
  if (/Windows/.test(ua)) os = 'Windows';
  else if (/Android/.test(ua)) os = 'Android';
  else if (/iPhone|iPad|iOS/.test(ua)) os = 'iOS';
  else if (/Mac OS X|Macintosh/.test(ua)) os = 'macOS';
  else if (/Linux/.test(ua)) os = 'Linux';
  return os ? browser + ' di ' + os : browser;
}

function profileRow(titleText, metaText, badgeText, actionLabel, onAction) {
  const row = document.createElement('div');
  row.className = 'session-row';
  const info = document.createElement('div');
  info.className = 'session-info';
  const title = document.createElement('div');
  title.className = 'session-title';
  title.textContent = titleText;
  if (badgeText) {
    const badge = document.createElement('span');
    badge.className = 'badge-current';
    badge.textContent = badgeText;
    title.appendChild(badge);
  }
  const meta = document.createElement('div');
  meta.className = 'session-meta';
  meta.textContent = metaText;
  info.appendChild(title);
  info.appendChild(meta);
  row.appendChild(info);
  if (actionLabel) {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'btn-ghost-sm';
    btn.textContent = actionLabel;
    btn.addEventListener('click', onAction);
    row.appendChild(btn);
  }
  return row;
}

function fmtWhen(s) {
  return (typeof formatDateTime === 'function') ? formatDateTime(s) : String(s || '');
}

async function loadSessions() {
  const el = document.getElementById('sessionList');
  if (!el) return;
  const sessions = await api('/auth/sessions');
  el.textContent = '';
  if (!Array.isArray(sessions) || sessions.length === 0) {
    const p = document.createElement('p');
    p.className = 'profile-section-hint';
    p.textContent = 'Tidak ada sesi aktif yang tercatat.';
    el.appendChild(p);
    return;
  }
  sessions.forEach(s => {
    const meta = (s.ip || 'IP tidak diketahui') + ' · masuk ' + fmtWhen(s.created_at);
    el.appendChild(profileRow(
      describeDevice(s.user_agent), meta,
      s.current ? 'Perangkat ini' : '',
      s.current ? '' : 'Keluar',
      () => revokeSessionById(s.id)
    ));
  });
}

async function revokeSessionById(id) {
  const res = await api('/auth/sessions/' + encodeURIComponent(id), { method: 'DELETE' });
  if (res && res.error) {
    showToast(res.error, 'error');
    return;
  }
  showToast('Sesi dicabut.', 'success');
  loadSessions();
}

async function logoutAllDevices() {
  if (!confirm('Keluar dari semua perangkat? Anda perlu masuk lagi di setiap perangkat.')) return;
  const btn = document.getElementById('logoutAllBtn');
  if (btn) btn.disabled = true;
  const res = await api('/auth/logout-all', { method: 'POST' });
  if (res && res.error) {
    if (btn) btn.disabled = false;
    showToast(res.error, 'error');
    return;
  }
  logout(); // sesi di perangkat ini juga berakhir
}

const LOGIN_EVENT_LABEL = {
  login: 'Login berhasil',
  login_failed: 'Percobaan login gagal',
  logout: 'Logout',
  logout_all: 'Keluar dari semua perangkat',
  password_change: 'Password diubah',
  password_reset: 'Password direset',
  session_revoke: 'Sesi dicabut',
  register: 'Akun dibuat'
};

async function loadLoginHistory() {
  const el = document.getElementById('loginHistoryList');
  if (!el) return;
  const events = await api('/activity?limit=15&action=' + Object.keys(LOGIN_EVENT_LABEL).join(','));
  el.textContent = '';
  if (!Array.isArray(events) || events.length === 0) {
    const p = document.createElement('p');
    p.className = 'profile-section-hint';
    p.textContent = 'Belum ada riwayat login.';
    el.appendChild(p);
    return;
  }
  events.forEach(e => {
    const row = profileRow(
      LOGIN_EVENT_LABEL[e.action] || e.description || e.action,
      (e.ip || 'IP tidak diketahui') + ' · ' + fmtWhen(e.created_at),
      '', '', null
    );
    if (e.action === 'login_failed') row.classList.add('is-warning');
    el.appendChild(row);
  });
}

function closeProfileModal() {
  document.getElementById('profileModal').style.display = 'none';
}

async function loadProfileIntoModal() {
  const profile = await api('/profile');
  if (!profile) {
    showToast('Gagal memuat profil', 'error');
    return;
  }

  const initial = (profile.username || '?').charAt(0).toUpperCase();

  const avatarEl = document.getElementById('profileAvatar');
  if (profile.avatar) {
    avatarEl.innerHTML = '<img src="' + escapeHtml(profile.avatar) + '" style="width:100%;height:100%;object-fit:cover;border-radius:50%;" onerror="this.parentElement.textContent=\'' + initial + '\'">';
  } else {
    avatarEl.textContent = initial;
  }

  document.getElementById('profileDisplayName').textContent = profile.username || '-';
  document.getElementById('profileDisplayEmail').textContent = profile.email || '-';
  document.getElementById('profileDisplayRole').textContent = 'Role: ' + (profile.role || 'user');

  document.getElementById('profileUsernameInput').value = profile.username || '';
  document.getElementById('profileEmailInput').value = profile.email || '';
  document.getElementById('profileAvatarInput').value = profile.avatar || '';

  window._currentProfile = profile;
}

async function saveProfileChanges() {
  const username = document.getElementById('profileUsernameInput').value.trim();
  const avatar = document.getElementById('profileAvatarInput').value.trim();

  if (!username) {
    showToast('Username wajib diisi', 'error');
    return;
  }

  if (username.length < 3) {
    showToast('Username minimal 3 karakter', 'error');
    return;
  }

  const res = await api('/profile', {
    method: 'PUT',
    body: JSON.stringify({ username, avatar })
  });

  if (res) {
    showToast('Profil berhasil diupdate', 'success');
    await loadProfileIntoModal();
  } else {
    showToast('Gagal menyimpan profil', 'error');
  }
}

// ============================================================
// CHANGE PASSWORD
// ============================================================

function openChangePasswordModal() {
  document.getElementById('cpOldPassword').value = '';
  document.getElementById('cpNewPassword').value = '';
  document.getElementById('cpConfirmPassword').value = '';
  document.getElementById('cpStrength').textContent = '';

  document.getElementById('changePasswordModal').style.display = 'flex';

  setTimeout(() => {
    document.getElementById('cpOldPassword').focus();
  }, 100);
}

function closeChangePasswordModal() {
  document.getElementById('changePasswordModal').style.display = 'none';
}

document.addEventListener('DOMContentLoaded', () => {
  const newPwInput = document.getElementById('cpNewPassword');
  if (newPwInput) {
    newPwInput.addEventListener('input', () => {
      updatePasswordStrength(newPwInput.value);
    });
  }

  ['cpOldPassword', 'cpNewPassword', 'cpConfirmPassword'].forEach(id => {
    const el = document.getElementById(id);
    if (el) {
      el.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          submitChangePassword();
        }
      });
    }
  });
});

function updatePasswordStrength(pw) {
  const el = document.getElementById('cpStrength');
  if (!el) return;

  if (!pw) {
    el.textContent = '';
    el.style.color = 'var(--text-muted)';
    return;
  }

  let score = 0;
  if (pw.length >= 6) score++;
  if (pw.length >= 10) score++;
  if (/[a-z]/.test(pw) && /[A-Z]/.test(pw)) score++;
  if (/\d/.test(pw)) score++;
  if (/[^a-zA-Z0-9]/.test(pw)) score++;

  const labels = ['Sangat lemah', 'Lemah', 'Cukup', 'Kuat', 'Sangat kuat', 'Sangat kuat'];
  const colors = ['#dc2626', '#dc2626', '#f59e0b', '#16a34a', '#16a34a', '#16a34a'];

  el.textContent = 'Kekuatan: ' + labels[score];
  el.style.color = colors[score];
}

async function submitChangePassword() {
  const oldPassword = document.getElementById('cpOldPassword').value;
  const newPassword = document.getElementById('cpNewPassword').value;
  const confirmPassword = document.getElementById('cpConfirmPassword').value;

  if (!oldPassword) {
    showToast('Password lama wajib diisi', 'error');
    return;
  }
  if (!newPassword) {
    showToast('Password baru wajib diisi', 'error');
    return;
  }
  if (newPassword.length < 10) {
    showToast('Password baru minimal 10 karakter', 'error');
    return;
  }
  if (newPassword === oldPassword) {
    showToast('Password baru tidak boleh sama dengan password lama', 'error');
    return;
  }
  if (newPassword !== confirmPassword) {
    showToast('Konfirmasi password tidak cocok', 'error');
    return;
  }

  const btn = document.getElementById('cpSubmitBtn');
  btn.disabled = true;
  btn.textContent = 'Menyimpan...';

  const res = await api('/profile/change-password', {
    method: 'POST',
    body: JSON.stringify({
      old_password: oldPassword,
      new_password: newPassword
    })
  });

  btn.disabled = false;
  btn.textContent = 'Ubah Password';

  if (res && res.message) {
    showToast('Password berhasil diubah. Silakan login ulang...', 'success', 3000);
    closeChangePasswordModal();
    closeProfileModal();

    setTimeout(() => {
      logout();
    }, 2000);
  } else {
    // Tampilkan alasan dari server (password lama salah, atau aturan password baru).
    showToast((res && res.error) || 'Gagal mengubah password', 'error', 5000);
  }
}

console.log('2-auth.js loaded');