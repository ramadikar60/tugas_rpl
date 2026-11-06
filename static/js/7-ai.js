// ============================================================
// 7-AI.JS — Asisten AI di editor catatan
//
// Alur: pilih aksi di panel → POST /api/ai/generate → pratinjau hasil
//       → pengguna memilih: ganti / sisipkan / salin / terapkan.
// Teks TIDAK pernah diubah otomatis tanpa persetujuan pengguna.
// API key hanya ada di server (.env), tidak pernah sampai ke browser.
// ============================================================

// ---------- 1. DEFINISI AKSI ----------
// kind: transform = ganti/sisipkan teks, generate = sisipkan di bawah,
//       continue = sisipkan di kursor, title / tags / tasks = hasil khusus
const AI_ACTIONS = {
  improve:   { label: 'Perbaiki tulisan', kind: 'transform' },
  fix:       { label: 'Perbaiki ejaan',   kind: 'transform' },
  shorter:   { label: 'Persingkat',       kind: 'transform' },
  longer:    { label: 'Perpanjang',       kind: 'transform' },
  formal:    { label: 'Lebih formal',     kind: 'transform' },
  casual:    { label: 'Lebih santai',     kind: 'transform' },
  translate: { label: 'Terjemahkan',      kind: 'transform' },
  custom:    { label: 'Instruksi bebas',  kind: 'transform' },
  summarize: { label: 'Ringkas',          kind: 'generate' },
  continue:  { label: 'Lanjutkan menulis', kind: 'continue' },
  title:     { label: 'Buat judul',       kind: 'title' },
  tags:      { label: 'Saran tag',        kind: 'tags' },
  tasks:     { label: 'Ekstrak tugas',    kind: 'tasks' }
};

const AI_LANGUAGES = [
  'Inggris', 'Indonesia', 'Jawa', 'Sunda', 'Arab', 'Mandarin',
  'Jepang', 'Korea', 'Prancis', 'Jerman', 'Spanyol'
];

const AI_TAG_COLORS = ['#6366f1', '#0ea5e9', '#16a34a', '#f59e0b', '#ec4899', '#8b5cf6'];

// ---------- 2. STATE ----------
const aiState = {
  open: false,
  status: null,        // respons /ai/status
  action: null,
  scope: 'note',       // 'selection' | 'note'
  range: null,         // {index, length} milik teks terpilih / posisi kursor
  text: '',            // teks yang dikirim ke AI
  editable: false,
  busy: false,
  result: null,        // respons /ai/generate
  controller: null,
  runId: 0
};

// ---------- 3. UTIL ----------
function aiWordCount(s) {
  const t = (s || '').trim();
  return t ? t.split(/\s+/).length : 0;
}

function aiEl(id) {
  return document.getElementById(id);
}

// ---------- 4. PARSER MARKDOWN SEDERHANA → BLOK ----------
// Dipakai untuk pratinjau (HTML) sekaligus untuk menyisipkan ke Quill (Delta).
function aiParseInline(text) {
  const runs = [];
  const re = /\*\*([^*\n]+)\*\*|`([^`\n]+)`|\*([^*\s][^*\n]*?)\*/g;
  let last = 0, m;
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) runs.push({ text: text.slice(last, m.index) });
    if (m[1] !== undefined) runs.push({ text: m[1], bold: true });
    else if (m[2] !== undefined) runs.push({ text: m[2], code: true });
    else runs.push({ text: m[3], italic: true });
    last = re.lastIndex;
  }
  if (last < text.length) runs.push({ text: text.slice(last) });
  return runs.length ? runs : [{ text: '' }];
}

function aiParseBlocks(markdown) {
  const blocks = [];
  const lines = String(markdown || '').replace(/\r/g, '').split('\n');

  for (const raw of lines) {
    if (/^\s*```/.test(raw)) continue;        // abaikan pagar kode
    if (!raw.trim()) continue;                 // baris kosong = pemisah paragraf

    let m;
    if ((m = raw.match(/^\s{0,3}(#{1,6})\s+(.*)$/))) {
      blocks.push({ type: 'header', level: Math.min(m[1].length, 3), runs: aiParseInline(m[2]) });
    } else if ((m = raw.match(/^(\s*)[-*•]\s+\[( |x|X)\]\s+(.*)$/))) {
      blocks.push({ type: 'check', checked: m[2] !== ' ', indent: Math.min(2, Math.floor(m[1].length / 2)), runs: aiParseInline(m[3]) });
    } else if ((m = raw.match(/^(\s*)[-*•]\s+(.*)$/))) {
      blocks.push({ type: 'bullet', indent: Math.min(2, Math.floor(m[1].length / 2)), runs: aiParseInline(m[2]) });
    } else if ((m = raw.match(/^(\s*)\d+[.)]\s+(.*)$/))) {
      blocks.push({ type: 'ordered', indent: Math.min(2, Math.floor(m[1].length / 2)), runs: aiParseInline(m[2]) });
    } else if ((m = raw.match(/^\s*>\s?(.*)$/))) {
      blocks.push({ type: 'quote', runs: aiParseInline(m[1]) });
    } else {
      blocks.push({ type: 'p', runs: aiParseInline(raw.trim()) });
    }
  }
  return blocks;
}

