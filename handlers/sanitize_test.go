package handlers

import (
	"net/url"
	"strings"
	"testing"
	"time"

	xhtml "golang.org/x/net/html"
)

// allowedTagsForTest harus sama dengan daftar elemen pada kebijakan.
var allowedTagsForTest = map[string]bool{
	"p": true, "br": true, "hr": true, "div": true, "span": true, "blockquote": true, "pre": true, "code": true,
	"strong": true, "b": true, "em": true, "i": true, "u": true, "s": true, "strike": true, "del": true, "ins": true,
	"sub": true, "sup": true, "mark": true, "small": true, "ul": true, "ol": true, "li": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true, "td": true, "th": true,
	"colgroup": true, "col": true, "caption": true, "details": true, "summary": true, "button": true,
	"a": true, "img": true, "iframe": true,
}

// assertSafeHTML men-tokenisasi hasil dan gagal bila ada elemen/atribut berbahaya.
func assertSafeHTML(t *testing.T, label, out string) {
	t.Helper()
	z := xhtml.NewTokenizer(strings.NewReader(out))
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			return
		}
		if tt != xhtml.StartTagToken && tt != xhtml.SelfClosingTagToken {
			continue
		}
		name, hasAttr := z.TagName()
		tag := string(name)
		if !allowedTagsForTest[tag] {
			t.Errorf("[%s] elemen tidak diizinkan <%s> pada: %s", label, tag, out)
		}
		for hasAttr {
			var k, v []byte
			k, v, hasAttr = z.TagAttr()
			key, val := strings.ToLower(string(k)), strings.ToLower(strings.TrimSpace(string(v)))
			if strings.HasPrefix(key, "on") {
				t.Errorf("[%s] atribut event %q pada: %s", label, key, out)
			}
			if key == "style" && (strings.Contains(val, "javascript") || strings.Contains(val, "expression") ||
				strings.Contains(val, "url(") || strings.Contains(val, "position")) {
				t.Errorf("[%s] style berbahaya %q pada: %s", label, val, out)
			}
			if key == "href" || key == "src" {
				_, perr := url.Parse(val)
				rel := perr == nil && !strings.Contains(strings.SplitN(val, "/", 2)[0], ":")
				ok := rel || strings.HasPrefix(val, "http://") || strings.HasPrefix(val, "https://") ||
					strings.HasPrefix(val, "mailto:") || strings.HasPrefix(val, "tel:") ||
					strings.HasPrefix(val, "/") || strings.HasPrefix(val, "#") ||
					strings.HasPrefix(val, "data:image/png") || strings.HasPrefix(val, "data:image/jpeg") ||
					strings.HasPrefix(val, "data:image/gif") || strings.HasPrefix(val, "data:image/webp")
				if !ok {
					t.Errorf("[%s] %s tidak aman %q pada: %s", label, key, val, out)
				}
			}
		}
	}
}

