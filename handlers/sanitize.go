package handlers

// ============================================================
// SANITIZE.GO - Sanitasi HTML isi catatan (Prompt 13)
//
// Semua HTML yang berasal dari klien WAJIB melewati sanitizeNoteHTML()
// sebelum disimpan ke database atau dikirim ke halaman publik.
//
// Dua lapis:
//  1. Pass pohon (x/net/html): menyaring class, membuang hasil render
//     turunan (KaTeX, Mermaid, TOC) yang dibangun ulang dari atribut
//     sumbernya di klien, dan memeriksa iframe.
//  2. Whitelist bluemonday: hanya tag/atribut/style yang dihasilkan
//     editor (Quill 2 + better-table + blok kustom 6-advanced.js).
//
// Env SANITIZE_MODE:
//   enforce (bawaan) : simpan hasil sanitasi.
//   log              : simpan ASLI, hanya catat bila ada perubahan.
//                      SEMENTARA, untuk menelusuri whitelist yang terlalu
//                      ketat. Jangan dipakai permanen.
// ============================================================

import (
	"bytes"
	"html"
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/microcosm-cc/bluemonday"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"notebook-app/config"
)

// ---------- Pola ----------

var (
	// Token class yang boleh bertahan (prefix milik Quill/blok kustom/tabel).
	reClassToken = regexp.MustCompile(`^(?:ql-[a-z0-9-]+|adv-[a-z0-9-]+|callout-[a-z0-9-]+|toggle-[a-z0-9-]+|is-open|` +
		`math-[a-z0-9-]+|mermaid-[a-z0-9-]+|footnote-[a-z0-9-]+|bookmark-[a-z0-9-]+|embed-[a-z0-9-]+|` +
		`columns-[a-z0-9-]+|column-[a-z0-9-]+|cols-[0-9]{1,2}|col-btn|toc-[a-z0-9-]+|` +
		`quill-better-table(?:-[a-z0-9-]+)?|qlbt-[a-z0-9-]+|hljs(?:-[a-z0-9-]+)?|` +
		`language-[a-z0-9+#-]{1,20}|wiki-link|mention)$`)

	// Nilai atribut class setelah disaring: token dipisah spasi.
	reClassAttr = regexp.MustCompile(`^[A-Za-z0-9_+# -]{1,400}$`)

	reBool     = regexp.MustCompile(`^(?:true|false)$`)
	reSmallInt = regexp.MustCompile(`^[0-9]{1,4}$`)
	reToken    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	reID       = regexp.MustCompile(`^(?:heading-[0-9]{1,5}|mermaid-[A-Za-z0-9_-]{1,64})$`)
	reDim      = regexp.MustCompile(`^[0-9]{1,4}(?:\.[0-9]{1,3})?(?:px|%)?$`)
	reAnyText  = regexp.MustCompile(`(?s)^.*$`)

	// Warna dan ukuran untuk atribut style.
	reColor = regexp.MustCompile(`^(?:#[0-9a-fA-F]{3,8}|rgba?\(\s*[0-9.%\s,/]{1,40}\)|hsla?\(\s*[0-9.%\s,/a-z]{1,40}\)|` +
		`[a-zA-Z]{3,20}|var\(--[a-zA-Z0-9-]{1,40}\))$`)
	reCSSLen = regexp.MustCompile(`^(?:auto|[0-9]{1,4}(?:\.[0-9]{1,3})?(?:px|%|em|rem)?)$`)

	// URL http(s) atau relatif untuk atribut data-url.
	reSafeURL = regexp.MustCompile(`^(?:https?://[^\s"'<>\x00-\x1f]+|/[^\s"'<>\x00-\x1f]*|#[^\s"'<>\x00-\x1f]*)?$`)

	// Data URI gambar: hanya raster. SVG sengaja ditolak.
	reDataImage = regexp.MustCompile(`(?i)^data:image/(?:png|jpe?g|gif|webp)(?:;base64)?,`)

	// Embed hanya dari penyedia tepercaya (YouTube, Vimeo, Spotify).
	reEmbedURL = regexp.MustCompile(`^(?:https://(?:www\.youtube(?:-nocookie)?\.com/embed/[A-Za-z0-9_-]{1,40}|` +
		`player\.vimeo\.com/video/[0-9]{1,15}|` +
		`open\.spotify\.com/embed/(?:track|album|playlist)/[A-Za-z0-9]{1,40})(?:\?[A-Za-z0-9=&_%.-]{0,200})?)?$`)
)

