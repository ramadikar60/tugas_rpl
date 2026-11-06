// ============================================================
// 3-NOTES.JS — Notebooks, Notes, Editor, Attachments, Tags, Quick Capture
// ============================================================

// ============================================================
// 8. QUILL EDITOR
// ============================================================
function initQuill() {
  if (quill) return;

  try {
    Quill.register({ 'modules/better-table': window.QuillBetterTable }, true);
  } catch (e) {
    console.warn('QuillBetterTable tidak ter-load:', e);
  }

  quill = new Quill('#editor', {
    theme: 'snow',
    placeholder: 'Mulai menulis...',
    modules: {
      toolbar: '#toolbar',
      table: false,
      'better-table': {
        operationMenu: { items: { unmergeCells: { text: 'Unmerge' } } }
      },
      keyboard: {
        bindings: window.QuillBetterTable ? QuillBetterTable.keyboardBindings : {}
      }
    }
  });

  const insertTableBtn = document.getElementById('insertTableBtn');
  if (insertTableBtn) {
    insertTableBtn.addEventListener('click', () => {
      if (!window.QuillBetterTable) {
        showToast('Plugin tabel tidak ter-load', 'error');
        return;
      }
      const table = quill.getModule('better-table');
      if (table) table.insertTable(3, 3);
    });
  }

  const hrBtn = document.getElementById('insertHrBtn');
  if (hrBtn) {
    hrBtn.addEventListener('click', () => {
      const range = quill.getSelection(true);
      quill.insertEmbed(range.index, 'divider', true);
      quill.insertText(range.index + 1, '\n');
      quill.setSelection(range.index + 2);
    });
  }

  quill.on('text-change', (delta, oldDelta, source) => {
    scheduleSave();
    updateWordCount();
    if (source === 'user') {
      handleSlashTrigger();
      handleLinkAutocomplete();
    }
  });

  quill.root.addEventListener('paste', handlePaste, true);
  quill.root.addEventListener('drop', handleDrop, true);
  quill.root.addEventListener('keydown', handleEditorKeydown, true);

  // Setup advanced toolbar (dari 6-advanced.js)
  if (typeof setupAdvancedToolbar === 'function') {
    setTimeout(() => setupAdvancedToolbar(), 200);
  }
}

function updateWordCount() {
  if (!quill) return;
  const text = quill.getText().trim();
  const words = text ? text.split(/\s+/).filter(w => w.length > 0).length : 0;
  const chars = text.length;
  const readTime = Math.max(1, Math.ceil(words / 200));

  const wc = document.getElementById('wordCount');
  const cc = document.getElementById('charCount');
  const rt = document.getElementById('readTime');
  if (wc) wc.textContent = words + ' kata';
  if (cc) cc.textContent = chars + ' karakter';
  if (rt) rt.textContent = readTime + ' menit baca';
}

// ---------- SLASH COMMAND ----------
let slashMenuOpen = false;
let slashStartIndex = -1;

function handleSlashTrigger() {
  if (!quill) return;
  const range = quill.getSelection();
  if (!range) return;

  const textBefore = quill.getText(0, range.index);
  const lastChar = textBefore.slice(-1);

  if (lastChar === '/') {
    const beforeSlash = textBefore.slice(-2, -1);
    if (beforeSlash === '' || beforeSlash === '\n' || beforeSlash === ' ') {
      slashStartIndex = range.index - 1;
      showSlashMenu(range);
      return;
    }
  }

  if (slashMenuOpen) {
    if (textBefore.length <= slashStartIndex + 1 && !textBefore.endsWith('/')) {
      closeSlashMenu();
    }
  }
}

function showSlashMenu(range) {
  if (!quill) return;
  const bounds = quill.getBounds(range.index);
  const containerRect = document.getElementById('editor').getBoundingClientRect();

  const menu = document.getElementById('slashMenu');
  menu.style.display = 'block';

  let top = containerRect.top + bounds.top + bounds.height + 4;
  let left = containerRect.left + bounds.left;

  const menuRect = menu.getBoundingClientRect();
  if (top + menuRect.height > window.innerHeight - 20) {
    top = containerRect.top + bounds.top - menuRect.height - 4;
  }
  if (left + menuRect.width > window.innerWidth - 20) {
    left = window.innerWidth - menuRect.width - 20;
  }

  menu.style.top = Math.max(10, top) + 'px';
  menu.style.left = Math.max(10, left) + 'px';

  slashMenuOpen = true;

  const items = menu.querySelectorAll('.slash-item');
  items.forEach((it, i) => it.classList.toggle('selected', i === 0));
}

function closeSlashMenu() {
  const menu = document.getElementById('slashMenu');
  if (menu) menu.style.display = 'none';
  slashMenuOpen = false;
  slashStartIndex = -1;
}

function executeSlashAction(action) {
  if (!quill) return;

  if (slashStartIndex >= 0) {
    const range = quill.getSelection();
    if (range) {
      quill.deleteText(slashStartIndex, range.index - slashStartIndex);
      quill.setSelection(slashStartIndex);
    }
  }

  const range = quill.getSelection(true);
  if (!range) return;

  switch (action) {
    case 'h1': quill.format('header', 1); break;
    case 'h2': quill.format('header', 2); break;
    case 'h3': quill.format('header', 3); break;
    case 'bullet': quill.format('list', 'bullet'); break;
    case 'number': quill.format('list', 'ordered'); break;
    case 'check': quill.format('list', 'checked'); break;
    case 'quote': quill.format('blockquote', true); break;
    case 'code': quill.format('code-block', true); break;
    case 'table':
      if (window.QuillBetterTable) {
        quill.getModule('better-table').insertTable(3, 3);
      }
      break;
    case 'hr':
      quill.insertEmbed(range.index, 'divider', true);
      quill.insertText(range.index + 1, '\n');
      quill.setSelection(range.index + 2);
      break;
    case 'image':
      showAttachMenu();
      break;
  }

  closeSlashMenu();
}