func TestSanitizeBlocksXSSVectors(t *testing.T) {
	vectors := map[string]string{
		"script":             `<p>ok</p><script>alert(1)</script>`,
		"img-onerror":        `<img src=x onerror=alert(1)>`,
		"img-onerror-quoted": `<img src="x" onerror="alert(document.cookie)">`,
		"js-href":            `<a href="javascript:alert(1)">x</a>`,
		"js-href-case":       `<a href="JaVaScRiPt:alert(1)">x</a>`,
		"js-href-entity":     `<a href="&#106;avascript:alert(1)">x</a>`,
		"js-href-space":      `<a href=" javascript:alert(1)">x</a>`,
		"js-href-tab":        "<a href=\"java\tscript:alert(1)\">x</a>",
		"data-href":          `<a href="data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==">x</a>`,
		"vbscript":           `<a href="vbscript:msgbox(1)">x</a>`,
		"svg-onload":         `<svg onload=alert(1)>`,
		"svg-script":         `<svg><script>alert(1)</script></svg>`,
		"svg-style":          `<svg><style>*{x:expression(alert(1))}</style></svg>`,
		"mxss-math":          `<math><mtext><table><mglyph><style><img src=x onerror=alert(1)>`,
		"mxss-noscript":      `<noscript><p title="</noscript><img src=x onerror=alert(1)>">`,
		"iframe-js":          `<iframe src="javascript:alert(1)"></iframe>`,
		"iframe-evil":        `<iframe src="https://evil.example/phish"></iframe>`,
		"iframe-srcdoc":      `<iframe srcdoc="<script>alert(1)</script>"></iframe>`,
		"style-url":          `<div style="background:url(javascript:alert(1))">x</div>`,
		"style-expression":   `<div style="width:expression(alert(1))">x</div>`,
		"style-position":     `<div style="position:fixed;top:0;left:0;width:100%;height:100%">x</div>`,
		"onclick":            `<p onclick="alert(1)">x</p>`,
		"onmouseover":        `<div onmouseover="alert(1)">x</div>`,
		"form":               `<form action="https://evil.example"><input name=p><button>go</button></form>`,
		"meta-refresh":       `<meta http-equiv="refresh" content="0;url=https://evil.example">`,
		"link-css":           `<link rel="stylesheet" href="https://evil.example/x.css">`,
		"base":               `<base href="https://evil.example/">`,
		"object":             `<object data="https://evil.example/x.swf"></object>`,
		"embed":              `<embed src="https://evil.example/x.swf">`,
		"svg-data-img":       `<img src="data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9YWxlcnQoMSk+">`,
		"html-data-img":      `<img src="data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==">`,
		"callout-title":      `<div class="adv-callout" data-type="info" data-title="&quot;&gt;&lt;img src=x onerror=alert(1)&gt;">x</div>`,
		"bookmark-js":        `<div class="adv-bookmark" data-url="javascript:alert(1)" data-title="x"></div>`,
		"embed-url-js":       `<div class="adv-embed" data-url="x" data-embed-url="javascript:alert(1)"></div>`,
		"details-ontoggle":   `<details open ontoggle=alert(1)><summary>x</summary></details>`,
		"button-formaction":  `<button formaction="javascript:alert(1)">x</button>`,
		"id-clobber":         `<div id="mermaid">x</div><a id="token">y</a>`,
	}
	for name, in := range vectors {
		out := sanitizeNoteHTMLStrict(in)
		assertSafeHTML(t, name, out)
		low := strings.ToLower(out)
		for _, bad := range []string{"<script", "javascript:", "vbscript:", "<svg", "<style", "<form", "<meta", "<link", "<base", "<object", "<embed", "srcdoc", "formaction"} {
			if strings.Contains(low, bad) {
				t.Errorf("[%s] hasil masih memuat %q: %s", name, bad, out)
			}
		}
	}
}

func TestSanitizeDropsAttributeValuesNotJustTags(t *testing.T) {
	// data-url / data-embed-url tidak aman harus hilang total.
	out := sanitizeNoteHTMLStrict(`<div class="adv-bookmark" data-url="javascript:alert(1)" data-title="x"></div>`)
	if strings.Contains(out, "data-url") {
		t.Errorf("data-url berbahaya masih ada: %s", out)
	}
	out = sanitizeNoteHTMLStrict(`<div class="adv-embed" data-embed-url="https://evil.example/x"></div>`)
	if strings.Contains(out, "data-embed-url") {
		t.Errorf("data-embed-url non-whitelist masih ada: %s", out)
	}
	// id sembarang hilang, id heading TOC tetap.
	out = sanitizeNoteHTMLStrict(`<h1 id="heading-3">A</h1><h2 id="token">B</h2>`)
	if !strings.Contains(out, `id="heading-3"`) || strings.Contains(out, `id="token"`) {
		t.Errorf("penyaringan id salah: %s", out)
	}
}

