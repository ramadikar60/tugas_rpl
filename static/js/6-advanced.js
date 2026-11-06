// ============================================================
// 6-ADVANCED.JS — Search, Templates, Comments, Versions, Graph,
//                 Share, Admin, Notifications, Quick Capture,
//                 Command Palette, Dashboard, Productivity,
//                 Data Management, Advanced Editor (Quill Blots)
// ============================================================

// ============================================================
// STATE
// ============================================================
let searchHistoryCache = [];
let tplSearchTimer = null;
let currentTplCategory = 'all';
let currentShareTab = 'link';
let selectedShareUser = null;
let shareUserTimer = null;
let currentEditorTab = 'content';
let notifPollInterval = null;
let graphAnimationId = null;
let currentProductivityTab = 'pomodoro';
let currentDmTab = 'storage';

// ============================================================
// ADVANCED EDITOR — Custom Quill Blots
// (di-merge dari advanced-editor.js)
// ============================================================
(function setupAdvancedEditor() {
  'use strict';

  if (typeof Quill === 'undefined') {
    console.warn('Quill not loaded yet, retrying advanced editor...');
    setTimeout(setupAdvancedEditor, 100);
    return;
  }

  const BlockEmbed = Quill.import('blots/block/embed');

  // ------------------------------------------------------------
  // Helper escape
  // ------------------------------------------------------------
  function advEscapeHtml(s) {
    if (!s) return '';
    return String(s).replace(/[&<>"']/g, c => ({
      '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
    }[c]));
  }

  function advEscapeAttr(s) {
    return advEscapeHtml(s).replace(/"/g, '&quot;');
  }

  // ------------------------------------------------------------
  // Keamanan (Prompt 13): URL http(s) saja; embed hanya dari penyedia tepercaya
  // ------------------------------------------------------------
  const ADV_EMBED_RE = /^https:\/\/(?:www\.youtube(?:-nocookie)?\.com\/embed\/[A-Za-z0-9_-]{1,40}|player\.vimeo\.com\/video\/[0-9]{1,15}|open\.spotify\.com\/embed\/(?:track|album|playlist)\/[A-Za-z0-9]{1,40})(?:\?[A-Za-z0-9=&_%.-]{0,200})?$/;

  function advSafeHttpUrl(u) {
    try {
      const p = new URL(String(u || '').trim());
      return (p.protocol === 'http:' || p.protocol === 'https:') ? p.href : '';
    } catch (e) {
      return '';
    }
  }

  function advEmbedAllowed(u) {
    return ADV_EMBED_RE.test(String(u || ''));
  }

  // HTML kaya (isi callout/toggle) selalu disanitasi; tanpa helper, di-escape.
  function safeBlotHTML(h) {
    return window.safeNoteHTML ? window.safeNoteHTML(h) : advEscapeHtml(h);
  }

  // ============================================================
  // 1. CALLOUT BLOT
  // ============================================================
  class CalloutBlot extends BlockEmbed {
    static create(value) {
      const node = super.create();

      let type = 'info';
      let title = '';
      let content = '';

      if (typeof value === 'string') {
        content = value;
      } else if (typeof value === 'object') {
        type = value.type || 'info';
        title = value.title || '';
        content = value.content || '';
      }

      if (!/^[a-z]{1,20}$/.test(type)) type = 'info';

      node.setAttribute('data-type', type);
      node.setAttribute('data-title', title);
      node.setAttribute('contenteditable', 'false');
      node.classList.add('adv-callout', 'callout-' + type);

      const icons = {
        info: 'i', warning: '!', success: '✓',
        danger: '✕', tip: '✦', note: '✎'
      };

      node.innerHTML = `
        <div class="callout-header">
          <span class="callout-icon">${icons[type] || 'i'}</span>
          <span class="callout-title">${advEscapeHtml(title || CalloutBlot.defaultTitle(type))}</span>
        </div>
        <div class="callout-body">${safeBlotHTML(content || 'Tulis isi callout...')}</div>
      `;

      return node;
    }

    static defaultTitle(type) {
      const titles = {
        info: 'Info', warning: 'Warning', success: 'Success',
        danger: 'Danger', tip: 'Tip', note: 'Note'
      };
      return titles[type] || 'Info';
    }

    static value(node) {
      return {
        type: node.getAttribute('data-type') || 'info',
        title: node.getAttribute('data-title') || '',
        content: node.querySelector('.callout-body')?.innerHTML || ''
      };
    }

    static formats(node) {
      return {
        type: node.getAttribute('data-type') || 'info',
        title: node.getAttribute('data-title') || ''
      };
    }

    format(name, value) {
      if (name === 'type' || name === 'title') {
        this.domNode.setAttribute('data-' + name, value);
        if (name === 'type') {
          this.domNode.classList.forEach(c => {
            if (c.startsWith('callout-')) this.domNode.classList.remove(c);
          });
          this.domNode.classList.add('callout-' + value);
        }
      } else {
        super.format(name, value);
      }
    }

    html() {
      const data = this.value(this.domNode);
      return `<div class="adv-callout callout-${data.type}" data-type="${data.type}" data-title="${advEscapeAttr(data.title)}">
        <div class="callout-header"><span class="callout-title">${advEscapeHtml(data.title)}</span></div>
        <div class="callout-body">${data.content}</div>
      </div>`;
    }
  }
  CalloutBlot.blotName = 'callout';
  CalloutBlot.tagName = 'div';
  CalloutBlot.className = 'adv-callout';

  // ============================================================
  // 2. TOGGLE BLOT
  // ============================================================
  class ToggleBlot extends BlockEmbed {
    static create(value) {
      const node = super.create();

      let title = 'Toggle Section';
      let content = 'Klik untuk expand/collapse...';
      let open = false;

      if (typeof value === 'string') {
        title = value;
      } else if (typeof value === 'object') {
        title = value.title || title;
        content = value.content || content;
        open = value.open || false;
      }

      node.setAttribute('data-title', title);
      node.setAttribute('data-open', open ? 'true' : 'false');
      node.setAttribute('contenteditable', 'false');
      node.classList.add('adv-toggle');
      if (open) node.classList.add('is-open');

      node.innerHTML = `
        <div class="toggle-header">
          <span class="toggle-arrow">${open ? '▼' : '▶'}</span>
          <span class="toggle-title">${advEscapeHtml(title)}</span>
        </div>
        <div class="toggle-body" style="${open ? '' : 'display:none'}">${safeBlotHTML(content)}</div>
      `;

      const header = node.querySelector('.toggle-header');
      header.addEventListener('click', () => {
        const isOpen = node.classList.toggle('is-open');
        node.setAttribute('data-open', isOpen ? 'true' : 'false');
        node.querySelector('.toggle-arrow').textContent = isOpen ? '▼' : '▶';
        const body = node.querySelector('.toggle-body');
        body.style.display = isOpen ? '' : 'none';
      });

      return node;
    }

    static value(node) {
      const body = node.querySelector('.toggle-body');
      return {
        title: node.getAttribute('data-title') || 'Toggle',
        content: body?.innerHTML || '',
        open: node.getAttribute('data-open') === 'true'
      };
    }

    static formats(node) {
      return {
        title: node.getAttribute('data-title') || '',
        open: node.getAttribute('data-open') === 'true'
      };
    }

    html() {
      const data = this.value(this.domNode);
      return `<details class="adv-toggle" ${data.open ? 'open' : ''}>
        <summary>${advEscapeHtml(data.title)}</summary>
        <div class="toggle-body">${data.content}</div>
      </details>`;
    }
  }
  ToggleBlot.blotName = 'toggle';
  ToggleBlot.tagName = 'div';
  ToggleBlot.className = 'adv-toggle';

  // ============================================================
  // 3. MATH BLOT
  // ============================================================
  class MathBlot extends BlockEmbed {
    static create(value) {
      const node = super.create();

      let formula = '';
      let displayMode = true;

      if (typeof value === 'string') {
        formula = value;
      } else if (typeof value === 'object') {
        formula = value.formula || '';
        displayMode = value.display !== false;
      }

      node.setAttribute('data-formula', formula);
      node.setAttribute('data-display', displayMode ? 'true' : 'false');
      node.setAttribute('contenteditable', 'false');
      node.classList.add('adv-math');
      if (displayMode) node.classList.add('math-block');
      else node.classList.add('math-inline');

      node.innerHTML = `<div class="math-content"></div>`;

      setTimeout(() => renderMath(node), 0);

      node.addEventListener('click', () => {
        const newFormula = prompt('Edit formula (LaTeX):', node.getAttribute('data-formula') || '');
        if (newFormula !== null) {
          node.setAttribute('data-formula', newFormula);
          renderMath(node);
        }
      });

      return node;
    }

    static value(node) {
      return {
        formula: node.getAttribute('data-formula') || '',
        display: node.getAttribute('data-display') === 'true'
      };
    }

    static formats(node) {
      return {
        formula: node.getAttribute('data-formula') || '',
        display: node.getAttribute('data-display') === 'true'
      };
    }

    html() {
      const data = this.value(this.domNode);
      const delimiter = data.display ? '$$' : '$';
      return `${delimiter}${data.formula}${delimiter}`;
    }
  }
  MathBlot.blotName = 'math';
  MathBlot.tagName = 'div';
  MathBlot.className = 'adv-math';

  function renderMath(node) {
    const content = node.querySelector('.math-content');
    const formula = node.getAttribute('data-formula') || '';
    const display = node.getAttribute('data-display') === 'true';

    if (!content || !formula) {
      if (content) content.innerHTML = '<em style="color:var(--text-faint);">Klik untuk edit rumus...</em>';
      return;
    }

    if (typeof katex === 'undefined') {
      content.textContent = formula;
      return;
    }

    try {
      katex.render(formula, content, {
        displayMode: display,
        throwOnError: false,
        errorColor: '#dc2626'
      });
    } catch (e) {
      content.textContent = formula;
      content.style.color = '#dc2626';
    }
  }

  // ============================================================
  // 4. MERMAID BLOT
  // ============================================================
  class MermaidBlot extends BlockEmbed {
    static create(value) {
      const node = super.create();

      let code = '';
      if (typeof value === 'string') {
        code = value;
      } else if (typeof value === 'object') {
        code = value.code || '';
      }

      if (!code) {
        code = 'graph TD;\n  A[Start] --> B{Decision};\n  B -->|Yes| C[OK];\n  B -->|No| D[Cancel];';
      }

      node.setAttribute('data-code', code);
      node.setAttribute('contenteditable', 'false');
      node.classList.add('adv-mermaid');

      const id = 'mermaid-' + Date.now() + '-' + Math.random().toString(36).substr(2, 9);

      node.innerHTML = `
        <div class="mermaid-toolbar">
          <span class="mermaid-label">◇ Mermaid Diagram</span>
          <button class="mermaid-edit-btn" title="Edit">✎</button>
        </div>
        <div class="mermaid-content" id="${id}"></div>
      `;

      setTimeout(() => renderMermaid(node, id), 100);

      const editBtn = node.querySelector('.mermaid-edit-btn');
      editBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        const newCode = prompt('Edit Mermaid code:', node.getAttribute('data-code') || '');
        if (newCode !== null) {
          node.setAttribute('data-code', newCode);
          renderMermaid(node, id);
        }
      });

      return node;
    }

    static value(node) {
      return node.getAttribute('data-code') || '';
    }

    static formats(node) {
      return node.getAttribute('data-code') || '';
    }

    html() {
      const code = this.value(this.domNode);
      return `<div class="adv-mermaid" data-code="${advEscapeAttr(code)}"><pre>${advEscapeHtml(code)}</pre></div>`;
    }
  }
  MermaidBlot.blotName = 'mermaid';
  MermaidBlot.tagName = 'div';
  MermaidBlot.className = 'adv-mermaid';

  async function renderMermaid(node, id) {
    const content = node.querySelector('.mermaid-content');
    const code = node.getAttribute('data-code') || '';

    if (!content || !code) return;

    if (typeof mermaid === 'undefined') {
      content.innerHTML = `<pre style="font-size:12px;">${advEscapeHtml(code)}</pre>`;
      return;
    }

    try {
      content.textContent = code; // teks, bukan HTML (mermaid membaca dan men-decode sendiri)
      content.removeAttribute('data-processed');

      await mermaid.run({
        nodes: [content],
        suppressErrors: true
      });
    } catch (e) {
      console.warn('Mermaid render error:', e);
      content.innerHTML = `<pre style="font-size:12px;color:#dc2626;">${advEscapeHtml(code)}</pre>`;
    }
  }

  // ============================================================
  // 5. FOOTNOTE BLOT
  // ============================================================
  let footnoteCounter = 0;
  class FootnoteBlot extends BlockEmbed {
    static create(value) {
      const node = super.create();

      let text = '';
      if (typeof value === 'string') text = value;
      else if (typeof value === 'object') text = value.text || '';

      footnoteCounter++;
      const number = footnoteCounter;

      node.setAttribute('data-text', text);
      node.setAttribute('data-number', number);
      node.setAttribute('contenteditable', 'false');
      node.classList.add('adv-footnote');
      node.innerHTML = `<sup class="footnote-marker">[${number}]</sup>`;

      node.title = text || 'Klik untuk edit';

      node.addEventListener('click', () => {
        const newText = prompt('Isi footnote:', node.getAttribute('data-text') || '');
        if (newText !== null) {
          node.setAttribute('data-text', newText);
          node.title = newText;
        }
      });

      return node;
    }

    static value(node) {
      return {
        text: node.getAttribute('data-text') || '',
        number: node.getAttribute('data-number') || '0'
      };
    }

    html() {
      const data = this.value(this.domNode);
      return `<sup class="footnote-marker" title="${advEscapeAttr(data.text)}">[${data.number}]</sup>`;
    }
  }
  FootnoteBlot.blotName = 'footnote';
  FootnoteBlot.tagName = 'sup';
  FootnoteBlot.className = 'adv-footnote';

  // ============================================================
  // 6. BOOKMARK BLOT
  // ============================================================
  class BookmarkBlot extends BlockEmbed {
    static create(value) {
      const node = super.create();

      let url = '';
      let title = '';
      let description = '';

      if (typeof value === 'string') {
        url = value;
      } else if (typeof value === 'object') {
        url = value.url || '';
        title = value.title || '';
        description = value.description || '';
      }

      url = advSafeHttpUrl(url);

      node.setAttribute('data-url', url);
      node.setAttribute('data-title', title);
      node.setAttribute('data-description', description);
      node.setAttribute('contenteditable', 'false');
      node.classList.add('adv-bookmark');

      let domain = '';
      try {
        domain = new URL(url).hostname;
      } catch (e) {
        domain = url;
      }

      node.innerHTML = `
        <a href="${advEscapeAttr(url || '#')}" target="_blank" rel="noopener noreferrer" class="bookmark-link">
          <div class="bookmark-icon">↗</div>
          <div class="bookmark-info">
            <div class="bookmark-domain">${advEscapeHtml(domain)}</div>
            <div class="bookmark-title">${advEscapeHtml(title || url)}</div>
            ${description ? `<div class="bookmark-desc">${advEscapeHtml(description)}</div>` : ''}
          </div>
        </a>
      `;

      return node;
    }

    static value(node) {
      return {
        url: node.getAttribute('data-url') || '',
        title: node.getAttribute('data-title') || '',
        description: node.getAttribute('data-description') || ''
      };
    }

    static formats(node) {
      return {
        url: node.getAttribute('data-url') || '',
        title: node.getAttribute('data-title') || '',
        description: node.getAttribute('data-description') || ''
      };
    }

    html() {
      const data = this.value(this.domNode);
      return `<a href="${advEscapeAttr(data.url)}" target="_blank" class="adv-bookmark">${advEscapeHtml(data.title || data.url)}</a>`;
    }
  }
  BookmarkBlot.blotName = 'bookmark';
  BookmarkBlot.tagName = 'div';
  BookmarkBlot.className = 'adv-bookmark';

  // ============================================================
  // 7. EMBED BLOT
  // ============================================================
  class EmbedBlot extends BlockEmbed {
    static create(value) {
      const node = super.create();

      let url = '';
      let embedType = 'iframe';
      let embedUrl = '';

      if (typeof value === 'string') {
        url = value;
      } else if (typeof value === 'object') {
        url = value.url || '';
        embedUrl = value.embed_url || '';
      }

      if (!embedUrl) {
        const detected = detectEmbed(url);
        embedType = detected.type;
        embedUrl = detected.embedUrl;
      }

      url = advSafeHttpUrl(url);
      if (embedType === 'iframe' && !advEmbedAllowed(embedUrl)) {
        embedType = 'link';
        embedUrl = '';
      }

      node.setAttribute('data-url', url);
      node.setAttribute('data-embed-url', embedUrl);
      node.setAttribute('data-embed-type', embedType);
      node.setAttribute('contenteditable', 'false');
      node.classList.add('adv-embed');

      if (embedType === 'iframe') {
        node.innerHTML = `
          <div class="embed-toolbar">
            <span class="embed-label">⧉ Embed</span>
            <button class="embed-edit-btn">✎</button>
          </div>
          <div class="embed-content">
            <iframe src="${advEscapeAttr(embedUrl)}" frameborder="0" allowfullscreen loading="lazy" sandbox="allow-scripts allow-same-origin allow-presentation allow-popups"></iframe>
          </div>
        `;
      } else {
        node.innerHTML = `
          <div class="embed-toolbar">
            <span class="embed-label">⧉ Link</span>
          </div>
          <div class="embed-content">
            <a href="${advEscapeAttr(url || '#')}" target="_blank" rel="noopener noreferrer">${advEscapeHtml(url)}</a>
          </div>
        `;
      }

      const editBtn = node.querySelector('.embed-edit-btn');
      if (editBtn) {
        editBtn.addEventListener('click', (e) => {
          e.stopPropagation();
          const newUrl = prompt('Edit URL embed:', node.getAttribute('data-url') || '');
          const safeNew = advSafeHttpUrl(newUrl);
          if (safeNew) {
            const detected = detectEmbed(safeNew);
            // Hanya penyedia tepercaya yang boleh dipasang ke iframe.
            if (detected.type === 'iframe' && advEmbedAllowed(detected.embedUrl)) {
              node.setAttribute('data-url', safeNew);
              node.setAttribute('data-embed-url', detected.embedUrl);
              const iframe = node.querySelector('iframe');
              if (iframe) iframe.src = detected.embedUrl;
            }
          }
        });
      }

      return node;
    }

    static value(node) {
      return {
        url: node.getAttribute('data-url') || '',
        embed_url: node.getAttribute('data-embed-url') || ''
      };
    }

    html() {
      const data = this.value(this.domNode);
      return `<div class="adv-embed" data-url="${advEscapeAttr(data.url)}"></div>`;
    }
  }
  EmbedBlot.blotName = 'embed';
  EmbedBlot.tagName = 'div';
  EmbedBlot.className = 'adv-embed';

  function detectEmbed(url) {
    const ytMatch = url.match(/(?:youtube\.com\/watch\?v=|youtu\.be\/)([a-zA-Z0-9_-]+)/);
    if (ytMatch) {
      return { type: 'iframe', embedUrl: 'https://www.youtube.com/embed/' + ytMatch[1] };
    }

    const vimeoMatch = url.match(/vimeo\.com\/(\d+)/);
    if (vimeoMatch) {
      return { type: 'iframe', embedUrl: 'https://player.vimeo.com/video/' + vimeoMatch[1] };
    }

    const spotifyMatch = url.match(/spotify\.com\/(track|album|playlist)\/([a-zA-Z0-9]+)/);
    if (spotifyMatch) {
      return {
        type: 'iframe',
        embedUrl: `https://open.spotify.com/embed/${spotifyMatch[1]}/${spotifyMatch[2]}`
      };
    }

    // URL lain tidak di-embed (hanya ditampilkan sebagai tautan).
    return { type: 'link', embedUrl: '' };
  }

  // ============================================================
  // 8. COLUMNS BLOT
  // ============================================================
  class ColumnsBlot extends BlockEmbed {
    static create(value) {
      const node = super.create();

      let count = 2;
      if (typeof value === 'object' && value.count) count = value.count;

      node.setAttribute('data-count', count);
      node.setAttribute('contenteditable', 'false');
      node.classList.add('adv-columns');
      node.classList.add('cols-' + count);

      let html = '<div class="columns-wrapper">';
      for (let i = 0; i < count; i++) {
        html += `<div class="column-block" data-col="${i}">
          <div class="column-content" contenteditable="true">Kolom ${i + 1}</div>
        </div>`;
      }
      html += '</div>';

      html += `<div class="columns-controls">
        <button class="col-btn" data-count="2">2 Kolom</button>
        <button class="col-btn" data-count="3">3 Kolom</button>
        <button class="col-btn" data-count="4">4 Kolom</button>
      </div>`;

      node.innerHTML = html;

      node.querySelectorAll('.col-btn').forEach(btn => {
        btn.addEventListener('click', (e) => {
          e.stopPropagation();
          const newCount = parseInt(btn.dataset.count);
          const newNode = ColumnsBlot.create({ count: newCount });
          node.parentNode.replaceChild(newNode, node);
        });
      });

      return node;
    }

    static value(node) {
      return { count: parseInt(node.getAttribute('data-count')) || 2 };
    }

    html() {
      const count = this.value(this.domNode).count;
      const cols = this.domNode.querySelectorAll('.column-content');
      let html = `<div class="adv-columns cols-${count}">`;
      cols.forEach((col, i) => {
        html += `<div class="column-block"><div class="column-content">${col.innerHTML}</div></div>`;
      });
      html += '</div>';
      return html;
    }
  }
  ColumnsBlot.blotName = 'columns';
  ColumnsBlot.tagName = 'div';
  ColumnsBlot.className = 'adv-columns';

  // ============================================================
  // 9. TOC BLOT
  // ============================================================
  class TocBlot extends BlockEmbed {
    static create(value) {
      const node = super.create();

      node.setAttribute('contenteditable', 'false');
      node.classList.add('adv-toc');

      setTimeout(() => renderTOC(node), 100);

      return node;
    }

    static value(node) {
      return true;
    }

    html() {
      return '<div class="adv-toc" data-toc="true"></div>';
    }
  }
  TocBlot.blotName = 'toc';
  TocBlot.tagName = 'div';
  TocBlot.className = 'adv-toc';

  function renderTOC(node) {
    const editor = document.getElementById('editor');
    if (!editor) return;

    const headings = editor.querySelectorAll('h1, h2, h3');
    if (headings.length === 0) {
      node.innerHTML = '<div class="toc-empty">Belum ada heading. Tambahkan H1/H2/H3 untuk melihat daftar isi.</div>';
      return;
    }

    let html = '<div class="toc-header">Daftar Isi</div><ul class="toc-list">';

    headings.forEach((h, i) => {
      const level = parseInt(h.tagName.substring(1));
      const text = h.textContent.trim();
      const id = 'heading-' + i;

      if (!h.id) h.id = id;

      html += `<li class="toc-item toc-level-${level}">
        <a href="#${h.id}" onclick="document.getElementById('${h.id}').scrollIntoView({behavior:'smooth'});return false;">${advEscapeHtml(text)}</a>
      </li>`;
    });

    html += '</ul>';
    node.innerHTML = html;
  }

  function renderTOCNodes() {
    document.querySelectorAll('.adv-toc').forEach(node => renderTOC(node));
  }

  // ============================================================
  // 10. REGISTER SEMUA BLOT
  // ============================================================
  Quill.register(CalloutBlot);
  Quill.register(ToggleBlot);
  Quill.register(MathBlot);
  Quill.register(MermaidBlot);
  Quill.register(FootnoteBlot);
  Quill.register(BookmarkBlot);
  Quill.register(EmbedBlot);
  Quill.register(ColumnsBlot);
  Quill.register(TocBlot);

  // ============================================================
  // 11. CODE HIGHLIGHTING
  // ============================================================
  function highlightCodeBlocks() {
    if (typeof hljs === 'undefined') return;

    const codeBlocks = document.querySelectorAll('.ql-editor pre.ql-syntax');
    codeBlocks.forEach(block => {
      if (block.dataset.highlighted) return;

      let lang = 'plaintext';
      const match = block.className.match(/language-(\w+)/);
      if (match) lang = match[1];

      try {
        const code = block.textContent;
        block.innerHTML = '<code class="hljs">' + advEscapeHtml(code) + '</code>';
        hljs.highlightElement(block.querySelector('code'));
        block.dataset.highlighted = 'true';
      } catch (e) {
        console.warn('Highlight error:', e);
      }
    });
  }

  // ============================================================
  // 12. EXPORT KE WINDOW.ADVANCEDEDITOR
  // ============================================================
  window.AdvancedEditor = {
    insertCallout: function(quill, type) {
      type = type || 'info';
      const range = quill.getSelection(true);
      const defaultTexts = {
        info: 'Informasi penting di sini...',
        warning: 'Perhatian! Ada hal penting...',
        success: 'Berhasil! Selamat...',
        danger: 'Bahaya! Jangan lakukan ini...',
        tip: 'Tips: Coba lakukan ini...',
        note: 'Catatan: Ingat hal ini...'
      };
      quill.insertEmbed(range.index, 'callout', {
        type: type,
        title: CalloutBlot.defaultTitle(type),
        content: defaultTexts[type] || 'Isi callout...'
      });
      quill.setSelection(range.index + 1);
    },

    insertToggle: function(quill) {
      const title = prompt('Judul toggle:', 'Toggle Section');
      if (title === null) return;
      const range = quill.getSelection(true);
      quill.insertEmbed(range.index, 'toggle', {
        title: title || 'Toggle Section',
        content: 'Isi tersembunyi di sini...',
        open: false
      });
      quill.setSelection(range.index + 1);
    },

    insertColumns: function(quill, count) {
      count = count || 2;
      const range = quill.getSelection(true);
      quill.insertEmbed(range.index, 'columns', { count: count });
      quill.setSelection(range.index + 1);
    },

    insertMath: function(quill, display) {
      const formula = prompt('Masukkan formula LaTeX:\n\nContoh: \\frac{a}{b}, x^2, \\sqrt{x}',
        display ? '\\frac{a}{b}' : 'x^2');
      if (!formula) return;
      const range = quill.getSelection(true);
      quill.insertEmbed(range.index, 'math', {
        formula: formula,
        display: display !== false
      });
      quill.setSelection(range.index + 1);
    },

    insertMermaid: function(quill) {
      const code = prompt('Mermaid diagram code:\n\nContoh: graph TD; A-->B;',
        'graph TD;\n  A[Start] --> B{Decision};\n  B -->|Yes| C[OK];\n  B -->|No| D[Cancel];');
      if (!code) return;
      const range = quill.getSelection(true);
      quill.insertEmbed(range.index, 'mermaid', code);
      quill.setSelection(range.index + 1);
    },

    insertFootnote: function(quill) {
      const text = prompt('Isi footnote:');
      if (!text) return;
      const range = quill.getSelection(true);
      quill.insertEmbed(range.index, 'footnote', text);
      quill.setSelection(range.index + 1);
    },

    insertBookmark: function(quill) {
      const url = prompt('URL:');
      if (!url) return;
      const title = prompt('Judul (opsional):', '') || '';
      const description = prompt('Deskripsi (opsional):', '') || '';

      const range = quill.getSelection(true);
      quill.insertEmbed(range.index, 'bookmark', {
        url: url, title: title, description: description
      });
      quill.setSelection(range.index + 1);
    },

    insertEmbed: function(quill) {
      const url = prompt('URL untuk embed (YouTube, Vimeo, Spotify, dll):');
      if (!url) return;
      const range = quill.getSelection(true);
      quill.insertEmbed(range.index, 'embed', { url: url });
      quill.setSelection(range.index + 1);
    },

    insertTOC: function(quill) {
      const range = quill.getSelection(true);
      quill.insertEmbed(range.index, 'toc', true);
      quill.setSelection(range.index + 1);
    },

    refreshTOC: function() {
      renderTOCNodes();
    },

    // Bangun ulang blok turunan (rumus, diagram, daftar isi) setelah catatan dimuat.
    // Server menyimpan hanya sumbernya (data-formula / data-code); hasil render dibuang.
    rehydrate: function(root) {
      root = root || document.getElementById('editor');
      if (!root) return;
      root.querySelectorAll('.adv-math').forEach(n => renderMath(n));
      root.querySelectorAll('.adv-mermaid').forEach((n, i) => {
        const c = n.querySelector('.mermaid-content');
        if (c && !c.id) c.id = 'mermaid-' + Date.now() + '-' + i;
        renderMermaid(n, c ? c.id : '');
      });
      renderTOCNodes();
    },

    highlightCodeBlocks: highlightCodeBlocks,

    onEditorUpdate: function(quill) {
      setTimeout(() => {
        renderTOCNodes();
        highlightCodeBlocks();
      }, 300);
    }
  };

  // ============================================================
  // 13. AUTO-SETUP SETELAH DOM READY
  // ============================================================
  setTimeout(() => {
    highlightCodeBlocks();

    const editor = document.getElementById('editor');
    if (editor) {
      const observer = new MutationObserver(() => {
        highlightCodeBlocks();
      });
      observer.observe(editor, { childList: true, subtree: true });
    }
  }, 500);

  console.log('Advanced Editor blots registered (9 blots)');
})();

