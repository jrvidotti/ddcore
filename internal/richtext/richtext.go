// Package richtext is the one place the framework decides what markup a
// document may hold.
//
// A `Text Editor` field stores HTML, and the server cleans it on the way in
// (`castValueWith`) rather than on the way out, so the value in the database is
// already the value every reader gets: the API, print, export and the desk all
// see the same bytes. `Markdown Editor` stores its source instead and is
// rendered here, late, through the same allowlist.
//
// The allowlist is deliberately the set the desk's editor can round-trip. A tag
// the server keeps but the editor cannot represent is data a user destroys by
// opening the form and pressing save.
package richtext

import (
	"html"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
)

// imageSrcRe is what an <img> may point at: a file this site serves. A printed
// document is rendered by headless Chrome or Gotenberg against the site's own
// base address, so an external URL would make that renderer — a server, not the
// reader's browser — fetch an address the document's author chose, and would
// turn every reader into a hit on somebody else's server. Allowing it is a
// later opt-in, not a default.
//
// No path segment may begin with a dot, which keeps `..` out of a path that is
// resolved against the site.
var imageSrcRe = regexp.MustCompile(`^/(private/)?files/[A-Za-z0-9_%+-][A-Za-z0-9._%+-]*(/[A-Za-z0-9_%+-][A-Za-z0-9._%+-]*)*$`)

// codeClassRe keeps the one class the editor writes, so a code block survives a
// round trip with its language.
var codeClassRe = regexp.MustCompile(`^language-[a-zA-Z0-9+#_.-]{1,20}$`)

// olStartRe: a numbered list that does not start at 1 is a list the user
// deliberately continued.
var olStartRe = regexp.MustCompile(`^[0-9]{1,6}$`)

// dimensionRe: a bare number of pixels, which is what a resized image carries.
var dimensionRe = regexp.MustCompile(`^[0-9]{1,4}$`)

var (
	htmlPolicy     = newPolicy(false)
	markdownPolicy = newPolicy(true)
)

// newPolicy builds the allowlist. `wide` adds what a Markdown document can
// produce but the rich-text editor cannot: deeper headings and tables.
func newPolicy(wide bool) *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "hr", "blockquote", "h1", "h2", "h3", "h4",
		"strong", "b", "em", "i", "u", "s", "del", "code", "pre", "ul", "li")
	p.AllowAttrs("start").Matching(olStartRe).OnElements("ol")
	p.AllowElements("ol")
	p.AllowAttrs("class").Matching(codeClassRe).OnElements("code")

	p.AllowAttrs("href", "title").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto", "tel")
	p.AllowRelativeURLs(true)
	p.RequireParseableURLs(true)
	// A link out of the site opens in a new tab and carries no referrer or
	// ranking. These are the same attributes the desk's editor writes, which is
	// what keeps a saved document byte-identical to what was loaded.
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.RequireNoReferrerOnFullyQualifiedLinks(true)
	p.RequireNoFollowOnFullyQualifiedLinks(true)

	p.AllowAttrs("src").Matching(imageSrcRe).OnElements("img")
	p.AllowAttrs("alt", "title").OnElements("img")
	p.AllowAttrs("width", "height").Matching(dimensionRe).OnElements("img")

	if wide {
		p.AllowElements("h5", "h6", "table", "thead", "tbody", "tr", "th", "td")
		p.AllowAttrs("align").Matching(regexp.MustCompile(`^(left|right|center)$`)).OnElements("th", "td")
	}
	return p
}

// Sanitize removes everything outside the allowlist. It is idempotent:
// Sanitize(Sanitize(x)) == Sanitize(x), which is what lets a document be saved
// twice without the second save looking like an edit.
func Sanitize(s string) string { return strings.TrimSpace(htmlPolicy.Sanitize(s)) }

// A value is markup when it carries a *closing* tag, or a void element with an
// attribute. Everything this package writes, and everything an editor writes,
// satisfies one of those.
//
// The narrow test is the point. A single opening tag is not enough, because
// `compare a<b and b>c` contains one — `b` is a tag name and " and b" reads as
// two attributes — and treating that sentence as markup deletes the words
// between the brackets, permanently and without an error. Prose that mentions
// `</p>` or `<img src=…>` is read as markup instead, which loses the literal
// tag but keeps the sentence.
var (
	closingTagRe = regexp.MustCompile(`(?i)</(p|div|blockquote|h[1-6]|strong|b|em|i|u|s|del|code|pre|ul|ol|li|a|table|thead|tbody|tr|th|td|span)\s*>`)
	voidTagRe    = regexp.MustCompile(`(?i)<(br|hr)\s*/?>|<img\s[^<>]*=[^<>]*>`)
)