document.addEventListener('click', e => {
  const item = e.target.closest('.slash-item');
  if (item) executeSlashAction(item.dataset.action);
});

function handleEditorKeydown(e) {
  if (slashMenuOpen) {
    const menu = document.getElementById('slashMenu');
    const items = Array.from(menu.querySelectorAll('.slash-item'));
    const currentIdx = items.findIndex(i => i.classList.contains('selected'));

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      const next = (currentIdx + 1) % items.length;
      items.forEach((it, i) => it.classList.toggle('selected', i === next));
      items[next].scrollIntoView({ block: 'nearest' });
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      const prev = (currentIdx - 1 + items.length) % items.length;
      items.forEach((it, i) => it.classList.toggle('selected', i === prev));
      items[prev].scrollIntoView({ block: 'nearest' });
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (currentIdx >= 0) executeSlashAction(items[currentIdx].dataset.action);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      closeSlashMenu();
    }
    return;
  }

  if (typeof mentionDropdown !== 'undefined' && mentionDropdown) {
    const items = Array.from(mentionDropdown.querySelectorAll('.mention-item'));
    const currentIdx = items.findIndex(i => i.classList.contains('selected'));

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      const next = (currentIdx + 1) % items.length;
      items.forEach((it, i) => it.classList.toggle('selected', i === next));
      items[next].scrollIntoView({ block: 'nearest' });
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      const prev = (currentIdx - 1 + items.length) % items.length;
      items.forEach((it, i) => it.classList.toggle('selected', i === prev));
      items[prev].scrollIntoView({ block: 'nearest' });
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      if (currentIdx >= 0) {
        e.preventDefault();
        items[currentIdx].click();
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      hideMentionDropdown();
    }
  }
}

// ---------- LINK AUTOCOMPLETE ----------
let mentionDropdown = null;

function handleLinkAutocomplete() {
  if (!quill) return;
  const range = quill.getSelection();
  if (!range) return;

  const textBefore = quill.getText(0, range.index);

  const lastOpen = textBefore.lastIndexOf('[[');
  const lastClose = textBefore.lastIndexOf(']]');
  if (lastOpen > lastClose && range.index - lastOpen <= 50) {
    const query = textBefore.substring(lastOpen + 2);
    if (!query.includes('\n')) {
      showWikiLinkDropdown(query, range.index);
      return;
    }
  }

  const lastAt = textBefore.lastIndexOf('@');
  if (lastAt >= 0 && range.index - lastAt <= 30) {
    const query = textBefore.substring(lastAt + 1);
    if (!query.includes(' ') && !query.includes('\n')) {
      showUserMentionDropdown(query, range.index);
      return;
    }
  }

  hideMentionDropdown();
}

async function showWikiLinkDropdown(query, cursorIndex) {
  const results = await api('/note-titles?q=' + encodeURIComponent(query));
  if (!results || results.length === 0) {
    hideMentionDropdown();
    return;
  }
  renderMentionDropdown(results.map(r => ({
    type: 'note', id: r.id, label: r.title, subtitle: 'Catatan'
  })), cursorIndex);
}

async function showUserMentionDropdown(query, cursorIndex) {
  if (!query || query.length < 1) {
    hideMentionDropdown();
    return;
  }
  const results = await api('/users/search?q=' + encodeURIComponent(query));
  if (!results || results.length === 0) {
    hideMentionDropdown();
    return;
  }
  renderMentionDropdown(results.map(r => ({
    type: 'user', id: r.id, label: r.username, subtitle: 'User'
  })), cursorIndex);
}

function renderMentionDropdown(items, cursorIndex) {
  hideMentionDropdown();

  const bounds = quill.getBounds(cursorIndex);
  const containerRect = document.getElementById('editor').getBoundingClientRect();

  mentionDropdown = document.createElement('div');
  mentionDropdown.className = 'mention-dropdown';
  mentionDropdown.style.top = (containerRect.top + bounds.top + bounds.height + 4) + 'px';
  mentionDropdown.style.left = (containerRect.left + bounds.left) + 'px';

  items.slice(0, 8).forEach((item, i) => {
    const el = document.createElement('div');
    el.className = 'mention-item' + (i === 0 ? ' selected' : '');
    el.innerHTML = `<strong>${escapeHtml(item.label)}</strong><span style="color:var(--text-faint);font-size:11px;">${item.subtitle}</span>`;
    el.onclick = () => insertMention(item);
    mentionDropdown.appendChild(el);
  });

  document.body.appendChild(mentionDropdown);
  mentionDropdown.dataset.cursorIndex = cursorIndex;
}

function hideMentionDropdown() {
  if (mentionDropdown) {
    mentionDropdown.remove();
    mentionDropdown = null;
  }
}