// ============================================================
// 13. SEARCH
// ============================================================
let searchTimer = null;
function debouncedSearch() {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    const q = document.getElementById('searchInput').value.trim();
    currentSearchQuery = q;
    const editorMode = document.getElementById('editorMode');
    if (editorMode && editorMode.style.display !== 'none') {
      showListMode();
    }
    loadNotes(currentFilter, q);

    clearTimeout(window._shTimer);
    window._shTimer = setTimeout(() => {
      if (q.length >= 3) {
        saveSearchHistory(q).then(() => loadSearchHistory());
      }
    }, 1200);
  }, 300);
}

async function loadSearchHistory() {
  const history = await api('/search/history');
  searchHistoryCache = history || [];
  return searchHistoryCache;
}

async function saveSearchHistory(keyword) {
  if (!keyword || keyword.length < 2) return;
  await api('/search/history', {
    method: 'POST',
    body: JSON.stringify({ keyword })
  });
}

function showSearchHistoryDropdown() {
  const input = document.getElementById('searchInput');
  if (!input) return;
  const wrapper = input.parentElement;

  const existing = wrapper.querySelector('.search-history-dropdown');
  if (existing) existing.remove();

  if (searchHistoryCache.length === 0) return;

  const dropdown = document.createElement('div');
  dropdown.className = 'search-history-dropdown';

  const header = document.createElement('div');
  header.className = 'search-history-header';
  header.innerHTML = `
    <span>Pencarian terbaru</span>
    <button onclick="clearSearchHistoryUI()">Hapus semua</button>
  `;
  dropdown.appendChild(header);

  searchHistoryCache.slice(0, 10).forEach(h => {
    const item = document.createElement('div');
    item.className = 'search-history-item';
    item.textContent = h.keyword;
    item.onclick = () => {
      input.value = h.keyword;
      hideSearchHistoryDropdown();
      debouncedSearch();
    };
    dropdown.appendChild(item);
  });

  wrapper.appendChild(dropdown);

  setTimeout(() => {
    document.addEventListener('click', outsideSearchClickHandler);
  }, 0);
}

function outsideSearchClickHandler(e) {
  const wrapper = document.querySelector('.search-wrapper');
  if (!wrapper) return;
  if (!wrapper.contains(e.target)) {
    hideSearchHistoryDropdown();
    document.removeEventListener('click', outsideSearchClickHandler);
  }
}

function hideSearchHistoryDropdown() {
  const dd = document.querySelector('.search-history-dropdown');
  if (dd) dd.remove();
  document.removeEventListener('click', outsideSearchClickHandler);
}

async function clearSearchHistoryUI() {
  if (!confirm('Hapus semua riwayat pencarian?')) return;
  await api('/search/history', { method: 'DELETE' });
  searchHistoryCache = [];
  hideSearchHistoryDropdown();
  showToast('Riwayat dihapus.', 'success');
}

function openSearchModal() {
  populateNotebookDropdown();
  populateTagDropdown();
  loadPresets();

  document.getElementById('searchModal').style.display = 'flex';
  document.body.style.overflow = 'hidden';
  setTimeout(() => document.getElementById('advKeyword').focus(), 100);
}

function closeSearchModal() {
  document.getElementById('searchModal').style.display = 'none';
  document.body.style.overflow = '';
}

async function populateNotebookDropdown() {
  const tree = await api('/notebooks');
  const select = document.getElementById('advNotebook');
  select.innerHTML = '<option value="">Semua folder</option>';
  if (!tree) return;

  function walk(items, prefix = '') {
    items.forEach(nb => {
      const opt = document.createElement('option');
      opt.value = nb.id;
      opt.textContent = prefix + nb.name;
      select.appendChild(opt);
      if (nb.children && nb.children.length) walk(nb.children, prefix + '— ');
    });
  }
  walk(tree);
}

async function populateTagDropdown() {
  const tags = await api('/tags');
  const select = document.getElementById('advTag');
  select.innerHTML = '<option value="">Semua tag</option>';
  if (!tags) return;
  tags.forEach(t => {
    const opt = document.createElement('option');
    opt.value = t.id;
    opt.textContent = '#' + t.name;
    select.appendChild(opt);
  });
}

function applyAdvancedSearch() {
  const params = new URLSearchParams();

  const kw = document.getElementById('advKeyword').value.trim();
  if (kw) params.set('q', kw);

  const from = document.getElementById('advDateFrom').value;
  if (from) params.set('date_from', from);

  const to = document.getElementById('advDateTo').value;
  if (to) params.set('date_to', to);

  const nb = document.getElementById('advNotebook').value;
  if (nb) params.set('notebook_id', nb);

  const tag = document.getElementById('advTag').value;
  if (tag) params.set('tag_id', tag);

  const filter = document.getElementById('advFilter').value;
  if (filter) params.set('filter', filter);

  const sort = document.getElementById('advSort').value;
  if (sort) params.set('sort', sort);

  const cl = document.getElementById('advChecklist').value;
  if (cl) params.set('has_checklist', cl);

  const att = document.getElementById('advAttachment').value;
  if (att) params.set('has_attachment', att);

  closeSearchModal();
  searchWithParams(params);
}

function resetAdvancedSearch() {
  ['advKeyword', 'advDateFrom', 'advDateTo'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.value = '';
  });
  ['advNotebook', 'advTag', 'advChecklist', 'advAttachment'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.value = '';
  });
  const f = document.getElementById('advFilter');
  if (f) f.value = 'all';
  const s = document.getElementById('advSort');
  if (s) s.value = 'updated';
}

async function searchWithParams(params) {
  const url = '/search/advanced?' + params.toString();
  const notes = await api(url);
  if (!notes) return;
  renderSearchResults(notes);
  currentFilter = 'advanced';
}

