package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/Pnakotic-Codebase/forum.p2p/addr"
	"github.com/Pnakotic-Codebase/forum.p2p/obj"
)

type Writer struct {
	Public *Public
	token  string
}

func NewWriter(p *Public) *Writer {
	b := make([]byte, 32)
	rand.Read(b)
	return &Writer{Public: p, token: hex.EncodeToString(b)}
}

func loopbackHost(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	h = strings.Trim(h, "[]")
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func (wr *Writer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !loopbackHost(r.Host) {
		fail(w, http.StatusForbidden, "The writer answers only on a loopback address.")
		return
	}
	if ra, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if ip := net.ParseIP(ra); ip == nil || !ip.IsLoopback() {
			fail(w, http.StatusForbidden, "The writer answers only to this machine.")
			return
		}
	}
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/w", http.StatusSeeOther)
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/w") {
		wr.Public.ServeHTTP(w, r)
		return
	}
	if r.Method == http.MethodPost {
		if !wr.sameOrigin(r) || subtle.ConstantTimeCompare([]byte(r.FormValue("token")), []byte(wr.token)) != 1 {
			fail(w, http.StatusForbidden, "This form is stale or came from somewhere else. Reload the writer and try again.")
			return
		}
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/w"), "/"), "/")
	switch {
	case parts[0] == "" && r.Method == http.MethodGet:
		wr.home(w, r.URL.Query().Get("m"))
	case parts[0] == "new" && r.Method == http.MethodPost:
		wr.newPub(w, r)
	case parts[0] == "follow" && r.Method == http.MethodPost:
		wr.follow(w, r)
	case parts[0] == "sync" && r.Method == http.MethodPost:
		wr.sync(w, r)
	default:
		a, err := addr.Parse(parts[0])
		if err != nil {
			fail(w, http.StatusNotFound, "Not a publication address.")
			return
		}
		wr.pub(w, r, a, parts[1:])
	}
}

func (wr *Writer) sameOrigin(r *http.Request) bool {
	for _, h := range []string{"Origin", "Referer"} {
		v := r.Header.Get(h)
		if v == "" || v == "null" {
			continue
		}
		u, err := url.Parse(v)
		if err != nil || u.Host != r.Host {
			return false
		}
	}
	return true
}

func back(w http.ResponseWriter, r *http.Request, path, msg string) {
	http.Redirect(w, r, path+"?m="+url.QueryEscape(msg), http.StatusSeeOther)
}

type ownedView struct {
	Address, Title, Label string
}

