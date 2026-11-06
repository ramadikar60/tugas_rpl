// ============================================================
// 0-SANITIZE.JS - Sanitasi HTML dari server (Prompt 13)
//
// Semua HTML isi catatan yang berasal dari server WAJIB lewat
// window.safeNoteHTML() sebelum dimasukkan ke DOM.
// Aturannya mencerminkan whitelist server (handlers/sanitize.go).
//
// Bila DOMPurify gagal dimuat (CDN diblokir, SRI tidak cocok),
// fungsi ini GAGAL TERTUTUP: isi ditampilkan sebagai teks polos.
// ============================================================
(function () {
  'use strict';

  var ALLOWED_TAGS = [
    'p', 'br', 'hr', 'div', 'span', 'blockquote', 'pre', 'code',
    'strong', 'b', 'em', 'i', 'u', 's', 'strike', 'del', 'ins', 'sub', 'sup', 'mark', 'small',
    'ul', 'ol', 'li', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
    'table', 'thead', 'tbody', 'tfoot', 'tr', 'td', 'th', 'colgroup', 'col', 'caption',
    'details', 'summary', 'button', 'a', 'img', 'iframe'
  ];

  var ALLOWED_ATTR = [
    'class', 'id', 'title', 'dir', 'contenteditable', 'spellcheck',
    'href', 'target', 'rel', 'src', 'alt', 'width', 'height', 'style',
    'colspan', 'rowspan', 'start', 'type',
    'frameborder', 'loading', 'allowfullscreen', 'sandbox'
  ];

  var RE_CLASS_TOKEN = /^(?:ql-[a-z0-9-]+|adv-[a-z0-9-]+|callout-[a-z0-9-]+|toggle-[a-z0-9-]+|is-open|math-[a-z0-9-]+|mermaid-[a-z0-9-]+|footnote-[a-z0-9-]+|bookmark-[a-z0-9-]+|embed-[a-z0-9-]+|columns-[a-z0-9-]+|column-[a-z0-9-]+|cols-[0-9]{1,2}|col-btn|toc-[a-z0-9-]+|quill-better-table(?:-[a-z0-9-]+)?|qlbt-[a-z0-9-]+|hljs(?:-[a-z0-9-]+)?|language-[a-z0-9+#-]{1,20}|wiki-link|mention)$/;
  var RE_ID = /^(?:heading-[0-9]{1,5}|mermaid-[A-Za-z0-9_-]{1,64})$/;
  var RE_SAFE_URL = /^(?:https?:\/\/[^\s"'<>\u0000-\u001f]+|\/[^\s"'<>\u0000-\u001f]*|#[^\s"'<>\u0000-\u001f]*)?$/;
  var RE_EMBED_URL = /^(?:https:\/\/(?:www\.youtube(?:-nocookie)?\.com\/embed\/[A-Za-z0-9_-]{1,40}|player\.vimeo\.com\/video\/[0-9]{1,15}|open\.spotify\.com\/embed\/(?:track|album|playlist)\/[A-Za-z0-9]{1,40})(?:\?[A-Za-z0-9=&_%.-]{0,200})?)?$/;
  var RE_DATA_IMG = /^data:image\/(?:png|jpe?g|gif|webp)(?:;base64)?,/i;
  var RE_COLOR = /^(?:#[0-9a-fA-F]{3,8}|rgba?\(\s*[0-9.%\s,\/]{1,40}\)|hsla?\(\s*[0-9.%\s,\/a-z]{1,40}\)|[a-zA-Z]{3,20}|var\(--[a-zA-Z0-9-]{1,40}\))$/;
  var RE_LEN = /^(?:auto|[0-9]{1,4}(?:\.[0-9]{1,3})?(?:px|%|em|rem)?)$/;
  var EMBED_SANDBOX = 'allow-scripts allow-same-origin allow-presentation allow-popups';

  // Style: hanya properti aman dengan nilai tervalidasi (sama dengan server).
  function cleanStyle(style) {
    var out = [];
    String(style || '').split(';').forEach(function (decl) {
      var i = decl.indexOf(':');
      if (i < 1) return;
      var prop = decl.slice(0, i).trim().toLowerCase();
      var val = decl.slice(i + 1).trim();
      var ok = false;
      if (prop === 'color' || prop === 'background-color') ok = RE_COLOR.test(val);
      else if (prop === 'text-align') ok = /^(?:left|right|center|justify)$/.test(val);
      else if (prop === 'display') ok = /^(?:none|block)$/.test(val);
      else if (prop === 'width' || prop === 'height' || prop === 'max-width') ok = RE_LEN.test(val);
      if (ok) out.push(prop + ': ' + val);
    });
    return out.join('; ');
  }

  var hooksInstalled = false;
  function installHooks() {
    if (hooksInstalled || typeof DOMPurify === 'undefined') return;
    hooksInstalled = true;

    DOMPurify.addHook('uponSanitizeAttribute', function (node, data) {
      var name = data.attrName;
      var val = data.attrValue;

      if (name === 'class') {
        var keep = String(val).split(/\s+/).filter(function (t) { return RE_CLASS_TOKEN.test(t); });
        if (keep.length) data.attrValue = keep.join(' ');
        else data.keepAttr = false;
      } else if (name === 'id') {
        if (!RE_ID.test(val)) data.keepAttr = false;
      } else if (name === 'style') {
        var cleaned = cleanStyle(val);
        if (cleaned) data.attrValue = cleaned;
        else data.keepAttr = false;
      } else if (name === 'data-url') {
        if (!RE_SAFE_URL.test(val)) data.keepAttr = false;
      } else if (name === 'data-embed-url') {
        if (!RE_EMBED_URL.test(val)) data.keepAttr = false;
      } else if (name === 'target') {
        if (val !== '_blank') data.keepAttr = false;
      } else if (name === 'src' && node.nodeName === 'IMG') {
        if (/^\s*data:/i.test(val) && !RE_DATA_IMG.test(val.trim())) data.keepAttr = false;
      } else if (name === 'src' && node.nodeName === 'IFRAME') {
        if (!RE_EMBED_URL.test(val)) data.keepAttr = false;
      } else if (name === 'sandbox') {
        data.keepAttr = false; // dipasang paksa di afterSanitizeAttributes
      }
    });

    DOMPurify.addHook('afterSanitizeAttributes', function (node) {
      if (node.nodeName === 'A') {
        if (node.getAttribute('target') === '_blank') node.setAttribute('rel', 'noopener noreferrer');
      } else if (node.nodeName === 'IFRAME') {
        var src = node.getAttribute('src') || '';
        if (!src || !RE_EMBED_URL.test(src)) {
          node.parentNode && node.parentNode.removeChild(node);
        } else {
          node.setAttribute('sandbox', EMBED_SANDBOX);
        }
      }
    });
  }

  // Hasil render turunan (KaTeX, Mermaid, TOC) dibuang; dibangun ulang dari
  // data-formula / data-code / daftar heading oleh AdvancedEditor.rehydrate().
  function normalizeDerived(html) {
    var tpl = document.createElement('template'); // isi template inert: tidak ada eksekusi
    tpl.innerHTML = html;
    var root = tpl.content;

    root.querySelectorAll('.adv-math').forEach(function (m) {
      var c = m.querySelector('.math-content');
      if (c) c.textContent = m.getAttribute('data-formula') || '';
    });
    root.querySelectorAll('.adv-mermaid').forEach(function (m) {
      var c = m.querySelector('.mermaid-content');
      if (c) { c.textContent = m.getAttribute('data-code') || ''; c.removeAttribute('data-processed'); }
    });
    root.querySelectorAll('.adv-toc').forEach(function (t) { t.textContent = ''; });

    return tpl.innerHTML;
  }

  function textFallback(html) {
    var tpl = document.createElement('template');
    tpl.innerHTML = html;
    var text = tpl.content.textContent || '';
    var p = document.createElement('p');
    p.textContent = text;
    return p.outerHTML;
  }

  // Sanitasi isi catatan lengkap (editor, pratinjau versi, halaman berbagi).
  function safeNoteHTML(html) {
    html = (html === null || html === undefined) ? '' : String(html);
    if (html === '') return '';
    if (typeof DOMPurify === 'undefined') {
      console.error('[sanitize] DOMPurify tidak tersedia; isi ditampilkan sebagai teks polos.');
      return textFallback(html);
    }
    installHooks();
    return DOMPurify.sanitize(normalizeDerived(html), {
      ALLOWED_TAGS: ALLOWED_TAGS,
      ALLOWED_ATTR: ALLOWED_ATTR,
      ALLOW_DATA_ATTR: true,
      ALLOW_ARIA_ATTR: false,
      ADD_DATA_URI_TAGS: ['img'],
      FORBID_TAGS: ['style', 'script', 'form', 'input', 'textarea', 'select', 'object', 'embed', 'svg', 'math', 'link', 'meta', 'base']
      // id sudah dibatasi whitelist (RE_ID), jadi SANITIZE_NAMED_PROPS tidak dipakai
      // (opsi itu mengubah id menjadi "user-content-..." dan memutus tautan TOC).
    });
  }

  window.safeNoteHTML = safeNoteHTML;
  // Bentuk "sumber" catatan (hasil render KaTeX/Mermaid/TOC dibuang), sama dengan yang disimpan server.
  window.noteSourceHTML = normalizeDerived;
})();