function renderSearchResults(notes) {
  const grid = document.getElementById('notesGrid');
  grid.innerHTML = '';

  document.getElementById('listTitle').textContent = 'Hasil Pencarian';
  document.getElementById('listSubtitle').textContent = `${notes.length} hasil ditemukan`;

  if (notes.length === 0) {
    grid.innerHTML = '<div class="notes-empty">Tidak ada hasil ditemukan</div>';
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

    const preview = stripHtml(n.content || '').substring(0, 160);

    let sharedBadgeHtml = '';
    if (isShared) {
      const icon = isSharedEditor ? '✎' : '◎';
      const label = isSharedEditor ? 'Editor' : 'Viewer';
      sharedBadgeHtml = `
        <div class="shared-badge ${isSharedEditor ? 'editor' : 'viewer'}">
          <span class="shared-badge-icon">${icon}</span>
          <span class="shared-badge-text">Dibagikan oleh @${escapeHtml(n.owner_username || 'user')}</span>
          <span class="shared-badge-perm">${label}</span>
        </div>
      `;
    }

    card.innerHTML = `
      ${sharedBadgeHtml}
      <div class="card-title">${escapeHtml(n.title || 'Tanpa judul')}</div>
      <div class="card-preview">${escapeHtml(preview) || 'Tidak ada isi'}</div>
      <div class="card-footer">
        <span>${formatDate(n.updated_at)}</span>
      </div>
    `;
    card.onclick = () => openNote(n.id);
    grid.appendChild(card);
  });
}

async function loadPresets() {
  const presets = await api('/search/presets');
  const list = document.getElementById('presetList');
  list.innerHTML = '';

  if (!presets || presets.length === 0) {
    list.innerHTML = '<span class="preset-empty">Belum ada preset tersimpan</span>';
    return;
  }

  presets.forEach(p => {
    const el = document.createElement('div');
    el.className = 'preset-item';
    el.innerHTML = `
      <span>${escapeHtml(p.name)}</span>
      <span class="preset-del" onclick="event.stopPropagation(); deletePreset(${p.id})">×</span>
    `;
    el.onclick = () => applyPreset(p.filters);
    list.appendChild(el);
  });
}

function applyPreset(filters) {
  resetAdvancedSearch();
  if (filters.q) document.getElementById('advKeyword').value = filters.q;
  if (filters.date_from) document.getElementById('advDateFrom').value = filters.date_from;
  if (filters.date_to) document.getElementById('advDateTo').value = filters.date_to;
  if (filters.notebook_id) document.getElementById('advNotebook').value = filters.notebook_id;
  if (filters.tag_id) document.getElementById('advTag').value = filters.tag_id;
  if (filters.filter) document.getElementById('advFilter').value = filters.filter;
  if (filters.sort) document.getElementById('advSort').value = filters.sort;
  if (filters.has_checklist) document.getElementById('advChecklist').value = filters.has_checklist;
  if (filters.has_attachment) document.getElementById('advAttachment').value = filters.has_attachment;
  applyAdvancedSearch();
}

function showSavePresetDialog() {
  document.getElementById('presetDialog').style.display = 'flex';
  document.getElementById('presetName').value = '';
  setTimeout(() => document.getElementById('presetName').focus(), 100);
}

function closePresetDialog() {
  document.getElementById('presetDialog').style.display = 'none';
}

async function saveCurrentPreset() {
  const name = document.getElementById('presetName').value.trim();
  if (!name) {
    showToast('Nama preset wajib diisi.', 'error');
    return;
  }

  const filters = {};
  const kw = document.getElementById('advKeyword').value.trim();
  if (kw) filters.q = kw;
  const from = document.getElementById('advDateFrom').value;
  if (from) filters.date_from = from;
  const to = document.getElementById('advDateTo').value;
  if (to) filters.date_to = to;
  const nb = document.getElementById('advNotebook').value;
  if (nb) filters.notebook_id = nb;
  const tag = document.getElementById('advTag').value;
  if (tag) filters.tag_id = tag;
  const filter = document.getElementById('advFilter').value;
  if (filter) filters.filter = filter;
  const sort = document.getElementById('advSort').value;
  if (sort) filters.sort = sort;
  const cl = document.getElementById('advChecklist').value;
  if (cl) filters.has_checklist = cl;
  const att = document.getElementById('advAttachment').value;
  if (att) filters.has_attachment = att;

  const res = await api('/search/presets', {
    method: 'POST',
    body: JSON.stringify({ name, filters })
  });
  if (res && res.id) {
    showToast('Preset disimpan.', 'success');
    closePresetDialog();
    loadPresets();
  }
}

async function deletePreset(id) {
  if (!confirm('Hapus preset ini?')) return;
  await api('/search/presets/' + id, { method: 'DELETE' });
  loadPresets();
}

// ============================================================
// 16. NOTIFICATIONS
// ============================================================
function openNotificationPanel() {
  const panel = document.getElementById('notifPanel');
  panel.style.display = 'flex';
  setTimeout(() => panel.classList.add('show'), 10);
  loadNotifications();
}

function closeNotificationPanel() {
  const panel = document.getElementById('notifPanel');
  panel.classList.remove('show');
  setTimeout(() => panel.style.display = 'none', 250);
}

function openNotificationCenter() {
  openNotificationPanel();
  if (window.innerWidth <= 767) closeSidebar();
}

async function loadNotifications() {
  const notifs = await api('/notifications');
  if (!notifs) return;

  const el = document.getElementById('notifList');
  el.innerHTML = '';

  if (notifs.length === 0) {
    el.innerHTML = '<div class="notif-empty">Tidak ada notifikasi</div>';
    return;
  }

  notifs.forEach(n => {
    const item = document.createElement('div');
    item.className = 'notif-item' + (n.is_read ? '' : ' unread');
    item.innerHTML = `
      <div class="notif-title">${escapeHtml(n.title)}</div>
      <div class="notif-message">${escapeHtml(n.message)}</div>
      <div class="notif-time">${formatDate(n.created_at)}</div>
    `;
    item.onclick = async () => {
      if (!n.is_read) {
        await api('/notifications/' + n.id + '/read', { method: 'PUT' });
        updateNotifBadge();
      }
      if (n.link) {
        closeNotificationPanel();
        handleNotifLink(n.link);
      }
    };
    el.appendChild(item);
  });
}

function handleNotifLink(link) {
  if (link.startsWith('/tasks')) {
    const id = new URLSearchParams(link.split('?')[1]).get('id');
    navigateTo('tasks', {
      callback: () => {
        if (id) setTimeout(() => editTask(id), 500);
      }
    });
  } else if (link.startsWith('/notes')) {
    const id = new URLSearchParams(link.split('?')[1]).get('id');
    if (id) {
      openNote(id);
    }
  }
}

async function updateNotifBadge() {
  const res = await api('/notifications/unread-count');
  if (!res) return;
  const count = res.count || 0;

  ['notifBadge', 'notifBadgeInline'].forEach(id => {
    const el = document.getElementById(id);
    if (!el) return;
    if (count > 0) {
      el.textContent = count > 99 ? '99+' : count;
      el.style.display = 'inline-flex';
    } else {
      el.style.display = 'none';
    }
  });
}

async function markAllNotificationsRead() {
  await api('/notifications/read-all', { method: 'PUT' });
  loadNotifications();
  updateNotifBadge();
  showToast('Semua notifikasi ditandai dibaca.', 'success');
}

async function clearAllNotifications() {
  if (!confirm('Hapus semua notifikasi?')) return;
  await api('/notifications', { method: 'DELETE' });
  loadNotifications();
  updateNotifBadge();
}

function startNotifPolling() {
  if (notifPollInterval) clearInterval(notifPollInterval);
  updateNotifBadge();
  notifPollInterval = setInterval(() => {
    updateNotifBadge();
    const panel = document.getElementById('notifPanel');
    if (panel && panel.classList.contains('show')) {
      loadNotifications();
    }
  }, 60000);
}

function requestNotifPermissionIfNeeded() {
  if (!('Notification' in window)) return;
  if (Notification.permission === 'default') {
    document.getElementById('notifPermissionBanner').style.display = 'flex';
  }
}

async function requestNotifPermission() {
  document.getElementById('notifPermissionBanner').style.display = 'none';
  if (!('Notification' in window)) {
    showToast('Browser tidak mendukung notifikasi.', 'error');
    return;
  }

  const perm = await Notification.requestPermission();
  if (perm === 'granted') {
    showToast('Notifikasi browser aktif.', 'success');
    new Notification('Mindora', {
      body: 'Notifikasi browser aktif. Anda akan diingatkan saat ada tugas jatuh tempo.'
    });
  } else {
    showToast('Notifikasi browser ditolak.', 'error');
  }
}

function dismissNotifPermission() {
  document.getElementById('notifPermissionBanner').style.display = 'none';
  localStorage.setItem('notif_banner_dismissed', 'true');
}

// ============================================================
// 17. TEMPLATES
// ============================================================
async function showTemplatePicker() {
  document.getElementById('tplSearchInput').value = '';
  currentTplCategory = 'all';

  await loadTemplateCategories();
  await loadTemplateList();

  document.getElementById('templateModal').style.display = 'flex';
}

async function loadTemplateCategories() {
  const cats = await api('/templates/categories');
  const el = document.getElementById('tplCategories');
  el.innerHTML = '';

  const allBtn = document.createElement('button');
  allBtn.className = 'tpl-cat active';
  allBtn.textContent = 'Semua';
  allBtn.onclick = () => {
    currentTplCategory = 'all';
    document.querySelectorAll('.tpl-cat').forEach(b => b.classList.remove('active'));
    allBtn.classList.add('active');
    loadTemplateList();
  };
  el.appendChild(allBtn);

  (cats || []).forEach(cat => {
    const btn = document.createElement('button');
    btn.className = 'tpl-cat';
    btn.textContent = cat;
    btn.onclick = () => {
      currentTplCategory = cat;
      document.querySelectorAll('.tpl-cat').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      loadTemplateList();
    };
    el.appendChild(btn);
  });
}

async function loadTemplateList() {
  const params = new URLSearchParams();
  if (currentTplCategory && currentTplCategory !== 'all') {
    params.set('category', currentTplCategory);
  }
  const q = document.getElementById('tplSearchInput').value.trim();
  if (q) params.set('q', q);

  const templates = await api('/templates?' + params.toString());
  if (!templates) return;

  const list = document.getElementById('templateList');
  list.innerHTML = '';

  if (templates.length === 0) {
    list.innerHTML = '<div class="task-empty" style="grid-column:1/-1;">Tidak ada template</div>';
    return;
  }

  templates.forEach(t => {
    const card = document.createElement('div');
    card.className = 'template-card';
    const iconColor = t.icon_color || '#18181b';

    card.innerHTML = `
      <div class="tpl-header">
        <div class="tpl-icon" style="background:${iconColor}">${(t.icon || 'doc').substring(0, 3)}</div>
        <div style="flex:1;min-width:0;">
          <div class="template-name">${escapeHtml(t.name)}</div>
          ${t.is_system ? '<span class="template-badge">Bawaan</span>' : ''}
        </div>
      </div>
      ${t.description ? `<div class="template-desc">${escapeHtml(t.description)}</div>` : ''}
      <div class="tpl-footer">
        <span class="tpl-usage">${t.usage_count || 0}x dipakai</span>
        <div class="tpl-actions">
          ${!t.is_system ? `
            <button onclick="event.stopPropagation(); editTemplate(${t.id})">Edit</button>
            <button class="danger" onclick="event.stopPropagation(); deleteTemplate(${t.id})">Hapus</button>
          ` : ''}
          <button onclick="event.stopPropagation(); duplicateTemplate(${t.id})">Duplikat</button>
        </div>
      </div>
      <button class="tpl-fav-btn ${t.is_favorite ? 'active' : ''}" onclick="event.stopPropagation(); toggleTplFavorite(${t.id}, ${!t.is_favorite})">★</button>
    `;

    card.onclick = () => createFromTemplate(t.id);
    list.appendChild(card);
  });
}

function debouncedTemplateSearch() {
  clearTimeout(tplSearchTimer);
  tplSearchTimer = setTimeout(loadTemplateList, 300);
}

async function toggleTplFavorite(id, makeFav) {
  await api('/templates/' + id, {
    method: 'PUT',
    body: JSON.stringify({ is_favorite: makeFav })
  });
  loadTemplateList();
}

async function editTemplate(id) {
  const templates = await api('/templates');
  const t = templates.find(x => x.id === id);
  if (!t) return;

  document.getElementById('tplName').value = t.name;
  document.getElementById('tplDesc').value = t.description || '';
  document.getElementById('tplCategory').value = t.category || 'general';
  document.getElementById('tplTags').value = t.tags || '';

  document.querySelectorAll('#tplColorPicker .accent-dot').forEach(dot => {
    dot.classList.toggle('active', dot.dataset.color === t.icon_color);
  });

  document.getElementById('saveTemplateModal').dataset.editingId = id;
  document.getElementById('saveTemplateModal').style.display = 'flex';
}

async function deleteTemplate(id) {
  if (!confirm('Hapus template ini?')) return;
  await api('/templates/' + id, { method: 'DELETE' });
  showToast('Template dihapus.', 'success');
  loadTemplateList();
}

async function duplicateTemplate(id) {
  const res = await api('/templates/' + id + '/duplicate', { method: 'POST' });
  if (res && res.id) {
    showToast('Template diduplikat.', 'success');
    loadTemplateList();
  }
}

async function createFromTemplate(templateId) {
  const res = await api('/notes/from-template', {
    method: 'POST',
    body: JSON.stringify({
      template_id: templateId,
      notebook_id: currentNotebookId
    })
  });
  if (res && res.id) {
    closeTemplateModal();
    showToast('Catatan dibuat dari template.', 'success');
    loadNotes(currentFilter);
    openNote(res.id);
  }
}

function closeTemplateModal() {
  document.getElementById('templateModal').style.display = 'none';
}

function openSaveTemplateModal() {
  if (!currentNoteId) {
    showToast('Buka catatan dulu', 'error');
    return;
  }

  const title = document.getElementById('noteTitle').value;
  document.getElementById('tplName').value = title || '';
  document.getElementById('tplDesc').value = '';
  document.getElementById('tplCategory').value = 'general';
  document.getElementById('tplTags').value = '';

  document.querySelectorAll('#tplColorPicker .accent-dot').forEach((dot, i) => {
    dot.classList.toggle('active', i === 0);
  });

  delete document.getElementById('saveTemplateModal').dataset.editingId;

  document.getElementById('saveTemplateModal').style.display = 'flex';
}

function closeSaveTemplateModal() {
  document.getElementById('saveTemplateModal').style.display = 'none';
}

async function saveAsTemplate() {
  const name = document.getElementById('tplName').value.trim();
  if (!name) {
    showToast('Nama template wajib diisi.', 'error');
    return;
  }

  const editingId = document.getElementById('saveTemplateModal').dataset.editingId;
  const activeDot = document.querySelector('#tplColorPicker .accent-dot.active');
  const color = activeDot ? activeDot.dataset.color : '#18181b';

  const payload = {
    name,
    description: document.getElementById('tplDesc').value.trim(),
    category: document.getElementById('tplCategory').value,
    tags: document.getElementById('tplTags').value.trim(),
    icon_color: color
  };

  if (editingId) {
    const res = await api('/templates/' + editingId, {
      method: 'PUT',
      body: JSON.stringify(payload)
    });
    if (res) {
      showToast('Template diperbarui.', 'success');
      closeSaveTemplateModal();
      loadTemplateList();
    }
  } else {
    payload.note_id = currentNoteId;
    payload.icon = 'doc';
    const res = await api('/templates', {
      method: 'POST',
      body: JSON.stringify(payload)
    });
    if (res && res.id) {
      showToast('Template disimpan.', 'success');
      closeSaveTemplateModal();
    }
  }
}

// ============================================================
// 18. EDITOR TABS
// ============================================================
function switchEditorTab(tab) {
  currentEditorTab = tab;

  ['content', 'backlinks', 'comments', 'versions'].forEach(t => {
    const panelId = 'panel' + t.charAt(0).toUpperCase() + t.slice(1);
    const panel = document.getElementById(panelId);
    if (panel) panel.style.display = (t === tab) ? 'flex' : 'none';
  });

  document.querySelectorAll('.editor-tab').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.tab === tab);
  });

  if (tab === 'backlinks' && currentNoteId) loadBacklinks();
  if (tab === 'comments' && currentNoteId) loadComments();
  if (tab === 'versions' && currentNoteId) loadVersions();
}

