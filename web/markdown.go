package web

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Pnakotic-Codebase/forum.p2p/addr"
	"github.com/Pnakotic-Codebase/forum.p2p/store"
	"github.com/Pnakotic-Codebase/forum.p2p/verify"
)

func isoDay(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format("2006-01-02")
}

func isoTime(ms int64) string { return time.UnixMilli(ms).UTC().Format(time.RFC3339) }

func yamlString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}

func IndexMarkdown(list []store.Summary) string {
	var b strings.Builder
	b.WriteString("# forum://\n\nPublications this host keeps a verified copy of. Every one can be read as Markdown by appending `.md` to its path, or by asking for `text/markdown`.\n\n")
	if len(list) == 0 {
		b.WriteString("Nothing here yet.\n")
	}
	for _, s := range list {
		t := s.Title
		if t == "" {
			t = "(untitled)"
		}
		fmt.Fprintf(&b, "- [%s](/%s.md): `forum://%s`, head %d\n", mdText(t), s.Address, s.Address, s.Sequence)
	}
	return b.String()
}

func PublicationMarkdown(p *verify.Publication) string {
	var b strings.Builder
	a := p.Address.String()
	fmt.Fprintf(&b, "---\nforum: %s\naddress: \"forum://%s\"\ntitle: %s\nverified: true\nhead_sequence: %d\nupdated: %s\nknown_copies: [%s]\n---\n\n",
		"v0", a, yamlString(p.Head.Title), p.Head.Sequence, isoTime(p.Head.Updated), quoteList(p.Head.KnownCopies))
	fmt.Fprintf(&b, "# %s\n\n", p.Head.Title)
	if p.Head.Description != "" {
		b.WriteString(p.Head.Description + "\n\n")
	}
	posts, pg := docs(p)
	b.WriteString("## Posts\n\n")
	if len(posts) == 0 {
		b.WriteString("None yet.\n")
	}
	for _, v := range posts {
		fmt.Fprintf(&b, "- %s [%s](/%s/%s.md): `forum://%s/%s`\n", isoDay(v.Published), mdText(v.Title), a, v.Doc, a, v.Doc)
	}
	if len(pg) > 0 {
		b.WriteString("\n## Pages\n\n")
		for _, v := range pg {
			fmt.Fprintf(&b, "- [%s](/%s/%s.md): `forum://%s/%s`\n", mdText(v.Title), a, v.Doc, a, v.Doc)
		}
	}
	return b.String()
}

func DocumentMarkdown(p *verify.Publication, v *docView) string {
	var b strings.Builder
	a := p.Address.String()
	r := v.Rev
	parent := ""
	if r.Parent != nil {
		parent = hex.EncodeToString(r.Parent[:])
	}
	fmt.Fprintf(&b, "---\nforum: v0\naddress: \"forum://%s/%s\"\npublication: \"forum://%s\"\npublication_title: %s\nkind: %s\ntitle: %s\npublished: %s\nrevised: %s\nrevision: %s\nparent_revision: %s\nassets: %d\nverified: true\n---\n\n",
		a, v.Doc, a, yamlString(p.Head.Title), v.Entry.Kind, yamlString(r.Title), isoTime(r.Published), isoTime(r.Created),
		hex.EncodeToString(v.Entry.Revision[:]), parent, len(r.Assets))
	fmt.Fprintf(&b, "# %s\n\n", r.Title)
	b.WriteString(r.Body)
	if !strings.HasSuffix(r.Body, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func mdText(s string) string {
	return strings.NewReplacer(`[`, `\[`, `]`, `\]`).Replace(s)
}

func quoteList(xs []string) string {
	q := make([]string, len(xs))
	for i, x := range xs {
		q[i] = yamlString(x)
	}
	return strings.Join(q, ", ")
}

func wantsMarkdown(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	md := strings.Index(accept, "text/markdown")
	if md < 0 {
		return false
	}
	html := strings.Index(accept, "text/html")
	return html < 0 || md < html
}

func writeMarkdown(w http.ResponseWriter, body string) {
	h := w.Header()
	h.Set("Content-Type", "text/markdown; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Vary", "Accept")
	w.Write([]byte(body))
}

func LoadDocument(p *verify.Publication, doc string) (*docView, bool) { return find(p, doc) }

func ParseDocument(s string) (addr.ID, error) { return addr.Parse(strings.TrimSuffix(s, ".md")) }
