// ============================================================
// 1A-NAV.JS - Navigasi (Prompt 1)
//
//  - 5 menu utama: Beranda, Catatan, Rencana, Pustaka, Asisten AI
//  - Pengalih tampilan (sub-navigasi) per kelompok
//  - Menu "Lainnya" (sidebar) dan menu aksi catatan (header editor)
//  - Onboarding 3 langkah untuk pengguna baru
//
// Tidak mengubah logika tampilan lama: semua halaman tetap dibuka lewat
// navigateTo() di 1-core.js. Sorotan menu dihitung dari mode yang BENAR-BENAR
// tampil, sehingga tetap benar saat perpindahan ditahan (simpan gagal).
// ============================================================

// Tab per kelompok. 'list' memakai filter catatan; yang lain memakai nama view.
const NAV_TABS = {
  notes: [
    { key: 'all', label: 'Semua' },
    { key: 'pinned', label: 'Tersemat' },
    { key: 'favorites', label: 'Favorit' },
    { key: 'archived', label: 'Arsip' },
    { key: 'trash', label: 'Sampah' }
  ],
  plan: [
    { key: 'tasks', label: 'Tugas' },
    { key: 'boards', label: 'Papan Tugas' },
    { key: 'calendar', label: 'Kalender' },
    { key: 'timeline', label: 'Garis Waktu' },
    { key: 'projects', label: 'Proyek' }
  ],
  library: [
    { key: 'clipper', label: 'Klip Web' },
    { key: 'research', label: 'Riset dan Referensi' },
    { key: 'media', label: 'Media' }
  ]
};

// Mode yang sedang tampil -> kelompok menu utama.
const NAV_MODE_GROUP = {
  dashboardMode: 'home',
  listMode: 'notes', editorMode: 'notes',
  tasksMode: 'plan', boardsMode: 'plan', boardDetailMode: 'plan', calendarMode: 'plan',
  timelineMode: 'plan', projectsMode: 'plan', projectDetailMode: 'plan',
  clipperMode: 'library', researchMode: 'library', mediaMode: 'library'
};

// Mode -> tab aktif (untuk kelompok non-catatan).
const NAV_MODE_TAB = {
  tasksMode: 'tasks', boardsMode: 'boards', boardDetailMode: 'boards', calendarMode: 'calendar',
  timelineMode: 'timeline', projectsMode: 'projects', projectDetailMode: 'projects',
  clipperMode: 'clipper', researchMode: 'research', mediaMode: 'media'
};

const navLastTab = { plan: 'tasks', library: 'clipper' };

function navVisibleMode() {
  const ids = (typeof ALL_MODES !== 'undefined') ? ALL_MODES : Object.keys(NAV_MODE_GROUP);
  for (const id of ids) {
    const el = document.getElementById(id);
    if (el && el.style.display !== 'none' && el.style.display !== '') return id;
  }
  return null;
}

// ---------- Menu utama ----------

function navMain(group) {
  if (group === 'home') return navigateTo('dashboard');
  if (group === 'notes') return navigateTo('list', { filter: 'all', notebookId: null, tagId: null });
  if (group === 'plan') return navigateTo(navLastTab.plan);
  if (group === 'library') return navigateTo(navLastTab.library);
  if (group === 'ai') return navAI();
}

function navTab(group, key) {
  if (group === 'notes') return navigateTo('list', { filter: key, notebookId: null, tagId: null });
  return navigateTo(key);
}

async function navAI() {
  const editor = document.getElementById('editorMode');
  const editorOpen = editor && editor.style.display !== 'none' && editor.style.display !== '';
  if (editorOpen && typeof openAIPanel === 'function') { openAIPanel(); return; }

  let msg = 'Buka sebuah catatan, lalu klik Asisten AI untuk meringkas, memperbaiki tulisan, atau menerjemahkan.';
  try {
    const s = await api('/ai/status');
    if (s && s.enabled === false) msg = 'Asisten AI belum aktif. Isi AI_API_KEY di file .env lalu jalankan ulang server.';
    else if (s && s.enabled) msg += ' Sisa jatah hari ini: ' + s.remaining_today + '.';
  } catch (e) { /* abaikan: pesan bawaan sudah cukup */ }
  showToast(msg, undefined, 5000);
}