function aiRunsToHtml(runs) {
  return runs.map(r => {
    let h = escapeHtml(r.text);
    if (r.code) h = '<code>' + h + '</code>';
    if (r.bold) h = '<strong>' + h + '</strong>';
    if (r.italic) h = '<em>' + h + '</em>';
    return h;
  }).join('');
}

function aiBlocksToHtml(blocks) {
  let html = '';
  let openList = null;
  const close = () => { if (openList) { html += '</' + openList + '>'; openList = null; } };

  for (const b of blocks) {
    const inner = aiRunsToHtml(b.runs);
    if (b.type === 'bullet' || b.type === 'check' || b.type === 'ordered') {
      const tag = b.type === 'ordered' ? 'ol' : 'ul';
      if (openList !== tag) { close(); html += '<' + tag + ' class="ai-md-list">'; openList = tag; }
      const box = b.type === 'check' ? (b.checked ? '☑ ' : '☐ ') : '';
      html += '<li>' + box + inner + '</li>';
    } else {
      close();
      if (b.type === 'header') html += '<div class="ai-md-h ai-md-h' + b.level + '">' + inner + '</div>';
      else if (b.type === 'quote') html += '<blockquote class="ai-md-quote">' + inner + '</blockquote>';
      else html += '<p>' + inner + '</p>';
    }
  }
  close();
  return html;
}

// Blok → ops Delta Quill. inline=true: blok terakhir (paragraf biasa) tidak diberi
// newline penutup sehingga menyatu dengan kalimat di sekitarnya.
function aiBlocksToOps(blocks, inline) {
  const ops = [];
  blocks.forEach((b, i) => {
    for (const r of b.runs) {
      if (!r.text) continue;
      const attrs = {};
      if (r.bold) attrs.bold = true;
      if (r.italic) attrs.italic = true;
      if (r.code) attrs.code = true;
      ops.push(Object.keys(attrs).length ? { insert: r.text, attributes: attrs } : { insert: r.text });
    }

    const lineAttrs = {};
    if (b.type === 'header') lineAttrs.header = b.level;
    else if (b.type === 'bullet') lineAttrs.list = 'bullet';
    else if (b.type === 'ordered') lineAttrs.list = 'ordered';
    else if (b.type === 'check') lineAttrs.list = b.checked ? 'checked' : 'unchecked';
    else if (b.type === 'quote') lineAttrs.blockquote = true;
    if (b.indent) lineAttrs.indent = b.indent;

    const isLast = i === blocks.length - 1;
    const plain = Object.keys(lineAttrs).length === 0;
    if (inline && isLast && plain) return;   // tanpa newline penutup
    ops.push(Object.keys(lineAttrs).length ? { insert: '\n', attributes: lineAttrs } : { insert: '\n' });
  });
  return ops;
}