// Struktur yang dihasilkan Quill/blok kustom harus bertahan.
func TestSanitizePreservesEditorStructures(t *testing.T) {
	type tc struct {
		name, in string
		must     []string // potongan yang HARUS ada di hasil
		mustNot  []string // potongan yang TIDAK boleh ada
	}
	cases := []tc{
		{"inline", `<p>Halo <strong>dunia</strong> <em>miring</em> <u>u</u> <s>s</s></p>`,
			[]string{`<strong>dunia</strong>`, `<em>miring</em>`, `<u>u</u>`, `<s>s</s>`}, nil},
		{"heading-quote-hr", `<h1>Judul</h1><blockquote>kutip</blockquote><hr><h3>Sub</h3>`,
			[]string{`<h1>Judul</h1>`, `<blockquote>kutip</blockquote>`, `<hr`, `<h3>Sub</h3>`}, nil},
		{"list-bullet", `<ol><li data-list="bullet"><span class="ql-ui" contenteditable="false"></span>Item</li></ol>`,
			[]string{`data-list="bullet"`, `class="ql-ui"`, `contenteditable="false"`, `Item`}, nil},
		{"checklist", `<ol><li data-list="checked"><span class="ql-ui" contenteditable="false"></span>Selesai</li><li data-list="unchecked"><span class="ql-ui" contenteditable="false"></span>Belum</li></ol>`,
			[]string{`data-list="checked"`, `data-list="unchecked"`}, nil},
		{"align-indent-size", `<p class="ql-align-center ql-indent-1"><span class="ql-size-large">Besar</span></p>`,
			[]string{`ql-align-center`, `ql-indent-1`, `ql-size-large`}, nil},
		{"warna", `<span style="color: rgb(230, 0, 0); background-color: rgb(255, 255, 0);">w</span>`,
			[]string{`color: rgb(230, 0, 0)`, `background-color: rgb(255, 255, 0)`}, nil},
		{"link", `<a href="https://example.com/a?b=1" target="_blank" rel="noopener noreferrer">x</a>`,
			[]string{`href="https://example.com/a?b=1"`, `target="_blank"`}, nil},
		{"wiki-link", `<a href="#wiki-12">[[Judul Lain]]</a>`,
			[]string{`href="#wiki-12"`, `[[Judul Lain]]`}, nil},
		{"mailto", `<a href="mailto:a@b.co">surel</a>`, []string{`mailto:a@b.co`}, nil},
		{"gambar-upload", `<p><img src="/uploads/abc123.png"></p>`, []string{`src="/uploads/abc123.png"`}, nil},
		{"gambar-data-png", `<p><img src="data:image/png;base64,iVBORw0KGgo="></p>`, []string{`data:image/png;base64,iVBORw0KGgo=`}, nil},
		{"gambar-ukuran", `<img src="/uploads/a.png" width="300" style="width: 300px;">`,
			[]string{`width="300"`, `width: 300px`}, nil},
		{"code-block-quill2", `<div class="ql-code-block-container" spellcheck="false"><div class="ql-code-block" data-language="plain">let x = 1;</div></div>`,
			[]string{`ql-code-block-container`, `ql-code-block`, `data-language="plain"`, `let x = 1;`}, nil},
		{"code-hljs", `<pre class="ql-syntax"><code class="hljs"><span class="hljs-keyword">const</span> a</code></pre>`,
			[]string{`ql-syntax`, `hljs-keyword`}, nil},
		{"tabel-better-table", `<table class="quill-better-table" style="width: 300px;"><colgroup><col width="100"><col width="100"></colgroup><tbody>` +
			`<tr data-row="row-ab12"><td data-row="row-ab12" rowspan="1" colspan="1"><p class="qlbt-cell-line" data-row="row-ab12" data-cell="cell-xy9" data-rowspan="1" data-colspan="1">A</p></td>` +
			`<td data-row="row-ab12" rowspan="1" colspan="1"><p class="qlbt-cell-line" data-row="row-ab12" data-cell="cell-zz1" data-rowspan="1" data-colspan="1">B</p></td></tr></tbody></table>`,
			[]string{`quill-better-table`, `<col width="100"`, `qlbt-cell-line`, `data-cell="cell-xy9"`, `colspan="1"`, `data-row="row-ab12"`}, nil},
		{"callout", `<div class="adv-callout callout-info" data-type="info" data-title="Info" contenteditable="false">` +
			`<div class="callout-header"><span class="callout-icon">ℹ️</span><span class="callout-title">Info</span></div>` +
			`<div class="callout-body">Isi penting <strong>di sini</strong></div></div>`,
			[]string{`adv-callout callout-info`, `data-type="info"`, `data-title="Info"`, `callout-header`, `callout-body`, `<strong>di sini</strong>`}, nil},
		{"toggle", `<div class="adv-toggle" data-title="Buka" data-open="false" contenteditable="false">` +
			`<div class="toggle-header"><span class="toggle-arrow">▶</span><span class="toggle-title">Buka</span></div>` +
			`<div class="toggle-body" style="display:none">tersembunyi</div></div>`,
			[]string{`adv-toggle`, `data-open="false"`, `toggle-body`, `display: none`, `tersembunyi`}, nil},
		{"footnote", `<sup class="adv-footnote" data-text="Sumber: buku" data-number="1" contenteditable="false">` +
			`<sup class="footnote-marker" title="Sumber: buku">[1]</sup></sup>`,
			[]string{`adv-footnote`, `data-number="1"`, `data-text="Sumber: buku"`, `footnote-marker`, `title="Sumber: buku"`}, nil},
		{"bookmark", `<div class="adv-bookmark" data-url="https://example.com/x" data-title="Contoh" data-description="Deskripsi" contenteditable="false">` +
			`<a href="https://example.com/x" target="_blank" class="bookmark-link"><div class="bookmark-icon">🔗</div></a></div>`,
			[]string{`adv-bookmark`, `data-url="https://example.com/x"`, `data-title="Contoh"`, `bookmark-link`}, nil},
		{"embed-youtube", `<div class="adv-embed" data-url="https://youtu.be/abcDEF12345" data-embed-url="https://www.youtube.com/embed/abcDEF12345" data-embed-type="iframe" contenteditable="false">` +
			`<div class="embed-toolbar"><span class="embed-label">⧉ Embed</span><button class="embed-edit-btn">✎</button></div>` +
			`<div class="embed-content"><iframe src="https://www.youtube.com/embed/abcDEF12345" frameborder="0" allowfullscreen="" loading="lazy"></iframe></div></div>`,
			[]string{`adv-embed`, `data-embed-url="https://www.youtube.com/embed/abcDEF12345"`, `<iframe src="https://www.youtube.com/embed/abcDEF12345"`,
				`sandbox="allow-scripts allow-same-origin allow-presentation allow-popups"`, `embed-edit-btn`}, nil},
		{"embed-vimeo-spotify", `<iframe src="https://player.vimeo.com/video/123456"></iframe><iframe src="https://open.spotify.com/embed/track/4uLU6hMCjMI75M1A2tKUQC"></iframe>`,
			[]string{`player.vimeo.com/video/123456`, `open.spotify.com/embed/track/4uLU6hMCjMI75M1A2tKUQC`}, nil},
		{"columns", `<div class="adv-columns cols-2" data-count="2" contenteditable="false"><div class="columns-wrapper">` +
			`<div class="column-block" data-col="0"><div class="column-content" contenteditable="true">Kolom 1</div></div>` +
			`<div class="column-block" data-col="1"><div class="column-content" contenteditable="true">Kolom 2</div></div></div>` +
			`<div class="columns-controls"><button class="col-btn" data-count="2">2 Kolom</button></div></div>`,
			[]string{`adv-columns cols-2`, `data-count="2"`, `column-block`, `data-col="1"`, `contenteditable="true"`, `col-btn`, `2 Kolom`}, nil},
		{"toc-id-heading", `<h1 id="heading-0">Bab 1</h1><h2 id="heading-1">Bab 1.1</h2>`,
			[]string{`id="heading-0"`, `id="heading-1"`}, nil},
		{"class-asing-dibuang", `<p class="ql-align-center modal-overlay sidebar">x</p>`,
			[]string{`ql-align-center`}, []string{`modal-overlay`, `sidebar`}},
		{"tag-asing-teks-tetap", `<custom-tag>halo</custom-tag>`, []string{`halo`}, []string{`custom-tag`}},
	}
	for _, c := range cases {
		out := sanitizeNoteHTMLStrict(c.in)
		assertSafeHTML(t, c.name, out)
		for _, m := range c.must {
			if !strings.Contains(out, m) {
				t.Errorf("[%s] hilang %q.\n  masuk : %s\n  keluar: %s", c.name, m, c.in, out)
			}
		}
		for _, m := range c.mustNot {
			if strings.Contains(out, m) {
				t.Errorf("[%s] seharusnya tidak ada %q: %s", c.name, m, out)
			}
		}
	}
}