// LooksLikeHTML reports whether the value carries markup this package knows.
func LooksLikeHTML(s string) bool {
	return closingTagRe.MatchString(s) || voidTagRe.MatchString(s)
}

var newlines = regexp.MustCompile(`\r\n?`)

// FromPlainText turns text into markup: blank lines separate paragraphs and a
// single newline is a break. Everything else is escaped, so text that happens
// to contain `<` keeps it.
func FromPlainText(s string) string {
	s = newlines.ReplaceAllString(s, "\n")
	var b strings.Builder
	for _, para := range strings.Split(s, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		b.WriteString("<p>")
		b.WriteString(strings.ReplaceAll(escapeText(para), "\n", "<br>"))
		b.WriteString("</p>")
	}
	return b.String()
}

// escapeText escapes what a text node must escape and nothing else. Go's
// html.EscapeString also rewrites quotes and apostrophes, which matter inside
// an attribute and not here: a comment reading "applied action 'Approve'"
// would be stored, exported and diffed full of &#39;.
var textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escapeText(s string) string { return textEscaper.Replace(s) }

// Normalize is what a Text Editor value goes through on its way into the
// database: markup is cleaned, and anything else is read as the plain text it
// is. Values written before rich text existed arrive here as plain text and
// come out as paragraphs.
func Normalize(s string) string {
	if !LooksLikeHTML(s) {
		return Sanitize(FromPlainText(s))
	}
	out := Sanitize(s)
	// Cleaning can leave text with no block around it — `<div onclick=…>x</div>`
	// becomes `x` — and that text would not look like markup to the next pass,
	// which would then escape its entities a second time. Wrapping it here
	// settles the value in one step instead.
	if out != "" && !blockLevelRe.MatchString(out) {
		out = "<p>" + out + "</p>"
	}
	return out
}

// blockLevelRe answers whether cleaned markup already carries a block, which is
// what the editor and print expect at the top level.
var blockLevelRe = regexp.MustCompile(`(?i)<(p|h[1-6]|ul|ol|li|blockquote|pre|hr|table)(\s[^<>]*)?/?>`)

// IsEmpty reports whether cleaned markup carries nothing a reader would see.
// Every editor leaves `<p></p>` behind when a field is cleared, and a required
// field that accepts it is a required field in name only.
func IsEmpty(s string) bool {
	// an <img> the allowlist stripped of its src shows nothing, so it does not
	// count as content and cannot satisfy `reqd`
	if imgWithSrcRe.MatchString(s) {
		return false
	}
	return strings.TrimSpace(ToText(s)) == ""
}

var imgWithSrcRe = regexp.MustCompile(`(?i)<img\s[^<>]*src\s*=`)

var (
	blockBoundaryRe = regexp.MustCompile(`(?i)</(p|div|h[1-6]|li|tr|blockquote|pre)>|<br\s*/?>`)
	tagRe           = regexp.MustCompile(`<[^>]*>`)
	spaceRunRe      = regexp.MustCompile(`[ \t]+`)
	blankRunRe      = regexp.MustCompile(`\n{3,}`)
)

// ToText reduces markup to readable text, for a list cell, a child-table cell,
// a version diff and anywhere else a single line is wanted. It does not parse
// HTML: it runs over values the allowlist already cleaned, and its output is
// text, never markup.
func ToText(s string) string {
	s = blockBoundaryRe.ReplaceAllString(s, "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = spaceRunRe.ReplaceAllString(s, " ")
	s = blankRunRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

var md = goldmark.New(
	goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.Linkify),
	goldmark.WithRendererOptions(goldmarkhtml.WithHardWraps()),
)

// Markdown renders a Markdown source to HTML. goldmark is not given
// `WithUnsafe`, so raw HTML in the source is already dropped; the result goes
// through the allowlist anyway, because "already safe" is a claim that should
// not have to be re-verified at every call site.
func Markdown(s string) string {
	var b strings.Builder
	if err := md.Convert([]byte(s), &b); err != nil {
		// goldmark fails only on a writer error, which a strings.Builder does
		// not have. Falling back to the escaped source keeps the text readable
		// instead of losing it.
		return Sanitize(FromPlainText(s))
	}
	return strings.TrimSpace(markdownPolicy.Sanitize(b.String()))
}