// ---------- 5. PANEL (dibuat sekali, dipakai ulang) ----------
function aiEnsurePanel() {
  let modal = aiEl('aiModal');
  if (modal) return modal;

  const langOptions = AI_LANGUAGES.map(l => `<option value="${l}">${l}</option>`).join('');
  const chip = (a) => `<button class="ai-chip" data-ai="${a}">${AI_ACTIONS[a].label}</button>`;

  modal = document.createElement('div');
  modal.id = 'aiModal';
  modal.className = 'modal ai-modal';
  modal.style.display = 'none';
  modal.innerHTML = `
    <div class="modal-content ai-panel" role="dialog" aria-modal="true" aria-labelledby="aiTitle" tabindex="-1">
      <div class="modal-header">
        <div>
          <h3 id="aiTitle">Asisten AI</h3>
          <div class="ai-scope" id="aiScope"></div>
        </div>
        <button class="btn-ghost-sm" data-ai-close aria-label="Tutup">✕</button>
      </div>

      <div class="ai-body" id="aiBody">
        <div class="ai-setup" id="aiSetup" style="display:none;"></div>

        <div id="aiControls">
          <div class="ai-group">
            <div class="ai-group-label">Tulisan</div>
            <div class="ai-chips">
              ${['improve', 'fix', 'shorter', 'longer', 'formal', 'casual', 'continue'].map(chip).join('')}
            </div>
          </div>
          <div class="ai-group">
            <div class="ai-group-label">Pahami</div>
            <div class="ai-chips">${chip('summarize')}</div>
          </div>
          <div class="ai-group">
            <div class="ai-group-label">Catatan ini</div>
            <div class="ai-chips">${['title', 'tags', 'tasks'].map(chip).join('')}</div>
          </div>
          <div class="ai-group">
            <div class="ai-group-label">Terjemahkan ke</div>
            <div class="ai-row">
              <select id="aiLang" aria-label="Bahasa tujuan">${langOptions}</select>
              <button class="ai-chip" data-ai="translate">Terjemahkan</button>
            </div>
          </div>
          <div class="ai-group">
            <div class="ai-group-label">Instruksi bebas</div>
            <div class="ai-row">
              <input id="aiCustom" type="text" maxlength="500" placeholder="Contoh: ubah jadi daftar poin, atau buat lebih menarik">
              <button class="ai-chip" data-ai="custom">Jalankan</button>
            </div>
          </div>
        </div>

        <div class="ai-output" id="aiOutput" style="display:none;" aria-live="polite"></div>
      </div>

      <div class="modal-footer ai-footer" id="aiFooter">
        <span class="ai-quota" id="aiQuota"></span>
      </div>
    </div>`;
  document.body.appendChild(modal);

  // Tutup: tombol ✕, klik di luar panel, atau Esc
  modal.addEventListener('mousedown', (e) => {
    if (e.target === modal) aiClose();
  });
  modal.querySelector('[data-ai-close]').addEventListener('click', aiClose);

  // Klik aksi
  modal.addEventListener('click', (e) => {
    if (e.target.closest('[data-ai-back]')) { aiBackToControls(); return; }
    const btn = e.target.closest('[data-ai]');
    if (btn && !aiState.busy) aiRun(btn.dataset.ai);
  });
  // Esc menutup panel meskipun fokus sedang tidak ada di dalam panel
  // (fase capture + stopImmediatePropagation, supaya handler Esc milik app tidak ikut jalan).
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && aiState.open) {
      e.preventDefault();
      e.stopImmediatePropagation();
      aiClose();
    }
  }, true);
  aiEl('aiCustom').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); if (!aiState.busy) aiRun('custom'); }
  });

  return modal;
}

// ---------- 6. BUKA / TUTUP ----------
function aiCaptureContext() {
  aiState.editable = !!(quill && quill.isEnabled());

  // Saat tombol AI diklik, editor kehilangan fokus; pakai posisi terakhir yang tersimpan.
  let r = quill.getSelection();
  if (!r && quill.selection && quill.selection.savedRange) r = quill.selection.savedRange;

  let range = r ? { index: r.index, length: r.length } : null;
  if (range && range.length > 0) {
    // Jangan ikutkan newline di ujung pilihan supaya baris berikutnya tidak tergabung
    let t = quill.getText(range.index, range.length);
    while (t.endsWith('\n') && range.length > 0) { t = t.slice(0, -1); range.length--; }
    if (range.length === 0) range = { index: range.index, length: 0 };
  }
  aiState.range = range;
}