// Sinkronkan sorotan menu dan pengalih tampilan dengan mode yang tampil.
function navSync() {
  const mode = navVisibleMode();
  const group = mode ? NAV_MODE_GROUP[mode] : null;

  document.querySelectorAll('#primaryNav a[data-nav]').forEach(a => {
    const on = a.dataset.nav === group;
    a.classList.toggle('active', on);
    if (on) a.setAttribute('aria-current', 'page'); else a.removeAttribute('aria-current');
  });

  const bar = document.getElementById('subNav');
  if (!bar) return;

  const tabs = group ? NAV_TABS[group] : null;
  // Di editor, pengalih tampilan disembunyikan agar tidak menyaingi tombol "Kembali".
  if (!tabs || mode === 'editorMode') {
    bar.style.display = 'none';
    bar.textContent = '';
    return;
  }

  let active = null;
  if (group === 'notes') {
    active = (currentNotebookId || currentTagId) ? null : currentFilter;
  } else {
    active = NAV_MODE_TAB[mode] || null;
    if (active) navLastTab[group] = active;
  }

  bar.textContent = '';
  bar.setAttribute('role', 'tablist');
  bar.setAttribute('aria-label', group === 'notes' ? 'Filter catatan' : 'Tampilan');
  tabs.forEach(t => {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'subnav-tab' + (t.key === active ? ' active' : '');
    b.setAttribute('role', 'tab');
    b.setAttribute('aria-selected', t.key === active ? 'true' : 'false');
    b.dataset.key = t.key;
    b.textContent = t.label;
    b.addEventListener('click', () => navTab(group, t.key));
    bar.appendChild(b);
  });
  bar.style.display = 'flex';
}

// Bungkus navigateTo supaya sorotan selalu ikut. Pemanggil lain (onclick, kode lama)
// memanggil navigateTo lewat nama global, jadi otomatis memakai versi pembungkus.
(function wrapNavigate() {
  const original = window.navigateTo;
  if (typeof original !== 'function') return;
  window.navigateTo = async function () {
    const result = await original.apply(this, arguments);
    try { navSync(); } catch (e) { console.error('navSync:', e); }
    return result;
  };
})();

// ---------- Popover (Lainnya dan menu aksi catatan) ----------

let navOpenMenu = null; // { menu, button }

function navPlaceMenu(menu, button, mode) {
  const r = button.getBoundingClientRect();
  menu.style.position = 'fixed';
  if (mode === 'up') {            // sidebar bawah: buka ke atas
    menu.style.top = 'auto';
    menu.style.right = 'auto';
    menu.style.left = Math.max(8, r.left) + 'px';
    menu.style.bottom = (window.innerHeight - r.top + 6) + 'px';
  } else {                        // header editor: buka ke bawah, rata kanan
    menu.style.bottom = 'auto';
    menu.style.left = 'auto';
    menu.style.top = (r.bottom + 6) + 'px';
    menu.style.right = Math.max(8, window.innerWidth - r.right) + 'px';
  }
}

function navCloseMenu() {
  if (!navOpenMenu) return;
  navOpenMenu.menu.style.display = 'none';
  navOpenMenu.button.setAttribute('aria-expanded', 'false');
  navOpenMenu = null;
}

function navToggleMenu(ev, menuId, buttonId, mode) {
  if (ev) { ev.preventDefault(); ev.stopPropagation(); }
  const menu = document.getElementById(menuId);
  const button = document.getElementById(buttonId);
  if (!menu || !button) return;
  const wasOpen = navOpenMenu && navOpenMenu.menu === menu;
  navCloseMenu();
  if (wasOpen) return;
  menu.style.display = 'block';
  navPlaceMenu(menu, button, mode);
  button.setAttribute('aria-expanded', 'true');
  navOpenMenu = { menu, button };
  const first = menu.querySelector('a, button');
  if (first) first.focus();
}