function insertMention(item) {
  if (!mentionDropdown) return;
  const range = quill.getSelection();
  if (!range) return;

  const textBefore = quill.getText(0, range.index);

  if (item.type === 'note') {
    const lastOpen = textBefore.lastIndexOf('[[');
    if (lastOpen >= 0) {
      quill.deleteText(lastOpen, range.index - lastOpen);
      quill.insertText(lastOpen, `[[${item.label}]]`, 'link', `#wiki-${item.id}`);
      quill.setSelection(lastOpen + item.label.length + 4);
    }
  } else {
    const lastAt = textBefore.lastIndexOf('@');
    if (lastAt >= 0) {
      quill.deleteText(lastAt, range.index - lastAt);
      quill.insertText(lastAt, `@${item.label} `, 'link', `#mention-${item.id}`);
      quill.setSelection(lastAt + item.label.length + 2);
    }
  }

  hideMentionDropdown();
}

// ---------- KEYBOARD SHORTCUTS ----------
document.addEventListener('keydown', e => {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault();
    if (currentNoteId) {
      saveNote().then(ok => {
        if (ok) showToast('Tersimpan.', 'success', 1200);
        else showToast('Gagal menyimpan. Periksa koneksi lalu coba lagi.', 'error', 3000);
      });
    }
  }

  if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
    e.preventDefault();
    const si = document.getElementById('searchInput');
    if (si) si.focus();
  }

  if ((e.ctrlKey || e.metaKey) && e.key === 'n') {
    e.preventDefault();
    createNote();
  }

  if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === 'H') {
    e.preventDefault();
    if (window.innerWidth <= 767) toggleSidebar();
  }

  if (e.key === 'Escape') {
    if (slashMenuOpen) { closeSlashMenu(); return; }
    if (mentionDropdown) { hideMentionDropdown(); return; }
    const previewModal = document.getElementById('previewModal');
    if (previewModal && previewModal.style.display !== 'none') {
      closePreviewModal();
      return;
    }
    if (document.getElementById('editorMode') &&
        document.getElementById('editorMode').style.display !== 'none') {
      closeEditor();
    }
  }
});

// ============================================================
// 9. NOTEBOOKS & NOTES
// ============================================================
async function loadAll() {
  await Promise.all([loadNotebooks(), loadTags()]);
  await loadNotes(currentFilter);
}

async function loadNotebooks() {
  const tree = await api('/notebooks');
  if (!tree) return;
  const ul = document.getElementById('notebookTree');
  ul.innerHTML = '';
  renderNotebooks(tree, ul);
}

