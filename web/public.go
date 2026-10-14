package web

import (
	"encoding/hex"
	"encoding/xml"
	"errors"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Pnakotic-Codebase/forum.p2p/addr"
	"github.com/Pnakotic-Codebase/forum.p2p/digest"
	"github.com/Pnakotic-Codebase/forum.p2p/node"
	"github.com/Pnakotic-Codebase/forum.p2p/obj"
	"github.com/Pnakotic-Codebase/forum.p2p/store"
	"github.com/Pnakotic-Codebase/forum.p2p/verify"
)

type Public struct {
	Node  *node.Node
	mu    sync.Mutex
	cache map[addr.ID]*verify.Publication
}

func NewPublic(n *node.Node) *Public {
	return &Public{Node: n, cache: map[addr.ID]*verify.Publication{}}
}

const csp = "default-src 'none'; img-src 'self' https: data:; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'"

func secure(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Security-Policy", csp)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
}

func fail(w http.ResponseWriter, status int, msg string) {
	secure(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	pages.ExecuteTemplate(w, "error", map[string]any{"Status": http.StatusText(status), "Message": msg})
}

func (s *Public) load(a addr.ID) (*verify.Publication, error) {
	held, err := s.Node.Store.Held(a)
	if err != nil {
		return nil, err
	}
	if held == nil {
		return nil, store.ErrNotFound
	}
	s.mu.Lock()
	p, ok := s.cache[a]
	s.mu.Unlock()
	if ok && p.Head.Sequence == held.Sequence {
		return p, nil
	}
	p, err = s.Node.Store.Load(a)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cache[a] = p
	s.mu.Unlock()
	return p, nil
}

type docView struct {
	Doc       string
	Title     string
	Date      string
	Published int64
	Entry     obj.Entry
	Rev       *obj.Revision
}

func docs(p *verify.Publication) (posts, pagesOut []docView) {
	for _, e := range p.Head.Entries {
		r := p.Revisions[e.Revision]
		v := docView{Doc: addr.FromDigest(obj.DocumentDigest(p.Digest, e.DocumentNonce)).String(), Title: r.Title,
			Published: r.Published, Entry: e, Rev: r, Date: day(r.Published)}
		if e.Kind == obj.Page {
			pagesOut = append(pagesOut, v)
		} else {
			posts = append(posts, v)
		}
	}
	sort.SliceStable(posts, func(i, j int) bool { return posts[i].Published > posts[j].Published })
	return
}

func day(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format("2 January 2006")
}

func (s *Public) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		fail(w, http.StatusMethodNotAllowed, "Read only.")
		return
	}
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	md := wantsMarkdown(r)
	switch {
	case path == "" || path == "index.md":
		s.index(w, md || path == "index.md")
	case parts[0] == "forum" && len(parts) >= 4 && parts[1] == "v0":
		s.protocol(w, parts[2:])
	case len(parts) == 1:
		s.publication(w, strings.TrimSuffix(parts[0], ".md"), md || strings.HasSuffix(parts[0], ".md"))
	case len(parts) == 2 && parts[1] == "feed.atom":
		s.feed(w, r, parts[0])
	case len(parts) == 2:
		s.document(w, parts[0], parts[1], md)
	case len(parts) == 3 && parts[1] == "asset":
		s.asset(w, parts[0], parts[2])
	default:
		fail(w, http.StatusNotFound, "Nothing at this path.")
	}
}

func (s *Public) index(w http.ResponseWriter, md bool) {
	list, err := s.Node.Store.List()
	if err != nil {
		fail(w, http.StatusInternalServerError, "The store could not be read.")
		return
	}
	if md {
		writeMarkdown(w, IndexMarkdown(list))
		return
	}
	secure(w)
	w.Header().Set("Link", `</index.md>; rel="alternate"; type="text/markdown"`)
	w.Header().Set("Vary", "Accept")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	pages.ExecuteTemplate(w, "index", list)
}

func (s *Public) pub(w http.ResponseWriter, raw string) (*verify.Publication, bool) {
	a, err := addr.Parse(raw)
	if err != nil {
		fail(w, http.StatusNotFound, "That is not a forum:// publication address.")
		return nil, false
	}
	p, err := s.load(a)
	if errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusNotFound, "This host keeps no copy of that publication.")
		return nil, false
	}
	if err != nil {
		fail(w, http.StatusBadGateway, "The stored copy did not verify, so it is not shown.")
		return nil, false
	}
	return p, true
}

func (s *Public) publication(w http.ResponseWriter, raw string, md bool) {
	p, ok := s.pub(w, raw)
	if !ok {
		return
	}
	if md {
		writeMarkdown(w, PublicationMarkdown(p))
		return
	}
	posts, pg := docs(p)
	secure(w)
	w.Header().Set("Link", "</"+p.Address.String()+`.md>; rel="alternate"; type="text/markdown"`)
	w.Header().Set("Vary", "Accept")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	pages.ExecuteTemplate(w, "publication", map[string]any{
		"Title": p.Head.Title, "Description": p.Head.Description, "Address": p.Address.String(),
		"Sequence": p.Head.Sequence, "Posts": posts, "Pages": pg,
	})
}

func find(p *verify.Publication, doc string) (*docView, bool) {
	want, err := addr.Parse(strings.TrimSuffix(doc, ".md"))
	if err != nil {
		return nil, false
	}
	posts, pg := docs(p)
	for _, v := range append(posts, pg...) {
		if v.Doc == want.String() {
			return &v, true
		}
	}
	return nil, false
}