function aiScopeFor(action) {
  const kind = AI_ACTIONS[action].kind;
  const r = aiState.range;
  const hasSel = !!(r && r.length > 0);

  // Judul / tag / tugas selalu memakai seluruh catatan
  if (kind === 'title' || kind === 'tags' || kind === 'tasks') return 'note';
  return hasSel ? 'selection' : 'note';
}

function aiTextFor(action, scope) {
  const r = aiState.range;
  if (scope === 'selection') return quill.getText(r.index, r.length);
  if (action === 'continue' && r && r.index > 0) {
    // Lanjutkan dari posisi kursor: konteksnya adalah tulisan sebelum kursor
    return quill.getText(0, r.index);
  }
  return quill.getText();
}

function aiUpdateScopeLabel() {
  const el = aiEl('aiScope');
  if (!el) return;
  const r = aiState.range;
  const hasSel = !!(r && r.length > 0);
  if (hasSel) {
    const words = aiWordCount(quill.getText(r.index, r.length));
    el.textContent = `Teks terpilih · ${words} kata`;
  } else {
    el.textContent = `Seluruh catatan · ${aiWordCount(quill.getText())} kata`;
  }
}

async function openAIPanel(presetAction) {
  if (typeof quill === 'undefined' || !quill || !currentNoteId) {
    showToast('Buka sebuah catatan dulu.', 'error');
    return;
  }

  aiCaptureContext();
  const modal = aiEnsurePanel();
  aiResetOutput();
  aiUpdateScopeLabel();

  modal.style.display = 'flex';
  aiState.open = true;
  modal.querySelector('.ai-panel').focus();

  // Cek apakah AI sudah dikonfigurasi di server
  const st = await api('/ai/status');
  aiState.status = st;
  const setup = aiEl('aiSetup');
  const controls = aiEl('aiControls');

  if (!st) {
    setup.style.display = 'block';
    setup.innerHTML = '<strong>Tidak bisa memeriksa status AI.</strong><p>Periksa koneksi ke server lalu buka panel ini lagi.</p>';
    controls.style.display = 'none';
    aiEl('aiQuota').textContent = '';
    return;
  }

  if (st.enabled === false) {
    setup.style.display = 'block';
    setup.innerHTML = `
      <strong>Asisten AI belum aktif.</strong>
      <p>Buat API key gratis di <a href="https://aistudio.google.com/apikey" target="_blank" rel="noopener">Google AI Studio</a>,
      lalu tambahkan baris ini di file <code>.env</code> dan jalankan ulang server (<code>go run .</code>):</p>
      <pre>AI_API_KEY=isi_api_key_kamu</pre>`;
    controls.style.display = 'none';
    aiEl('aiQuota').textContent = '';
    return;
  }

  setup.style.display = 'none';
  controls.style.display = '';
  aiShowQuota(st.remaining_today, st.per_day);

  if (presetAction && AI_ACTIONS[presetAction]) aiRun(presetAction);
}

function aiClose() {
  if (aiState.controller) aiState.controller.abort();
  aiState.busy = false;
  aiState.open = false;
  const modal = aiEl('aiModal');
  if (modal) modal.style.display = 'none';
  if (quill && quill.isEnabled()) quill.focus();
}

function aiShowQuota(remaining, perDay) {
  const el = aiEl('aiQuota');
  if (!el) return;
  el.textContent = (typeof remaining === 'number')
    ? `Sisa jatah AI hari ini: ${remaining} dari ${perDay}`
    : '';
}

function aiResetOutput() {
  aiState.result = null;
  aiState.action = null;
  const out = aiEl('aiOutput');
  if (out) { out.style.display = 'none'; out.innerHTML = ''; }
  const body = aiEl('aiBody');
  if (body) body.querySelectorAll('.ai-chip').forEach(c => c.classList.remove('active'));
  const controls = aiEl('aiControls');
  if (controls && !(aiState.status && aiState.status.enabled === false)) controls.style.display = '';
  if (body) body.scrollTop = 0;
  const panel = document.querySelector('#aiModal .ai-panel');
  if (panel && aiState.open) panel.focus({ preventScroll: true });
}