function renderNotebooks(items, parentEl) {
  items.forEach(nb => {
    const li = document.createElement('li');
    li.dataset.id = nb.id;
    li.draggable = true;
    const safeName = escapeHtml(nb.name).replace(/'/g, "\\'");
    li.innerHTML = `
      <span class="item-name">${escapeHtml(nb.name)}</span>
      <span class="actions">
        <button onclick="event.stopPropagation(); renameNotebook(${nb.id}, '${safeName}')" title="Rename">Edit</button>
        <button onclick="event.stopPropagation(); deleteNotebook(${nb.id})" title="Hapus">Hapus</button>
      </span>
    `;
    li.onclick = (e) => {
      if (e.target.tagName !== 'BUTTON') {
        navigateTo('list', {
          filter: 'all', notebookId: nb.id, tagId: null, searchQuery: ''
        });
      }
    };

    li.addEventListener('dragstart', e => e.dataTransfer.setData('notebook_id', nb.id));
    li.addEventListener('dragover', e => { e.preventDefault(); li.classList.add('dragover'); });
    li.addEventListener('dragleave', () => li.classList.remove('dragover'));
    li.addEventListener('drop', async e => {
      e.preventDefault();
      li.classList.remove('dragover');
      const draggedId = e.dataTransfer.getData('notebook_id');
      if (draggedId && draggedId != nb.id) {
        await api('/notebooks/' + draggedId, {
          method: 'PUT',
          body: JSON.stringify({ parent_id: nb.id })
        });
        loadNotebooks();
      }
    });

    parentEl.appendChild(li);

    if (nb.children && nb.children.length) {
      const sub = document.createElement('ul');
      sub.className = 'tree';
      sub.style.marginLeft = '14px';
      sub.style.marginTop = '1px';
      parentEl.appendChild(sub);
      renderNotebooks(nb.children, sub);
    }
  });
}

async function createNotebook() {
  const name = prompt('Nama folder:', 'Folder Baru');
  if (!name) return;
  await api('/notebooks', {
    method: 'POST',
    body: JSON.stringify({ name, parent_id: currentNotebookId })
  });
  loadNotebooks();
}

async function renameNotebook(id, old) {
  const name = prompt('Nama baru:', old);
  if (!name) return;
  await api('/notebooks/' + id, { method: 'PUT', body: JSON.stringify({ name }) });
  loadNotebooks();
}

async function deleteNotebook(id) {
  if (!confirm('Hapus folder ini beserta isinya?')) return;
  await api('/notebooks/' + id, { method: 'DELETE' });
  if (currentNotebookId === id) currentNotebookId = null;
  loadAll();
}

// ============================================================
// 10. LOAD NOTES
// ============================================================
async function loadNotes(filter, searchQuery) {
  currentFilter = filter || currentFilter;
  if (searchQuery !== undefined) currentSearchQuery = searchQuery;

  document.querySelectorAll('.nav a[data-filter]').forEach(a => {
    a.classList.toggle('active', a.dataset.filter === currentFilter);
  });

  updateListHeader(currentFilter);

  const trashBanner = document.getElementById('trashActions');
  if (trashBanner) {
    trashBanner.style.display = currentFilter === 'trash' ? 'flex' : 'none';
  }

  let url = '/notes?filter=' + currentFilter;
  if (currentNotebookId) url += '&notebook_id=' + currentNotebookId;
  if (currentTagId) url += '&tag_id=' + currentTagId;
  if (currentSearchQuery) {
    url = '/search?q=' + encodeURIComponent(currentSearchQuery)
      + (currentNotebookId ? '&notebook_id=' + currentNotebookId : '')
      + (currentTagId ? '&tag_id=' + currentTagId : '');
  }

  const notes = await api(url);
  if (!notes) return;

  const grid = document.getElementById('notesGrid');
  grid.innerHTML = '';

  if (notes.length === 0) {
    const msg = currentSearchQuery
      ? 'Tidak ada hasil untuk pencarian ini'
      : currentFilter === 'trash'
        ? 'Sampah kosong'
        : 'Belum ada catatan di sini';
    grid.textContent = '';
    const empty = document.createElement('div');
    empty.className = 'notes-empty';
    const p = document.createElement('p');
    p.textContent = msg;
    empty.appendChild(p);
    // Ajakan bertindak hanya di daftar utama yang kosong (bukan hasil cari/sampah).
    if (!currentSearchQuery && currentFilter === 'all') {
      const hint = document.createElement('p');
      hint.className = 'notes-empty-hint';
      hint.textContent = 'Mulai dengan satu catatan. Folder, tag, dan tugas bisa menyusul.';
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'btn-primary';
      btn.textContent = 'Buat catatan pertama';
      btn.addEventListener('click', () => createNote());
      empty.appendChild(hint);
      empty.appendChild(btn);
    }
    grid.appendChild(empty);
    return;
  }

  notes.forEach(n => {
    const card = document.createElement('div');
    card.className = 'note-card';

    const isShared = n.access_permission === 'editor' || n.access_permission === 'viewer';
    const isSharedViewer = n.access_permission === 'viewer';
    const isSharedEditor = n.access_permission === 'editor';

    if (isShared) card.classList.add('shared');
    if (isSharedViewer) card.classList.add('shared-viewer');
    if (isSharedEditor) card.classList.add('shared-editor');
    if (n.is_locked) card.classList.add('locked');
    if (n.is_readonly) card.classList.add('readonly');

    const preview = stripHtml(n.content || '').substring(0, 160);
    const badges = [];
    if (n.is_pinned) badges.push('<span class="badge">Pin</span>');
    if (n.is_favorite) badges.push('<span class="badge">Fav</span>');
    if (n.is_archived) badges.push('<span class="badge">Arsip</span>');
    if (n.has_checklist && n.checklist_total > 0) {
      badges.push(`<span class="badge">${n.checklist_done}/${n.checklist_total}</span>`);
    }

    let expiredHtml = '';
    if (n.is_trashed && n.trashed_at) {
      const daysLeft = daysUntilDelete(n.trashed_at);
      expiredHtml = `<span class="expired">Hapus dalam ${daysLeft} hari</span>`;
    }

    let sharedBadgeHtml = '';
    if (isShared) {
      const icon = isSharedEditor ? '✎' : '◎';
      const label = isSharedEditor ? 'Editor' : 'Viewer';
      sharedBadgeHtml = `
        <div class="shared-badge ${isSharedEditor ? 'editor' : 'viewer'}">
          <span class="shared-badge-icon">${icon}</span>
          <span class="shared-badge-text">Dari @${escapeHtml(n.owner_username || 'user')}</span>
          <span class="shared-badge-perm">${label}</span>
        </div>
      `;
    }

    card.innerHTML = `
      ${sharedBadgeHtml}
      <div class="card-title">${escapeHtml(n.title || 'Tanpa judul')}</div>
      <div class="card-preview">${escapeHtml(preview) || 'Tidak ada isi'}</div>
      <div class="card-footer">
        <div class="badges">${badges.join('')}</div>
        <span>${formatDate(n.updated_at)}</span>
      </div>
      ${expiredHtml}
    `;
    card.onclick = () => openNote(n.id);
    card.tabIndex = 0;
    card.setAttribute('role', 'button');
    card.addEventListener('keydown', ev => {
      if ((ev.key === 'Enter' || ev.key === ' ') && ev.target === card) { ev.preventDefault(); openNote(n.id); }
    });
    grid.appendChild(card);
  });
}

function updateListHeader(filter) {
  const titles = {
    all: ['Semua Catatan', 'Klik catatan untuk membuka dan mengedit'],
    pinned: ['Tersemat', 'Catatan yang disematkan'],
    favorites: ['Favorit', 'Catatan favorit Anda'],
    archived: ['Arsip', 'Catatan yang diarsipkan'],
    trash: ['Sampah', 'Catatan yang dihapus sementara']
  };
  const [title, subtitle] = titles[filter] || titles.all;

  let finalTitle = title;
  if (currentSearchQuery) finalTitle = `Hasil: "${currentSearchQuery}"`;
  document.getElementById('listTitle').textContent = finalTitle;
  document.getElementById('listSubtitle').textContent = subtitle;
}

function showListMode() {
  showMode('listMode');
  setMobileTitle('Mindora');
}

function showEditorMode() {
  showMode('editorMode');
  setMobileTitle('Edit Catatan');

  const backBtn = document.querySelector('.editor-header .editor-back');
  if (backBtn) {
    backBtn.style.display = window.innerWidth <= 767 ? 'inline-flex' : 'none';
  }
}

function closeEditor() {
  navigateTo('list', { filter: currentFilter });
}

async function createNote() {
  showTemplatePicker();
}

// ============================================================
// 11. OPEN NOTE
// ============================================================
async function openNote(id) {
  // Simpan catatan yang sedang terbuka lebih dulu (ditahan bila gagal tersimpan).
  if (currentNoteId && !(await ensureSavedBeforeLeave())) return;

  const note = await api('/notes/' + id);
  if (!note) return;

  const isOwner = note.access_permission === 'owner' || !note.access_permission;
  const isSharedViewer = note.access_permission === 'viewer';
  const isSharedEditor = note.access_permission === 'editor';

  currentNoteId = note.id;
  document.getElementById('noteTitle').value = note.title || '';
  quill.root.innerHTML = safeNoteHTML(note.content || '');
  autosaveResetForNote();

  window._currentNoteAccess = {
    isOwner, isSharedViewer, isSharedEditor,
    ownerUsername: note.owner_username || '',
    permission: note.access_permission || 'owner'
  };

  const canEdit = isOwner || isSharedEditor;
  const isReadonly = note.is_readonly || isSharedViewer;
  const isLocked = note.is_locked && isOwner;

  if (!canEdit || isReadonly || isLocked) {
    quill.enable(false);
  } else {
    quill.enable(true);
  }

  document.getElementById('pinBtn').classList.toggle('active', note.is_pinned);
  document.getElementById('favBtn').classList.toggle('active', note.is_favorite);
  document.getElementById('archBtn').classList.toggle('active', note.is_archived);
  document.getElementById('lockBtn').classList.toggle('active', note.is_locked);
  document.getElementById('lockBtn').textContent = note.is_locked ? 'Buka Kunci' : 'Kunci';
  document.getElementById('readonlyBtn').classList.toggle('active', note.is_readonly);
  document.getElementById('readonlyBtn').textContent = note.is_readonly ? 'Mode Edit' : 'Hanya-baca';

  const isTrashed = note.is_trashed === true;

  const ownerOnlyButtons = [
    'pinBtn', 'favBtn', 'archBtn', 'lockBtn', 'readonlyBtn',
    'duplicateBtn', 'trashBtn', 'restoreBtn', 'deleteForeverBtn'
  ];

  if (!isOwner) {
    ownerOnlyButtons.forEach(btnId => {
      const btn = document.getElementById(btnId);
      if (btn) btn.style.display = 'none';
    });
  } else {
    document.getElementById('trashBtn').style.display = isTrashed ? 'none' : '';
    document.getElementById('restoreBtn').style.display = isTrashed ? '' : 'none';
    document.getElementById('deleteForeverBtn').style.display = isTrashed ? '' : 'none';
    document.getElementById('pinBtn').style.display = isTrashed ? 'none' : '';
    document.getElementById('favBtn').style.display = isTrashed ? 'none' : '';
    document.getElementById('archBtn').style.display = isTrashed ? 'none' : '';
    document.getElementById('lockBtn').style.display = isTrashed ? 'none' : '';
    document.getElementById('readonlyBtn').style.display = isTrashed ? 'none' : '';
    document.getElementById('duplicateBtn').style.display = isTrashed ? 'none' : '';
  }

  const shareBtn = document.querySelector('.editor-actions button[onclick*="openShareModal"]');
  if (shareBtn) shareBtn.style.display = isOwner ? '' : 'none';

  const tplBtn = document.querySelector('.editor-actions button[onclick*="openSaveTemplateModal"]');
  if (tplBtn) tplBtn.style.display = isOwner ? '' : 'none';

  const syncTaskBtn = document.querySelector('.editor-actions button[onclick*="syncChecklistToTasks"]');
  if (syncTaskBtn) syncTaskBtn.style.display = isOwner ? '' : 'none';

  const attachBtn = document.querySelector('.editor-actions button[onclick*="showAttachMenu"]');
  if (attachBtn) attachBtn.style.display = canEdit ? '' : 'none';

  const editorMode = document.getElementById('editorMode');
  const existingBanner = editorMode.querySelector('.readonly-banner, .locked-banner, .shared-banner');
  if (existingBanner) existingBanner.remove();

  let bannerText = '';
  let bannerClass = 'shared-banner';

  if (isSharedViewer) {
    bannerClass = 'shared-banner viewer-banner';
    bannerText = `Dibagikan oleh @${note.owner_username} · Mode Viewer (hanya bisa dilihat)`;
  } else if (isSharedEditor) {
    bannerClass = 'shared-banner editor-banner';
    bannerText = `Dibagikan oleh @${note.owner_username} · Mode Editor (perubahan Anda akan tercatat)`;
  } else if (isLocked) {
    bannerClass = 'locked-banner';
    bannerText = 'Catatan terkunci. Klik "Buka Kunci" untuk mengedit.';
  } else if (note.is_readonly) {
    bannerClass = 'readonly-banner';
    bannerText = 'Mode hanya-baca. Buka menu aksi lalu pilih "Mode Edit" untuk mulai mengedit.';
  }

  if (bannerText) {
    const banner = document.createElement('div');
    banner.className = bannerClass;
    banner.textContent = bannerText;
    document.querySelector('.editor-header').after(banner);
  }

  renderAttachments(note.attachments || []);
  setSaveStatus('idle');

  updateWordCount();
  switchEditorTab('content');
  showEditorMode();
  document.getElementById('noteTitle').focus();

  // Post-process (dari 6-advanced.js)
  setTimeout(() => {
    if (window.AdvancedEditor) {
      window.AdvancedEditor.highlightCodeBlocks();
      window.AdvancedEditor.rehydrate();
    }
  }, 300);
}

// scheduleSave(), onTitleChange(), saveNote() dan logika simpan otomatis ada di 3a-autosave.js

async function togglePin() {
  if (!currentNoteId) return;
  const note = await api('/notes/' + currentNoteId);
  await api('/notes/' + currentNoteId, {
    method: 'PUT',
    body: JSON.stringify({ is_pinned: !note.is_pinned })
  });
  document.getElementById('pinBtn').classList.toggle('active', !note.is_pinned);
}

async function toggleFavorite() {
  if (!currentNoteId) return;
  const note = await api('/notes/' + currentNoteId);
  await api('/notes/' + currentNoteId, {
    method: 'PUT',
    body: JSON.stringify({ is_favorite: !note.is_favorite })
  });
  document.getElementById('favBtn').classList.toggle('active', !note.is_favorite);
}

async function toggleArchive() {
  if (!currentNoteId) return;
  const note = await api('/notes/' + currentNoteId);
  await api('/notes/' + currentNoteId, {
    method: 'PUT',
    body: JSON.stringify({ is_archived: !note.is_archived })
  });
  document.getElementById('archBtn').classList.toggle('active', !note.is_archived);
}

async function toggleLock() {
  if (!currentNoteId) return;
  const note = await api('/notes/' + currentNoteId);
  if (!note) return;

  if (note.is_locked) {
    const pw = prompt('Note ini terkunci.\n\nMasukkan password (kosongkan jika tanpa password):');
    if (pw === null) return;
    const res = await api('/notes/' + currentNoteId + '/unlock', {
      method: 'POST',
      body: JSON.stringify({ password: pw || '' })
    });
    if (res && res.message) {
      showToast('Note dibuka.', 'success');
      openNote(currentNoteId);
    } else {
      showToast('Gagal membuka note.', 'error');
    }
  } else {
    const pw = prompt('Password untuk mengunci (opsional):');
    const res = await api('/notes/' + currentNoteId + '/lock', {
      method: 'POST',
      body: JSON.stringify({ password: pw || '' })
    });
    if (res && res.message) {
      showToast('Note terkunci.', 'success');
      openNote(currentNoteId);
    }
  }
}

async function toggleReadOnly() {
  if (!currentNoteId) return;
  const note = await api('/notes/' + currentNoteId);
  if (!note) return;
  const newState = !note.is_readonly;
  await api('/notes/' + currentNoteId + '/readonly', {
    method: 'PUT',
    body: JSON.stringify({ is_readonly: newState })
  });
  showToast(newState ? 'Mode hanya-baca aktif.' : 'Mode hanya-baca dimatikan.', 'success');
  openNote(currentNoteId);
}

async function duplicateNote() {
  if (!currentNoteId) return;
  const data = await api('/notes/' + currentNoteId + '/duplicate', { method: 'POST' });
  if (data && data.id) {
    showToast('Catatan diduplikat.', 'success');
    openNote(data.id);
  }
}

async function trashNote() {
  if (!currentNoteId) return;
  if (!confirm('Pindahkan ke sampah?\n\nCatatan akan dihapus otomatis setelah 30 hari.')) return;

  await api('/notes/' + currentNoteId + '/trash', { method: 'POST' });
  showToast('Catatan dipindahkan ke sampah.', 'success');

  navigateTo('list', { filter: currentFilter });
}

async function restoreNote() {
  if (!currentNoteId) return;
  await api('/notes/' + currentNoteId + '/restore', { method: 'POST' });
  showToast('Catatan berhasil dipulihkan.', 'success');

  navigateTo('list', { filter: currentFilter });
}

async function deleteForever() {
  if (!currentNoteId) return;
  if (!confirm('Hapus PERMANEN catatan ini?\n\nTindakan ini tidak bisa dibatalkan.')) return;

  await api('/notes/' + currentNoteId, { method: 'DELETE' });
  showToast('Catatan dihapus permanen.', 'success');

  navigateTo('list', { filter: currentFilter });
}

async function emptyTrash() {
  if (!confirm('Kosongkan SEMUA sampah?\n\nSemua catatan di sampah akan dihapus permanen.')) return;

  const res = await api('/trash', { method: 'DELETE' });
  if (res && res.message) showToast('Sampah berhasil dikosongkan.', 'success');
  loadNotes(currentFilter, currentSearchQuery);
}

// ============================================================
// 12. ATTACHMENTS
// ============================================================
function showAttachMenu() {
  if (!currentNoteId) { showToast('Buka catatan dulu', 'error'); return; }
  document.getElementById('fileInput').click();
}

document.addEventListener('DOMContentLoaded', () => {
  const fileInput = document.getElementById('fileInput');
  if (!fileInput) return;

  fileInput.addEventListener('change', async (e) => {
    const files = e.target.files;
    if (!files.length) return;

    for (const file of files) {
      try {
        await uploadFileWithProgress(file);
      } catch (err) {
        showToast('Gagal upload ' + file.name, 'error');
      }
    }
    e.target.value = '';
    const note = await api('/notes/' + currentNoteId);
    renderAttachments(note.attachments || []);
  });
});

async function uploadFileWithProgress(file) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const fd = new FormData();
    fd.append('file', file);

    const container = document.getElementById('attachmentsPreview');
    const progressDiv = document.createElement('div');
    progressDiv.className = 'upload-progress';
    progressDiv.innerHTML = `
      <span class="attach-name" style="flex:1;min-width:0;font-size:12px;">${escapeHtml(file.name)}</span>
      <div class="upload-progress-bar">
        <div class="upload-progress-fill" style="width:0%"></div>
      </div>
      <span class="upload-progress-text">0%</span>
    `;
    container.appendChild(progressDiv);

    const fill = progressDiv.querySelector('.upload-progress-fill');
    const text = progressDiv.querySelector('.upload-progress-text');

    xhr.upload.addEventListener('progress', (e) => {
      if (e.lengthComputable) {
        const pct = Math.round((e.loaded / e.total) * 100);
        fill.style.width = pct + '%';
        text.textContent = pct + '%';
      }
    });

    xhr.addEventListener('load', () => {
      progressDiv.remove();
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(JSON.parse(xhr.responseText));
      } else {
        reject(new Error('Upload gagal: ' + xhr.status));
      }
    });

    xhr.addEventListener('error', () => {
      progressDiv.remove();
      reject(new Error('Upload error'));
    });

    xhr.open('POST', '/api/notes/' + currentNoteId + '/attachments');
    xhr.setRequestHeader('Authorization', 'Bearer ' + token);
    if (csrfToken) xhr.setRequestHeader('X-CSRF-Token', csrfToken);
    xhr.withCredentials = true;
    xhr.send(fd);
  });
}