// Atribut sandbox yang dipasang paksa pada semua iframe embed.
const embedSandbox = "allow-scripts allow-same-origin allow-presentation allow-popups"

// Batas panjang atribut (byte). Atribut yang melebihi dibuang.
var maxAttrLen = map[string]int{
	"data-code":        20000,
	"data-formula":     5000,
	"data-title":       500,
	"data-text":        2000,
	"data-description": 1000,
	"title":            500,
}

const defaultMaxAttrLen = 2100

// ---------- Kebijakan bluemonday ----------

var (
	noteHTMLPolicy     *bluemonday.Policy
	noteHTMLPolicyOnce sync.Once

	sanitizeModeValue string
	sanitizeModeOnce  sync.Once
)

func getNotePolicy() *bluemonday.Policy {
	noteHTMLPolicyOnce.Do(func() { noteHTMLPolicy = buildNoteHTMLPolicy() })
	return noteHTMLPolicy
}

func buildNoteHTMLPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	// Elemen yang dihasilkan editor.
	p.AllowElements(
		"p", "br", "hr", "div", "span", "blockquote", "pre", "code",
		"strong", "b", "em", "i", "u", "s", "strike", "del", "ins", "sub", "sup", "mark", "small",
		"ul", "ol", "li", "h1", "h2", "h3", "h4", "h5", "h6",
		"table", "thead", "tbody", "tfoot", "tr", "td", "th", "colgroup", "col", "caption",
		"details", "summary", "button", "a", "img", "iframe",
	)
	p.SkipElementsContent("script", "style", "noscript", "template", "object", "applet", "embed", "textarea", "title")

	// URL.
	p.AllowURLSchemes("http", "https", "mailto", "tel")
	p.AllowRelativeURLs(true)
	p.AllowDataURIImages()
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)

	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("target").Matching(regexp.MustCompile(`^_blank$`)).OnElements("a")
	p.AllowAttrs("src").OnElements("img")
	p.AllowAttrs("alt").Matching(reAnyText).OnElements("img")
	p.AllowAttrs("width", "height").Matching(reDim).OnElements("img", "col", "td", "th", "table", "iframe")

	// Global.
	p.AllowAttrs("class").Matching(reClassAttr).Globally()
	p.AllowAttrs("title").Matching(reAnyText).Globally()
	p.AllowAttrs("id").Matching(reID).Globally()
	p.AllowAttrs("dir").Matching(regexp.MustCompile(`^(?:ltr|rtl)$`)).Globally()
	p.AllowAttrs("contenteditable", "spellcheck").Matching(reBool).Globally()

	// Struktur Quill / tabel.
	p.AllowAttrs("colspan", "rowspan").Matching(reSmallInt).OnElements("td", "th")
	p.AllowAttrs("start").Matching(reSmallInt).OnElements("ol")
	p.AllowAttrs("data-list").Matching(regexp.MustCompile(`^(?:bullet|ordered|checked|unchecked)$`)).OnElements("li")
	p.AllowAttrs("data-language").Matching(regexp.MustCompile(`^[A-Za-z0-9+#_-]{1,30}$`)).OnElements("div", "pre")
	p.AllowAttrs("data-row", "data-cell").Matching(reToken).OnElements("tr", "td", "th", "p", "div")
	p.AllowAttrs("data-rowspan", "data-colspan").Matching(reSmallInt).OnElements("p", "div", "td", "th")

	// Blok kustom (6-advanced.js).
	p.AllowAttrs("data-type").Matching(regexp.MustCompile(`^[a-z]{1,20}$`)).OnElements("div")
	p.AllowAttrs("data-title", "data-description", "data-text", "data-code", "data-formula").
		Matching(reAnyText).OnElements("div", "sup")
	p.AllowAttrs("data-display", "data-open", "data-toc").Matching(reBool).OnElements("div")
	p.AllowAttrs("data-count", "data-col", "data-number").Matching(reSmallInt).OnElements("div", "sup", "button")
	p.AllowAttrs("data-url").Matching(reSafeURL).OnElements("div")
	p.AllowAttrs("data-embed-url").Matching(reEmbedURL).OnElements("div")
	p.AllowAttrs("data-embed-type").Matching(regexp.MustCompile(`^(?:iframe|link)$`)).OnElements("div")
	p.AllowAttrs("type").Matching(regexp.MustCompile(`^button$`)).OnElements("button")

	// iframe embed.
	p.AllowAttrs("src").Matching(reEmbedURL).OnElements("iframe")
	p.AllowAttrs("sandbox").Matching(regexp.MustCompile(`^` + regexp.QuoteMeta(embedSandbox) + `$`)).OnElements("iframe")
	p.AllowAttrs("frameborder").Matching(regexp.MustCompile(`^[01]$`)).OnElements("iframe")
	p.AllowAttrs("loading").Matching(regexp.MustCompile(`^(?:lazy|eager)$`)).OnElements("iframe")
	p.AllowAttrs("allowfullscreen").Matching(regexp.MustCompile(`^(?:|allowfullscreen|true)$`)).OnElements("iframe")

	// Style: hanya properti aman dengan nilai tervalidasi.
	p.AllowStyles("color", "background-color").Matching(reColor).Globally()
	p.AllowStyles("text-align").Matching(regexp.MustCompile(`^(?:left|right|center|justify)$`)).Globally()
	p.AllowStyles("display").Matching(regexp.MustCompile(`^(?:none|block)$`)).Globally()
	p.AllowStyles("width", "height", "max-width").Matching(reCSSLen).Globally()

	return p
}

