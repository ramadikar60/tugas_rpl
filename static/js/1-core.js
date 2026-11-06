// ============================================================
// 1-CORE.JS — Global State, Navigation, Theme, Utilities
// ============================================================

// ============================================================
// 1. GLOBAL STATE
// ============================================================
let token = null;
let csrfToken = null;
let currentNoteId = null;
let currentFilter = 'all';
let currentNotebookId = null;
let currentTagId = null;
let saveTimer = null;
let quill = null;
let refreshInFlight = null;
let currentSearchQuery = '';
let isNavigating = false;

// ============================================================
// 2. NAVIGATION HELPER (TERPUSAT)
// ============================================================
const ALL_MODES = [
  'listMode', 'editorMode', 'tasksMode',
  'calendarMode', 'timelineMode',
  'projectsMode', 'projectDetailMode',
  'boardsMode', 'boardDetailMode',
  'mindmapsMode', 'mindmapEditorMode',
  'whiteboardsMode', 'whiteboardEditorMode',
  'clipperMode', 'researchMode', 'mediaMode',
  'dashboardMode', 'productivityMode', 'datamanageMode'
];

function hideAllModes(except = null) {
  ALL_MODES.forEach(id => {
    if (id === except) return;
    const el = document.getElementById(id);
    if (el) el.style.display = 'none';
  });
}

function showMode(modeId) {
  hideAllModes(modeId);
  const el = document.getElementById(modeId);
  if (el) el.style.display = 'flex';
}