function renderAttachments(list) {
  const el = document.getElementById('attachmentsPreview');
  if (!el) return;
  el.innerHTML = '';

  if (list.length === 0) {
    el.style.display = 'none';
    return;
  }
  el.style.display = 'flex';

  const access = window._currentNoteAccess || {};
  const canEdit = access.isOwner || access.isSharedEditor;

  list.forEach(a => {
    const div = document.createElement('div');
    div.className = 'attach-item';
    div.dataset.id = a.id;
    div.dataset.filename = a.filename;
    div.dataset.mime = a.mime_type;
    div.dataset.url = a.url;
    div.dataset.size = a.size;

    let previewHtml = '';
    if (a.mime_type && a.mime_type.startsWith('image/')) {
      previewHtml = `<img src="${a.url}" alt="" loading="lazy">`;
    } else if (a.mime_type && a.mime_type.startsWith('video/')) {
      previewHtml = `<span class="file-icon">VID</span>`;
    } else if (a.mime_type && a.mime_type.startsWith('audio/')) {
      previewHtml = `<span class="file-icon">AUD</span>`;
    } else if (a.mime_type && a.mime_type.includes('pdf')) {
      previewHtml = `<span class="file-icon">PDF</span>`;
    } else if (a.mime_type && a.mime_type.includes('word')) {
      previewHtml = `<span class="file-icon">DOC</span>`;
    } else if (a.mime_type && (a.mime_type.includes('excel') || a.mime_type.includes('spreadsheet'))) {
      previewHtml = `<span class="file-icon">XLS</span>`;
    } else {
      previewHtml = `<span class="file-icon">FILE</span>`;
    }

    const actionsHtml = canEdit
      ? `
        <button onclick="event.stopPropagation(); downloadAttachment(${a.id})" title="Download">↓</button>
        <button onclick="event.stopPropagation(); renameAttachment(${a.id}, '${escapeHtml(a.filename).replace(/'/g, "\\'")}')" title="Rename">✎</button>
        <button class="danger" onclick="event.stopPropagation(); deleteAttachment(${a.id})" title="Hapus">✕</button>
      `
      : `<button onclick="event.stopPropagation(); downloadAttachment(${a.id})" title="Download">↓</button>`;

    div.innerHTML = `
      <div class="attach-preview" onclick="openPreview(${a.id})">${previewHtml}</div>
      <div class="attach-info" onclick="openPreview(${a.id})">
        <span class="attach-name">${escapeHtml(a.filename)}</span>
        <span class="attach-meta">${formatBytes(a.size || 0)}</span>
      </div>
      <div class="attach-actions">${actionsHtml}</div>
    `;
    el.appendChild(div);
  });
}