// Kembali ke daftar aksi (membatalkan permintaan yang sedang berjalan)
function aiBackToControls() {
  if (aiState.controller) aiState.controller.abort();
  aiState.busy = false;
  aiState.runId++;
  aiResetOutput();
}

// Baris atas tampilan hasil: tombol kembali + nama aksi
function aiBar(label, loading) {
  return `<div class="ai-bar">
    <button class="btn-ghost-sm" data-ai-back>${loading ? 'Batal' : '← Aksi lain'}</button>
    <span class="ai-bar-title">${escapeHtml(label)}</span>
  </div>`;
}

function aiShowOutput(html) {
  const out = aiEl('aiOutput');
  aiEl('aiControls').style.display = 'none';
  out.style.display = 'block';
  out.innerHTML = html;
  aiEl('aiBody').scrollTop = 0;
  // Isi panel dirender ulang, jadi fokus lama hilang. Kembalikan ke tombol utama / panel.
  const target = out.querySelector('.btn-primary') || aiEl('aiModal').querySelector('.ai-panel');
  if (target) target.focus({ preventScroll: true });
  return out;
}

// ---------- 7. JALANKAN AKSI ----------
async function aiRun(action) {
  const def = AI_ACTIONS[action];
  if (!def || !quill) return;

  const instruction = (aiEl('aiCustom').value || '').trim();
  if (action === 'custom' && !instruction) {
    aiEl('aiCustom').focus();
    showToast('Tulis instruksinya dulu.', 'error');
    return;
  }

  aiCaptureContext();
  aiUpdateScopeLabel();
  const scope = aiScopeFor(action);
  const text = aiTextFor(action, scope).trim();
  if (!text) {
    aiShowError({ error: 'Catatan masih kosong. Tulis sesuatu atau pilih teks dulu.' });
    return;
  }

  aiState.action = action;
  aiState.scope = scope;
  aiState.text = text;

  // Tandai tombol aktif
  aiEl('aiBody').querySelectorAll('.ai-chip').forEach(c => c.classList.toggle('active', c.dataset.ai === action));

  const runId = ++aiState.runId;
  if (aiState.controller) aiState.controller.abort();
  aiState.controller = new AbortController();
  aiState.busy = true;
  aiShowLoading(def.label);

  let data = null;
  try {
    data = await api('/ai/generate', {
      method: 'POST',
      signal: aiState.controller.signal,
      body: JSON.stringify({
        action,
        text,
        instruction,
        language: aiEl('aiLang').value,
        note_id: currentNoteId
      })
    });
  } catch (e) {
    if (e && e.name === 'AbortError') return;           // dibatalkan pengguna
    data = { error: 'Tidak bisa terhubung ke server. Periksa koneksi lalu coba lagi.' };
  }

  if (runId !== aiState.runId) return;                    // sudah ada permintaan yang lebih baru
  aiState.busy = false;

  if (!data) {
    aiShowError({ error: 'Server tidak memberi respons. Coba lagi.' });
    return;
  }
  if (data.error) {
    aiShowError(data);
    return;
  }

  aiState.result = data;
  if (typeof data.remaining_today === 'number') {
    aiShowQuota(data.remaining_today, aiState.status ? aiState.status.per_day : '');
  }
  aiShowResult(action, data);
}

// ---------- 8. TAMPILAN HASIL ----------
function aiShowLoading(label) {
  aiShowOutput(aiBar(label, true) + `
    <div class="ai-loading">
      <span class="ai-dots" aria-hidden="true"><i></i><i></i><i></i></span>
      <span>AI sedang bekerja…</span>
    </div>`);
}