async function navigateTo(target, options = {}) {
  if (isNavigating) return;
  isNavigating = true;

  const sidebar = document.getElementById('sidebar');
  if (sidebar) sidebar.classList.add('navigating');
  document.body.style.cursor = 'progress';

  try {
    // Auto-save note sebelum pindah
    const inEditor = document.getElementById('editorMode') &&
      document.getElementById('editorMode').style.display !== 'none';

    if (inEditor && currentNoteId && quill) {
      // Simpan dulu; bila gagal, tetap di catatan (klik sekali lagi untuk tetap pindah).
      if (!(await ensureSavedBeforeLeave())) return;
    }

    // Sembunyikan semua mode dulu
    hideAllModes();

    // Update state
    if (options.filter !== undefined) currentFilter = options.filter;
    if (options.notebookId !== undefined) currentNotebookId = options.notebookId;
    if (options.tagId !== undefined) currentTagId = options.tagId;
    if (options.searchQuery !== undefined) currentSearchQuery = options.searchQuery;

    const editorMode = document.getElementById('editorMode');
    if (editorMode) editorMode.classList.remove('open');

    // Dispatch ke handler masing-masing mode
    switch (target) {
      case 'list':
        showMode('listMode');
        setMobileTitle('Mindora');
        await loadNotes(currentFilter, currentSearchQuery);
        break;

      case 'tasks':
        showMode('tasksMode');
        setMobileTitle('Tugas');
        await loadTaskStats();
        await loadTasks();
        await populateTaskNotebookDropdown();
        setupTasksBackBtn();
        break;

      case 'calendar':
        showMode('calendarMode');
        setMobileTitle('Kalender');
        await loadCalendarStats();
        await loadCalendarMonth();
        setupEditorBackBtn('.calendar-header .editor-back');
        break;

      case 'timeline':
        showMode('timelineMode');
        setMobileTitle('Timeline');
        await loadHeatmap();
        await loadTimeline();
        setupEditorBackBtn('.timeline-header .editor-back');
        break;

      case 'projects':
        showMode('projectsMode');
        setMobileTitle('Proyek');
        await loadProjects();
        setupEditorBackBtn('.projects-header .editor-back');
        break;

      case 'boards':
        showMode('boardsMode');
        setMobileTitle('Kanban');
        await loadBoards();
        await populateBoardProjectDropdown();
        setupEditorBackBtn('.boards-header .editor-back');
        break;

      case 'mindmaps':
        showMode('mindmapsMode');
        setMobileTitle('Peta Pikiran');
        await loadMindmaps();
        setupEditorBackBtn('.mindmaps-header .editor-back');
        break;

      case 'whiteboards':
        showMode('whiteboardsMode');
        setMobileTitle('Papan Tulis');
        await loadWhiteboards();
        setupEditorBackBtn('.whiteboards-header .editor-back');
        break;

      case 'clipper':
        showMode('clipperMode');
        setMobileTitle('Web Clipper');
        await loadClipperTab(currentClipperTab);
        setupEditorBackBtn('.clipper-header .editor-back');
        break;

      case 'research':
        showMode('researchMode');
        setMobileTitle('Research');
        await loadSources();
        setupEditorBackBtn('.research-header .editor-back');
        break;

      case 'media':
        showMode('mediaMode');
        setMobileTitle('Media');
        await loadMediaStats();
        await loadMediaList();
        setupEditorBackBtn('.media-header .editor-back');
        break;

      case 'dashboard':
        showMode('dashboardMode');
        setMobileTitle('Beranda');
        await loadDashboard();
        setupEditorBackBtn('.dashboard-header .editor-back');
        break;

      case 'productivity':
        showMode('productivityMode');
        setMobileTitle('Productivity');
        await switchProductivityTab(currentProductivityTab);
        setupEditorBackBtn('.productivity-header .editor-back');
        break;

      case 'datamanage':
        showMode('datamanageMode');
        setMobileTitle('Data Management');
        switchDmTab(currentDmTab);
        setupEditorBackBtn('.datamanage-header .editor-back');
        break;

      case 'graph':
        showMode('listMode');
        setMobileTitle('Graph View');
        await loadNotes(currentFilter, currentSearchQuery);
        await openGraphView();
        break;

      case 'activity':
        showMode('listMode');
        setMobileTitle('Aktivitas');
        await loadNotes(currentFilter, currentSearchQuery);
        await openActivityLog();
        break;

      case 'shared':
        showMode('listMode');
        setMobileTitle('Dibagikan ke Saya');
        await loadNotes(currentFilter, currentSearchQuery);
        await openSharedWithMe();
        break;

      case 'admin':
        window.location.href = '/admin';
        break;

      case 'notifications':
        openNotificationPanel();
        break;

      case 'profile':
        await openProfileModal();
        break;

      case 'note':
        if (options.noteId) await openNote(options.noteId);
        break;

      default:
        showMode('listMode');
        setMobileTitle('Mindora');
        await loadNotes('all');
    }

    if (window.innerWidth <= 767) closeSidebar();

    if (typeof options.callback === 'function') options.callback();
  } finally {
    isNavigating = false;
    if (sidebar) sidebar.classList.remove('navigating');
    document.body.style.cursor = '';
  }
}

// ============================================================
// Helper functions untuk navigateTo
// ============================================================
function setMobileTitle(text) {
  const el = document.getElementById('mobileBarTitle');
  if (el) el.textContent = text;
}

function setupEditorBackBtn(selector) {
  const btn = document.querySelector(selector);
  if (!btn) return;
  btn.style.display = window.innerWidth <= 767 ? 'inline-flex' : 'none';
}

function setupTasksBackBtn() {
  const btn = document.querySelector('.tasks-header .editor-back');
  if (!btn) return;
  btn.style.display = window.innerWidth <= 767 ? 'inline-flex' : 'none';
}