async function downloadAttachment(id) {
  const el = document.querySelector(`.attach-item[data-id="${id}"]`);
  if (!el) return;
  try {
    const res = await fetch(`/api/attachments/${id}/download`, {
      headers: { 'Authorization': 'Bearer ' + token }
    });
    if (!res.ok) throw new Error('Download gagal');
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = el.dataset.filename || 'file';
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  } catch (e) {
    showToast('Gagal download: ' + e.message, 'error');
  }
}

async function renameAttachment(id, currentName) {
  const newName = prompt('Nama file baru:', currentName);
  if (!newName || newName === currentName) return;

  const res = await api(`/attachments/${id}/rename`, {
    method: 'PUT',
    body: JSON.stringify({ filename: newName })
  });
  if (res && res.message) {
    showToast('Nama file diubah.', 'success');
    const note = await api('/notes/' + currentNoteId);
    renderAttachments(note.attachments || []);
  }
}

async function deleteAttachment(id) {
  if (!confirm('Hapus lampiran ini?')) return;
  await api('/attachments/' + id, { method: 'DELETE' });
  const note = await api('/notes/' + currentNoteId);
  renderAttachments(note.attachments || []);
  showToast('Lampiran dihapus.', 'success');
}

function openPreview(id) {
  const el = document.querySelector(`.attach-item[data-id="${id}"]`);
  if (!el) return;

  let currentPreviewId = id;
  window._currentPreviewId = id;

  const filename = el.dataset.filename;
  const mime = el.dataset.mime;
  const size = parseInt(el.dataset.size) || 0;

  document.getElementById('previewFilename').textContent = filename;
  document.getElementById('previewMeta').textContent = `${mime} · ${formatBytes(size)}`;

  const body = document.getElementById('previewBody');
  body.innerHTML = '';

  const url = el.dataset.url;

  if (mime && mime.startsWith('image/')) {
    const img = document.createElement('img');
    img.src = url;
    img.alt = filename;
    img.onclick = () => window.open(url, '_blank');
    body.appendChild(img);
  } else if (mime && mime.startsWith('video/')) {
    const video = document.createElement('video');
    video.src = url;
    video.controls = true;
    body.appendChild(video);
  } else if (mime && mime.startsWith('audio/')) {
    const audio = document.createElement('audio');
    audio.src = url;
    audio.controls = true;
    body.appendChild(audio);
  } else if (mime && mime.includes('pdf')) {
    const iframe = document.createElement('iframe');
    iframe.src = url + '#toolbar=1&navpanes=0';
    body.appendChild(iframe);
  } else if (mime && (mime.includes('text/') || mime.includes('json'))) {
    fetch(url, { headers: { 'Authorization': 'Bearer ' + token } })
      .then(r => r.text())
      .then(text => {
        const pre = document.createElement('pre');
        pre.style.cssText = 'color:#eee;font-family:monospace;font-size:13px;padding:20px;white-space:pre-wrap;word-break:break-all;max-width:900px;margin:auto;text-align:left;';
        pre.textContent = text;
        body.appendChild(pre);
      });
  } else {
    body.innerHTML = `
      <div class="unsupported">
        <p style="margin-bottom:16px;">Tipe file ini tidak bisa dipreview langsung.</p>
        <button class="btn-primary" onclick="downloadAttachment(${id})">Download File</button>
      </div>
    `;
  }

  document.getElementById('previewModal').style.display = 'flex';
  document.body.style.overflow = 'hidden';
}