// ============================================================
// 19. BACKLINKS
// ============================================================
async function loadBacklinks() {
  const panel = document.getElementById('panelBacklinks');
  panel.innerHTML = '<div class="panel-empty">Memuat...</div>';

  const [backlinks, outgoing, related] = await Promise.all([
    api(`/notes/${currentNoteId}/backlinks`),
    api(`/notes/${currentNoteId}/outgoing-links`),
    api(`/notes/${currentNoteId}/related`)
  ]);

  let html = '<div class="backlinks-panel">';

  html += `
    <div class="guide-section">
      <div class="guide-header" onclick="toggleGuide()">
        <h4>Cara Menggunakan</h4>
        <span class="guide-toggle" id="guideToggleIcon">▼</span>
      </div>
      <div class="guide-content" id="guideContent">
        <div class="guide-item">
          <div class="guide-icon">[[</div>
          <div class="guide-text">
            <strong>Link ke catatan lain</strong>
            <p>Ketik <code>[[</code> di editor, lalu pilih judul catatan yang muncul.</p>
          </div>
        </div>
        <div class="guide-item">
          <div class="guide-icon">@</div>
          <div class="guide-text">
            <strong>Mention user</strong>
            <p>Ketik <code>@</code> di editor, lalu pilih username.</p>
          </div>
        </div>
        <div class="guide-item">
          <div class="guide-icon">←</div>
          <div class="guide-text">
            <strong>Backlink</strong>
            <p>Daftar catatan yang link <em>ke</em> catatan ini.</p>
          </div>
        </div>
        <div class="guide-item">
          <div class="guide-icon">→</div>
          <div class="guide-text">
            <strong>Link Keluar</strong>
            <p>Daftar catatan yang di-link <em>dari</em> catatan ini.</p>
          </div>
        </div>
        <div class="guide-item">
          <div class="guide-icon">≈</div>
          <div class="guide-text">
            <strong>Catatan Terkait</strong>
            <p>Catatan lain yang memiliki <strong>tag yang sama</strong>.</p>
          </div>
        </div>
      </div>
    </div>
  `;

  html += '<div class="backlinks-section">';
  html += `<h4>← Backlink (${backlinks ? backlinks.length : 0})</h4>`;
  html += '<p class="section-desc">Catatan yang menyebut catatan ini</p>';

  if (backlinks && backlinks.length > 0) {
    backlinks.forEach(b => {
      html += `
        <div class="backlink-item" onclick="navigateTo('note', {noteId: ${b.source_note_id}})">
          <div class="backlink-icon">←</div>
          <div class="backlink-info">
            <div class="backlink-title">${escapeHtml(b.source_title)}</div>
            <div class="backlink-meta">menyebut catatan ini</div>
          </div>
        </div>
      `;
    });
  } else {
    html += `
      <div class="panel-empty-small">
        Belum ada catatan yang link ke sini.<br>
        <span class="empty-hint">Coba di catatan lain ketik <code>[[Judul Catatan]]</code></span>
      </div>
    `;
  }
  html += '</div>';

  html += '<div class="backlinks-section">';
  html += `<h4>→ Link Keluar (${outgoing ? outgoing.length : 0})</h4>`;
  html += '<p class="section-desc">Catatan yang di-link dari catatan ini</p>';

  if (outgoing && outgoing.length > 0) {
    outgoing.forEach(b => {
      html += `
        <div class="backlink-item" onclick="navigateTo('note', {noteId: ${b.target_note_id}})">
          <div class="backlink-icon">→</div>
          <div class="backlink-info">
            <div class="backlink-title">${escapeHtml(b.target_title)}</div>
            <div class="backlink-meta">dari [[${escapeHtml(b.link_text)}]]</div>
          </div>
        </div>
      `;
    });
  } else {
    html += `
      <div class="panel-empty-small">
        Belum ada link keluar.<br>
        <span class="empty-hint">Ketik <code>[[</code> di editor untuk link ke catatan lain</span>
      </div>
    `;
  }
  html += '</div>';

  html += '<div class="backlinks-section">';
  html += `<h4>≈ Catatan Terkait (${related ? related.length : 0})</h4>`;
  html += '<p class="section-desc">Catatan dengan tag yang sama</p>';

  if (related && related.length > 0) {
    related.forEach(r => {
      html += `
        <div class="backlink-item" onclick="navigateTo('note', {noteId: ${r.id}})">
          <div class="backlink-icon">≈</div>
          <div class="backlink-info">
            <div class="backlink-title">${escapeHtml(r.title)}</div>
            <div class="backlink-meta">${r.shared_tags} tag sama</div>
          </div>
        </div>
      `;
    });
  } else {
    html += `
      <div class="panel-empty-small">
        Belum ada catatan dengan tag yang sama.<br>
        <span class="empty-hint">Tambahkan tag di catatan ini dan catatan lain</span>
      </div>
    `;
  }
  html += '</div>';

  html += '</div>';
  panel.innerHTML = html;

  const guideState = localStorage.getItem('backlink_guide_open');
  if (guideState === 'false') {
    const content = document.getElementById('guideContent');
    const icon = document.getElementById('guideToggleIcon');
    if (content) content.style.display = 'none';
    if (icon) icon.textContent = '▶';
  }
}

function toggleGuide() {
  const content = document.getElementById('guideContent');
  const icon = document.getElementById('guideToggleIcon');
  if (!content || !icon) return;

  const isOpen = content.style.display !== 'none';
  if (isOpen) {
    content.style.display = 'none';
    icon.textContent = '▶';
    localStorage.setItem('backlink_guide_open', 'false');
  } else {
    content.style.display = 'block';
    icon.textContent = '▼';
    localStorage.setItem('backlink_guide_open', 'true');
  }
}

// ============================================================
// 20. COMMENTS
// ============================================================
async function loadComments() {
  const panel = document.getElementById('panelComments');
  const comments = await api(`/notes/${currentNoteId}/comments`);

  let html = '<div class="comments-panel">';

  const access = window._currentNoteAccess || {};
  const canComment = access.isOwner || access.isSharedEditor || access.isSharedViewer;

  if (canComment) {
    html += `
      <div class="comment-composer">
        <textarea id="commentInput" placeholder="Tulis komentar..."></textarea>
        <div class="comment-composer-actions">
          <button class="btn-primary" onclick="submitComment()">Kirim</button>
        </div>
      </div>
    `;
  }

  if (!comments || comments.length === 0) {
    html += '<div class="panel-empty">Belum ada komentar. Jadilah yang pertama!</div>';
  } else {
    comments.forEach(c => {
      html += renderComment(c);
    });
  }

  html += '</div>';
  panel.innerHTML = html;
}

function renderComment(c) {
  const initials = (c.username || '?').charAt(0).toUpperCase();
  const timeStr = formatDate(c.created_at);

  const access = window._currentNoteAccess || {};
  const canManage = access.isOwner;

  let html = `
    <div class="comment-item ${c.is_resolved ? 'comment-resolved' : ''}">
      <div class="comment-header">
        <div class="comment-avatar">${initials}</div>
        <span class="comment-author">${escapeHtml(c.username)}</span>
        <span class="comment-time">${timeStr}</span>
      </div>
      <div class="comment-body">${escapeHtml(c.content)}</div>
      <div class="comment-actions">
        <button onclick="replyToComment(${c.id}, '${escapeHtml(c.username)}')">Balas</button>
        ${canManage ? `
          ${!c.is_resolved
            ? `<button onclick="resolveComment(${c.id}, true)">Tandai selesai</button>`
            : `<button onclick="resolveComment(${c.id}, false)">Buka lagi</button>`}
          <button onclick="deleteComment(${c.id})" style="color:var(--danger);">Hapus</button>
        ` : ''}
      </div>
  `;

  if (c.replies && c.replies.length > 0) {
    html += '<div class="comment-replies">';
    c.replies.forEach(r => {
      html += renderComment(r);
    });
    html += '</div>';
  }

  html += '</div>';
  return html;
}

async function submitComment() {
  const input = document.getElementById('commentInput');
  const content = input.value.trim();
  if (!content) return;

  const res = await api(`/notes/${currentNoteId}/comments`, {
    method: 'POST',
    body: JSON.stringify({ content })
  });
  if (res) {
    showToast('Komentar dikirim.', 'success');
    loadComments();
  }
}

function replyToComment(parentId, username) {
  const input = document.getElementById('commentInput');
  input.value = `@${username} `;
  input.focus();
}

async function resolveComment(id, resolved) {
  await api(`/comments/${id}/resolve`, {
    method: 'PUT',
    body: JSON.stringify({ is_resolved: resolved })
  });
  loadComments();
}

async function deleteComment(id) {
  if (!confirm('Hapus komentar?')) return;
  await api(`/comments/${id}`, { method: 'DELETE' });
  loadComments();
}

// ============================================================
// 21. VERSION HISTORY
// ============================================================
async function loadVersions() {
  const panel = document.getElementById('panelVersions');
  const versions = await api(`/notes/${currentNoteId}/versions`);

  let html = '<div class="versions-panel">';
  html += '<div class="backlinks-section"><h4>Riwayat Versi</h4>';

  if (!versions || versions.length === 0) {
    html += '<div class="panel-empty">Belum ada versi tersimpan. Versi otomatis dibuat saat Anda mengedit.</div>';
  } else {
    versions.forEach(v => {
      const timeStr = formatDate(v.created_at);
      html += `
        <div class="version-item">
          <div class="version-number">v${v.version_number}</div>
          <div class="version-info">
            <div class="version-title">${escapeHtml(v.title || 'Tanpa judul')}</div>
            <div class="version-meta">
              <span>${timeStr}</span>
              <span>${v.word_count} kata</span>
              <span>oleh ${escapeHtml(v.username)}</span>
              ${v.is_auto ? '<span class="version-badge">Auto</span>' : ''}
            </div>
          </div>
          <div class="version-actions">
            <button onclick="previewVersion(${v.id})">Lihat</button>
            <button onclick="restoreVersion(${v.id})">Pulihkan</button>
            <button class="danger" onclick="deleteVersion(${v.id})">Hapus</button>
          </div>
        </div>
      `;
    });
  }
  html += '</div></div>';
  panel.innerHTML = html;
}

async function previewVersion(versionId) {
  const v = await api(`/notes/${currentNoteId}/versions/${versionId}`);
  if (!v) return;

  const modal = document.getElementById('previewModal');
  document.getElementById('previewFilename').textContent = `Versi v${v.version_number}`;
  document.getElementById('previewMeta').textContent = formatDate(v.created_at);

  const body = document.getElementById('previewBody');
  body.innerHTML = '';

  const wrap = document.createElement('div');
  wrap.style.cssText = 'background:var(--bg-panel);color:var(--text);padding:32px 48px;max-width:900px;margin:0 auto;text-align:left;width:100%;height:100%;overflow:auto;';
  wrap.innerHTML = `<h1 style="margin-bottom:16px;">${escapeHtml(v.title || 'Tanpa judul')}</h1>${safeNoteHTML(v.content || '')}`;
  body.appendChild(wrap);

  modal.style.display = 'flex';
  document.body.style.overflow = 'hidden';
}

async function restoreVersion(versionId) {
  if (!confirm('Pulihkan catatan ke versi ini? Versi saat ini akan otomatis disimpan dulu.')) return;
  const res = await api(`/notes/${currentNoteId}/versions/${versionId}/restore`, { method: 'POST' });
  if (res) {
    showToast('Versi dipulihkan.', 'success');
    openNote(currentNoteId);
    loadVersions();
  }
}

async function deleteVersion(versionId) {
  if (!confirm('Hapus versi ini?')) return;
  await api(`/notes/${currentNoteId}/versions/${versionId}`, { method: 'DELETE' });
  loadVersions();
}

// ============================================================
// 22. GRAPH VIEW
// ============================================================
async function openGraphView() {
  const modal = document.getElementById('graphModal');
  modal.style.display = 'flex';
  if (window.innerWidth <= 767) closeSidebar();

  const data = await api('/graph');
  if (!data) return;

  renderGraph(data);
}

function closeGraphView() {
  document.getElementById('graphModal').style.display = 'none';
  if (graphAnimationId) {
    cancelAnimationFrame(graphAnimationId);
    graphAnimationId = null;
  }
}

function renderGraph(data) {
  const canvas = document.getElementById('graphCanvas');
  const ctx = canvas.getContext('2d');

  function resize() {
    canvas.width = canvas.clientWidth * devicePixelRatio;
    canvas.height = canvas.clientHeight * devicePixelRatio;
    ctx.setTransform(devicePixelRatio, 0, 0, devicePixelRatio, 0, 0);
  }
  resize();
  window.addEventListener('resize', resize);

  const nodes = data.nodes.map((n, i) => ({
    ...n,
    x: canvas.clientWidth / 2 + Math.cos(i * 0.5) * 200 + (Math.random() - 0.5) * 100,
    y: canvas.clientHeight / 2 + Math.sin(i * 0.5) * 200 + (Math.random() - 0.5) * 100,
    vx: 0, vy: 0
  }));

  const edges = data.edges.map(e => ({
    source: nodes.find(n => n.id === e.source),
    target: nodes.find(n => n.id === e.target)
  })).filter(e => e.source && e.target);

  let hoveredNode = null;
  let draggingNode = null;
  let dragOffset = { x: 0, y: 0 };

  function simulate() {
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const a = nodes[i], b = nodes[j];
        const dx = b.x - a.x;
        const dy = b.y - a.y;
        const distSq = Math.max(dx * dx + dy * dy, 100);
        const force = 1500 / distSq;
        const angle = Math.atan2(dy, dx);
        a.vx -= Math.cos(angle) * force;
        a.vy -= Math.sin(angle) * force;
        b.vx += Math.cos(angle) * force;
        b.vy += Math.sin(angle) * force;
      }
    }

    edges.forEach(e => {
      const dx = e.target.x - e.source.x;
      const dy = e.target.y - e.source.y;
      const dist = Math.sqrt(dx * dx + dy * dy) || 1;
      const force = (dist - 120) * 0.005;
      const angle = Math.atan2(dy, dx);
      e.source.vx += Math.cos(angle) * force;
      e.source.vy += Math.sin(angle) * force;
      e.target.vx -= Math.cos(angle) * force;
      e.target.vy -= Math.sin(angle) * force;
    });

    nodes.forEach(n => {
      n.vx += (canvas.clientWidth / 2 - n.x) * 0.0005;
      n.vy += (canvas.clientHeight / 2 - n.y) * 0.0005;
    });

    nodes.forEach(n => {
      if (n === draggingNode) return;
      n.vx *= 0.9;
      n.vy *= 0.9;
      n.x += n.vx;
      n.y += n.vy;
      n.x = Math.max(30, Math.min(canvas.clientWidth - 30, n.x));
      n.y = Math.max(30, Math.min(canvas.clientHeight - 30, n.y));
    });
  }

  function draw() {
    ctx.clearRect(0, 0, canvas.clientWidth, canvas.clientHeight);

    const isDark = document.documentElement.getAttribute('data-theme') === 'dark';
    const nodeColor = isDark ? '#e4e4e7' : '#18181b';
    const edgeColor = isDark ? '#3f3f46' : '#e4e4e7';
    const textColor = isDark ? '#a1a1aa' : '#71717a';

    ctx.strokeStyle = edgeColor;
    ctx.lineWidth = 1.2;
    edges.forEach(e => {
      ctx.beginPath();
      ctx.moveTo(e.source.x, e.source.y);
      ctx.lineTo(e.target.x, e.target.y);
      ctx.stroke();
    });

    nodes.forEach(n => {
      const isHovered = hoveredNode === n;
      const radius = isHovered ? 14 : 8;

      ctx.fillStyle = nodeColor;
      ctx.beginPath();
      ctx.arc(n.x, n.y, radius, 0, Math.PI * 2);
      ctx.fill();

      if (isHovered) {
        ctx.fillStyle = textColor;
        ctx.font = '12px Inter, sans-serif';
        ctx.textAlign = 'center';
        const label = n.title.length > 30 ? n.title.substring(0, 30) + '...' : n.title;
        ctx.fillText(label, n.x, n.y - radius - 6);
      }
    });
  }

  function loop() {
    simulate();
    draw();
    graphAnimationId = requestAnimationFrame(loop);
  }
  loop();

  canvas.addEventListener('mousemove', e => {
    const rect = canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;

    if (draggingNode) {
      draggingNode.x = mx - dragOffset.x;
      draggingNode.y = my - dragOffset.y;
      return;
    }

    hoveredNode = nodes.find(n => {
      const dx = n.x - mx;
      const dy = n.y - my;
      return dx * dx + dy * dy < 200;
    });
    canvas.style.cursor = hoveredNode ? 'pointer' : 'default';
  });

  canvas.addEventListener('mousedown', e => {
    const rect = canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;
    const n = nodes.find(n => {
      const dx = n.x - mx;
      const dy = n.y - my;
      return dx * dx + dy * dy < 200;
    });
    if (n) {
      draggingNode = n;
      dragOffset = { x: mx - n.x, y: my - n.y };
    }
  });

  canvas.addEventListener('mouseup', () => { draggingNode = null; });

  canvas.addEventListener('click', () => {
    if (hoveredNode && !draggingNode) {
      const id = hoveredNode.id;
      closeGraphView();
      navigateTo('note', { noteId: id });
    }
  });
}

// ============================================================
// 23. SHARE
// ============================================================
function openShareModal() {
  if (!currentNoteId) {
    showToast('Buka catatan dulu', 'error');
    return;
  }
  document.getElementById('shareModal').style.display = 'flex';
  loadExistingShares();
  switchShareTab('link');
}

function closeShareModal() {
  document.getElementById('shareModal').style.display = 'none';
  selectedShareUser = null;
  document.getElementById('shareUserSearch').value = '';
  document.getElementById('shareUserResults').classList.remove('open');
  document.getElementById('shareUserResults').innerHTML = '';
}

function switchShareTab(tab) {
  currentShareTab = tab;
  document.querySelectorAll('.share-tab').forEach(t => {
    t.classList.toggle('active', t.dataset.tab === tab);
  });
  document.getElementById('shareTabLink').style.display = tab === 'link' ? 'block' : 'none';
  document.getElementById('shareTabUser').style.display = tab === 'user' ? 'block' : 'none';
}