// ---------- Pass pohon ----------

func nodeAttr(n *xhtml.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val, true
		}
	}
	return "", false
}

func setNodeAttr(n *xhtml.Node, key, val string) {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, xhtml.Attribute{Key: key, Val: val})
}

func delNodeAttr(n *xhtml.Node, key string) {
	out := n.Attr[:0]
	for _, a := range n.Attr {
		if a.Key != key {
			out = append(out, a)
		}
	}
	n.Attr = out
}

func nodeHasClass(n *xhtml.Node, class string) bool {
	v, ok := nodeAttr(n, "class")
	if !ok {
		return false
	}
	for _, t := range strings.Fields(v) {
		if t == class {
			return true
		}
	}
	return false
}

func findByClass(n *xhtml.Node, class string) *xhtml.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.ElementNode {
			if nodeHasClass(c, class) {
				return c
			}
			if f := findByClass(c, class); f != nil {
				return f
			}
		}
	}
	return nil
}

func replaceChildrenWithText(n *xhtml.Node, text string) {
	for n.FirstChild != nil {
		n.RemoveChild(n.FirstChild)
	}
	if text != "" {
		n.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: text})
	}
}

func clearChildren(n *xhtml.Node) {
	for n.FirstChild != nil {
		n.RemoveChild(n.FirstChild)
	}
}

