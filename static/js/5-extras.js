// ============================================================
// 5-EXTRAS.JS — Mindmap, Whiteboard, Web Clipper, Research, Media
// ============================================================

// ============================================================
// STATE
// ============================================================
let currentMindMapId = null;
let mindMapData = { nodes: [], edges: [] };
let selectedMindMapNode = null;

let currentWhiteboardId = null;
let wbCanvas = null;
let wbCtx = null;
let wbData = { elements: [], viewport: { x: 0, y: 0, zoom: 1 } };
let wbTool = 'select';
let wbColor = '#18181b';
let wbStrokeSize = 4;
let wbDrawing = false;
let wbCurrentPath = null;
let wbStartPoint = null;
let wbUndoStack = [];
let wbRedoStack = [];
let wbPanning = false;
let wbPanStart = null;

let currentClipperTab = 'clips';

let currentMediaType = 'all';
let mediaSearchTimer = null;

// ============================================================
// FASE 8b: MIND MAP & WHITEBOARD
// ============================================================

// ============================================================
// MIND MAP — LIST
// ============================================================
async function openMindmaps() {
  showMode('mindmapsMode');
  setMobileTitle('Mind Maps');
  setupEditorBackBtn('.mindmaps-header .editor-back');
  if (window.innerWidth <= 767) closeSidebar();

  await loadMindmaps();
}

function closeMindmaps() {
  navigateTo('list', { filter: currentFilter });
}

async function loadMindmaps() {
  const maps = await api('/mindmaps');
  const el = document.getElementById('mindmapsList');
  el.innerHTML = '';

  if (!maps || maps.length === 0) {
    el.innerHTML = `
      <div class="panel-empty" style="grid-column:1/-1;padding:60px 24px;">
        Belum ada mind map.<br>
        <span style="font-size:12px;color:var(--text-faint);">Klik "+ Mind Map Baru" untuk mulai.</span>
      </div>
    `;
    return;
  }

  maps.forEach(m => {
    const card = document.createElement('div');
    card.className = 'mindmap-card';
    card.innerHTML = `
      <button class="item-delete-btn" title="Hapus mind map">✕</button>
      <div>
        <div class="mindmap-card-icon">🧠</div>
        <div class="mindmap-card-name">${escapeHtml(m.name)}</div>
      </div>
      <div class="mindmap-card-meta">
        ${m.note_id ? '<span>📄 Dari catatan</span>' : ''}
        <span>${formatDate(m.updated_at)}</span>
      </div>
    `;

    card.querySelector('.item-delete-btn').onclick = (e) => {
      e.stopPropagation();
      deleteMindMap(m.id, m.name);
    };

    card.onclick = () => openMindmapEditor(m.id);
    el.appendChild(card);
  });
}

async function deleteMindMap(id, name) {
  if (!confirm(`Hapus mind map "${name}"?\n\nTindakan ini tidak bisa dibatalkan.`)) return;

  const res = await api('/mindmaps/' + id, { method: 'DELETE' });
  if (res) {
    showToast('Mind map dihapus.', 'success');
    loadMindmaps();
  } else {
    showToast('Gagal menghapus mind map.', 'error');
  }
}

// ============================================================
// MIND MAP — EDITOR
// ============================================================
async function openMindmapEditor(id) {
  currentMindMapId = id;

  document.getElementById('mindmapsMode').style.display = 'none';
  document.getElementById('mindmapEditorMode').style.display = 'flex';

  const map = await api('/mindmaps/' + id);
  if (!map) return;

  document.getElementById('mindmapNameInput').value = map.name || 'Mind Map';

  if (map.data && map.data.nodes) {
    mindMapData = {
      nodes: map.data.nodes.map(n => ({ ...n, collapsed: n.collapsed || false })),
      edges: map.data.edges || []
    };
  } else {
    mindMapData = {
      nodes: [{ id: 'root', text: 'Central Idea', x: 0, y: 0, color: '#6366f1' }],
      edges: []
    };
  }

  selectedMindMapNode = null;
  renderMindMap();
  setupMindMapInteractions();
}

function closeMindmapEditor() {
  currentMindMapId = null;
  mindMapData = { nodes: [], edges: [] };
  navigateTo('mindmaps');
}

async function saveMindMap() {
  if (!currentMindMapId) return;

  const name = document.getElementById('mindmapNameInput').value.trim() || 'Mind Map';

  await api('/mindmaps/' + currentMindMapId, {
    method: 'PUT',
    body: JSON.stringify({ name, data: mindMapData })
  });

  document.getElementById('mindmapSaveBtn').textContent = '✓ Tersimpan';
  setTimeout(() => {
    document.getElementById('mindmapSaveBtn').textContent = 'Simpan';
  }, 1500);

  showToast('Mind map disimpan.', 'success', 1500);
}

// ---------- Render mind map dengan SVG ----------
function renderMindMap() {
  const svg = document.getElementById('mindmapSvg');
  const canvas = document.getElementById('mindmapCanvas');
  const w = canvas.clientWidth;
  const h = canvas.clientHeight;

  svg.setAttribute('viewBox', `0 0 ${w} ${h}`);
  svg.innerHTML = '';

  const cx = w / 2;
  const cy = h / 2;

  const nodeMap = {};
  mindMapData.nodes.forEach(n => { nodeMap[n.id] = n; });

  const hiddenNodes = new Set();
  mindMapData.nodes.forEach(n => {
    if (n.collapsed) {
      hideDescendants(n.id, hiddenNodes);
    }
  });

  function hideDescendants(parentId, set) {
    mindMapData.nodes.forEach(n => {
      if (n.parent_id === parentId) {
        set.add(n.id);
        hideDescendants(n.id, set);
      }
    });
  }

  // Draw edges
  mindMapData.edges.forEach(e => {
    const from = nodeMap[e.from];
    const to = nodeMap[e.to];
    if (!from || !to) return;
    if (hiddenNodes.has(e.from) || hiddenNodes.has(e.to)) return;

    const x1 = cx + from.x;
    const y1 = cy + from.y;
    const x2 = cx + to.x;
    const y2 = cy + to.y;

    const midX = (x1 + x2) / 2;
    const midY = (y1 + y2) / 2;

    const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    path.setAttribute('class', 'mindmap-edge');
    path.setAttribute('d', `M ${x1} ${y1} Q ${midX} ${y1} ${midX} ${midY} T ${x2} ${y2}`);
    path.setAttribute('stroke', to.color || '#9ca3af');
    path.setAttribute('stroke-width', '2');
    path.setAttribute('opacity', '0.5');
    svg.appendChild(path);
  });

  // Draw nodes
  mindMapData.nodes.forEach(n => {
    if (hiddenNodes.has(n.id)) return;

    const x = cx + n.x;
    const y = cy + n.y;

    const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
    g.setAttribute('class', 'mindmap-node' + (selectedMindMapNode === n.id ? ' selected' : ''));
    g.setAttribute('data-id', n.id);
    g.setAttribute('transform', `translate(${x}, ${y})`);

    const fontSize = 13;
    const textWidth = (n.text || '').length * fontSize * 0.6;
    const nodeW = Math.max(80, Math.min(240, textWidth + 32));
    const nodeH = 36;

    const rect = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
    rect.setAttribute('x', -nodeW / 2);
    rect.setAttribute('y', -nodeH / 2);
    rect.setAttribute('width', nodeW);
    rect.setAttribute('height', nodeH);
    rect.setAttribute('rx', 10);
    rect.setAttribute('fill', n.color || '#6366f1');
    rect.setAttribute('stroke', selectedMindMapNode === n.id ? '#fff' : 'transparent');
    rect.setAttribute('stroke-width', '2');
    g.appendChild(rect);

    const text = document.createElementNS('http://www.w3.org/2000/svg', 'text');
    text.setAttribute('text-anchor', 'middle');
    text.setAttribute('dominant-baseline', 'middle');
    text.setAttribute('fill', '#fff');
    text.setAttribute('font-family', 'Inter, sans-serif');
    text.setAttribute('font-size', fontSize);
    text.setAttribute('font-weight', '600');
    text.setAttribute('pointer-events', 'none');

    let displayText = n.text || '';
    if (displayText.length > 30) {
      displayText = displayText.substring(0, 28) + '...';
    }
    text.textContent = displayText;
    g.appendChild(text);

    const hasChildren = mindMapData.nodes.some(c => c.parent_id === n.id);
    if (hasChildren) {
      const indicator = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
      indicator.setAttribute('cx', nodeW / 2 - 8);
      indicator.setAttribute('cy', -nodeH / 2 + 2);
      indicator.setAttribute('r', 8);
      indicator.setAttribute('fill', '#fff');
      indicator.setAttribute('stroke', n.color || '#6366f1');
      indicator.setAttribute('stroke-width', '2');
      indicator.style.cursor = 'pointer';
      g.appendChild(indicator);

      const indicatorText = document.createElementNS('http://www.w3.org/2000/svg', 'text');
      indicatorText.setAttribute('x', nodeW / 2 - 8);
      indicatorText.setAttribute('y', -nodeH / 2 + 6);
      indicatorText.setAttribute('text-anchor', 'middle');
      indicatorText.setAttribute('font-size', '11');
      indicatorText.setAttribute('font-weight', '700');
      indicatorText.setAttribute('fill', n.color || '#6366f1');
      indicatorText.setAttribute('pointer-events', 'none');
      indicatorText.textContent = n.collapsed ? '+' : '−';
      g.appendChild(indicatorText);
    }

    g.addEventListener('click', (e) => {
      e.stopPropagation();
      selectMindMapNode(n.id);
    });

    g.addEventListener('dblclick', (e) => {
      e.stopPropagation();
      editMindMapNode(n.id);
    });

    setupNodeDrag(g, n);

    svg.appendChild(g);
  });
}