function closePreviewModal() {
  document.getElementById('previewModal').style.display = 'none';
  document.getElementById('previewBody').innerHTML = '';
  document.body.style.overflow = '';
  window._currentPreviewId = null;
}

function downloadCurrentAttachment() {
  if (window._currentPreviewId) downloadAttachment(window._currentPreviewId);
}

function renameCurrentAttachment() {
  if (!window._currentPreviewId) return;
  const el = document.querySelector(`.attach-item[data-id="${window._currentPreviewId}"]`);
  if (el) renameAttachment(window._currentPreviewId, el.dataset.filename);
}

async function deleteCurrentAttachment() {
  if (!window._currentPreviewId) return;
  if (!confirm('Hapus lampiran ini?')) return;
  await api('/attachments/' + window._currentPreviewId, { method: 'DELETE' });
  closePreviewModal();
  const note = await api('/notes/' + currentNoteId);
  renderAttachments(note.attachments || []);
}

async function handlePaste(e) {
  const items = e.clipboardData && e.clipboardData.items;
  if (!items) return;
  for (const item of items) {
    if (item.type.indexOf('image') === 0) {
      e.preventDefault();
      const blob = item.getAsFile();
      const reader = new FileReader();
      reader.onload = async (ev) => {
        const res = await api('/notes/' + currentNoteId + '/attachments/clipboard', {
          method: 'POST',
          body: JSON.stringify({ data_url: ev.target.result, filename: 'pasted.png' })
        });
        if (res && res.url) {
          const range = quill.getSelection(true);
          quill.insertEmbed(range.index, 'image', res.url);
        }
      };
      reader.readAsDataURL(blob);
    }
  }
}