// normalizeNode menyaring satu elemen dan turunannya.
func normalizeNode(n *xhtml.Node) {
	if n.Type != xhtml.ElementNode {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			normalizeNode(c)
			c = next
		}
		return
	}

	// 1. Saring token class.
	if v, ok := nodeAttr(n, "class"); ok {
		keep := make([]string, 0, 4)
		for _, t := range strings.Fields(v) {
			if reClassToken.MatchString(t) {
				keep = append(keep, t)
			}
		}
		if len(keep) == 0 {
			delNodeAttr(n, "class")
		} else {
			setNodeAttr(n, "class", strings.Join(keep, " "))
		}
	}

	// 2. Buang atribut yang terlalu panjang.
	for _, a := range append([]xhtml.Attribute(nil), n.Attr...) {
		limit, ok := maxAttrLen[a.Key]
		if !ok {
			limit = defaultMaxAttrLen
		}
		if a.Key == "src" && n.Data == "img" {
			continue // data URI gambar boleh besar; dibatasi oleh ukuran request
		}
		if len(a.Val) > limit {
			delNodeAttr(n, a.Key)
		}
	}

	// 2b. Gambar: data URI hanya untuk format raster.
	if n.Data == "img" {
		if src, ok := nodeAttr(n, "src"); ok {
			s := strings.TrimSpace(src)
			if strings.HasPrefix(strings.ToLower(s), "data:") && !reDataImage.MatchString(s) {
				delNodeAttr(n, "src")
			}
		}
	}

	// 3. Hasil render turunan diganti teks sumber. Klien membangun ulang
	//    dari data-formula / data-code / daftar heading.
	switch {
	case n.Data == "div" && nodeHasClass(n, "adv-math"):
		if mc := findByClass(n, "math-content"); mc != nil {
			f, _ := nodeAttr(n, "data-formula")
			replaceChildrenWithText(mc, f)
		}
	case n.Data == "div" && nodeHasClass(n, "adv-mermaid"):
		if mc := findByClass(n, "mermaid-content"); mc != nil {
			code, _ := nodeAttr(n, "data-code")
			replaceChildrenWithText(mc, code)
			delNodeAttr(mc, "data-processed")
		}
	case n.Data == "div" && nodeHasClass(n, "adv-toc"):
		clearChildren(n)
	}

	// 4. iframe: hanya embed tepercaya, dengan sandbox paksa.
	if n.Data == "iframe" {
		src, _ := nodeAttr(n, "src")
		if src == "" || !reEmbedURL.MatchString(src) {
			// Ubah jadi node teks kosong (aman untuk node level atas yang tak punya Parent).
			clearChildren(n)
			n.Type, n.Data, n.DataAtom, n.Attr = xhtml.TextNode, "", 0, nil
			return
		}
		clearChildren(n)
		setNodeAttr(n, "sandbox", embedSandbox)
		return
	}

	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		normalizeNode(c)
		c = next
	}
}

// preprocessNoteHTML menjalankan pass pohon. Bila parsing gagal, kembalikan
// string kosong (lebih aman daripada meneruskan HTML yang tidak bisa dibaca).
func preprocessNoteHTML(s string) (string, bool) {
	ctx := &xhtml.Node{Type: xhtml.ElementNode, DataAtom: atom.Div, Data: "div"}
	nodes, err := xhtml.ParseFragment(strings.NewReader(s), ctx)
	if err != nil {
		return "", false
	}
	var buf bytes.Buffer
	for _, n := range nodes {
		normalizeNode(n)
	}
	for _, n := range nodes {
		if err := xhtml.Render(&buf, n); err != nil {
			return "", false
		}
	}
	return buf.String(), true
}

// ---------- API ----------

func sanitizeMode() string {
	sanitizeModeOnce.Do(func() {
		sanitizeModeValue = strings.ToLower(strings.TrimSpace(config.GetEnv("SANITIZE_MODE", "enforce")))
		if sanitizeModeValue != "log" {
			sanitizeModeValue = "enforce"
		}
	})
	return sanitizeModeValue
}

// sanitizeNoteHTMLStrict selalu menerapkan sanitasi (tanpa mode).
func sanitizeNoteHTMLStrict(s string) string {
	if s == "" {
		return ""
	}
	pre, ok := preprocessNoteHTML(s)
	if !ok {
		return ""
	}
	return getNotePolicy().Sanitize(pre)
}

// sanitizeNoteHTML dipakai di semua jalur tulis isi catatan dan output publik.
func sanitizeNoteHTML(s string) string {
	out := sanitizeNoteHTMLStrict(s)
	if sanitizeMode() == "log" {
		if out != s {
			log.Printf("[sanitize] isi berubah: %d -> %d byte (SANITIZE_MODE=log, isi asli disimpan)", len(s), len(out))
		}
		return s
	}
	return out
}

// escHTML meng-escape teks biasa untuk disisipkan ke HTML.
func escHTML(s string) string { return html.EscapeString(s) }

// safeHTTPURL mengembalikan URL bila skemanya http/https dan bentuknya wajar,
// selain itu string kosong.
func safeHTTPURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2000 {
		return ""
	}
	for _, r := range raw {
		if r < 0x20 || r == '"' || r == '\'' || r == '<' || r == '>' || r == ' ' {
			return ""
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return raw
}