// ============================================================
// 3. INITIALIZATION
// ============================================================
document.addEventListener('DOMContentLoaded', () => {
  const savedTheme = localStorage.getItem('theme') || 'light';
  document.documentElement.setAttribute('data-theme', savedTheme);
  updateThemeButtons(savedTheme);

  // Setup Enter handler untuk login & register
  if (typeof setupAuthEnterHandlers === 'function') {
    setupAuthEnterHandlers();
  }

  // Tautan reset password (#reset=...): tampilkan form reset, jangan langsung masuk ke aplikasi.
  if (typeof initResetLink === 'function') initResetLink();

  tryRefresh().then(ok => {
    if (ok && !window.__pendingResetToken) {
      showApp();
      if (typeof initQuill === 'function') initQuill();
      loadAll();
      if (typeof startNotifPolling === 'function') startNotifPolling();
      setTimeout(() => {
        if (typeof loadPreferences === 'function') loadPreferences();
        if (typeof checkAdminAccess === 'function') checkAdminAccess();
      }, 800);
    }
  });

  // Register DividerBlot
  try {
    const BlockEmbed = Quill.import('blots/block/embed');
    class DividerBlot extends BlockEmbed {}
    DividerBlot.blotName = 'divider';
    DividerBlot.tagName = 'hr';
    Quill.register(DividerBlot);
  } catch (e) {
    console.warn('DividerBlot register failed', e);
  }

  // Template color picker
  const picker = document.getElementById('tplColorPicker');
  if (picker) {
    picker.addEventListener('click', e => {
      const dot = e.target.closest('.accent-dot');
      if (!dot) return;
      picker.querySelectorAll('.accent-dot').forEach(d => d.classList.remove('active'));
      dot.classList.add('active');
    });
  }

  // Search input focus
  const si = document.getElementById('searchInput');
  if (si) {
    si.addEventListener('focus', async () => {
      if (typeof loadSearchHistory === 'function') await loadSearchHistory();
      if (typeof searchHistoryCache !== 'undefined' &&
          searchHistoryCache.length > 0 && si.value === '') {
        if (typeof showSearchHistoryDropdown === 'function') showSearchHistoryDropdown();
      }
    });
  }

  if (localStorage.getItem('notif_banner_dismissed') !== 'true') {
    setTimeout(() => {
      if (typeof requestNotifPermissionIfNeeded === 'function') requestNotifPermissionIfNeeded();
    }, 5000);
  }
});

// ============================================================
// 4. THEME
// ============================================================
function toggleTheme() {
  const cur = document.documentElement.getAttribute('data-theme');
  const next = cur === 'dark' ? 'light' : 'dark';
  document.documentElement.setAttribute('data-theme', next);
  localStorage.setItem('theme', next);
  updateThemeButtons(next);

  // Hook dari 6-advanced.js
  if (typeof onThemeChanged === 'function') onThemeChanged(next);
}

// Ikon SVG statis (bukan emoji); aman lewat innerHTML karena konstanta.
const THEME_ICON_MOON = '<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/></svg>';
const THEME_ICON_SUN = '<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>';

function updateThemeButtons(theme) {
  const icon = theme === 'dark' ? THEME_ICON_SUN : THEME_ICON_MOON;
  ['authThemeBtn', 'sideThemeBtn', 'mobileThemeBtn'].forEach(id => {
    const el = document.getElementById(id);
    if (!el) return;
    const iconEl = el.querySelector('.theme-icon');
    (iconEl || el).innerHTML = icon;
  });

  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute('content', theme === 'dark' ? '#0F1A14' : '#FBF4E0');
}

// ============================================================
// 5. TOAST & UTILITIES
// ============================================================
function showToast(message, type = 'info', duration = 3000) {
  const el = document.getElementById('toast');
  if (!el) return;
  el.textContent = message;
  el.className = 'toast show ' + type;
  clearTimeout(el._timeout);
  el._timeout = setTimeout(() => {
    el.classList.remove('show');
  }, duration);
}