function setupNodeDrag(g, node) {
  let isDragging = false;
  let startX = 0;
  let startY = 0;
  let offsetX = 0;
  let offsetY = 0;

  g.addEventListener('mousedown', e => {
    if (e.detail === 2) return;
    isDragging = true;
    startX = e.clientX;
    startY = e.clientY;
    offsetX = node.x;
    offsetY = node.y;
    e.stopPropagation();
  });

  function onMouseMove(e) {
    if (!isDragging) return;
    node.x = offsetX + (e.clientX - startX);
    node.y = offsetY + (e.clientY - startY);
    g.setAttribute('transform', `translate(${document.getElementById('mindmapCanvas').clientWidth/2 + node.x}, ${document.getElementById('mindmapCanvas').clientHeight/2 + node.y})`);
    renderMindMapEdgesOnly();
  }

  function onMouseUp() {
    if (isDragging) {
      isDragging = false;
      renderMindMap();
    }
    document.removeEventListener('mousemove', onMouseMove);
    document.removeEventListener('mouseup', onMouseUp);
  }

  g.addEventListener('mousedown', () => {
    document.addEventListener('mousemove', onMouseMove);
    document.addEventListener('mouseup', onMouseUp);
  });
}

function renderMindMapEdgesOnly() {
  renderMindMap();
}

function selectMindMapNode(id) {
  selectedMindMapNode = id;
  renderMindMap();
}

function editMindMapNode(id) {
  const node = mindMapData.nodes.find(n => n.id === id);
  if (!node) return;

  const newText = prompt('Edit node:', node.text);
  if (newText !== null && newText.trim() !== '') {
    node.text = newText.trim();
    renderMindMap();
  }
}

function addMindMapNode() {
  if (mindMapData.nodes.length === 0) {
    mindMapData.nodes.push({
      id: 'root',
      text: 'Central Idea',
      x: 0,
      y: 0,
      color: '#6366f1'
    });
    renderMindMap();
    return;
  }

  let parentId = selectedMindMapNode || 'root';
  const parent = mindMapData.nodes.find(n => n.id === parentId);
  if (!parent) {
    parentId = mindMapData.nodes[0].id;
  }

  const text = prompt('Teks node baru:', 'New Node');
  if (!text) return;

  const nodeId = 'node-' + Date.now();
  const parentNode = mindMapData.nodes.find(n => n.id === parentId);

  const existingChildren = mindMapData.nodes.filter(n => n.parent_id === parentId).length;
  const angle = (existingChildren * 60) * Math.PI / 180;
  const radius = 180;

  mindMapData.nodes.push({
    id: nodeId,
    parent_id: parentId,
    text: text,
    x: parentNode.x + radius * Math.cos(angle),
    y: parentNode.y + radius * Math.sin(angle),
    color: '#0ea5e9'
  });

  mindMapData.edges.push({ from: parentId, to: nodeId });

  selectedMindMapNode = nodeId;
  renderMindMap();
}

function deleteMindMapNode() {
  if (!selectedMindMapNode || selectedMindMapNode === 'root') {
    showToast('Tidak bisa hapus root node', 'error');
    return;
  }

  const toDelete = new Set([selectedMindMapNode]);

  function collectDescendants(id) {
    mindMapData.nodes.forEach(n => {
      if (n.parent_id === id && !toDelete.has(n.id)) {
        toDelete.add(n.id);
        collectDescendants(n.id);
      }
    });
  }
  collectDescendants(selectedMindMapNode);

  mindMapData.nodes = mindMapData.nodes.filter(n => !toDelete.has(n.id));
  mindMapData.edges = mindMapData.edges.filter(e =>
    !toDelete.has(e.from) && !toDelete.has(e.to));

  selectedMindMapNode = null;
  renderMindMap();
}

function toggleCollapseNode(id) {
  const node = mindMapData.nodes.find(n => n.id === id);
  if (!node) return;
  node.collapsed = !node.collapsed;
  renderMindMap();
}

function autoLayoutMindMap() {
  if (mindMapData.nodes.length === 0) return;

  const root = mindMapData.nodes.find(n => !n.parent_id) || mindMapData.nodes[0];
  root.x = 0;
  root.y = 0;

  const children = {};
  mindMapData.nodes.forEach(n => {
    if (n.parent_id) {
      if (!children[n.parent_id]) children[n.parent_id] = [];
      children[n.parent_id].push(n);
    }
  });

  function layout(parentId, level, angleStart, angleEnd) {
    const kids = children[parentId] || [];
    if (kids.length === 0) return;

    const angleStep = (angleEnd - angleStart) / kids.length;
    const radius = 160 * (level + 1);

    kids.forEach((k, i) => {
      const angle = angleStart + angleStep * (i + 0.5);
      const parentNode = mindMapData.nodes.find(n => n.id === parentId);
      k.x = (parentNode.x || 0) + radius * Math.cos(angle);
      k.y = (parentNode.y || 0) + radius * Math.sin(angle);
      layout(k.id, level + 1, angle - angleStep / 2, angle + angleStep / 2);
    });
  }

  layout(root.id, 0, 0, 2 * Math.PI);
  renderMindMap();
  showToast('Auto layout diterapkan.', 'success');
}