// Hasil render turunan diganti teks sumber; klien membangun ulang.
func TestSanitizeStripsDerivedRenderings(t *testing.T) {
	katex := `<div class="adv-math math-block" data-formula="E=mc^2" data-display="true" contenteditable="false">` +
		`<div class="math-content"><span class="katex-display"><span class="katex"><span class="katex-mathml"><math xmlns="http://www.w3.org/1998/Math/MathML">` +
		`<semantics><mrow><mi>E</mi></mrow><annotation encoding="application/x-tex">E=mc^2</annotation></semantics></math></span>` +
		`<span class="katex-html" aria-hidden="true"><span class="base"><span class="strut" style="height:0.8641em;"></span><span class="mord mathnormal">E</span></span></span>` +
		`<svg xmlns="http://www.w3.org/2000/svg" width="400em" height="1.08em"><path d="M0 0"/></svg></span></span></div></div>`
	out := sanitizeNoteHTMLStrict(katex)
	assertSafeHTML(t, "katex", out)
	for _, bad := range []string{"katex", "<svg", "<math", "semantics", "annotation", "mord"} {
		if strings.Contains(out, bad) {
			t.Errorf("render KaTeX tidak dibuang (%q): %s", bad, out)
		}
	}
	for _, ok := range []string{`adv-math math-block`, `data-formula="E=mc^2"`, `<div class="math-content">E=mc^2</div>`} {
		if !strings.Contains(out, ok) {
			t.Errorf("sumber rumus hilang (%q): %s", ok, out)
		}
	}

	mer := `<div class="adv-mermaid" data-code="graph TD;&#10;A--&gt;B" contenteditable="false"><div class="mermaid-toolbar"><span class="mermaid-label">Mermaid</span><button class="mermaid-edit-btn">x</button></div>` +
		`<div class="mermaid-content" id="mermaid-1700000000000-ab12cd34e" data-processed="true"><svg id="m1" xmlns="http://www.w3.org/2000/svg"><style>#m1{fill:red}</style>` +
		`<g><foreignObject><div xmlns="http://www.w3.org/1999/xhtml"><img src=x onerror=alert(1)></div></foreignObject></g></svg></div></div>`
	out = sanitizeNoteHTMLStrict(mer)
	assertSafeHTML(t, "mermaid", out)
	for _, bad := range []string{"<svg", "foreignObject", "<style", "data-processed", "onerror=alert"} {
		if strings.Contains(out, bad) {
			t.Errorf("render Mermaid tidak dibuang (%q): %s", bad, out)
		}
	}
	for _, ok := range []string{`adv-mermaid`, `data-code=`, `mermaid-content`, `graph TD;`, `id="mermaid-1700000000000-ab12cd34e"`, `mermaid-edit-btn`} {
		if !strings.Contains(out, ok) {
			t.Errorf("sumber Mermaid hilang (%q): %s", ok, out)
		}
	}

	toc := `<div class="adv-toc" contenteditable="false"><div class="toc-header">Daftar</div><ul class="toc-list"><li class="toc-item"><a href="#heading-0" onclick="x()">Bab</a></li></ul></div>`
	out = sanitizeNoteHTMLStrict(toc)
	if strings.Contains(out, "toc-list") || strings.Contains(out, "onclick") || !strings.Contains(out, "adv-toc") {
		t.Errorf("TOC tidak dikosongkan dengan benar: %s", out)
	}
}