function escapeHtml(s) {
  if (!s) return '';
  return String(s).replace(/[&<>"']/g, c => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[c]));
}

function stripHtml(s) {
  // Inert: <template> tidak menjalankan skrip/handler (div.innerHTML akan menjalankan <img onerror>).
  // Spasi ditambahkan antar blok supaya kata dari paragraf berbeda tidak menyatu.
  const spaced = String(s || '').replace(/<\/(p|div|h[1-6]|li|tr|td|th|blockquote|pre)>|<br\s*\/?>/gi, ' $&');
  const tpl = document.createElement('template');
  tpl.innerHTML = spaced;
  return (tpl.content.textContent || '').replace(/\s+/g, ' ').trim();
}

function formatDate(s) {
  if (!s) return '';
  const d = new Date(s);
  const now = new Date();
  const diff = (now - d) / 1000;
  if (diff < 60) return 'Baru saja';
  if (diff < 3600) return Math.floor(diff / 60) + ' menit lalu';
  if (diff < 86400) return Math.floor(diff / 3600) + ' jam lalu';
  if (diff < 604800) return Math.floor(diff / 86400) + ' hari lalu';
  return d.toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' });
}

function formatDateTime(isoString) {
  if (!isoString) return '';
  const d = new Date(isoString);
  const now = new Date();
  const diff = d - now;
  const days = Math.ceil(diff / (24 * 60 * 60 * 1000));

  const dateStr = d.toLocaleDateString('id-ID', { day: 'numeric', month: 'short' });
  const timeStr = d.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' });

  if (days === 0 && d.toDateString() === now.toDateString()) return 'Hari ini, ' + timeStr;
  if (days === 1) return 'Besok, ' + timeStr;
  if (days === -1) return 'Kemarin, ' + timeStr;
  return dateStr + ', ' + timeStr;
}

function toDatetimeLocal(isoString) {
  if (!isoString) return '';
  const d = new Date(isoString);
  const pad = n => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

function daysUntilDelete(trashedAt) {
  const RETENTION_DAYS = 30;
  const trashed = new Date(trashedAt);
  const deleteAt = new Date(trashed.getTime() + RETENTION_DAYS * 24 * 60 * 60 * 1000);
  const now = new Date();
  const diffMs = deleteAt - now;
  const days = Math.ceil(diffMs / (24 * 60 * 60 * 1000));
  return days > 0 ? days : 0;
}

// ============================================================
// 7. API HELPER
// ============================================================
async function api(path, opts = {}) {
  opts.headers = opts.headers || {};
  if (token) opts.headers['Authorization'] = 'Bearer ' + token;

  const method = (opts.method || 'GET').toUpperCase();
  if (csrfToken && ['POST', 'PUT', 'DELETE', 'PATCH'].includes(method)) {
    opts.headers['X-CSRF-Token'] = csrfToken;
  }
  if (opts.body && !(opts.body instanceof FormData)) {
    opts.headers['Content-Type'] = 'application/json';
  }
  opts.credentials = 'same-origin';

  let res = await fetch('/api' + path, opts);

  if (res.status === 401) {
    const ok = await tryRefresh();
    if (ok) {
      opts.headers['Authorization'] = 'Bearer ' + token;
      if (csrfToken && ['POST', 'PUT', 'DELETE', 'PATCH'].includes(method)) {
        opts.headers['X-CSRF-Token'] = csrfToken;
      }
      res = await fetch('/api' + path, opts);
    } else {
      if (typeof logout === 'function') logout();
      return null;
    }
  }

  if (res.status === 403) {
    const refreshed = await fetch('/api/csrf', {
      headers: { 'Authorization': 'Bearer ' + token },
      credentials: 'same-origin'
    });
    if (refreshed.ok) {
      const d = await refreshed.json();
      csrfToken = d.csrf_token;
      localStorage.setItem('csrf', csrfToken);
      opts.headers['X-CSRF-Token'] = csrfToken;
      res = await fetch('/api' + path, opts);
    }
  }

  if (res.status === 204) return null;
  try { return await res.json(); } catch { return null; }
}

// ============================================================
// 28. MOBILE
// ============================================================
function toggleSidebar() {
  document.getElementById('sidebar').classList.toggle('open');
  document.getElementById('sidebarOverlay').classList.toggle('open');
}

function closeSidebar() {
  document.getElementById('sidebar').classList.remove('open');
  document.getElementById('sidebarOverlay').classList.remove('open');
}

// Resize handler untuk editor back button
window.addEventListener('resize', () => {
  const editorMode = document.getElementById('editorMode');
  if (editorMode && editorMode.style.display !== 'none') {
    const backBtn = document.querySelector('.editor-header .editor-back');
    if (backBtn) {
      backBtn.style.display = window.innerWidth <= 767 ? 'inline-flex' : 'none';
    }
  }
});

console.log('1-core.js loaded');