function aiShowError(data) {
  const label = aiState.action ? AI_ACTIONS[aiState.action].label : 'Asisten AI';
  const detail = data.detail ? `<div class="ai-error-detail">${escapeHtml(data.detail)}</div>` : '';
  const out = aiShowOutput(aiBar(label, false) + `
    <div class="ai-error" role="alert">
      <strong>${escapeHtml(data.error || 'Terjadi kesalahan.')}</strong>
      ${detail}
      ${aiState.action ? '<div class="ai-actions"><button class="btn-ghost-sm" data-ai-retry>Coba lagi</button></div>' : ''}
    </div>`);
  const retry = out.querySelector('[data-ai-retry]');
  if (retry) retry.addEventListener('click', () => aiRun(aiState.action));
}

function aiActionButtons(action, data) {
  const kind = AI_ACTIONS[action].kind;
  const hasSel = aiState.scope === 'selection';
  const edit = aiState.editable;
  const btns = [];

  if (kind === 'transform') {
    if (edit && hasSel) btns.push('<button class="btn-primary" data-ai-do="replace">Ganti teks terpilih</button>');
    if (edit) btns.push(`<button class="${hasSel ? 'btn-ghost-sm' : 'btn-primary'}" data-ai-do="below">Sisipkan di bawah</button>`);
  } else if (kind === 'generate') {
    if (edit) btns.push('<button class="btn-primary" data-ai-do="below">Sisipkan di bawah</button>');
  } else if (kind === 'continue') {
    if (edit) btns.push('<button class="btn-primary" data-ai-do="cursor">Sisipkan di kursor</button>');
  }
  if (kind === 'transform' || kind === 'generate' || kind === 'continue') {
    btns.push('<button class="btn-ghost-sm" data-ai-do="copy">Salin</button>');
  }
  btns.push('<button class="btn-ghost-sm" data-ai-retry>Coba lagi</button>');
  return btns.join('');
}

function aiShowResult(action, data) {
  const kind = AI_ACTIONS[action].kind;

  const note = data.truncated
    ? '<div class="ai-note">Teks terlalu panjang, jadi hanya sebagian yang diproses AI.</div>' : '';
  const readonlyNote = (!aiState.editable && ['transform', 'generate', 'continue', 'title', 'tags', 'tasks'].includes(kind))
    ? '<div class="ai-note">Catatan ini tidak bisa diedit, jadi hasilnya hanya bisa disalin.</div>' : '';

  let inner = '';

  if (kind === 'title') {
    inner = `
      <div class="ai-result-label">Judul yang disarankan</div>
      <div class="ai-result ai-result-title">${escapeHtml(data.result)}</div>
      <div class="ai-actions">
        ${aiState.editable ? '<button class="btn-primary" data-ai-do="title">Pakai judul ini</button>' : ''}
        <button class="btn-ghost-sm" data-ai-retry>Coba lagi</button>
      </div>`;
  } else if (kind === 'tags') {
    const items = data.items || [];
    if (!items.length) {
      inner = `<div class="ai-result">AI tidak menemukan tag yang cocok.</div>
        <div class="ai-actions"><button class="btn-ghost-sm" data-ai-retry>Coba lagi</button></div>`;
    } else {
      inner = `
        <div class="ai-result-label">Saran tag (hapus centang untuk melewati)</div>
        <div class="ai-items" id="aiItems">
          ${items.map((t, i) => `<label class="ai-check"><input type="checkbox" checked data-i="${i}"><span>${escapeHtml(t)}</span></label>`).join('')}
        </div>
        <div class="ai-actions">
          ${aiState.editable ? '<button class="btn-primary" data-ai-do="tags">Terapkan tag</button>' : ''}
          <button class="btn-ghost-sm" data-ai-retry>Coba lagi</button>
        </div>`;
    }
  } else if (kind === 'tasks') {
    const items = data.items || [];
    if (!items.length) {
      inner = `<div class="ai-result">Tidak ada tugas yang ditemukan di catatan ini.</div>
        <div class="ai-actions"><button class="btn-ghost-sm" data-ai-retry>Coba lagi</button></div>`;
    } else {
      const prio = { high: 'Tinggi', medium: 'Sedang', low: 'Rendah' };
      inner = `
        <div class="ai-result-label">Tugas yang ditemukan (hapus centang untuk melewati)</div>
        <div class="ai-items" id="aiItems">
          ${items.map((t, i) => `<label class="ai-check"><input type="checkbox" checked data-i="${i}"><span>${escapeHtml(t.title)}</span><em class="ai-prio ai-prio-${t.priority}">${prio[t.priority] || ''}</em></label>`).join('')}
        </div>
        <div class="ai-actions">
          <button class="btn-primary" data-ai-do="tasks">Buat tugas</button>
          <button class="btn-ghost-sm" data-ai-retry>Coba lagi</button>
        </div>`;
    }
  } else {
    const blocks = aiParseBlocks(data.result);
    inner = `
      <div class="ai-result-label">Hasil</div>
      <div class="ai-result ai-result-text">${aiBlocksToHtml(blocks)}</div>
      <div class="ai-actions">${aiActionButtons(action, data)}</div>`;
  }

  const out = aiShowOutput(aiBar(AI_ACTIONS[action].label, false) + note + readonlyNote + inner);

  out.querySelectorAll('[data-ai-do]').forEach(b => b.addEventListener('click', () => aiApply(b.dataset.aiDo)));
  const retry = out.querySelector('[data-ai-retry]');
  if (retry) retry.addEventListener('click', () => aiRun(aiState.action));
}