async function handleDrop(e) {
  if (!e.dataTransfer || !e.dataTransfer.files.length) return;
  e.preventDefault();
  for (const file of e.dataTransfer.files) {
    if (file.type.startsWith('image/')) {
      await uploadFileWithProgress(file);
      const note = await api('/notes/' + currentNoteId);
      renderAttachments(note.attachments || []);
    }
  }
}

// ============================================================
// 14. TAGS
// ============================================================
async function loadTags() {
  const tags = await api('/tags');
  if (!tags) return;
  const ul = document.getElementById('tagList');
  ul.innerHTML = '';
  if (tags.length === 0) {
    ul.innerHTML = '<li style="background:var(--bg-muted);color:var(--text-faint);cursor:default;">Belum ada tag</li>';
    return;
  }
  tags.forEach(t => {
    const li = document.createElement('li');
    li.style.background = t.color + '22';
    li.style.setProperty('--tag-color', t.color); // warna tag hanya sebagai titik; teks dipaksa kontras oleh tema
    li.style.borderColor = t.color + '44';
    li.innerHTML = `${escapeHtml(t.name)} <span onclick="deleteTag(event, ${t.id})" style="cursor:pointer;opacity:0.7;">×</span>`;
    li.onclick = (e) => {
      if (e.target.tagName === 'SPAN') return;
      navigateTo('list', { filter: 'all', notebookId: null, tagId: t.id, searchQuery: '' });
    };
    ul.appendChild(li);
  });
}

async function createTag() {
  const name = prompt('Nama tag:');
  if (!name) return;
  const color = prompt('Warna (hex, contoh: #6366f1):', '#6366f1') || '#6366f1';
  await api('/tags', { method: 'POST', body: JSON.stringify({ name, color }) });
  loadTags();
}

async function deleteTag(e, id) {
  e.stopPropagation();
  if (!confirm('Hapus tag?')) return;
  await api('/tags/' + id, { method: 'DELETE' });
  if (currentTagId === id) currentTagId = null;
  loadTags();
  loadNotes(currentFilter);
}

console.log('3-notes.js loaded');