async function createShareLink() {
  const payload = {
    item_type: 'note',
    item_id: currentNoteId,
    share_mode: document.getElementById('shareLinkMode').value,
    permission: document.getElementById('shareLinkPermission').value,
    password: document.getElementById('shareLinkPassword').value
  };

  const expires = document.getElementById('shareLinkExpires').value;
  if (expires) payload.expires_at = expires;

  const res = await api('/shares', {
    method: 'POST',
    body: JSON.stringify(payload)
  });

  if (res && res.token) {
    const url = `${location.origin}/shared/${res.token}`;
    navigator.clipboard.writeText(url).then(() => {
      showToast('Link disalin ke clipboard!', 'success');
    }).catch(() => {
      prompt('Copy link ini:', url);
    });
    loadExistingShares();
  } else if (res) {
    showToast('Share dibuat (private).', 'success');
    loadExistingShares();
  }
}

function debouncedUserSearch() {
  clearTimeout(shareUserTimer);
  shareUserTimer = setTimeout(async () => {
    const q = document.getElementById('shareUserSearch').value.trim();
    const results = document.getElementById('shareUserResults');

    if (!q) {
      results.classList.remove('open');
      return;
    }

    const users = await api('/users/search?q=' + encodeURIComponent(q));
    results.innerHTML = '';

    if (!users || users.length === 0) {
      results.innerHTML = '<div style="padding:10px;font-size:12px;color:var(--text-faint);">User tidak ditemukan</div>';
      results.classList.add('open');
      return;
    }

    users.forEach(u => {
      const el = document.createElement('div');
      el.className = 'share-user-item';
      el.innerHTML = `
        <div class="share-user-avatar">${(u.username || '?').charAt(0).toUpperCase()}</div>
        <div>@${escapeHtml(u.username)}</div>
      `;
      el.onclick = () => {
        selectedShareUser = u;
        document.getElementById('shareUserSearch').value = '@' + u.username;
        results.classList.remove('open');
      };
      results.appendChild(el);
    });
    results.classList.add('open');
  }, 250);
}

async function shareToUser() {
  if (!selectedShareUser) {
    showToast('Pilih user dulu.', 'error');
    return;
  }

  const res = await api('/shares', {
    method: 'POST',
    body: JSON.stringify({
      item_type: 'note',
      item_id: currentNoteId,
      share_mode: 'private',
      permission: document.getElementById('shareUserPermission').value,
      shared_with_user_id: selectedShareUser.id
    })
  });

  if (res) {
    showToast(`Dibagikan ke @${selectedShareUser.username}`, 'success');
    selectedShareUser = null;
    document.getElementById('shareUserSearch').value = '';
    loadExistingShares();
  }
}

async function loadExistingShares() {
  const shares = await api(`/shares?item_type=note&item_id=${currentNoteId}`);
  const el = document.getElementById('shareListExisting');

  if (!shares || shares.length === 0) {
    el.innerHTML = '<div class="panel-empty" style="padding:16px;font-size:12px;">Belum ada share aktif</div>';
    return;
  }

  el.innerHTML = '';
  shares.forEach(s => {
    const div = document.createElement('div');
    div.className = 'share-existing-item';

    let meta = s.share_mode === 'public' ? 'Public link' : `Untuk @${s.shared_with_name || 'user'}`;
    if (s.expires_at) meta += ` · Kadaluarsa ${new Date(s.expires_at).toLocaleDateString('id-ID')}`;

    div.innerHTML = `
      <div class="share-existing-info">
        <div class="share-existing-title">${escapeHtml(s.title || 'Tanpa judul')}</div>
        <div class="share-existing-meta">${meta} · ${s.permission} · ${s.view_count}x dilihat</div>
      </div>
      <div class="share-existing-actions">
        ${s.share_mode === 'public' && s.token
          ? `<button onclick="copyShareLink('${s.token}')">Copy</button>`
          : ''}
        <button class="danger" onclick="revokeShare(${s.id})">Cabut</button>
      </div>
    `;
    el.appendChild(div);
  });
}

function copyShareLink(token) {
  const url = `${location.origin}/shared/${token}`;
  navigator.clipboard.writeText(url);
  showToast('Link disalin.', 'success');
}

async function revokeShare(id) {
  if (!confirm('Cabut share ini?')) return;
  await api(`/shares/${id}`, { method: 'DELETE' });
  showToast('Share dicabut.', 'success');
  loadExistingShares();
}

// ============================================================
// 24. SHARED WITH ME
// ============================================================
async function openSharedWithMe() {
  const shares = await api('/shares/with-me');
  const el = document.getElementById('sharedList');

  if (!shares || shares.length === 0) {
    el.innerHTML = `
      <div class="panel-empty">
        Belum ada catatan yang dibagikan ke Anda<br>
        <span style="font-size:12px;color:var(--text-faint);">
          Minta pemilik catatan untuk membagikan melalui tombol "Bagikan"
        </span>
      </div>
    `;
  } else {
    el.innerHTML = '';

    const info = document.createElement('div');
    info.style.cssText = 'padding:12px 16px;background:var(--bg-muted);border-radius:var(--radius);margin-bottom:12px;font-size:12.5px;color:var(--text-muted);';
    info.innerHTML = `Anda memiliki <strong>${shares.length}</strong> catatan yang dibagikan. Klik untuk membuka.`;
    el.appendChild(info);

    shares.forEach(s => {
      const div = document.createElement('div');
      div.className = 'shared-item';

      const permLabel = s.permission === 'editor'
        ? '<span style="color:var(--success);font-weight:600;">Editor</span>'
        : '<span style="color:var(--info);font-weight:600;">Viewer</span>';

      let expiresHtml = '';
      if (s.expires_at) {
        expiresHtml = `<span>· Kadaluarsa ${new Date(s.expires_at).toLocaleDateString('id-ID')}</span>`;
      }

      div.innerHTML = `
        <div class="shared-title">${escapeHtml(s.title)}</div>
        <div class="shared-meta">
          <span>Dibagikan oleh <strong>@${escapeHtml(s.owner_name)}</strong></span>
          <span>· Akses: ${permLabel}</span>
          <span>· ${formatDate(s.created_at)}</span>
          ${expiresHtml}
        </div>
      `;
      div.onclick = () => {
        closeSharedWithMe();
        if (s.item_type === 'note') {
          openNote(s.item_id);
        }
      };
      el.appendChild(div);
    });
  }

  document.getElementById('sharedModal').style.display = 'flex';
}

function closeSharedWithMe() {
  document.getElementById('sharedModal').style.display = 'none';
}

// ============================================================
// 25. ADMIN PANEL
// ============================================================
async function checkAdminAccess() {
  try {
    const users = await api('/admin/users');
    if (users && Array.isArray(users)) {
      const link = document.getElementById('adminMenuLink');
      if (link) link.style.display = 'block';
    }
  } catch (e) {
    // Silent fail
  }
}

async function openAdminPanel() {
  const users = await api('/admin/users');
  const el = document.getElementById('adminUsersList');

  if (!users || !Array.isArray(users)) {
    showToast('Akses ditolak.', 'error');
    return;
  }

  el.innerHTML = '';
  users.forEach(u => {
    const div = document.createElement('div');
    div.className = 'admin-user-row';
    div.innerHTML = `
      <div class="admin-user-avatar">${(u.username || '?').charAt(0).toUpperCase()}</div>
      <div class="admin-user-info">
        <div class="admin-user-name">${escapeHtml(u.username)}</div>
        <div class="admin-user-email">${escapeHtml(u.email)}</div>
      </div>
      <div class="admin-user-role ${u.role}">${u.role}</div>
      <div class="admin-user-actions">
        <button onclick="changeUserRole(${u.id}, '${u.role}')">Role</button>
        <button onclick="toggleUserActive(${u.id}, ${!u.is_active})">${u.is_active ? 'Disable' : 'Enable'}</button>
        <button onclick="resetUserPassword(${u.id})">Reset PW</button>
        <button class="danger" onclick="deleteUser(${u.id})" style="color:var(--danger);">Hapus</button>
      </div>
    `;
    el.appendChild(div);
  });

  document.getElementById('adminModal').style.display = 'flex';
}

function closeAdminPanel() {
  document.getElementById('adminModal').style.display = 'none';
}

async function changeUserRole(id, currentRole) {
  const newRole = prompt('Role baru (admin/user/viewer):', currentRole);
  if (!newRole || !['admin', 'user', 'viewer'].includes(newRole)) return;
  await api(`/admin/users/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ role: newRole })
  });
  showToast('Role diubah.', 'success');
  openAdminPanel();
}

async function toggleUserActive(id, newState) {
  await api(`/admin/users/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ is_active: newState })
  });
  showToast('Status diubah.', 'success');
  openAdminPanel();
}