function toggleMoreMenu(ev) { navToggleMenu(ev, 'moreMenu', 'moreBtn', 'up'); }
function toggleEditorMenu(ev) { navToggleMenu(ev, 'editorMenu', 'editorMoreBtn', 'down'); }
function closeMoreMenu() { navCloseMenu(); }

document.addEventListener('click', (e) => {
  if (!navOpenMenu) return;
  // Klik di dalam menu (pada item) menutup menu; klik di luar juga.
  if (navOpenMenu.menu.contains(e.target) && !e.target.closest('a, button')) return;
  navCloseMenu();
});

document.addEventListener('keydown', (e) => {
  if (!navOpenMenu) return;
  if (e.key === 'Escape') {
    const btn = navOpenMenu.button;
    navCloseMenu();
    btn.focus();
    return;
  }
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    const items = Array.from(navOpenMenu.menu.querySelectorAll('a, button')).filter(el => el.offsetParent !== null || el.style.display !== 'none');
    if (!items.length) return;
    e.preventDefault();
    const i = items.indexOf(document.activeElement);
    const next = e.key === 'ArrowDown' ? (i + 1) % items.length : (i <= 0 ? items.length - 1 : i - 1);
    items[next].focus();
  }
});

window.addEventListener('resize', navCloseMenu);

// ---------- Onboarding 3 langkah ----------

const NAV_ONBOARD_KEY = 'nb_onboarded_v1';
const NAV_ONBOARD_STEPS = [
  { title: 'Tulis catatan pertamamu', text: 'Semua dimulai dari satu catatan. Judul, isi, daftar tugas, tabel, dan gambar bisa dibuat langsung di editor.', action: { label: 'Buat catatan', run: () => createNote() } },
  { title: 'Rapikan dengan folder dan tag', text: 'Kelompokkan catatan ke dalam folder dan beri tag di panel kiri. Tab Rencana menampung tugas, kalender, dan proyek.', action: null },
  { title: 'Cari dan minta bantuan AI', text: 'Tekan Ctrl+K untuk mencari apa saja. Di dalam catatan, tombol Asisten AI membantu meringkas, memperbaiki tulisan, dan menerjemahkan.', action: null }
];

function navOnboardingSeen() {
  try { return localStorage.getItem(NAV_ONBOARD_KEY) === '1'; } catch (e) { return true; }
}

function navMarkOnboarded() {
  try { localStorage.setItem(NAV_ONBOARD_KEY, '1'); } catch (e) { /* abaikan */ }
}

function navCloseOnboarding() {
  const m = document.getElementById('onboardModal');
  if (m) m.style.display = 'none';
  navMarkOnboarded();
}

function navRenderOnboarding(step) {
  const s = NAV_ONBOARD_STEPS[step];
  const last = step === NAV_ONBOARD_STEPS.length - 1;
  document.getElementById('onboardStep').textContent = 'Langkah ' + (step + 1) + ' dari ' + NAV_ONBOARD_STEPS.length;
  document.getElementById('onboardTitle').textContent = s.title;
  document.getElementById('onboardText').textContent = s.text;

  const act = document.getElementById('onboardAction');
  act.style.display = s.action ? '' : 'none';
  if (s.action) {
    act.textContent = s.action.label;
    act.onclick = () => { navCloseOnboarding(); s.action.run(); };
  }
  const next = document.getElementById('onboardNext');
  next.textContent = last ? 'Mulai' : 'Lanjut';
  next.onclick = () => { if (last) navCloseOnboarding(); else navRenderOnboarding(step + 1); };
}

function navShowOnboarding() {
  if (navOnboardingSeen()) return;
  const m = document.getElementById('onboardModal');
  if (!m) return;
  m.style.display = 'flex';
  navRenderOnboarding(0);
  document.getElementById('onboardNext').focus();
}

document.addEventListener('DOMContentLoaded', () => {
  // showApp() didefinisikan di 2-auth.js (dimuat sesudah file ini); bungkus setelah semua skrip siap.
  if (typeof window.showApp === 'function') {
    const original = window.showApp;
    window.showApp = function () {
      const r = original.apply(this, arguments);
      try { navSync(); } catch (e) { console.error('navSync:', e); }
      setTimeout(navShowOnboarding, 700);
      return r;
    };
  }
});