func TestSanitizeNestedStructuresStillFiltered(t *testing.T) {
	// Class asing di dalam blok turunan tetap harus disaring.
	in := `<div class="adv-mermaid" data-code="x"><div class="mermaid-toolbar evil-class"><button class="mermaid-edit-btn big-red">x</button></div><div class="mermaid-content"></div></div>`
	out := sanitizeNoteHTMLStrict(in)
	if strings.Contains(out, "evil-class") || strings.Contains(out, "big-red") {
		t.Errorf("class asing di dalam blok lolos: %s", out)
	}
}

func TestSanitizeIdempotent(t *testing.T) {
	samples := []string{
		`<p>a &amp; b &lt; c &gt; d "q" 'r'</p>`,
		`<ol><li data-list="checked"><span class="ql-ui" contenteditable="false"></span>x</li></ol>`,
		`<a href="https://example.com/?a=1&b=2" target="_blank">x</a>`,
		`<div class="adv-math" data-formula="a&lt;b"><div class="math-content"><span class="katex">x</span></div></div>`,
		`<div class="adv-embed" data-embed-url="https://www.youtube.com/embed/abc"><iframe src="https://www.youtube.com/embed/abc"></iframe></div>`,
		`<p>emoji 😀 dan tab	karakter</p>`,
		`<table class="quill-better-table"><tbody><tr data-row="r1"><td data-row="r1"><p class="qlbt-cell-line">x</p></td></tr></tbody></table>`,
		`<img src="data:image/png;base64,iVBORw0KGgo=" width="10">`,
		`teks polos tanpa tag`,
		``,
	}
	for i, s := range samples {
		once := sanitizeNoteHTMLStrict(s)
		twice := sanitizeNoteHTMLStrict(once)
		if once != twice {
			t.Errorf("sampel %d tidak idempoten:\n  1x: %s\n  2x: %s", i, once, twice)
		}
	}
}