function setupMindMapInteractions() {
  const svg = document.getElementById('mindmapSvg');

  svg.addEventListener('click', e => {
    if (e.target === svg) {
      selectedMindMapNode = null;
      renderMindMap();
    }
  });

  document.addEventListener('keydown', function mmKeyHandler(e) {
    const editorOpen = document.getElementById('mindmapEditorMode') &&
      document.getElementById('mindmapEditorMode').style.display !== 'none';
    if (!editorOpen) return;
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') return;

    if (e.key === 'Tab') {
      e.preventDefault();
      addMindMapNode();
    } else if (e.key === 'Delete' || e.key === 'Backspace') {
      if (selectedMindMapNode) {
        e.preventDefault();
        deleteMindMapNode();
      }
    } else if (e.key === 'Enter') {
      if (selectedMindMapNode) {
        e.preventDefault();
        editMindMapNode(selectedMindMapNode);
      }
    }
  });
}

function exportMindMapSVG() {
  const svg = document.getElementById('mindmapSvg');
  const serializer = new XMLSerializer();
  const svgStr = serializer.serializeToString(svg);

  const blob = new Blob([svgStr], { type: 'image/svg+xml' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = (document.getElementById('mindmapNameInput').value || 'mindmap') + '.svg';
  a.click();
  URL.revokeObjectURL(url);

  showToast('Mind map diexport sebagai SVG.', 'success');
}

function showCreateMindMapModal() {
  document.getElementById('createMindMapName').value = '';
  document.getElementById('createMindMapNote').value = '';
  populateMindMapNoteDropdown();
  document.getElementById('createMindMapModal').style.display = 'flex';
  setTimeout(() => document.getElementById('createMindMapName').focus(), 100);
}

function closeCreateMindMapModal() {
  document.getElementById('createMindMapModal').style.display = 'none';
}

async function populateMindMapNoteDropdown() {
  const notes = await api('/notes?filter=all');
  const select = document.getElementById('createMindMapNote');
  select.innerHTML = '<option value="">— Kosong —</option>';
  if (!notes) return;
  notes.slice(0, 50).forEach(n => {
    const opt = document.createElement('option');
    opt.value = n.id;
    opt.textContent = n.title;
    select.appendChild(opt);
  });
}

async function saveCreateMindMap() {
  const name = document.getElementById('createMindMapName').value.trim();
  const noteId = document.getElementById('createMindMapNote').value;

  if (!name) {
    showToast('Nama mind map wajib diisi.', 'error');
    return;
  }

  let res;
  if (noteId) {
    res = await api('/mindmaps/from-note', {
      method: 'POST',
      body: JSON.stringify({ note_id: parseInt(noteId) })
    });
  } else {
    res = await api('/mindmaps', {
      method: 'POST',
      body: JSON.stringify({ name })
    });
  }

  if (res && res.id) {
    showToast('Mind map dibuat.', 'success');
    closeCreateMindMapModal();
    openMindmapEditor(res.id);
  } else {
    showToast('Gagal membuat mind map.', 'error');
  }
}

// ============================================================
// WHITEBOARD
// ============================================================
async function openWhiteboards() {
  showMode('whiteboardsMode');
  setMobileTitle('Whiteboard');
  setupEditorBackBtn('.whiteboards-header .editor-back');
  if (window.innerWidth <= 767) closeSidebar();

  await loadWhiteboards();
}

function closeWhiteboards() {
  navigateTo('list', { filter: currentFilter });
}

async function loadWhiteboards() {
  const boards = await api('/whiteboards');
  const el = document.getElementById('whiteboardsList');
  el.innerHTML = '';

  if (!boards || boards.length === 0) {
    el.innerHTML = `
      <div class="panel-empty" style="grid-column:1/-1;padding:60px 24px;">
        Belum ada whiteboard.<br>
        <span style="font-size:12px;color:var(--text-faint);">Klik "+ Whiteboard Baru" untuk mulai.</span>
      </div>
    `;
    return;
  }

  boards.forEach(w => {
    const card = document.createElement('div');
    card.className = 'whiteboard-card';
    card.innerHTML = `
      <button class="item-delete-btn" title="Hapus whiteboard">✕</button>
      <div>
        <div class="whiteboard-card-icon">🎨</div>
        <div class="whiteboard-card-name">${escapeHtml(w.name)}</div>
      </div>
      <div class="whiteboard-card-meta">
        ${w.project_id ? '<span>📁 Project</span>' : ''}
        <span>${formatDate(w.updated_at)}</span>
      </div>
    `;

    card.querySelector('.item-delete-btn').onclick = (e) => {
      e.stopPropagation();
      deleteWhiteboard(w.id, w.name);
    };

    card.onclick = () => openWhiteboardEditor(w.id);
    el.appendChild(card);
  });
}

async function deleteWhiteboard(id, name) {
  if (!confirm(`Hapus whiteboard "${name}"?\n\nSemua gambar & sticky note di dalamnya akan hilang.`)) return;

  const res = await api('/whiteboards/' + id, { method: 'DELETE' });
  if (res) {
    showToast('Whiteboard dihapus.', 'success');
    loadWhiteboards();
  } else {
    showToast('Gagal menghapus whiteboard.', 'error');
  }
}

async function openWhiteboardEditor(id) {
  currentWhiteboardId = id;

  document.getElementById('whiteboardsMode').style.display = 'none';
  document.getElementById('whiteboardEditorMode').style.display = 'flex';

  const wb = await api('/whiteboards/' + id);
  if (!wb) return;

  document.getElementById('whiteboardNameInput').value = wb.name || 'Whiteboard';

  if (wb.data && wb.data.elements) {
    wbData = {
      elements: wb.data.elements || [],
      viewport: wb.data.viewport || { x: 0, y: 0, zoom: 1 }
    };
  } else {
    wbData = { elements: [], viewport: { x: 0, y: 0, zoom: 1 } };
  }

  wbUndoStack = [];
  wbRedoStack = [];

  setTimeout(() => {
    initWhiteboardCanvas();
    redrawWhiteboard();
  }, 100);
}

function closeWhiteboardEditor() {
  currentWhiteboardId = null;
  wbData = { elements: [], viewport: { x: 0, y: 0, zoom: 1 } };
  navigateTo('whiteboards');
}

function initWhiteboardCanvas() {
  wbCanvas = document.getElementById('whiteboardCanvas');
  wbCtx = wbCanvas.getContext('2d');

  resizeWhiteboardCanvas();
  window.addEventListener('resize', resizeWhiteboardCanvas);

  document.querySelectorAll('.wb-tool[data-tool]').forEach(btn => {
    btn.onclick = () => {
      document.querySelectorAll('.wb-tool').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      wbTool = btn.dataset.tool;
      wbCanvas.style.cursor = wbTool === 'select' ? 'default' : 'crosshair';
    };
  });

  document.getElementById('wbColorPicker').oninput = (e) => {
    wbColor = e.target.value;
  };

  document.getElementById('wbStrokeSize').onchange = (e) => {
    wbStrokeSize = parseInt(e.target.value);
  };

  wbCanvas.onmousedown = handleWhiteboardMouseDown;
  wbCanvas.onmousemove = handleWhiteboardMouseMove;
  wbCanvas.onmouseup = handleWhiteboardMouseUp;
  wbCanvas.onmouseleave = handleWhiteboardMouseUp;

  wbCanvas.oncontextmenu = (e) => e.preventDefault();
}

function resizeWhiteboardCanvas() {
  if (!wbCanvas) return;
  const rect = wbCanvas.getBoundingClientRect();
  wbCanvas.width = rect.width * devicePixelRatio;
  wbCanvas.height = rect.height * devicePixelRatio;
  wbCanvas.style.width = rect.width + 'px';
  wbCanvas.style.height = rect.height + 'px';
  wbCtx.setTransform(devicePixelRatio, 0, 0, devicePixelRatio, 0, 0);
  redrawWhiteboard();
}

function handleWhiteboardMouseDown(e) {
  const rect = wbCanvas.getBoundingClientRect();
  const x = (e.clientX - rect.left - wbData.viewport.x) / wbData.viewport.zoom;
  const y = (e.clientY - rect.top - wbData.viewport.y) / wbData.viewport.zoom;

  if (e.button === 2 || wbTool === 'select') {
    if (e.button === 2 || e.shiftKey) {
      wbPanning = true;
      wbPanStart = { x: e.clientX - wbData.viewport.x, y: e.clientY - wbData.viewport.y };
      wbCanvas.style.cursor = 'grabbing';
    }
    return;
  }

  pushUndo();

  wbDrawing = true;
  wbStartPoint = { x, y };

  if (wbTool === 'pen') {
    wbCurrentPath = {
      id: 'el-' + Date.now(),
      type: 'path',
      points: [{ x, y }],
      color: wbColor,
      stroke: wbStrokeSize
    };
    wbData.elements.push(wbCurrentPath);
  } else if (wbTool === 'text') {
    const text = prompt('Teks:');
    if (text) {
      wbData.elements.push({
        id: 'el-' + Date.now(),
        type: 'text',
        x, y,
        text,
        color: wbColor,
        font_size: 16 + wbStrokeSize * 2
      });
      wbDrawing = false;
      redrawWhiteboard();
      autoSaveWhiteboard();
    } else {
      wbDrawing = false;
      wbUndoStack.pop();
    }
  } else if (wbTool === 'sticky') {
    const text = prompt('Isi sticky note:', 'Catatan...');
    if (text) {
      wbData.elements.push({
        id: 'el-' + Date.now(),
        type: 'sticky',
        x, y,
        width: 180,
        height: 140,
        text,
        fill: '#fef08a',
        color: '#854d0e'
      });
      wbDrawing = false;
      redrawWhiteboard();
      autoSaveWhiteboard();
    } else {
      wbDrawing = false;
      wbUndoStack.pop();
    }
  } else if (wbTool === 'rect' || wbTool === 'circle' || wbTool === 'arrow') {
    wbCurrentPath = {
      id: 'el-' + Date.now(),
      type: wbTool,
      x, y,
      width: 0,
      height: 0,
      color: wbColor,
      stroke: wbStrokeSize
    };
    wbData.elements.push(wbCurrentPath);
  }
}

function handleWhiteboardMouseMove(e) {
  if (wbPanning) {
    wbData.viewport.x = e.clientX - wbPanStart.x;
    wbData.viewport.y = e.clientY - wbPanStart.y;
    redrawWhiteboard();
    return;
  }

  if (!wbDrawing || !wbCurrentPath) return;

  const rect = wbCanvas.getBoundingClientRect();
  const x = (e.clientX - rect.left - wbData.viewport.x) / wbData.viewport.zoom;
  const y = (e.clientY - rect.top - wbData.viewport.y) / wbData.viewport.zoom;

  if (wbTool === 'pen') {
    wbCurrentPath.points.push({ x, y });
  } else if (wbTool === 'rect' || wbTool === 'circle') {
    wbCurrentPath.width = x - wbStartPoint.x;
    wbCurrentPath.height = y - wbStartPoint.y;
  } else if (wbTool === 'arrow') {
    wbCurrentPath.width = x - wbStartPoint.x;
    wbCurrentPath.height = y - wbStartPoint.y;
  }

  redrawWhiteboard();
}

function handleWhiteboardMouseUp() {
  if (wbPanning) {
    wbPanning = false;
    wbCanvas.style.cursor = wbTool === 'select' ? 'default' : 'crosshair';
    return;
  }

  if (wbDrawing) {
    wbDrawing = false;

    if (wbCurrentPath && (wbTool === 'rect' || wbTool === 'circle')) {
      if (wbCurrentPath.width < 0) {
        wbCurrentPath.x += wbCurrentPath.width;
        wbCurrentPath.width = Math.abs(wbCurrentPath.width);
      }
      if (wbCurrentPath.height < 0) {
        wbCurrentPath.y += wbCurrentPath.height;
        wbCurrentPath.height = Math.abs(wbCurrentPath.height);
      }
    }

    wbCurrentPath = null;
    autoSaveWhiteboard();
  }
}

function redrawWhiteboard() {
  if (!wbCtx) return;

  const w = wbCanvas.clientWidth;
  const h = wbCanvas.clientHeight;

  wbCtx.clearRect(0, 0, w, h);

  wbCtx.save();
  wbCtx.translate(wbData.viewport.x, wbData.viewport.y);
  wbCtx.scale(wbData.viewport.zoom, wbData.viewport.zoom);

  wbData.elements.forEach(el => drawWhiteboardElement(el));

  wbCtx.restore();

  const label = document.getElementById('wbZoomLabel');
  if (label) label.textContent = Math.round(wbData.viewport.zoom * 100) + '%';
}

function drawWhiteboardElement(el) {
  wbCtx.lineCap = 'round';
  wbCtx.lineJoin = 'round';

  if (el.type === 'path') {
    if (!el.points || el.points.length < 2) return;
    wbCtx.strokeStyle = el.color || '#18181b';
    wbCtx.lineWidth = el.stroke || 2;
    wbCtx.beginPath();
    wbCtx.moveTo(el.points[0].x, el.points[0].y);
    for (let i = 1; i < el.points.length; i++) {
      wbCtx.lineTo(el.points[i].x, el.points[i].y);
    }
    wbCtx.stroke();
  } else if (el.type === 'rect') {
    wbCtx.strokeStyle = el.color || '#18181b';
    wbCtx.lineWidth = el.stroke || 2;
    wbCtx.strokeRect(el.x, el.y, el.width, el.height);
  } else if (el.type === 'circle') {
    wbCtx.strokeStyle = el.color || '#18181b';
    wbCtx.lineWidth = el.stroke || 2;
    wbCtx.beginPath();
    wbCtx.ellipse(
      el.x + el.width / 2, el.y + el.height / 2,
      Math.abs(el.width / 2), Math.abs(el.height / 2),
      0, 0, 2 * Math.PI
    );
    wbCtx.stroke();
  } else if (el.type === 'arrow') {
    drawArrow(el);
  } else if (el.type === 'text') {
    wbCtx.fillStyle = el.color || '#18181b';
    wbCtx.font = `${el.font_size || 20}px Inter, sans-serif`;
    wbCtx.textBaseline = 'top';
    wbCtx.fillText(el.text || '', el.x, el.y);
  } else if (el.type === 'sticky') {
    wbCtx.fillStyle = 'rgba(0,0,0,0.08)';
    wbCtx.fillRect(el.x + 4, el.y + 4, el.width, el.height);

    wbCtx.fillStyle = el.fill || '#fef08a';
    wbCtx.fillRect(el.x, el.y, el.width, el.height);

    wbCtx.strokeStyle = 'rgba(0,0,0,0.1)';
    wbCtx.lineWidth = 1;
    wbCtx.strokeRect(el.x, el.y, el.width, el.height);

    wbCtx.fillStyle = el.color || '#854d0e';
    wbCtx.font = '14px Inter, sans-serif';
    wbCtx.textBaseline = 'top';

    const padding = 12;
    const maxWidth = el.width - padding * 2;
    const words = (el.text || '').split(' ');
    const lines = [];
    let currentLine = '';

    words.forEach(word => {
      const test = currentLine + (currentLine ? ' ' : '') + word;
      if (wbCtx.measureText(test).width > maxWidth && currentLine) {
        lines.push(currentLine);
        currentLine = word;
      } else {
        currentLine = test;
      }
    });
    if (currentLine) lines.push(currentLine);

    lines.slice(0, 8).forEach((line, i) => {
      wbCtx.fillText(line, el.x + padding, el.y + padding + i * 20);
    });
  }
}

function drawArrow(el) {
  const x1 = el.x;
  const y1 = el.y;
  const x2 = el.x + el.width;
  const y2 = el.y + el.height;

  wbCtx.strokeStyle = el.color || '#18181b';
  wbCtx.lineWidth = el.stroke || 2;
  wbCtx.beginPath();
  wbCtx.moveTo(x1, y1);
  wbCtx.lineTo(x2, y2);
  wbCtx.stroke();

  const angle = Math.atan2(y2 - y1, x2 - x1);
  const headLen = 12;

  wbCtx.beginPath();
  wbCtx.moveTo(x2, y2);
  wbCtx.lineTo(
    x2 - headLen * Math.cos(angle - Math.PI / 6),
    y2 - headLen * Math.sin(angle - Math.PI / 6)
  );
  wbCtx.moveTo(x2, y2);
  wbCtx.lineTo(
    x2 - headLen * Math.cos(angle + Math.PI / 6),
    y2 - headLen * Math.sin(angle + Math.PI / 6)
  );
  wbCtx.stroke();
}

// ---------- Undo / Redo ----------
function pushUndo() {
  wbUndoStack.push(JSON.stringify(wbData.elements));
  if (wbUndoStack.length > 50) wbUndoStack.shift();
  wbRedoStack = [];
}

function wbUndo() {
  if (wbUndoStack.length === 0) return;
  wbRedoStack.push(JSON.stringify(wbData.elements));
  wbData.elements = JSON.parse(wbUndoStack.pop());
  redrawWhiteboard();
  autoSaveWhiteboard();
}

function wbRedo() {
  if (wbRedoStack.length === 0) return;
  wbUndoStack.push(JSON.stringify(wbData.elements));
  wbData.elements = JSON.parse(wbRedoStack.pop());
  redrawWhiteboard();
  autoSaveWhiteboard();
}

function wbClear() {
  if (!confirm('Hapus semua elemen di whiteboard ini?')) return;
  pushUndo();
  wbData.elements = [];
  redrawWhiteboard();
  autoSaveWhiteboard();
}

function wbZoomIn() {
  wbData.viewport.zoom = Math.min(3, wbData.viewport.zoom * 1.2);
  redrawWhiteboard();
}

function wbZoomOut() {
  wbData.viewport.zoom = Math.max(0.2, wbData.viewport.zoom / 1.2);
  redrawWhiteboard();
}

function wbZoomReset() {
  wbData.viewport = { x: 0, y: 0, zoom: 1 };
  redrawWhiteboard();
}

// ---------- Auto-save ----------
let wbSaveTimer = null;
function autoSaveWhiteboard() {
  clearTimeout(wbSaveTimer);
  const btn = document.getElementById('whiteboardSaveBtn');
  if (btn) btn.textContent = 'Menyimpan...';

  wbSaveTimer = setTimeout(() => {
    saveWhiteboard();
  }, 1500);
}

async function saveWhiteboard() {
  if (!currentWhiteboardId) return;

  const name = document.getElementById('whiteboardNameInput').value.trim() || 'Whiteboard';

  await api('/whiteboards/' + currentWhiteboardId, {
    method: 'PUT',
    body: JSON.stringify({ name, data: wbData })
  });

  const btn = document.getElementById('whiteboardSaveBtn');
  if (btn) {
    btn.textContent = '✓ Tersimpan';
    setTimeout(() => { btn.textContent = 'Simpan'; }, 1500);
  }
}

// ---------- Modal ----------
function showCreateWhiteboardModal() {
  document.getElementById('createWhiteboardName').value = '';
  document.getElementById('createWhiteboardProject').value = '';
  populateWhiteboardProjectDropdown();
  document.getElementById('createWhiteboardModal').style.display = 'flex';
  setTimeout(() => document.getElementById('createWhiteboardName').focus(), 100);
}

function closeCreateWhiteboardModal() {
  document.getElementById('createWhiteboardModal').style.display = 'none';
}

async function populateWhiteboardProjectDropdown() {
  const projects = await api('/projects');
  const select = document.getElementById('createWhiteboardProject');
  select.innerHTML = '<option value="">— Tidak ada —</option>';
  if (!projects) return;
  projects.forEach(p => {
    const opt = document.createElement('option');
    opt.value = p.id;
    opt.textContent = p.name;
    select.appendChild(opt);
  });
}

async function saveCreateWhiteboard() {
  const name = document.getElementById('createWhiteboardName').value.trim();
  const projectId = document.getElementById('createWhiteboardProject').value;

  if (!name) {
    showToast('Nama whiteboard wajib diisi.', 'error');
    return;
  }

  const payload = { name };
  if (projectId) payload.project_id = parseInt(projectId);

  const res = await api('/whiteboards', {
    method: 'POST',
    body: JSON.stringify(payload)
  });

  if (res && res.id) {
    showToast('Whiteboard dibuat.', 'success');
    closeCreateWhiteboardModal();
    openWhiteboardEditor(res.id);
  }
}

// ============================================================
// FASE 9: WEB CLIPPER
// ============================================================
async function openClipper() {
  showMode('clipperMode');
  setMobileTitle('Web Clipper');
  setupEditorBackBtn('.clipper-header .editor-back');
  if (window.innerWidth <= 767) closeSidebar();

  await loadClipperTab(currentClipperTab);
}

function closeClipper() {
  navigateTo('list', { filter: currentFilter });
}

function switchClipperTab(tab) {
  currentClipperTab = tab;
  document.querySelectorAll('.clipper-tab').forEach(t => {
    t.classList.toggle('active', t.dataset.tab === tab);
  });

  ['clips', 'reading', 'highlights'].forEach(t => {
    const panel = document.getElementById('clipperPanel' + t.charAt(0).toUpperCase() + t.slice(1));
    if (panel) panel.style.display = (t === tab) ? 'grid' : 'none';
  });

  loadClipperTab(tab);
}

async function loadClipperTab(tab) {
  if (tab === 'clips') await loadClips();
  else if (tab === 'reading') await loadReadingList();
  else if (tab === 'highlights') await loadHighlights();
}

async function loadClips() {
  const clips = await api('/clips');
  const el = document.getElementById('clipperPanelClips');
  el.innerHTML = '';

  if (!clips || clips.length === 0) {
    el.innerHTML = `
      <div class="panel-empty" style="grid-column:1/-1;padding:60px 24px;">
        Belum ada clip.<br>
        <span style="font-size:12px;color:var(--text-faint);">Klik "+ Simpan URL" untuk mulai.</span>
      </div>
    `;
    return;
  }

  clips.forEach(c => {
    const card = document.createElement('div');
    card.className = 'clip-card';

    let coverHtml = '';
    if (c.cover_image) {
      coverHtml = `<img src="${c.cover_image}" class="clip-card-cover" alt="" onerror="this.style.display='none'">`;
    } else {
      coverHtml = `<div class="clip-card-cover-placeholder">🌐</div>`;
    }

    card.innerHTML = `
      ${coverHtml}
      <button class="item-delete-btn" title="Hapus clip">✕</button>
      <div class="clip-card-body">
        <div class="clip-card-site">${escapeHtml(c.site_name || 'Web')}</div>
        <div class="clip-card-title">${escapeHtml(c.title || 'Tanpa judul')}</div>
        <div class="clip-card-excerpt">${escapeHtml(c.excerpt || '')}</div>
        <div class="clip-card-footer">
          <span class="read-badge ${c.is_read ? 'read' : ''}">${c.is_read ? '✓ Dibaca' : `${c.read_time} menit`}</span>
          <span>${formatDate(c.created_at)}</span>
        </div>
      </div>
    `;

    card.querySelector('.item-delete-btn').onclick = (e) => {
      e.stopPropagation();
      deleteClip(c.id, c.title);
    };

    card.onclick = () => {
      if (c.note_id) openNote(c.note_id);
      else window.open(c.url, '_blank');
    };

    el.appendChild(card);
  });
}

async function deleteClip(id, title) {
  if (!confirm(`Hapus clip "${title}"?\n\nCatatan terkait (kalau ada) tidak akan terhapus.`)) return;

  const res = await api('/clips/' + id, { method: 'DELETE' });
  if (res) {
    showToast('Clip dihapus.', 'success');
    loadClips();
  } else {
    showToast('Gagal menghapus clip.', 'error');
  }
}

async function loadReadingList() {
  const items = await api('/reading-list');
  const el = document.getElementById('clipperPanelReading');
  el.innerHTML = '';

  if (!items || items.length === 0) {
    el.innerHTML = `
      <div class="panel-empty" style="grid-column:1/-1;padding:60px 24px;">
        Reading list kosong.<br>
        <span style="font-size:12px;color:var(--text-faint);">Tambahkan dari clip atau "+ Simpan URL".</span>
      </div>
    `;
    return;
  }

  items.forEach(item => {
    const div = document.createElement('div');
    div.className = 'reading-item priority-' + (item.priority || 'medium');

    div.innerHTML = `
      <div class="reading-item-info">
        <div class="reading-item-title">${escapeHtml(item.title || 'Tanpa judul')}</div>
        ${item.description ? `<div class="reading-item-desc">${escapeHtml(item.description)}</div>` : ''}
        <div class="reading-item-meta">
          <span class="reading-status ${item.status}">${item.status}</span>
          ${item.url ? `<span>${escapeHtml((item.url || '').substring(0, 40))}...</span>` : ''}
        </div>
      </div>
      <button class="item-delete-btn" title="Hapus dari reading list">✕</button>
    `;

    div.querySelector('.item-delete-btn').onclick = (e) => {
      e.stopPropagation();
      deleteReadingItem(item.id, item.title);
    };

    div.onclick = () => {
      if (item.url) window.open(item.url, '_blank');
    };

    el.appendChild(div);
  });
}

async function deleteReadingItem(id, title) {
  if (!confirm(`Hapus "${title}" dari reading list?`)) return;

  const res = await api('/reading-list/' + id, { method: 'DELETE' });
  if (res) {
    showToast('Item dihapus dari reading list.', 'success');
    loadReadingList();
  } else {
    showToast('Gagal menghapus.', 'error');
  }
}

async function loadHighlights() {
  const highlights = await api('/highlights');
  const el = document.getElementById('clipperPanelHighlights');
  el.innerHTML = '';

  if (!highlights || highlights.length === 0) {
    el.innerHTML = `
      <div class="panel-empty" style="grid-column:1/-1;padding:60px 24px;">
        Belum ada highlight.<br>
        <span style="font-size:12px;color:var(--text-faint);">Highlight akan muncul di sini.</span>
      </div>
    `;
    return;
  }

  el.style.gridTemplateColumns = '1fr';

  highlights.forEach(h => {
    const div = document.createElement('div');
    div.className = 'highlight-item';
    div.style.borderLeftColor = h.color || '#fef08a';

    div.innerHTML = `
      <button class="item-delete-btn" title="Hapus highlight">✕</button>
      <div class="highlight-text">"${escapeHtml(h.text)}"</div>
      ${h.note ? `<div class="highlight-note">📝 ${escapeHtml(h.note)}</div>` : ''}
      <div class="highlight-meta">
        ${h.source_title ? `<span>📄 ${escapeHtml(h.source_title)}</span>` : ''}
        <span>${formatDate(h.created_at)}</span>
      </div>
    `;

    div.querySelector('.item-delete-btn').onclick = (e) => {
      e.stopPropagation();
      deleteHighlight(h.id);
    };

    el.appendChild(div);
  });
}

async function deleteHighlight(id) {
  if (!confirm('Hapus highlight ini?')) return;

  const res = await api('/highlights/' + id, { method: 'DELETE' });
  if (res) {
    showToast('Highlight dihapus.', 'success');
    loadHighlights();
  } else {
    showToast('Gagal menghapus highlight.', 'error');
  }
}

// ---------- Clip URL Modal ----------
function showClipURLModal() {
  document.getElementById('clipURLInput').value = '';
  document.getElementById('clipURLTitle').value = '';
  document.getElementById('clipURLContent').value = '';
  document.getElementById('clipURLType').value = 'article';
  document.getElementById('clipURLCreateNote').checked = true;

  document.getElementById('clipURLModal').style.display = 'flex';
  setTimeout(() => document.getElementById('clipURLInput').focus(), 100);
}

function closeClipURLModal() {
  document.getElementById('clipURLModal').style.display = 'none';
}

async function submitClipURL() {
  const url = document.getElementById('clipURLInput').value.trim();
  const title = document.getElementById('clipURLTitle').value.trim();
  const content = document.getElementById('clipURLContent').value.trim();
  const clipType = document.getElementById('clipURLType').value;
  const createNote = document.getElementById('clipURLCreateNote').checked;

  if (!url) {
    showToast('URL wajib diisi.', 'error');
    return;
  }

  let htmlContent = '';
  if (content) {
    htmlContent = content.split('\n').filter(p => p.trim()).map(p => '<p>' + escapeHtml(p) + '</p>').join('');
  }

  const res = await api('/clips', {
    method: 'POST',
    body: JSON.stringify({
      url,
      title,
      content: htmlContent,
      clip_type: clipType,
      create_note: createNote
    })
  });

  if (res && res.id) {
    showToast('Clip berhasil disimpan.', 'success');
    closeClipURLModal();
    loadClips();
  } else {
    showToast('Gagal menyimpan clip.', 'error');
  }
}

// ============================================================
// RESEARCH SOURCES
// ============================================================
async function openResearch() {
  showMode('researchMode');
  setMobileTitle('Research');
  setupEditorBackBtn('.research-header .editor-back');
  if (window.innerWidth <= 767) closeSidebar();

  await loadSources();
}

function closeResearch() {
  navigateTo('list', { filter: currentFilter });
}

async function loadSources() {
  const sources = await api('/sources');
  const el = document.getElementById('sourcesList');
  el.innerHTML = '';

  if (!sources || sources.length === 0) {
    el.innerHTML = `
      <div class="panel-empty" style="grid-column:1/-1;padding:60px 24px;">
        Belum ada sumber referensi.<br>
        <span style="font-size:12px;color:var(--text-faint);">Klik "+ Sumber Baru" untuk mulai.</span>
      </div>
    `;
    return;
  }

  sources.forEach(s => {
    const card = document.createElement('div');
    card.className = 'source-card';

    const typeLabels = {
      website: '🌐 Website',
      book: '📖 Buku',
      article: '📰 Artikel',
      journal: '📚 Jurnal',
      thesis: '🎓 Thesis',
      other: '📄 Lainnya'
    };

    const tags = (s.tags || '').split(',').filter(t => t.trim());

    card.innerHTML = `
      <button class="item-delete-btn" title="Hapus sumber">✕</button>
      <div class="source-type-badge">${typeLabels[s.source_type] || s.source_type}</div>
      <div class="source-card-title">${escapeHtml(s.title)}</div>
      ${s.author ? `<div class="source-card-author">— ${escapeHtml(s.author)}${s.year ? ', ' + escapeHtml(s.year) : ''}</div>` : ''}
      ${tags.length > 0 ? `
        <div class="source-card-tags">
          ${tags.slice(0, 4).map(t => `<span class="source-card-tag">#${escapeHtml(t.trim())}</span>`).join('')}
        </div>
      ` : ''}
    `;

    card.querySelector('.item-delete-btn').onclick = (e) => {
      e.stopPropagation();
      deleteSource(s.id, s.title);
    };

    card.onclick = () => openSourceDetail(s.id);
    el.appendChild(card);
  });
}

async function deleteSource(id, title) {
  if (!confirm(`Hapus sumber "${title}"?`)) return;

  const res = await api('/sources/' + id, { method: 'DELETE' });
  if (res) {
    showToast('Sumber dihapus.', 'success');
    loadSources();
  } else {
    showToast('Gagal menghapus sumber.', 'error');
  }
}

function showCreateSourceModal() {
  document.getElementById('sourceModalTitle').textContent = 'Sumber Baru';
  document.getElementById('sourceModalId').value = '';
  ['sourceTitle', 'sourceAuthor', 'sourceYear', 'sourceURL', 'sourcePublisher',
   'sourcePublication', 'sourceTags', 'sourceNotes'].forEach(id => {
    document.getElementById(id).value = '';
  });
  document.getElementById('sourceType').value = 'website';
  document.getElementById('sourceCitationFormat').value = 'apa';

  document.getElementById('sourceModal').style.display = 'flex';
  setTimeout(() => document.getElementById('sourceTitle').focus(), 100);
}

function closeSourceModal() {
  document.getElementById('sourceModal').style.display = 'none';
}

async function saveSource() {
  const id = document.getElementById('sourceModalId').value;
  const title = document.getElementById('sourceTitle').value.trim();

  if (!title) {
    showToast('Judul wajib diisi.', 'error');
    return;
  }

  const payload = {
    title,
    author: document.getElementById('sourceAuthor').value.trim(),
    year: document.getElementById('sourceYear').value.trim(),
    url: document.getElementById('sourceURL').value.trim(),
    publisher: document.getElementById('sourcePublisher').value.trim(),
    publication: document.getElementById('sourcePublication').value.trim(),
    source_type: document.getElementById('sourceType').value,
    citation_format: document.getElementById('sourceCitationFormat').value,
    tags: document.getElementById('sourceTags').value.trim(),
    notes: document.getElementById('sourceNotes').value.trim()
  };

  let res;
  if (id) {
    res = await api('/sources/' + id, { method: 'PUT', body: JSON.stringify(payload) });
  } else {
    res = await api('/sources', { method: 'POST', body: JSON.stringify(payload) });
  }

  if (res) {
    showToast(id ? 'Sumber diperbarui.' : 'Sumber dibuat.', 'success');
    closeSourceModal();
    loadSources();
  }
}

async function openSourceDetail(id) {
  const source = await api('/sources/' + id);
  if (!source) return;

  document.getElementById('sourceDetailTitle').textContent = source.title || 'Detail Sumber';

  const highlightsHtml = (source.highlights || []).map(h => `
    <div class="highlight-item" style="border-left-color:${h.color || '#fef08a'};margin-top:8px;">
      <div class="highlight-text" style="font-size:12.5px;">"${escapeHtml(h.text)}"</div>
      ${h.note ? `<div class="highlight-note">${escapeHtml(h.note)}</div>` : ''}
    </div>
  `).join('');

  const body = document.getElementById('sourceDetailBody');
  body.innerHTML = `
    <div class="search-field">
      <label>${escapeHtml(source.source_type)}</label>
      <div style="font-size:14px;color:var(--text-strong);line-height:1.5;">
        ${source.author ? `<em>${escapeHtml(source.author)}</em>${source.year ? ' ('+source.year+')' : ''}. ` : ''}
        <strong>${escapeHtml(source.title)}</strong>.
        ${source.publisher ? escapeHtml(source.publisher) + '. ' : ''}
        ${source.url ? `<a href="${source.url}" target="_blank" style="color:var(--accent);">${escapeHtml(source.url)}</a>` : ''}
      </div>
    </div>

    <div class="search-field">
      <label>Citation (${source.citation_format.toUpperCase()})</label>
      <div class="citation-box">
        ${escapeHtml(source.citation)}
        <button onclick="copyCitation('${escapeHtml(source.citation).replace(/'/g, "\\'")}')">Copy</button>
      </div>
    </div>

    ${source.notes ? `
      <div class="search-field">
        <label>Catatan</label>
        <div style="font-size:13px;color:var(--text);line-height:1.6;">${escapeHtml(source.notes)}</div>
      </div>
    ` : ''}

    ${highlightsHtml ? `
      <div class="search-field">
        <label>Highlight (${source.highlights.length})</label>
        ${highlightsHtml}
      </div>
    ` : ''}

    <div style="display:flex;gap:8px;justify-content:flex-end;margin-top:12px;">
      <button class="btn-ghost-sm" onclick="editSourceFromDetail(${source.id})">Edit</button>
      <button class="btn-ghost-sm" onclick="deleteSource(${source.id})" style="color:var(--danger);">Hapus</button>
    </div>
  `;

  document.getElementById('sourceDetailModal').style.display = 'flex';
}

function closeSourceDetail() {
  document.getElementById('sourceDetailModal').style.display = 'none';
}

function copyCitation(text) {
  navigator.clipboard.writeText(text).then(() => {
    showToast('Citation disalin.', 'success', 1500);
  });
}

async function editSourceFromDetail(id) {
  closeSourceDetail();
  const source = await api('/sources/' + id);
  if (!source) return;

  document.getElementById('sourceModalTitle').textContent = 'Edit Sumber';
  document.getElementById('sourceModalId').value = source.id;
  document.getElementById('sourceTitle').value = source.title || '';
  document.getElementById('sourceAuthor').value = source.author || '';
  document.getElementById('sourceYear').value = source.year || '';
  document.getElementById('sourceURL').value = source.url || '';
  document.getElementById('sourcePublisher').value = source.publisher || '';
  document.getElementById('sourcePublication').value = source.publication || '';
  document.getElementById('sourceType').value = source.source_type || 'website';
  document.getElementById('sourceCitationFormat').value = source.citation_format || 'apa';
  document.getElementById('sourceTags').value = source.tags || '';
  document.getElementById('sourceNotes').value = source.notes || '';

  document.getElementById('sourceModal').style.display = 'flex';
}

// ============================================================
// MEDIA LIBRARY
// ============================================================
async function openMedia() {
  showMode('mediaMode');
  setMobileTitle('Media');
  setupEditorBackBtn('.media-header .editor-back');
  if (window.innerWidth <= 767) closeSidebar();

  await loadMediaStats();
  await loadMediaList();
}

function closeMedia() {
  navigateTo('list', { filter: currentFilter });
}

function switchMediaTab(type) {
  currentMediaType = type;
  document.querySelectorAll('.media-tab').forEach(t => {
    t.classList.toggle('active', t.dataset.type === type);
  });
  loadMediaList();
}

async function loadMediaStats() {
  const stats = await api('/media/stats');
  if (!stats) return;

  const el = document.getElementById('mediaStats');
  el.innerHTML = `
    <div class="stat-card info">
      <div class="stat-value">${stats.total || 0}</div>
      <div class="stat-label">Total</div>
    </div>
    <div class="stat-card">
      <div class="stat-value">${stats.image || 0}</div>
      <div class="stat-label">Gambar</div>
    </div>
    <div class="stat-card">
      <div class="stat-value">${stats.video || 0}</div>
      <div class="stat-label">Video</div>
    </div>
    <div class="stat-card">
      <div class="stat-value">${stats.audio || 0}</div>
      <div class="stat-label">Audio</div>
    </div>
    <div class="stat-card">
      <div class="stat-value">${(stats.pdf || 0) + (stats.document || 0)}</div>
      <div class="stat-label">Dokumen</div>
    </div>
    <div class="stat-card warning">
      <div class="stat-value">${formatBytes(stats.total_size || 0)}</div>
      <div class="stat-label">Ukuran</div>
    </div>
  `;
}

async function loadMediaList() {
  const params = new URLSearchParams();
  if (currentMediaType && currentMediaType !== 'all') params.set('type', currentMediaType);

  const keyword = document.getElementById('mediaSearchInput').value.trim();
  if (keyword) params.set('q', keyword);

  const items = await api('/media?' + params.toString());
  const el = document.getElementById('mediaGrid');
  el.innerHTML = '';

  if (!items || items.length === 0) {
    el.innerHTML = `
      <div class="panel-empty" style="grid-column:1/-1;padding:60px 24px;">
        Belum ada media.<br>
        <span style="font-size:12px;color:var(--text-faint);">Media akan muncul otomatis saat Anda upload lampiran ke catatan.</span>
      </div>
    `;
    return;
  }

  items.forEach(m => {
    const div = document.createElement('div');
    div.className = 'media-item';

    let previewHtml = '';
    if (m.media_type === 'image') {
      previewHtml = `<img src="${m.url}" alt="" loading="lazy">`;
    } else {
      const icons = {
        video: '🎥', audio: '🎵', pdf: '📄',
        document: '📁', other: '📎'
      };
      previewHtml = `<div class="media-icon">${icons[m.media_type] || '📎'}</div>`;
    }

    const typeLabels = {
      image: 'IMG', video: 'VID', audio: 'AUD',
      pdf: 'PDF', document: 'DOC', other: 'FILE'
    };

    let noteSourceHtml = '';
    if (m.note_title) {
      const isTrashed = m.note_trashed === true;
      const folderLabel = m.notebook_name ? ` · ${escapeHtml(m.notebook_name)}` : '';
      const trashedLabel = isTrashed ? ' (di sampah)' : '';

      noteSourceHtml = `
        <div class="media-item-note ${isTrashed ? 'trashed' : ''}" 
             onclick="event.stopPropagation(); ${isTrashed ? '' : `openNote(${m.note_id})`}"
             title="${isTrashed ? 'Catatan sudah dihapus' : 'Klik untuk buka catatan'}">
          <span class="media-item-note-icon">📄</span>
          <span class="media-item-note-name">${escapeHtml(m.note_title)}${trashedLabel}</span>
          ${folderLabel ? `<span class="media-item-note-folder">${folderLabel}</span>` : ''}
        </div>
      `;
    } else {
      noteSourceHtml = `
        <div class="media-item-note empty" title="File tidak terikat catatan">
          <span class="media-item-note-icon">📄</span>
          <span class="media-item-note-name">Tanpa catatan</span>
        </div>
      `;
    }

    div.innerHTML = `
      <div class="media-item-preview">
        ${previewHtml}
        <span class="media-item-type">${typeLabels[m.media_type] || 'FILE'}</span>
        <button class="media-item-favorite ${m.is_favorite ? 'active' : ''}" 
                onclick="event.stopPropagation(); toggleMediaFavorite(${m.id}, ${!m.is_favorite})">
          ${m.is_favorite ? '★' : '☆'}
        </button>
      </div>
      <div class="media-item-info">
        <div class="media-item-name">${escapeHtml(m.title || m.filename)}</div>
        <div class="media-item-meta">
          <span>${formatBytes(m.size || 0)}</span>
          <span>${formatDate(m.created_at)}</span>
        </div>
        ${noteSourceHtml}
      </div>
    `;

    div.onclick = () => {
      if (m.media_type === 'image' || m.media_type === 'video' || m.media_type === 'audio') {
        openMediaPreview(m);
      } else {
        window.open(m.url, '_blank');
      }
    };

    el.appendChild(div);
  });
}

function openMediaPreview(m) {
  window._currentPreviewId = m.attachment_id;

  document.getElementById('previewFilename').textContent = m.title || m.filename;
  document.getElementById('previewMeta').textContent = `${m.mime_type} · ${formatBytes(m.size || 0)}`;

  const body = document.getElementById('previewBody');
  body.innerHTML = '';

  if (m.media_type === 'image') {
    const img = document.createElement('img');
    img.src = m.url;
    body.appendChild(img);
  } else if (m.media_type === 'video') {
    const video = document.createElement('video');
    video.src = m.url;
    video.controls = true;
    body.appendChild(video);
  } else if (m.media_type === 'audio') {
    const audio = document.createElement('audio');
    audio.src = m.url;
    audio.controls = true;
    body.appendChild(audio);
  } else {
    window.open(m.url, '_blank');
    return;
  }

  document.getElementById('previewModal').style.display = 'flex';
  document.body.style.overflow = 'hidden';
}

async function toggleMediaFavorite(id, newState) {
  await api('/media/' + id, {
    method: 'PUT',
    body: JSON.stringify({ is_favorite: newState })
  });
  loadMediaList();
  showToast(newState ? 'Ditambahkan ke favorit.' : 'Dihapus dari favorit.', 'success', 1500);
}

function debouncedMediaSearch() {
  clearTimeout(mediaSearchTimer);
  mediaSearchTimer = setTimeout(loadMediaList, 300);
}

async function syncMediaLibrary() {
  if (!confirm('Scan semua lampiran lama yang belum muncul di Media Library?\n\nIni akan mendaftarkan file lama secara otomatis.')) return;

  showToast('Sedang scan...', 'info', 2000);

  const res = await api('/media/backfill', { method: 'POST' });
  if (res) {
    if (res.found === 0) {
      showToast('Semua lampiran sudah terdaftar.', 'success');
    } else {
      showToast(`${res.inserted} file ditambahkan ke Media Library.`, 'success');
    }
    await loadMediaStats();
    await loadMediaList();
  } else {
    showToast('Gagal sync.', 'error');
  }
}

console.log('✅ 5-extras.js loaded');