// ---------- 9. TERAPKAN HASIL ----------
function aiCheckedIndexes() {
  return Array.from(document.querySelectorAll('#aiItems input[type=checkbox]:checked'))
    .map(c => parseInt(c.dataset.i, 10));
}

// Posisi tepat setelah baris yang berisi index (awal baris berikutnya)
function aiEndOfLine(index) {
  const len = quill.getLength();
  try {
    const [line] = quill.getLine(Math.min(Math.max(index, 0), len - 1));
    if (line) return Math.min(quill.getIndex(line) + line.length(), len);
  } catch (e) { /* jatuh ke akhir dokumen */ }
  return len;
}

// Posisi kursor yang bermakna. Jika editor belum pernah difokuskan (posisi 0),
// anggap kursor ada di akhir catatan.
function aiCursorIndex() {
  const r = aiState.range;
  if (r && (r.length > 0 || r.index > 0)) return r.index + r.length;
  return Math.max(quill.getLength() - 1, 0);
}

async function aiApply(mode) {
  const res = aiState.result;
  if (!res || !quill) return;
  const Delta = Quill.import('delta');

  try {
    if (mode === 'copy') {
      await aiCopy(res.result);
      showToast('Hasil disalin.', 'success');
      return;
    }

    if (mode === 'title') {
      const input = aiEl('noteTitle');
      input.value = res.result;
      if (typeof onTitleChange === 'function') onTitleChange();
      aiClose();
      showToast('Judul diperbarui.', 'success');
      return;
    }

    if (mode === 'tags') {
      const names = aiCheckedIndexes().map(i => res.items[i]);
      if (!names.length) { showToast('Tidak ada tag yang dipilih.', 'error'); return; }
      const n = await aiApplyTags(names);
      aiClose();
      showToast(n + ' tag ditambahkan ke catatan.', 'success');
      return;
    }

    if (mode === 'tasks') {
      const picked = aiCheckedIndexes().map(i => res.items[i]);
      if (!picked.length) { showToast('Tidak ada tugas yang dipilih.', 'error'); return; }
      const n = await aiCreateTasks(picked);
      aiClose();
      showToast(n + ' tugas dibuat. Lihat di menu Tugas.', 'success');
      return;
    }

    // --- sisipkan ke editor ---
    if (!quill.isEnabled()) { showToast('Catatan ini tidak bisa diedit.', 'error'); return; }
    const blocks = aiParseBlocks(res.result);
    if (!blocks.length) return;
    const r = aiState.range;

    if (mode === 'replace' && r && r.length > 0) {
      const ops = aiBlocksToOps(blocks, true);
      quill.updateContents(new Delta().retain(r.index).delete(r.length).concat(new Delta(ops)), 'user');
      aiClose();
      showToast('Teks diganti. Tekan Ctrl+Z untuk membatalkan.', 'success');
      return;
    }

    if (mode === 'cursor') {
      const at = aiCursorIndex();
      const ops = aiBlocksToOps(blocks, true);
      // Mulai dengan spasi bila kursor berada tepat setelah karakter non-spasi
      const prev = at > 0 ? quill.getText(at - 1, 1) : '\n';
      if (prev && !/\s/.test(prev) && blocks[0].type === 'p') {
        ops.unshift({ insert: ' ' });
      } else if (prev && prev !== '\n' && blocks[0].type !== 'p') {
        ops.unshift({ insert: '\n' });
      }
      quill.updateContents(new Delta().retain(at).concat(new Delta(ops)), 'user');
      aiClose();
      showToast('Teks disisipkan. Tekan Ctrl+Z untuk membatalkan.', 'success');
      return;
    }

    // 'below' (dan 'replace' tanpa seleksi): sisipkan sebagai blok baru setelah baris terpilih
    // Teks terpilih → setelah baris pilihan. Seluruh catatan → di akhir catatan.
    const anchor = (aiState.scope === 'selection' && r) ? r.index + r.length : quill.getLength() - 1;
    const at = aiEndOfLine(anchor);
    const ops = aiBlocksToOps(blocks, false);
    quill.updateContents(new Delta().retain(at).concat(new Delta(ops)), 'user');
    aiClose();
    showToast('Hasil disisipkan. Tekan Ctrl+Z untuk membatalkan.', 'success');
  } catch (e) {
    console.error('AI apply error:', e);
    showToast('Gagal menerapkan hasil AI.', 'error');
  }
}