func TestSanitizeEmptyAndPlain(t *testing.T) {
	if got := sanitizeNoteHTMLStrict(""); got != "" {
		t.Errorf("string kosong harus tetap kosong, dapat %q", got)
	}
	if got := sanitizeNoteHTMLStrict("halo dunia"); got != "halo dunia" {
		t.Errorf("teks polos berubah: %q", got)
	}
	if sanitizeMode() != "enforce" {
		t.Errorf("mode bawaan harus enforce, dapat %q", sanitizeMode())
	}
}

func TestSanitizeLargeNoteIsFast(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20000; i++ {
		b.WriteString(`<p>Baris <strong>ke</strong> `)
		b.WriteString(strings.Repeat("x", 40))
		b.WriteString(`</p>`)
	}
	in := b.String() // sekitar 1,5 MB
	start := time.Now()
	out := sanitizeNoteHTMLStrict(in)
	el := time.Since(start)
	if len(out) < len(in)/2 {
		t.Errorf("isi besar terpotong: %d -> %d", len(in), len(out))
	}
	if el > 5*time.Second {
		t.Errorf("sanitasi terlalu lambat: %v untuk %d byte", el, len(in))
	}
	t.Logf("sanitasi %d byte: %v", len(in), el)
}

func TestSafeHTTPURL(t *testing.T) {
	ok := []string{"https://example.com", "http://a.b/c?d=1#e", "https://example.com/a%20b"}
	bad := []string{"", "javascript:alert(1)", "data:text/html,x", "ftp://x.y", "//evil.example", "/relatif", `https://a.com/"x`,
		"https://a.com/<b>", "https://a.com/x y", "vbscript:x", "https://", "http:///x"}
	for _, u := range ok {
		if safeHTTPURL(u) != u {
			t.Errorf("seharusnya diterima: %q", u)
		}
	}
	for _, u := range bad {
		if safeHTTPURL(u) != "" {
			t.Errorf("seharusnya ditolak: %q", u)
		}
	}
	if safeHTTPURL("  https://example.com  ") != "https://example.com" {
		t.Errorf("spasi tepi harus dipangkas")
	}
}