async function resetUserPassword(id) {
  const pw = prompt('Password baru:');
  if (!pw) return;
  const r = await api(`/admin/users/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ new_password: pw })
  });
  if (r && r.error) {
    showToast(r.error, 'error', 5000); // mis. password tidak memenuhi aturan
    return;
  }
  showToast('Password direset.', 'success');
}

async function deleteUser(id) {
  if (!confirm('Hapus user ini? Semua catatan akan hilang.')) return;
  await api(`/admin/users/${id}`, { method: 'DELETE' });
  showToast('User dihapus.', 'success');
  openAdminPanel();
}

// ============================================================
// 26. ACTIVITY LOG
// ============================================================
async function openActivityLog() {
  const log = await api('/activity');
  const el = document.getElementById('activityList');

  if (!log || log.length === 0) {
    el.innerHTML = '<div class="panel-empty">Belum ada aktivitas</div>';
  } else {
    el.innerHTML = '';
    log.forEach(item => {
      const div = document.createElement('div');
      div.className = 'activity-item';
      const initials = item.action.substring(0, 2).toUpperCase();
      div.innerHTML = `
        <div class="activity-icon">${initials}</div>
        <div class="activity-info">
          <div class="activity-desc">${escapeHtml(item.description || item.action)}</div>
          <div class="activity-time">${formatDate(item.created_at)}</div>
        </div>
      `;
      el.appendChild(div);
    });
  }

  document.getElementById('activityModal').style.display = 'flex';
  if (window.innerWidth <= 767) closeSidebar();
}

function closeActivityLog() {
  document.getElementById('activityModal').style.display = 'none';
}

// ============================================================
// FASE 6: QUICK CAPTURE & COMMAND PALETTE
// ============================================================
let commandPaletteOpen = false;
let commandItems = [];
let commandSelectedIndex = 0;
let commandFilteredItems = [];

function openCommandPalette() {
  if (commandPaletteOpen) return;
  commandPaletteOpen = true;

  const palette = document.getElementById('commandPalette');
  palette.style.display = 'flex';

  const input = document.getElementById('commandInput');
  input.value = '';
  input.focus();

  loadCommandPaletteItems();

  input.oninput = () => filterCommands();
  input.onkeydown = handleCommandKeydown;
}

function closeCommandPalette() {
  commandPaletteOpen = false;
  const palette = document.getElementById('commandPalette');
  palette.style.display = 'none';
  document.getElementById('commandInput').value = '';
}

async function loadCommandPaletteItems() {
  const actions = [
    { type: 'action', id: 'new-note', icon: _ico('doc'), title: 'Catatan Baru', desc: 'Buat catatan kosong', shortcut: ['Ctrl', 'N'], action: () => createNote() },
    { type: 'action', id: 'quick-note', icon: _ico('bolt'), title: 'Quick Note', desc: 'Catat cepat tanpa buka editor', shortcut: ['Ctrl', 'Shift', 'N'], action: () => openQuickCapture('note') },
    { type: 'action', id: 'quick-task', icon: _ico('check'), title: 'Quick Task', desc: 'Tambah tugas cepat', shortcut: ['Ctrl', 'Shift', 'T'], action: () => openQuickCapture('task') },
    { type: 'action', id: 'quick-reminder', icon: _ico('timer'), title: 'Quick Reminder', desc: 'Buat pengingat cepat', shortcut: ['Ctrl', 'Shift', 'R'], action: () => openQuickCapture('reminder') },
    { type: 'action', id: 'new-folder', icon: _ico('folder'), title: 'Folder Baru', desc: 'Buat folder baru', action: () => createNotebook() },
    { type: 'action', id: 'new-tag', icon: _ico('tag'), title: 'Tag Baru', desc: 'Buat tag baru', action: () => createTag() },
    { type: 'action', id: 'go-tasks', icon: _ico('check'), title: 'Buka Tugas', desc: 'Lihat semua tugas', action: () => navigateTo('tasks') },
    { type: 'action', id: 'go-graph', icon: _ico('graph'), title: 'Graph View', desc: 'Lihat jaringan catatan', action: () => navigateTo('graph') },
    { type: 'action', id: 'go-shared', icon: _ico('users'), title: 'Dibagikan ke Saya', desc: 'Catatan dari user lain', action: () => navigateTo('shared') },
    { type: 'action', id: 'go-activity', icon: _ico('bars'), title: 'Aktivitas', desc: 'Log aktivitas Anda', action: () => navigateTo('activity') },
    { type: 'action', id: 'go-trash', icon: _ico('trash'), title: 'Sampah', desc: 'Catatan yang dihapus', action: () => navigateTo('list', { filter: 'trash' }) },
    { type: 'action', id: 'go-favorites', icon: _ico('star'), title: 'Favorit', desc: 'Catatan favorit', action: () => navigateTo('list', { filter: 'favorites' }) },
    { type: 'action', id: 'go-pinned', icon: _ico('pin'), title: 'Tersemat', desc: 'Catatan yang disematkan', action: () => navigateTo('list', { filter: 'pinned' }) },
    { type: 'action', id: 'go-archived', icon: _ico('box'), title: 'Arsip', desc: 'Catatan yang diarsipkan', action: () => navigateTo('list', { filter: 'archived' }) },
    { type: 'action', id: 'toggle-theme', icon: _ico('theme'), title: 'Ganti Tema', desc: 'Toggle dark/light mode', action: () => toggleTheme() },
    { type: 'action', id: 'shortcuts', icon: _ico('keyboard'), title: 'Keyboard Shortcuts', desc: 'Lihat semua shortcut', shortcut: ['Ctrl', '/'], action: () => openShortcutsModal() },
    { type: 'action', id: 'logout', icon: _ico('logout'), title: 'Keluar', desc: 'Logout dari aplikasi', action: () => logout() },
  ];

  commandItems = [...actions];

  try {
    const recent = await api('/recent');
    if (recent) {
      (recent.notes || []).forEach(n => {
        commandItems.push({
          type: 'note', id: 'note-' + n.id, icon: _ico('doc'),
          title: n.title || 'Tanpa judul',
          desc: 'Catatan · ' + formatDate(n.updated_at),
          action: () => openNote(n.id),
          recent: true
        });
      });
      (recent.tasks || []).forEach(t => {
        commandItems.push({
          type: 'task', id: 'task-' + t.id, icon: '✓',
          title: t.title,
          desc: 'Tugas · ' + (t.status || 'pending'),
          action: () => {
            navigateTo('tasks', { callback: () => setTimeout(() => editTask(t.id), 400) });
          },
          recent: true
        });
      });
    }
  } catch (e) {
    console.warn('Failed to load recent items:', e);
  }

  renderCommandResults(commandItems);
}

function filterCommands() {
  const query = document.getElementById('commandInput').value.trim().toLowerCase();

  if (!query) {
    commandFilteredItems = commandItems;
  } else {
    commandFilteredItems = commandItems.filter(item => {
      const title = (item.title || '').toLowerCase();
      const desc = (item.desc || '').toLowerCase();
      return title.includes(query) || desc.includes(query);
    });
  }

  commandSelectedIndex = 0;
  renderCommandResults(commandFilteredItems);
}

function renderCommandResults(items) {
  const container = document.getElementById('commandResults');
  container.innerHTML = '';

  if (items.length === 0) {
    container.innerHTML = '<div class="command-empty">Tidak ada hasil untuk pencarian ini</div>';
    return;
  }

  const actions = items.filter(i => i.type === 'action');
  const recent = items.filter(i => i.type !== 'action');

  let html = '';

  if (actions.length > 0) {
    html += '<div class="command-group-label">Perintah</div>';
    actions.forEach((item, idx) => {
      html += renderCommandItem(item, idx);
    });
  }

  if (recent.length > 0) {
    html += '<div class="command-group-label">Terbaru</div>';
    recent.forEach((item, idx) => {
      html += renderCommandItem(item, actions.length + idx);
    });
  }

  container.innerHTML = html;

  container.querySelectorAll('.command-item').forEach((el, idx) => {
    el.onclick = () => executeCommandItem(items[idx]);
    el.onmouseenter = () => {
      commandSelectedIndex = idx;
      updateCommandSelection();
    };
  });

  updateCommandSelection();
}

function renderCommandItem(item, idx) {
  const shortcutHtml = (item.shortcut || [])
    .map(k => `<kbd>${k}</kbd>`)
    .join('');

  return `
    <div class="command-item" data-idx="${idx}">
      <div class="command-item-icon ${item.type === 'action' ? 'action' : ''}">${item.icon}</div>
      <div class="command-item-info">
        <div class="command-item-title">
          ${escapeHtml(item.title)}
          ${item.recent ? '<span class="command-recent-badge">recent</span>' : ''}
        </div>
        <div class="command-item-desc">${escapeHtml(item.desc || '')}</div>
      </div>
      ${shortcutHtml ? `<div class="command-item-shortcut">${shortcutHtml}</div>` : ''}
    </div>
  `;
}

function updateCommandSelection() {
  const items = document.querySelectorAll('#commandResults .command-item');
  items.forEach((el, idx) => {
    el.classList.toggle('selected', idx === commandSelectedIndex);
  });

  const selected = items[commandSelectedIndex];
  if (selected) {
    selected.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
  }
}

function handleCommandKeydown(e) {
  const total = commandFilteredItems.length;
  if (total === 0) return;

  if (e.key === 'ArrowDown') {
    e.preventDefault();
    commandSelectedIndex = (commandSelectedIndex + 1) % total;
    updateCommandSelection();
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    commandSelectedIndex = (commandSelectedIndex - 1 + total) % total;
    updateCommandSelection();
  } else if (e.key === 'Enter') {
    e.preventDefault();
    if (commandFilteredItems[commandSelectedIndex]) {
      executeCommandItem(commandFilteredItems[commandSelectedIndex]);
    }
  } else if (e.key === 'Escape') {
    e.preventDefault();
    closeCommandPalette();
  }
}

function executeCommandItem(item) {
  closeCommandPalette();
  setTimeout(() => {
    if (typeof item.action === 'function') item.action();
  }, 100);
}

let currentQuickType = 'note';

function openQuickCapture(type = 'note') {
  currentQuickType = type;
  const modal = document.getElementById('quickCaptureModal');
  modal.style.display = 'flex';

  switchQuickTab(type);
  loadQuickDraft(type);

  setTimeout(() => {
    const focusMap = {
      note: 'quickNoteContent',
      task: 'quickTaskTitle',
      reminder: 'quickReminderTitle'
    };
    const el = document.getElementById(focusMap[type]);
    if (el) el.focus();
  }, 100);
}

function closeQuickCapture() {
  document.getElementById('quickCaptureModal').style.display = 'none';

  document.getElementById('quickNoteTitle').value = '';
  document.getElementById('quickNoteContent').value = '';
  document.getElementById('quickTaskTitle').value = '';
  document.getElementById('quickTaskPriority').value = 'medium';
  document.getElementById('quickTaskDue').value = '';
  document.getElementById('quickReminderTitle').value = '';
  document.getElementById('quickReminderAt').value = '';
}

function switchQuickTab(type) {
  currentQuickType = type;

  document.querySelectorAll('.quick-tab').forEach(t => {
    t.classList.toggle('active', t.dataset.type === type);
  });

  document.getElementById('quickNoteForm').style.display = type === 'note' ? 'block' : 'none';
  document.getElementById('quickTaskForm').style.display = type === 'task' ? 'block' : 'none';
  document.getElementById('quickReminderForm').style.display = type === 'reminder' ? 'block' : 'none';

  const titles = {
    note: 'Quick Note',
    task: '✓ Quick Task',
    reminder: 'Quick Reminder'
  };
  document.getElementById('quickCaptureTitle').textContent = titles[type];

  loadQuickDraft(type);
}

async function loadQuickDraft(type) {
  const draft = await api('/draft?type=' + type);
  if (!draft || draft.empty) return;

  if (type === 'note') {
    const titleEl = document.getElementById('quickNoteTitle');
    const contentEl = document.getElementById('quickNoteContent');
    if (!titleEl.value && !contentEl.value) {
      titleEl.value = draft.title || '';
      contentEl.value = draft.content || '';
    }
  } else if (type === 'task') {
    const titleEl = document.getElementById('quickTaskTitle');
    if (!titleEl.value) {
      titleEl.value = draft.title || draft.content || '';
    }
  } else if (type === 'reminder') {
    const titleEl = document.getElementById('quickReminderTitle');
    if (!titleEl.value) {
      titleEl.value = draft.title || draft.content || '';
    }
  }
}

async function submitQuickCapture() {
  if (currentQuickType === 'note') {
    await submitQuickNote();
  } else if (currentQuickType === 'task') {
    await submitQuickTask();
  } else if (currentQuickType === 'reminder') {
    await submitQuickReminder();
  }
}

async function submitQuickNote() {
  const title = document.getElementById('quickNoteTitle').value.trim();
  const content = document.getElementById('quickNoteContent').value.trim();

  if (!content) {
    showToast('Isi catatan wajib diisi.', 'error');
    return;
  }

  const htmlContent = content
    .split('\n')
    .filter(p => p.trim())
    .map(p => '<p>' + escapeHtml(p) + '</p>')
    .join('');

  const res = await api('/quick/note', {
    method: 'POST',
    body: JSON.stringify({
      title,
      content: htmlContent,
      notebook_id: currentNotebookId
    })
  });

  if (res && res.id) {
    await api('/draft?type=note', { method: 'DELETE' });
    closeQuickCapture();
    showToast('Catatan berhasil dibuat.', 'success');
    loadNotes(currentFilter);
    setTimeout(() => openNote(res.id), 300);
  } else {
    showToast('Gagal membuat catatan.', 'error');
  }
}

async function submitQuickTask() {
  const title = document.getElementById('quickTaskTitle').value.trim();
  const priority = document.getElementById('quickTaskPriority').value;
  const due = document.getElementById('quickTaskDue').value;

  if (!title) {
    showToast('Judul tugas wajib diisi.', 'error');
    return;
  }

  const res = await api('/quick/task', {
    method: 'POST',
    body: JSON.stringify({
      title,
      priority,
      due_date: due || null,
      notebook_id: currentNotebookId
    })
  });

  if (res && res.id) {
    await api('/draft?type=task', { method: 'DELETE' });
    closeQuickCapture();
    showToast('Tugas berhasil dibuat.', 'success');
  } else {
    showToast('Gagal membuat tugas.', 'error');
  }
}

async function submitQuickReminder() {
  const title = document.getElementById('quickReminderTitle').value.trim();
  const reminderAt = document.getElementById('quickReminderAt').value;

  if (!title) {
    showToast('Judul reminder wajib diisi.', 'error');
    return;
  }
  if (!reminderAt) {
    showToast('Waktu reminder wajib diisi.', 'error');
    return;
  }

  const res = await api('/quick/reminder', {
    method: 'POST',
    body: JSON.stringify({
      title,
      reminder_at: reminderAt
    })
  });

  if (res && res.id) {
    await api('/draft?type=reminder', { method: 'DELETE' });
    closeQuickCapture();
    showToast('Pengingat berhasil dibuat.', 'success');
    requestNotifPermissionIfNeeded();
  } else {
    showToast('Gagal membuat reminder.', 'error');
  }
}

async function saveQuickDraft() {
  let title = '';
  let content = '';

  if (currentQuickType === 'note') {
    title = document.getElementById('quickNoteTitle').value;
    content = document.getElementById('quickNoteContent').value;
  } else if (currentQuickType === 'task') {
    title = document.getElementById('quickTaskTitle').value;
    content = title;
  } else if (currentQuickType === 'reminder') {
    title = document.getElementById('quickReminderTitle').value;
    content = title;
  }

  if (!title && !content) {
    showToast('Tidak ada yang bisa disimpan.', 'error');
    return;
  }

  await api('/draft', {
    method: 'POST',
    body: JSON.stringify({
      type: currentQuickType,
      title,
      content
    })
  });

  showToast('Draft tersimpan.', 'success');
}

function openShortcutsModal() {
  document.getElementById('shortcutsModal').style.display = 'flex';
}

function closeShortcutsModal() {
  document.getElementById('shortcutsModal').style.display = 'none';
}

document.addEventListener('keydown', e => {
  if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
    e.preventDefault();
    if (commandPaletteOpen) {
      closeCommandPalette();
    } else {
      openCommandPalette();
    }
    return;
  }

  if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === 'P') {
    e.preventDefault();
    openCommandPalette();
    return;
  }

  if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === ' ') {
    e.preventDefault();
    openQuickCapture('note');
    return;
  }

  if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === 'N') {
    e.preventDefault();
    openQuickCapture('note');
    return;
  }

  if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === 'T') {
    e.preventDefault();
    openQuickCapture('task');
    return;
  }

  if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key === 'R') {
    e.preventDefault();
    openQuickCapture('reminder');
    return;
  }

  if ((e.ctrlKey || e.metaKey) && e.key === '/') {
    e.preventDefault();
    openShortcutsModal();
    return;
  }

  if (e.key === 'Escape') {
    if (commandPaletteOpen) {
      closeCommandPalette();
      return;
    }
    const quickModal = document.getElementById('quickCaptureModal');
    if (quickModal && quickModal.style.display !== 'none') {
      closeQuickCapture();
      return;
    }
    const shortcutsModal = document.getElementById('shortcutsModal');
    if (shortcutsModal && shortcutsModal.style.display !== 'none') {
      closeShortcutsModal();
      return;
    }
  }

  if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
    const quickModal = document.getElementById('quickCaptureModal');
    if (quickModal && quickModal.style.display !== 'none') {
      e.preventDefault();
      submitQuickCapture();
    }
  }
});

setInterval(() => {
  const quickModal = document.getElementById('quickCaptureModal');
  if (quickModal && quickModal.style.display !== 'none') {
    let hasContent = false;

    if (currentQuickType === 'note') {
      const content = document.getElementById('quickNoteContent').value.trim();
      hasContent = content.length > 0;
    } else if (currentQuickType === 'task') {
      hasContent = document.getElementById('quickTaskTitle').value.trim().length > 0;
    } else if (currentQuickType === 'reminder') {
      hasContent = document.getElementById('quickReminderTitle').value.trim().length > 0;
    }

    if (hasContent) {
      saveQuickDraft();
    }
  }
}, 30000);

// ============================================================
// FASE 10: DASHBOARD
// ============================================================
async function openDashboard() {
  showMode('dashboardMode');
  setMobileTitle('Dashboard');
  setupEditorBackBtn('.dashboard-header .editor-back');
  if (window.innerWidth <= 767) closeSidebar();

  await loadDashboard();
}

function closeDashboard() {
  navigateTo('list', { filter: currentFilter });
}

async function loadDashboard() {
  const [stats, chartData, topData] = await Promise.all([
    api('/dashboard/stats'),
    api('/dashboard/chart?days=30'),
    api('/dashboard/top')
  ]);

  if (!stats) return;

  const el = document.getElementById('dashboardContent');
  el.innerHTML = '';

  const cards = [
    {
      icon: '<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5M9 13h6M9 17h6"/></svg>', label: 'Total Catatan',
      value: stats.notes.total,
      sub: `${stats.notes.active} aktif · ${stats.notes.trashed} di sampah`,
      accent: 'accent-info'
    },
    {
      icon: '<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="m8 12 3 3 5-6"/></svg>', label: 'Total Tugas',
      value: stats.tasks.total,
      sub: `${stats.tasks.completion_rate}% sudah selesai`,
      accent: 'accent-success'
    },
    {
      icon: '<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="13" r="8"/><path d="M12 9v4l2.5 2M9.5 2.5h5"/></svg>', label: 'Pomodoro Hari Ini',
      value: stats.productivity.pomodoro_today,
      sub: `Minggu ini: ${stats.productivity.pomodoro_week}`,
      accent: 'accent-warning'
    },
    {
      icon: '<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v6c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6"/></svg>', label: 'Penyimpanan',
      value: formatBytes(stats.storage.used),
      sub: `${stats.storage.percent}% dari ${formatBytes(stats.storage.quota)}`,
      accent: 'accent-info'
    },
    {
      icon: '<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 20V10M12 20V4M19 20v-7"/></svg>', label: 'Kebiasaan Aktif',
      value: stats.productivity.active_habits,
      sub: 'Trek kebiasaan harian',
      accent: 'accent-success'
    },
    {
      icon: '<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="5"/><circle cx="12" cy="12" r="1.2"/></svg>', label: 'Target Aktif',
      value: stats.productivity.active_goals,
      sub: `${stats.productivity.completed_goals} tercapai`,
      accent: 'accent-warning'
    },
    {
      icon: '<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M9 3h6l-1 6 3 3H7l3-3zM12 12v9"/></svg>', label: 'Tersemat',
      value: stats.notes.pinned,
      sub: `${stats.notes.favorites} favorit`,
      accent: 'accent-info'
    },
    {
      icon: '<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3 2.5 20h19z"/><path d="M12 10v4.5M12 17.5v.2"/></svg>', label: 'Tugas Terlambat',
      value: stats.tasks.overdue,
      sub: `${stats.tasks.today} jatuh tempo hari ini`,
      accent: 'accent-danger'
    }
  ];

  let cardsHtml = '<div class="dashboard-grid">';
  cards.forEach(c => {
    cardsHtml += `
      <div class="dash-card ${c.accent}">
        <div class="dash-card-icon">${c.icon}</div>
        <div class="dash-card-label">${c.label}</div>
        <div class="dash-card-value">${c.value}</div>
        <div class="dash-card-sub">${c.sub}</div>
      </div>
    `;
  });
  cardsHtml += '</div>';
  el.innerHTML = cardsHtml;

  if (chartData && chartData.length > 0) {
    const chartSection = document.createElement('div');
    chartSection.className = 'chart-section';

    const maxVal = Math.max(1, ...chartData.map(d =>
      d.notes_created + d.tasks_completed + d.pomodoro_count));

    let barsHtml = '<div class="bar-chart">';
    chartData.forEach(d => {
      const noteH = (d.notes_created / maxVal) * 100;
      const taskH = (d.tasks_completed / maxVal) * 100;
      const pomoH = (d.pomodoro_count / maxVal) * 100;
      const dateLabel = new Date(d.date).toLocaleDateString('id-ID', { day: 'numeric', month: 'short' });

      barsHtml += `
        <div class="bar-item">
          <div class="bar-tooltip">
            <strong>${dateLabel}</strong><br>
            ${d.notes_created} catatan<br>
            ${d.tasks_completed} tugas<br>
            ${d.pomodoro_count} pomodoro
          </div>
          <div class="bar-stack">
            <div class="bar-segment pomodoro" style="height:${pomoH}%"></div>
            <div class="bar-segment tasks" style="height:${taskH}%"></div>
            <div class="bar-segment notes" style="height:${noteH}%"></div>
          </div>
        </div>
      `;
    });
    barsHtml += '</div>';

    chartSection.innerHTML = `
      <h3>Aktivitas ${chartData.length} Hari Terakhir</h3>
      ${barsHtml}
      <div class="chart-legend">
        <div class="chart-legend-item"><span class="chart-legend-dot" data-series="notes"></span> Catatan dibuat</div>
        <div class="chart-legend-item"><span class="chart-legend-dot" data-series="tasks"></span> Tugas selesai</div>
        <div class="chart-legend-item"><span class="chart-legend-dot" data-series="pomodoro"></span> Pomodoro</div>
      </div>
    `;
    el.appendChild(chartSection);
  }

  if (topData) {
    const topSection = document.createElement('div');
    topSection.className = 'chart-section';

    let tagsHtml = '';
    if (topData.top_tags && topData.top_tags.length > 0) {
      tagsHtml = '<h3 style="margin-bottom:12px;">Tag Teratas</h3><div class="top-list" style="margin-bottom:24px;">';
      topData.top_tags.forEach(t => {
        tagsHtml += `
          <div class="top-item" onclick="navigateTo('list', {tagId: ${t.id}})">
            <span class="top-item-name">#${escapeHtml(t.name)}</span>
            <span class="top-item-count">${t.usage_count}</span>
          </div>
        `;
      });
      tagsHtml += '</div>';
    }

    let foldersHtml = '';
    if (topData.top_folders && topData.top_folders.length > 0) {
      foldersHtml = '<h3 style="margin-bottom:12px;">Folder Teratas</h3><div class="top-list" style="margin-bottom:24px;">';
      topData.top_folders.forEach(f => {
        foldersHtml += `
          <div class="top-item" onclick="navigateTo('list', {notebookId: ${f.id}})">
            <span class="top-item-name">${escapeHtml(f.name)}</span>
            <span class="top-item-count">${f.note_count}</span>
          </div>
        `;
      });
      foldersHtml += '</div>';
    }

    let notesHtml = '';
    if (topData.recent_notes && topData.recent_notes.length > 0) {
      notesHtml = '<h3 style="margin-bottom:12px;">Catatan Terbaru</h3><div class="top-list">';
      topData.recent_notes.forEach(n => {
        notesHtml += `
          <div class="top-item" onclick="openNote(${n.id})">
            <span class="top-item-name">${escapeHtml(n.title)}</span>
            <span class="top-item-count">${n.word_count} kata</span>
          </div>
        `;
      });
      notesHtml += '</div>';
    }

    topSection.innerHTML = tagsHtml + foldersHtml + notesHtml;
    el.appendChild(topSection);
  }
}

function exportNotesDialog() {
  document.getElementById('exportFormat').value = 'json';
  document.getElementById('exportFilter').value = 'all';
  document.getElementById('exportNotesModal').style.display = 'flex';
}

function closeExportNotesModal() {
  document.getElementById('exportNotesModal').style.display = 'none';
}

async function doExportNotes() {
  const format = document.getElementById('exportFormat').value;
  const filter = document.getElementById('exportFilter').value;

  const url = `/api/export/notes?format=${format}&filter=${filter}`;

  try {
    const res = await fetch(url, {
      headers: { 'Authorization': 'Bearer ' + token }
    });
    if (!res.ok) throw new Error('Export gagal');

    const blob = await res.blob();
    const objectUrl = URL.createObjectURL(blob);

    const a = document.createElement('a');
    a.href = objectUrl;
    a.download = `notes-${Date.now()}.${format}`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(objectUrl);

    closeExportNotesModal();
    showToast('Export berhasil.', 'success');
  } catch (e) {
    showToast('Gagal export: ' + e.message, 'error');
  }
}

// ============================================================
// PRODUCTIVITY
// ============================================================
async function openProductivity() {
  showMode('productivityMode');
  setMobileTitle('Productivity');
  setupEditorBackBtn('.productivity-header .editor-back');
  if (window.innerWidth <= 767) closeSidebar();

  await switchProductivityTab(currentProductivityTab);
}

function closeProductivity() {
  navigateTo('list', { filter: currentFilter });
}

function switchProductivityTab(tab) {
  currentProductivityTab = tab;

  document.querySelectorAll('.prod-tab').forEach(t => {
    t.classList.toggle('active', t.dataset.tab === tab);
  });

  ['pomodoro', 'planner', 'habits', 'goals'].forEach(t => {
    const panel = document.getElementById('prodPanel' + t.charAt(0).toUpperCase() + t.slice(1));
    if (panel) panel.style.display = (t === tab) ? 'block' : 'none';
  });

  if (tab === 'pomodoro') loadPomodoroPanel();
  if (tab === 'planner') loadPlannerPanel();
  if (tab === 'habits') loadHabitsPanel();
  if (tab === 'goals') loadGoalsPanel();
}

let pomodoroState = {
  isRunning: false,
  totalSeconds: 25 * 60,
  remainingSeconds: 25 * 60,
  mode: 'focus',
  sessionId: null,
  timerInterval: null
};

async function loadPomodoroPanel() {
  const panel = document.getElementById('prodPanelPomodoro');
  const stats = await api('/pomodoro/stats');

  panel.innerHTML = `
    <div class="pomodoro-container">
      <div class="pomodoro-timer" id="pomodoroTimer">25:00</div>
      <div class="pomodoro-status" id="pomodoroStatus">Fokus</div>
      <div class="pomodoro-progress">
        <div class="pomodoro-progress-fill" id="pomodoroProgress" style="width:0%"></div>
      </div>

      <div class="pomodoro-preset">
        <button data-preset="25" class="active" onclick="setPomodoroPreset(25, 'focus')">Fokus 25m</button>
        <button data-preset="5" onclick="setPomodoroPreset(5, 'short_break')">Break 5m</button>
        <button data-preset="15" onclick="setPomodoroPreset(15, 'long_break')">Long Break 15m</button>
      </div>

      <div class="pomodoro-controls">
        <button class="btn-primary" id="pomodoroStartBtn" onclick="togglePomodoro()">Mulai</button>
        <button class="btn-ghost" onclick="resetPomodoro()">Reset</button>
      </div>

      <div class="pomodoro-stats">
        <div class="pomodoro-stat">
          <div class="pomodoro-stat-value" id="pomoToday">${stats ? stats.today_sessions : 0}</div>
          <div class="pomodoro-stat-label">Hari ini</div>
        </div>
        <div class="pomodoro-stat">
          <div class="pomodoro-stat-value" id="pomoWeek">${stats ? stats.week_sessions : 0}</div>
          <div class="pomodoro-stat-label">Minggu ini</div>
        </div>
        <div class="pomodoro-stat">
          <div class="pomodoro-stat-value" id="pomoTotal">${stats ? stats.total_sessions : 0}</div>
          <div class="pomodoro-stat-label">Total</div>
        </div>
      </div>
    </div>
  `;

  updatePomodoroDisplay();
}

function setPomodoroPreset(minutes, mode) {
  if (pomodoroState.isRunning) {
    if (!confirm('Timer sedang berjalan. Hentikan?')) return;
    stopPomodoroTimer();
  }

  pomodoroState.mode = mode;
  pomodoroState.totalSeconds = minutes * 60;
  pomodoroState.remainingSeconds = minutes * 60;

  document.querySelectorAll('.pomodoro-preset button').forEach(b => {
    b.classList.toggle('active', parseInt(b.dataset.preset) === minutes);
  });

  const statusMap = {
    focus: 'Fokus', short_break: 'Istirahat Pendek', long_break: 'Istirahat Panjang'
  };
  document.getElementById('pomodoroStatus').textContent = statusMap[mode];

  updatePomodoroDisplay();
}

function togglePomodoro() {
  if (pomodoroState.isRunning) {
    pausePomodoro();
  } else {
    startPomodoro();
  }
}

async function startPomodoro() {
  const res = await api('/pomodoro/start', {
    method: 'POST',
    body: JSON.stringify({
      duration: Math.floor(pomodoroState.totalSeconds / 60),
      type: pomodoroState.mode
    })
  });

  if (res && res.id) {
    pomodoroState.sessionId = res.id;
  }

  pomodoroState.isRunning = true;
  document.getElementById('pomodoroStartBtn').textContent = 'Pause';
  document.getElementById('pomodoroStartBtn').classList.remove('btn-primary');
  document.getElementById('pomodoroStartBtn').classList.add('btn-ghost');
  document.getElementById('pomodoroTimer').classList.add('running');

  pomodoroState.timerInterval = setInterval(() => {
    pomodoroState.remainingSeconds--;

    if (pomodoroState.remainingSeconds <= 0) {
      completePomodoro();
    }

    updatePomodoroDisplay();
  }, 1000);
}

function pausePomodoro() {
  pomodoroState.isRunning = false;
  clearInterval(pomodoroState.timerInterval);
  document.getElementById('pomodoroStartBtn').textContent = 'Lanjut';
  document.getElementById('pomodoroStartBtn').classList.remove('btn-ghost');
  document.getElementById('pomodoroStartBtn').classList.add('btn-primary');
  document.getElementById('pomodoroTimer').classList.remove('running');
}

function stopPomodoroTimer() {
  pomodoroState.isRunning = false;
  clearInterval(pomodoroState.timerInterval);
}

async function completePomodoro() {
  stopPomodoroTimer();

  if (pomodoroState.sessionId) {
    await api('/pomodoro/' + pomodoroState.sessionId + '/complete', { method: 'POST' });
  }

  try {
    const ctx = new (window.AudioContext || window.webkitAudioContext)();
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.connect(gain);
    gain.connect(ctx.destination);
    osc.frequency.value = 800;
    gain.gain.value = 0.3;
    osc.start();
    setTimeout(() => osc.stop(), 200);
  } catch (e) {}

  if ('Notification' in window && Notification.permission === 'granted') {
    new Notification('Pomodoro Selesai!', {
      body: pomodoroState.mode === 'focus' ? 'Waktunya istirahat.' : 'Waktunya fokus lagi.',
    });
  } else {
    showToast('Pomodoro selesai!', 'success');
  }

  pomodoroState.remainingSeconds = pomodoroState.totalSeconds;
  document.getElementById('pomodoroTimer').classList.remove('running');
  document.getElementById('pomodoroStartBtn').textContent = 'Mulai';
  document.getElementById('pomodoroStartBtn').classList.remove('btn-ghost');
  document.getElementById('pomodoroStartBtn').classList.add('btn-primary');

  const stats = await api('/pomodoro/stats');
  if (stats) {
    const t = document.getElementById('pomoToday');
    const w = document.getElementById('pomoWeek');
    const tot = document.getElementById('pomoTotal');
    if (t) t.textContent = stats.today_sessions;
    if (w) w.textContent = stats.week_sessions;
    if (tot) tot.textContent = stats.total_sessions;
  }
}

function resetPomodoro() {
  stopPomodoroTimer();
  pomodoroState.remainingSeconds = pomodoroState.totalSeconds;
  document.getElementById('pomodoroStartBtn').textContent = 'Mulai';
  document.getElementById('pomodoroStartBtn').classList.remove('btn-ghost');
  document.getElementById('pomodoroStartBtn').classList.add('btn-primary');
  document.getElementById('pomodoroTimer').classList.remove('running');
  updatePomodoroDisplay();
}

function updatePomodoroDisplay() {
  const timer = document.getElementById('pomodoroTimer');
  const progress = document.getElementById('pomodoroProgress');
  if (!timer) return;

  const minutes = Math.floor(pomodoroState.remainingSeconds / 60);
  const seconds = pomodoroState.remainingSeconds % 60;
  timer.textContent = `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;

  if (progress) {
    const percent = ((pomodoroState.totalSeconds - pomodoroState.remainingSeconds) /
                     pomodoroState.totalSeconds) * 100;
    progress.style.width = percent + '%';
  }
}

// ============================================================
// DAILY PLANNER
// ============================================================
let plannerCurrentDate = new Date().toISOString().split('T')[0];

async function loadPlannerPanel() {
  const panel = document.getElementById('prodPanelPlanner');
  const data = await api('/daily-plan?date=' + plannerCurrentDate);

  const dateObj = new Date(plannerCurrentDate);
  const dayName = dateObj.toLocaleDateString('id-ID', { weekday: 'long' });
  const dateStr = dateObj.toLocaleDateString('id-ID', { day: 'numeric', month: 'long', year: 'numeric' });

  const moodEmojis = ['1', '2', '3', '4', '5'];
  const energyEmojis = ['1', '2', '3', '4', '5'];

  let moodHtml = '';
  for (let i = 1; i <= 5; i++) {
    moodHtml += `<button class="mood-btn ${data.mood === i ? 'active' : ''}" 
                        onclick="setPlannerMood('mood', ${i})">${moodEmojis[i-1]}</button>`;
  }

  let energyHtml = '';
  for (let i = 1; i <= 5; i++) {
    energyHtml += `<button class="mood-btn ${data.energy === i ? 'active' : ''}" 
                          onclick="setPlannerMood('energy', ${i})">${energyEmojis[i-1]}</button>`;
  }

  let tasksHtml = '';
  if (data.tasks && data.tasks.length > 0) {
    data.tasks.forEach(t => {
      const done = t.status === 'done';
      tasksHtml += `
        <div class="planner-task" style="${done ? 'opacity:0.5;' : ''}">
          <span style="color:${done ? 'var(--success)' : 'var(--text-muted)'};">${done ? '✓' : '○'}</span>
          <span style="flex:1;${done ? 'text-decoration:line-through;' : ''}">${escapeHtml(t.title)}</span>
        </div>
      `;
    });
  } else {
    tasksHtml = '<div style="padding:12px;text-align:center;color:var(--text-faint);font-size:12.5px;">Belum ada tugas hari ini</div>';
  }

  panel.innerHTML = `
    <div class="planner-container">
      <div class="planner-date-bar">
        <div>
          <h3>${dayName}</h3>
          <p>${dateStr}</p>
        </div>
        <input type="date" id="plannerDateInput" value="${plannerCurrentDate}" 
               onchange="plannerChangeDate(this.value)"
               style="padding:8px 12px;background:var(--bg-app);border:1px solid var(--border);border-radius:var(--radius);color:var(--text-strong);">
      </div>

      <div class="planner-section">
        <h4>Fokus Utama Hari Ini</h4>
        <input class="planner-focus-input" id="plannerFocus" 
               placeholder="Apa 1 hal terpenting hari ini?"
               value="${escapeHtml(data.focus || '')}"
               onblur="autoSavePlanner()">
      </div>

      <div class="planner-section">
        <h4>Mood & Energy</h4>
        <div class="planner-mood-row">
          <div class="planner-mood-item">
            <label>Mood</label>
            <div class="mood-scale">${moodHtml}</div>
          </div>
          <div class="planner-mood-item">
            <label>Energy</label>
            <div class="mood-scale">${energyHtml}</div>
          </div>
        </div>
      </div>

      <div class="planner-section">
        <h4>Tugas Hari Ini (${data.tasks ? data.tasks.length : 0})</h4>
        <div class="planner-task-list">${tasksHtml}</div>
      </div>

      <div class="planner-section">
        <h4>Catatan & Review</h4>
        <textarea id="plannerNotes" placeholder="Catatan, refleksi, review hari ini..."
                  style="width:100%;padding:12px 14px;background:var(--bg-app);border:1px solid var(--border);border-radius:var(--radius);font-family:inherit;font-size:14px;min-height:120px;resize:vertical;color:var(--text);"
                  onblur="autoSavePlanner()">${escapeHtml(data.notes || '')}</textarea>
      </div>
    </div>
  `;

  window._plannerState = {
    mood: data.mood || 3,
    energy: data.energy || 3
  };
}

function plannerChangeDate(date) {
  plannerCurrentDate = date;
  loadPlannerPanel();
}

function setPlannerMood(type, value) {
  window._plannerState[type] = value;

  const parent = event.target.parentElement;
  parent.querySelectorAll('.mood-btn').forEach(b => b.classList.remove('active'));
  event.target.classList.add('active');

  autoSavePlanner();
}

let plannerSaveTimer = null;
function autoSavePlanner() {
  clearTimeout(plannerSaveTimer);
  plannerSaveTimer = setTimeout(async () => {
    const focus = document.getElementById('plannerFocus')?.value || '';
    const notes = document.getElementById('plannerNotes')?.value || '';

    await api('/daily-plan', {
      method: 'POST',
      body: JSON.stringify({
        date: plannerCurrentDate,
        focus: focus,
        notes: notes,
        mood: window._plannerState?.mood || 3,
        energy: window._plannerState?.energy || 3
      })
    });

    showToast('Plan tersimpan.', 'success', 1200);
  }, 1500);
}

// ============================================================
// HABITS
// ============================================================
async function loadHabitsPanel() {
  const panel = document.getElementById('prodPanelHabits');
  const habits = await api('/habits');

  let habitsHtml = '';
  if (!habits || habits.length === 0) {
    habitsHtml = `
      <div class="panel-empty" style="padding:60px 24px;">
        Belum ada habit.<br>
        <button class="btn-primary" style="margin-top:12px;" onclick="createHabitDialog()">+ Habit Baru</button>
      </div>
    `;
  } else {
    habits.forEach(h => {
      const done = h.today_value > 0;

      habitsHtml += `
        <div class="habit-item">
          <button class="habit-check ${done ? 'done' : ''}" 
                  onclick="toggleHabit(${h.id}, ${done ? 0 : 1})">
            ${done ? '✓' : ''}
          </button>
          <div class="habit-info">
            <div class="habit-name">${escapeHtml(h.name)}</div>
            ${h.description ? `<div class="habit-desc">${escapeHtml(h.description)}</div>` : ''}
          </div>
          <div class="habit-streak">${h.week_count}/${h.target_per_week}</div>
          <button class="btn-ghost-sm" onclick="deleteHabit(${h.id})" style="color:var(--danger);">✕</button>
        </div>
      `;
    });
  }

  panel.innerHTML = `
    <div class="habits-container">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:20px;">
        <h3 style="font-size:16px;font-weight:700;color:var(--text-strong);">Habits Tracker</h3>
        <button class="btn-primary" onclick="createHabitDialog()">+ Habit Baru</button>
      </div>
      ${habitsHtml}
    </div>
  `;
}

async function createHabitDialog() {
  const name = prompt('Nama habit:');
  if (!name) return;

  const description = prompt('Deskripsi (opsional):', '') || '';

  await api('/habits', {
    method: 'POST',
    body: JSON.stringify({
      name,
      description,
      icon: 'star',
      color: '#6366f1',
      frequency: 'daily',
      target_per_week: 7
    })
  });

  showToast('Habit dibuat.', 'success');
  loadHabitsPanel();
}

async function toggleHabit(id, value) {
  await api('/habits/' + id + '/toggle', {
    method: 'POST',
    body: JSON.stringify({ value })
  });
  loadHabitsPanel();
}

async function deleteHabit(id) {
  if (!confirm('Hapus habit ini?')) return;
  await api('/habits/' + id, { method: 'DELETE' });
  showToast('Habit dihapus.', 'success');
  loadHabitsPanel();
}

// ============================================================
// GOALS
// ============================================================
async function loadGoalsPanel() {
  const panel = document.getElementById('prodPanelGoals');
  const goals = await api('/goals');

  let goalsHtml = '';
  if (!goals || goals.length === 0) {
    goalsHtml = `
      <div class="panel-empty" style="padding:60px 24px;">
        Belum ada goal.<br>
        <button class="btn-primary" style="margin-top:12px;" onclick="createGoalDialog()">+ Goal Baru</button>
      </div>
    `;
  } else {
    goals.forEach(g => {
      const statusLabel = {
        active: 'Active', completed: 'Completed',
        paused: 'Paused', cancelled: 'Cancelled'
      }[g.status] || g.status;

      goalsHtml += `
        <div class="goal-item">
          <div class="goal-header">
            <div style="flex:1;min-width:0;">
              <div class="goal-title">${escapeHtml(g.title)}</div>
              ${g.category ? `<div class="goal-category">${escapeHtml(g.category)}</div>` : ''}
            </div>
            <span class="goal-status-badge ${g.status}">${statusLabel}</span>
          </div>
          <div class="goal-progress-bar">
            <div class="goal-progress-fill" style="width:${g.progress}%"></div>
          </div>
          <div class="goal-progress-text">
            <span>${g.current_value} / ${g.target_value} ${g.unit}</span>
            <span>${g.progress}%</span>
          </div>
          <div style="display:flex;gap:8px;margin-top:12px;">
            <button class="btn-ghost-sm" onclick="updateGoalProgress(${g.id}, ${g.current_value}, ${g.target_value})">Update</button>
            <button class="btn-ghost-sm" onclick="deleteGoal(${g.id})" style="color:var(--danger);">Hapus</button>
          </div>
        </div>
      `;
    });
  }

  panel.innerHTML = `
    <div class="goals-container">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:20px;">
        <h3 style="font-size:16px;font-weight:700;color:var(--text-strong);">Goals Tracker</h3>
        <button class="btn-primary" onclick="createGoalDialog()">+ Goal Baru</button>
      </div>
      ${goalsHtml}
    </div>
  `;
}

async function createGoalDialog() {
  const title = prompt('Judul goal:');
  if (!title) return;

  const targetStr = prompt('Target value:', '100');
  if (!targetStr) return;

  const category = prompt('Kategori (opsional):', '') || '';

  await api('/goals', {
    method: 'POST',
    body: JSON.stringify({
      title,
      target_value: parseInt(targetStr),
      unit: '%',
      category,
      goal_type: 'short_term'
    })
  });

  showToast('Goal dibuat.', 'success');
  loadGoalsPanel();
}

async function updateGoalProgress(id, current, target) {
  const newVal = prompt('Progress baru:', current);
  if (!newVal) return;

  const newValInt = parseInt(newVal);
  let status = undefined;

  if (newValInt >= target) {
    status = 'completed';
  }

  await api('/goals/' + id, {
    method: 'PUT',
    body: JSON.stringify({
      current_value: newValInt,
      status
    })
  });

  showToast('Goal diupdate.', 'success');
  loadGoalsPanel();
}

async function deleteGoal(id) {
  if (!confirm('Hapus goal ini?')) return;
  await api('/goals/' + id, { method: 'DELETE' });
  showToast('Goal dihapus.', 'success');
  loadGoalsPanel();
}

// ============================================================
// DATA MANAGEMENT
// ============================================================
async function openDatamanage() {
  showMode('datamanageMode');
  setMobileTitle('Data Management');
  setupEditorBackBtn('.datamanage-header .editor-back');
  if (window.innerWidth <= 767) closeSidebar();

  switchDmTab(currentDmTab);
}

function closeDatamanage() {
  navigateTo('list', { filter: currentFilter });
}

function switchDmTab(tab) {
  currentDmTab = tab;

  document.querySelectorAll('.dm-tab').forEach(t => {
    t.classList.toggle('active', t.dataset.tab === tab);
  });

  ['storage', 'duplicates', 'importexport'].forEach(t => {
    const panel = document.getElementById('dmPanel' + t.charAt(0).toUpperCase() + t.slice(1));
    if (panel) panel.style.display = (t === tab) ? 'block' : 'none';
  });

  if (tab === 'storage') loadStoragePanel();
  if (tab === 'duplicates') loadDuplicatesPanel();
  if (tab === 'importexport') loadImportExportPanel();
}

async function loadStoragePanel() {
  const panel = document.getElementById('dmPanelStorage');
  const breakdown = await api('/storage/breakdown');

  if (!breakdown) {
    panel.innerHTML = '<div class="panel-empty">Gagal memuat data</div>';
    return;
  }

  let breakdownHtml = '';
  (breakdown.by_type || []).forEach(t => {
    breakdownHtml += `
      <div class="storage-type">
        <div class="storage-type-name">${t.type}</div>
        <div class="storage-type-value">${formatBytes(t.size)}</div>
        <div class="storage-type-count">${t.count} file</div>
      </div>
    `;
  });

  const isWarning = breakdown.percent > 80;

  panel.innerHTML = `
    <div style="max-width:720px;margin:0 auto;">
      <div class="storage-overview">
        <h3 style="font-size:16px;font-weight:700;color:var(--text-strong);margin-bottom:4px;">Storage Usage</h3>
        <p style="font-size:13px;color:var(--text-muted);">
          <strong style="color:var(--text-strong);">${formatBytes(breakdown.total_size)}</strong> 
          dari ${formatBytes(breakdown.quota)} (${breakdown.percent}%)
        </p>
        <div class="storage-quota-bar">
          <div class="storage-quota-fill ${isWarning ? 'warning' : ''}" 
               style="width:${breakdown.percent}%"></div>
        </div>
        ${isWarning ? '<p style="font-size:12px;color:var(--danger);margin-top:4px;">Storage hampir penuh!</p>' : ''}

        <div class="storage-breakdown">
          ${breakdownHtml}
        </div>
      </div>
    </div>
  `;
}

async function loadDuplicatesPanel() {
  const panel = document.getElementById('dmPanelDuplicates');
  const dups = await api('/duplicates');

  if (!dups || dups.length === 0) {
    panel.innerHTML = `
      <div class="panel-empty" style="padding:80px 24px;">
        Tidak ada duplikat ditemukan!<br>
        <span style="font-size:12px;color:var(--text-faint);margin-top:8px;display:block;">
          Semua catatan Anda memiliki judul yang unik.
        </span>
      </div>
    `;
    return;
  }

  let html = '<div style="max-width:720px;margin:0 auto;">';
  html += `<p style="font-size:13px;color:var(--text-muted);margin-bottom:16px;">
    Ditemukan <strong>${dups.length}</strong> grup dengan judul sama.
  </p>`;

  dups.forEach(group => {
    html += `
      <div class="dup-item">
        <div class="dup-header">
          <div class="dup-title">${escapeHtml(group.title)}</div>
          <span class="dup-count">${group.count} duplikat</span>
        </div>
        ${group.notes.map(n => `
          <div class="dup-note">
            <span class="dup-note-title">${escapeHtml(n.title)}</span>
            <span class="dup-note-meta">${formatDate(n.updated_at)} · ${n.word_count} kata</span>
            <button class="btn-ghost-sm" onclick="openNote(${n.id})">Buka</button>
            <button class="btn-ghost-sm" onclick="bulkTrash([${n.id}])" style="color:var(--danger);">Hapus</button>
          </div>
        `).join('')}
      </div>
    `;
  });

  html += '</div>';
  panel.innerHTML = html;
}

async function loadImportExportPanel() {
  const panel = document.getElementById('dmPanelImportexport');

  panel.innerHTML = `
    <div style="max-width:720px;margin:0 auto;">
      <div class="chart-section" style="margin-bottom:20px;">
        <h3>Ekspor</h3>
        <p style="font-size:13px;color:var(--text-muted);margin-bottom:16px;">
          Download semua catatan Anda dalam berbagai format.
        </p>
        <button class="btn-primary" onclick="exportNotesDialog()">Export Catatan</button>
      </div>

      <div class="chart-section">
        <h3>Import</h3>
        <p style="font-size:13px;color:var(--text-muted);margin-bottom:16px;">
          Import catatan dari file JSON.
        </p>
        <button class="btn-primary" onclick="openImportDialog()">Import dari JSON</button>
      </div>

      <div class="chart-section" style="border-left:4px solid var(--danger);">
        <h3 style="color:var(--danger);">Danger Zone</h3>
        <p style="font-size:13px;color:var(--text-muted);margin-bottom:16px;">
          Hapus SEMUA catatan di sampah secara permanen.
        </p>
        <button class="btn-danger" onclick="emptyTrash()">Kosongkan Sampah</button>
      </div>
    </div>
  `;
}

async function bulkTrash(ids) {
  if (!confirm(`Pindahkan ${ids.length} catatan ke sampah?`)) return;

  const res = await api('/bulk', {
    method: 'POST',
    body: JSON.stringify({
      action: 'trash',
      note_ids: ids
    })
  });

  if (res) {
    showToast(`${res.affected} catatan dipindahkan ke sampah.`, 'success');
    loadDuplicatesPanel();
  }
}

function openImportDialog() {
  document.getElementById('importJSONText').value = '';
  document.getElementById('importNotesModal').style.display = 'flex';

  const fileInput = document.getElementById('importFileInput');
  fileInput.onchange = (e) => {
    const file = e.target.files[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (ev) => {
      document.getElementById('importJSONText').value = ev.target.result;
    };
    reader.readAsText(file);
  };
}

function closeImportNotesModal() {
  document.getElementById('importNotesModal').style.display = 'none';
}

async function doImportNotes() {
  const text = document.getElementById('importJSONText').value.trim();
  if (!text) {
    showToast('Paste JSON atau pilih file.', 'error');
    return;
  }

  let jsonData;
  try {
    jsonData = JSON.parse(text);
  } catch (e) {
    showToast('JSON tidak valid: ' + e.message, 'error');
    return;
  }

  let notes = [];
  if (Array.isArray(jsonData)) {
    notes = jsonData;
  } else if (jsonData.notes && Array.isArray(jsonData.notes)) {
    notes = jsonData.notes;
  } else {
    showToast('Format JSON tidak dikenali.', 'error');
    return;
  }

  const res = await api('/import/notes', {
    method: 'POST',
    body: JSON.stringify({ notes })
  });

  if (res) {
    showToast(`${res.imported} catatan diimport${res.failed > 0 ? `, ${res.failed} gagal` : ''}.`, 'success');
    closeImportNotesModal();
    loadNotes(currentFilter);
  }
}

// ============================================================
// FASE 9b: ADVANCED EDITOR TOOLBAR INTEGRATION
// ============================================================
function setupAdvancedToolbar() {
  if (!quill || !window.AdvancedEditor) return;

  const calloutBtn = document.getElementById('insertCalloutBtn');
  if (calloutBtn) {
    calloutBtn.onclick = () => {
      const type = prompt('Pilih tipe callout:\n\n' +
        '1. info\n2. warning\n3. success\n4. danger\n5. tip\n6. note\n\n' +
        'Ketik angka atau nama tipe:', 'info');

      if (!type) return;

      const typeMap = {
        '1': 'info', '2': 'warning', '3': 'success',
        '4': 'danger', '5': 'tip', '6': 'note'
      };

      const finalType = typeMap[type] || type;
      const validTypes = ['info', 'warning', 'success', 'danger', 'tip', 'note'];

      if (validTypes.includes(finalType)) {
        window.AdvancedEditor.insertCallout(quill, finalType);
      } else {
        showToast('Tipe callout tidak valid', 'error');
      }
    };
  }

  const toggleBtn = document.getElementById('insertToggleBtn');
  if (toggleBtn) {
    toggleBtn.onclick = () => window.AdvancedEditor.insertToggle(quill);
  }

  const columnsBtn = document.getElementById('insertColumnsBtn');
  if (columnsBtn) {
    columnsBtn.onclick = () => {
      const count = prompt('Jumlah kolom (2, 3, atau 4):', '2');
      if (!count) return;
      const n = parseInt(count);
      if (n >= 2 && n <= 4) {
        window.AdvancedEditor.insertColumns(quill, n);
      } else {
        showToast('Jumlah kolom harus 2, 3, atau 4', 'error');
      }
    };
  }

  const mathBtn = document.getElementById('insertMathBtn');
  if (mathBtn) {
    mathBtn.onclick = () => window.AdvancedEditor.insertMath(quill, true);
  }

  const mermaidBtn = document.getElementById('insertMermaidBtn');
  if (mermaidBtn) {
    mermaidBtn.onclick = () => window.AdvancedEditor.insertMermaid(quill);
  }

  const tocBtn = document.getElementById('insertTocBtn');
  if (tocBtn) {
    tocBtn.onclick = () => window.AdvancedEditor.insertTOC(quill);
  }

  const footnoteBtn = document.getElementById('insertFootnoteBtn');
  if (footnoteBtn) {
    footnoteBtn.onclick = () => window.AdvancedEditor.insertFootnote(quill);
  }

  const bookmarkBtn = document.getElementById('insertBookmarkBtn');
  if (bookmarkBtn) {
    bookmarkBtn.onclick = () => window.AdvancedEditor.insertBookmark(quill);
  }

  const embedBtn = document.getElementById('insertEmbedBtn');
  if (embedBtn) {
    embedBtn.onclick = () => window.AdvancedEditor.insertEmbed(quill);
  }
}

const _origExecuteSlashActionFase9b = executeSlashAction;
executeSlashAction = function(action) {
  if (action === 'callout') {
    closeSlashMenu();
    setTimeout(() => {
      const type = prompt('Tipe callout (info/warning/success/danger/tip/note):', 'info');
      if (type && window.AdvancedEditor) {
        window.AdvancedEditor.insertCallout(quill, type);
      }
    }, 150);
    return;
  }

  if (action === 'toggle') {
    closeSlashMenu();
    setTimeout(() => {
      if (window.AdvancedEditor) window.AdvancedEditor.insertToggle(quill);
    }, 150);
    return;
  }

  if (action === 'columns') {
    closeSlashMenu();
    setTimeout(() => {
      const count = prompt('Jumlah kolom (2/3/4):', '2');
      if (count && window.AdvancedEditor) {
        window.AdvancedEditor.insertColumns(quill, parseInt(count));
      }
    }, 150);
    return;
  }

  if (action === 'math') {
    closeSlashMenu();
    setTimeout(() => {
      if (window.AdvancedEditor) window.AdvancedEditor.insertMath(quill, true);
    }, 150);
    return;
  }

  if (action === 'mermaid') {
    closeSlashMenu();
    setTimeout(() => {
      if (window.AdvancedEditor) window.AdvancedEditor.insertMermaid(quill);
    }, 150);
    return;
  }

  if (action === 'toc') {
    closeSlashMenu();
    setTimeout(() => {
      if (window.AdvancedEditor) window.AdvancedEditor.insertTOC(quill);
    }, 150);
    return;
  }

  if (action === 'footnote') {
    closeSlashMenu();
    setTimeout(() => {
      if (window.AdvancedEditor) window.AdvancedEditor.insertFootnote(quill);
    }, 150);
    return;
  }

  if (action === 'bookmark') {
    closeSlashMenu();
    setTimeout(() => {
      if (window.AdvancedEditor) window.AdvancedEditor.insertBookmark(quill);
    }, 150);
    return;
  }

  if (action === 'embed') {
    closeSlashMenu();
    setTimeout(() => {
      if (window.AdvancedEditor) window.AdvancedEditor.insertEmbed(quill);
    }, 150);
    return;
  }

  return _origExecuteSlashActionFase9b.call(this, action);
};

// ============================================================
// Hook untuk theme change (dari 1-core.js)
// ============================================================
function onThemeChanged(theme) {
  const hljsLight = document.getElementById('hljsLight');
  const hljsDark = document.getElementById('hljsDark');

  if (hljsLight && hljsDark) {
    if (theme === 'dark') {
      hljsLight.disabled = true;
      hljsDark.disabled = false;
    } else {
      hljsLight.disabled = false;
      hljsDark.disabled = true;
    }
  }

  setTimeout(() => {
    if (typeof mermaid !== 'undefined') {
      document.querySelectorAll('.adv-mermaid .mermaid-content').forEach(el => {
        el.removeAttribute('data-processed');
      });
    }
  }, 200);
}

// Init mermaid saat DOM ready
document.addEventListener('DOMContentLoaded', () => {
  if (typeof mermaid !== 'undefined') {
    mermaid.initialize({
      startOnLoad: false,
      theme: document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'default',
      securityLevel: 'strict'
    });
  }

  const theme = document.documentElement.getAttribute('data-theme') || 'light';
  const hljsLight = document.getElementById('hljsLight');
  const hljsDark = document.getElementById('hljsDark');
  if (hljsLight && hljsDark && theme === 'dark') {
    hljsLight.disabled = true;
    hljsDark.disabled = false;
  }
});

console.log('6-advanced.js loaded (dengan Advanced Editor)');

// ============================================================
// IKON SVG KECIL (menggantikan emoji; warna mengikuti teks)
// ============================================================
const _ICO_PATHS = {
  doc: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5M9 13h6M9 17h6"/>',
  bolt: '<path d="M13 2 4 14h7l-1 8 9-12h-7z"/>',
  check: '<circle cx="12" cy="12" r="9"/><path d="m8 12 3 3 5-6"/>',
  keyboard: '<rect x="2.5" y="6" width="19" height="12" rx="2"/><path d="M6 10h.01M10 10h.01M14 10h.01M18 10h.01M7 14h10"/>',
  timer: '<circle cx="12" cy="13" r="8"/><path d="M12 9v4l2.5 2M9.5 2.5h5"/>',
  folder: '<path d="M3 6a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
  tag: '<path d="M3 12V4h8l10 10-8 8z"/><circle cx="7.5" cy="8.5" r="1.2"/>',
  graph: '<circle cx="6" cy="7" r="2.5"/><circle cx="18" cy="6" r="2.5"/><circle cx="12" cy="18" r="2.5"/><path d="M8.3 8.2l2.4 7.6M15.8 8l-2.6 7.8M8.5 7h7"/>',
  users: '<circle cx="9" cy="8" r="3.5"/><path d="M2.5 20a6.5 6.5 0 0 1 13 0M16 4.8a3.5 3.5 0 0 1 0 6.4M18 14.5a6.5 6.5 0 0 1 3.5 5.5"/>',
  bars: '<path d="M5 20V10M12 20V4M19 20v-7"/>',
  trash: '<path d="M4 7h16M10 7V4h4v3M6 7l1 13h10l1-13M10 11v6M14 11v6"/>',
  star: '<path d="m12 3 2.7 5.6 6.1.9-4.4 4.3 1 6.1L12 17l-5.4 2.9 1-6.1L3.2 9.5l6.1-.9z"/>',
  pin: '<path d="M9 3h6l-1 6 3 3H7l3-3zM12 12v9"/>',
  box: '<path d="M3 7h18v4H3zM5 11v9h14v-9M10 15h4"/>',
  theme: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2.5M12 19.5V22M2 12h2.5M19.5 12H22M4.9 4.9l1.8 1.8M17.3 17.3l1.8 1.8M4.9 19.1l1.8-1.8M17.3 6.7l1.8-1.8"/>',
  logout: '<path d="M9 4H5a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h4M16 8l4 4-4 4M20 12H9"/>'
};
function _ico(name, size) {
  size = size || 18;
  return '<svg width="' + size + '" height="' + size + '" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' + (_ICO_PATHS[name] || '') + '</svg>';
}