func (wr *Writer) home(w http.ResponseWriter, flash string) {
	n := wr.Public.Node
	owned, err := n.Store.OwnedList()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	var views []ownedView
	for _, o := range owned {
		v := ownedView{Address: o.Address.String(), Label: o.Label}
		if p, err := wr.Public.load(o.Address); err == nil {
			v.Title = p.Head.Title
		}
		views = append(views, v)
	}
	events, _ := n.Store.Events(20)
	secure(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	writerPages.ExecuteTemplate(w, "w-home", map[string]any{"Owned": views, "Token": wr.token, "Flash": flash, "Events": events})
}

func (wr *Writer) newPub(w http.ResponseWriter, r *http.Request) {
	a, err := wr.Public.Node.NewPublication(r.FormValue("salt"), r.FormValue("title"), r.FormValue("description"), nil)
	if err != nil {
		back(w, r, "/w", err.Error())
		return
	}
	back(w, r, "/w/"+a.String(), "Created forum://"+a.String())
}

func (wr *Writer) follow(w http.ResponseWriter, r *http.Request) {
	a, err := addr.Parse(strings.TrimPrefix(strings.TrimSpace(r.FormValue("address")), "forum://"))
	if err != nil {
		back(w, r, "/w", "That is not a forum:// publication address.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	res, errs := wr.Public.Node.Follow(ctx, a, r.FormValue("from"))
	back(w, r, "/w", describe(a, res, errs))
}

func (wr *Writer) sync(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	reports, err := wr.Public.Node.Sync(ctx)
	if err != nil {
		back(w, r, "/w", err.Error())
		return
	}
	var msgs []string
	for _, rep := range reports {
		msgs = append(msgs, describe(rep.Address, rep.Result, rep.Errors))
	}
	if len(msgs) == 0 {
		msgs = append(msgs, "Nothing followed yet.")
	}
	back(w, r, "/w", strings.Join(msgs, " · "))
}

func (wr *Writer) pub(w http.ResponseWriter, r *http.Request, a addr.ID, rest []string) {
	n := wr.Public.Node
	if _, err := n.Store.GetOwned(a); err != nil {
		fail(w, http.StatusNotFound, "That publication is not yours on this machine.")
		return
	}
	p, err := wr.Public.load(a)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	self := "/w/" + a.String()
	switch {
	case len(rest) == 0 && r.Method == http.MethodGet:
		posts, pg := docs(p)
		secure(w)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		writerPages.ExecuteTemplate(w, "w-pub", map[string]any{"Address": a.String(), "Title": p.Head.Title,
			"Description": p.Head.Description, "Copies": p.Head.KnownCopies, "Docs": append(posts, pg...),
			"Token": wr.token, "Flash": r.URL.Query().Get("m")})
	case len(rest) == 2 && rest[0] == "edit" && r.Method == http.MethodGet:
		view := map[string]any{"Address": a.String(), "PubTitle": p.Head.Title, "Token": wr.token}
		if rest[1] != "new" {
			v, ok := find(p, rest[1])
			if !ok {
				fail(w, http.StatusNotFound, "No such document.")
				return
			}
			view["Doc"], view["Title"], view["Body"] = v.Doc, v.Title, v.Rev.Body
		}
		secure(w)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		writerPages.ExecuteTemplate(w, "w-edit", view)
	case len(rest) == 1 && rest[0] == "publish" && r.Method == http.MethodPost:
		kind := obj.Post
		if r.FormValue("kind") == "page" {
			kind = obj.Page
		}
		doc, err := n.Publish(a, nodeDraft(r.FormValue("doc"), kind, r.FormValue("title"), r.FormValue("body")))
		if err != nil {
			back(w, r, self, "Not published: "+err.Error())
			return
		}
		back(w, r, self, "Published forum://"+a.String()+"/"+doc.String())
	case len(rest) == 1 && rest[0] == "remove" && r.Method == http.MethodPost:
		if err := n.Remove(a, r.FormValue("doc")); err != nil {
			back(w, r, self, err.Error())
			return
		}
		back(w, r, self, "Removed from the publication.")
	case len(rest) == 1 && rest[0] == "meta" && r.Method == http.MethodPost:
		title, desc := r.FormValue("title"), r.FormValue("description")
		var copies []string
		for _, l := range strings.Split(r.FormValue("copies"), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				copies = append(copies, l)
			}
		}
		if err := n.SetMeta(a, metaOf(&title, &desc, copies)); err != nil {
			back(w, r, self, err.Error())
			return
		}
		back(w, r, self, "Details saved and signed.")
	case len(rest) == 1 && rest[0] == "asset" && r.Method == http.MethodPost:
		f, hdr, err := r.FormFile("file")
		if err != nil {
			back(w, r, self, "No file received.")
			return
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, 64<<20))
		if err != nil {
			back(w, r, self, err.Error())
			return
		}
		mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(hdr.Filename)))
		if mt == "" {
			mt = http.DetectContentType(b)
		}
		mt = strings.SplitN(mt, ";", 2)[0]
		id, err := n.AddAsset(mt, b)
		if err != nil {
			back(w, r, self, err.Error())
			return
		}
		back(w, r, self, fmt.Sprintf("Stored. Paste into a document: ![%s](asset:%s)", hdr.Filename, hex.EncodeToString(id[:])))
	default:
		fail(w, http.StatusNotFound, "Nothing at this path.")
	}
}