async function aiCopy(text) {
  try {
    await navigator.clipboard.writeText(text);
  } catch (e) {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    ta.remove();
  }
}

function aiTagColor(name) {
  let h = 0;
  for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return AI_TAG_COLORS[h % AI_TAG_COLORS.length];
}

async function aiApplyTags(names) {
  const existing = (await api('/tags')) || [];
  const byName = new Map(existing.map(t => [String(t.name).toLowerCase(), t]));
  let done = 0;

  for (const name of names) {
    let tag = byName.get(name.toLowerCase());
    if (!tag) {
      const created = await api('/tags', {
        method: 'POST',
        body: JSON.stringify({ name, color: aiTagColor(name) })
      });
      if (created && created.id) { tag = created; byName.set(name.toLowerCase(), tag); }
    }
    if (tag) {
      await api('/notes/' + currentNoteId + '/tags/' + tag.id, { method: 'POST' });
      done++;
    }
  }
  if (typeof loadTags === 'function') loadTags();
  return done;
}

async function aiCreateTasks(tasks) {
  let done = 0;
  for (const t of tasks) {
    const created = await api('/tasks', {
      method: 'POST',
      body: JSON.stringify({ title: t.title, priority: t.priority || 'medium', note_id: currentNoteId })
    });
    if (created && !created.error) done++;
  }
  return done;
}

// ---------- 10. SLASH MENU ----------
// Item "/" → AI. Dipasang di atas fungsi executeSlashAction yang sudah ada
// (termasuk pembungkus di 6-advanced.js).
const _aiPrevExecuteSlashAction = executeSlashAction;
executeSlashAction = function (action) {
  const map = { ai: null, ai_summarize: 'summarize', ai_continue: 'continue' };
  if (Object.prototype.hasOwnProperty.call(map, action)) {
    if (quill && slashStartIndex >= 0) {
      // Saat item diklik dengan mouse, editor kehilangan fokus dan getSelection() bernilai null.
      // Pakai posisi tersimpan, dan pastikan yang dihapus memang karakter "/".
      let range = quill.getSelection();
      if (!range && quill.selection && quill.selection.savedRange) range = quill.selection.savedRange;
      const end = range ? Math.max(range.index, slashStartIndex + 1) : slashStartIndex + 1;
      if (quill.getText(slashStartIndex, 1) === '/') {
        quill.deleteText(slashStartIndex, end - slashStartIndex, 'user');
        quill.setSelection(slashStartIndex, 0, 'silent');
      }
    }
    closeSlashMenu();
    setTimeout(() => openAIPanel(map[action]), 0);
    return;
  }
  return _aiPrevExecuteSlashAction.apply(this, arguments);
};

console.log('✅ 7-ai.js loaded (Asisten AI)');