func (s *Public) document(w http.ResponseWriter, rawPub, rawDoc string, md bool) {
	p, ok := s.pub(w, rawPub)
	if !ok {
		return
	}
	v, ok := find(p, rawDoc)
	if !ok {
		fail(w, http.StatusNotFound, "This publication has no such document.")
		return
	}
	secure(w)
	w.Header().Set("X-Forum-Revision", hex.EncodeToString(v.Entry.Revision[:]))
	if md || strings.HasSuffix(rawDoc, ".md") {
		writeMarkdown(w, DocumentMarkdown(p, v))
		return
	}
	w.Header().Set("Link", "</"+p.Address.String()+"/"+v.Doc+`.md>; rel="alternate"; type="text/markdown"`)
	w.Header().Set("Vary", "Accept")
	html, err := renderMarkdown(p.Address.String(), v.Rev.Body)
	if err != nil {
		fail(w, http.StatusInternalServerError, "The document could not be rendered.")
		return
	}
	edited := ""
	if v.Rev.Parent != nil {
		edited = day(v.Rev.Created)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	pages.ExecuteTemplate(w, "document", map[string]any{
		"Title": v.Title, "PubTitle": p.Head.Title, "Address": p.Address.String(), "Doc": v.Doc,
		"Date": v.Date, "Edited": edited, "HTML": template.HTML(html), "Revision": hex.EncodeToString(v.Entry.Revision[:16]),
	})
}

var inlineTypes = map[string]string{
	"image/png": "image/png", "image/jpeg": "image/jpeg", "image/gif": "image/gif",
	"image/webp": "image/webp", "image/avif": "image/avif", "text/plain": "text/plain; charset=utf-8",
}

func (s *Public) asset(w http.ResponseWriter, rawPub, rawID string) {
	if _, ok := s.pub(w, rawPub); !ok {
		return
	}
	id, ok := parseDigest(rawID)
	if !ok {
		fail(w, http.StatusNotFound, "Not an asset id.")
		return
	}
	b, err := s.Node.Store.Asset(id)
	if err != nil {
		fail(w, http.StatusNotFound, "This host keeps no such asset.")
		return
	}
	a, err := obj.DecodeAsset(b)
	if err != nil {
		fail(w, http.StatusInternalServerError, "Stored asset is damaged.")
		return
	}
	secure(w)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if ct, ok := inlineTypes[a.MediaType]; ok {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment")
	}
	w.Write(a.Content)
}

func parseDigest(s string) (digest.Digest, bool) {
	var d digest.Digest
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != digest.Size {
		return d, false
	}
	copy(d[:], b)
	return d, true
}

func (s *Public) protocol(w http.ResponseWriter, parts []string) {
	a, err := addr.Parse(parts[0])
	if err != nil {
		http.Error(w, "bad address", http.StatusNotFound)
		return
	}
	var b []byte
	switch {
	case len(parts) == 2 && parts[1] == "head":
		b, err = s.Node.Store.HeadEnv(a)
	case len(parts) == 2 && parts[1] == "bundle":
		b, err = s.Node.Export(a)
	case len(parts) == 3 && parts[1] == "revision":
		id, ok := parseDigest(parts[2])
		if !ok {
			http.Error(w, "bad id", http.StatusNotFound)
			return
		}
		b, err = s.Node.Store.Revision(id)
	case len(parts) == 3 && parts[1] == "asset":
		id, ok := parseDigest(parts[2])
		if !ok {
			http.Error(w, "bad id", http.StatusNotFound)
			return
		}
		b, err = s.Node.Store.Asset(id)
	default:
		http.Error(w, "unknown object path", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "not held here", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", node.MediaType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(b)
}

type atomFeed struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Link    []atomLink  `xml:"link"`
	Author  atomAuthor  `xml:"author"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomEntry struct {
	Title     string      `xml:"title"`
	ID        string      `xml:"id"`
	Updated   string      `xml:"updated"`
	Published string      `xml:"published"`
	Link      atomLink    `xml:"link"`
	Content   atomContent `xml:"content"`
}

type atomContent struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

func rfc3339(ms int64) string { return time.UnixMilli(ms).UTC().Format(time.RFC3339) }

func (s *Public) feed(w http.ResponseWriter, r *http.Request, raw string) {
	p, ok := s.pub(w, raw)
	if !ok {
		return
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	baseURL := scheme + "://" + r.Host + "/" + p.Address.String()
	f := atomFeed{Title: p.Head.Title, ID: "forum://" + p.Address.String(), Updated: rfc3339(p.Head.Updated),
		Link: []atomLink{{Href: baseURL}, {Href: baseURL + "/feed.atom", Rel: "self"}}, Author: atomAuthor{Name: p.Head.Title}}
	posts, _ := docs(p)
	for _, v := range posts {
		html, _ := renderMarkdown(p.Address.String(), v.Rev.Body)
		f.Entries = append(f.Entries, atomEntry{Title: v.Title, ID: "forum://" + p.Address.String() + "/" + v.Doc,
			Updated: rfc3339(v.Rev.Created), Published: rfc3339(v.Published), Link: atomLink{Href: baseURL + "/" + v.Doc},
			Content: atomContent{Type: "html", Body: html}})
	}
	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Encode(f)
}
