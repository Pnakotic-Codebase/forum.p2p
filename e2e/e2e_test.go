package e2e

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Pnakotic-Codebase/forum.p2p/addr"
	"github.com/Pnakotic-Codebase/forum.p2p/node"
	"github.com/Pnakotic-Codebase/forum.p2p/obj"
	"github.com/Pnakotic-Codebase/forum.p2p/web"
)

func open(t *testing.T) *node.Node {
	t.Helper()
	n, err := node.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { n.Close() })
	return n
}

func host(t *testing.T, n *node.Node) *httptest.Server {
	s := httptest.NewServer(web.NewPublic(n))
	t.Cleanup(s.Close)
	return s
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func fetchOK(t *testing.T, n *node.Node, a addr.ID, from string) *node.FetchResult {
	t.Helper()
	r, errs := n.Fetch(context.Background(), a, from)
	if r == nil || r.Failure != nil {
		t.Fatalf("fetch from %s failed: %v %v", from, r, errs)
	}
	return r
}

func TestPublishReplicateTamper(t *testing.T) {
	ctx := context.Background()
	author := open(t)
	words, err := author.Init()
	must(t, err)
	pub, err := author.NewPublication("", "Notes", "A test publication", nil)
	must(t, err)
	img := []byte("\x89PNG\r\n\x1a\nfake")
	aid, err := author.AddAsset("image/png", img)
	must(t, err)
	body := "Hello. ![dot](asset:" + hexs(aid[:]) + ")\n"
	doc, err := author.Publish(pub, node.Draft{Kind: obj.Post, Title: "One", Body: body})
	must(t, err)
	_, err = author.Publish(pub, node.Draft{Kind: obj.Page, Title: "About", Body: "About."})
	must(t, err)
	hostA := host(t, author)

	replica := open(t)
	r, errs := replica.Follow(ctx, pub, hostA.URL)
	if r == nil || r.Failure != nil {
		t.Fatalf("follow failed: %v", errs)
	}
	hostB := host(t, replica)

	reader := open(t)
	fetchOK(t, reader, pub, hostB.URL)

	resp, err := http.Get(hostB.URL + "/" + pub.String() + "/" + doc.String())
	must(t, err)
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Contains(page, []byte("Hello.")) || !bytes.Contains(page, []byte("/"+pub.String()+"/asset/")) {
		t.Fatalf("replica did not render the post: %s", page)
	}

	_, err = author.Publish(pub, node.Draft{Document: doc.String(), Title: "One", Body: "Hello, edited.\n"})
	must(t, err)
	reports, err := replica.Sync(ctx)
	must(t, err)
	if len(reports) != 1 || reports[0].Result == nil || reports[0].Result.Failure != nil || reports[0].Result.Publication.Head.Sequence != 4 {
		t.Fatalf("sync did not bring head 4: %+v", reports)
	}

	tamper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		up, err := http.Get(hostA.URL + req.URL.Path)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		b, _ := io.ReadAll(up.Body)
		up.Body.Close()
		if strings.Contains(req.URL.Path, "/revision/") {
			b = bytes.Replace(b, []byte("edited"), []byte("forged"), 1)
		}
		w.Write(b)
	}))
	defer tamper.Close()
	victim := open(t)
	res, _ := victim.FetchFrom(ctx, pub, tamper.URL)
	if res == nil || res.Failure == nil || res.Failure.Check != "revision id" {
		t.Fatalf("tampered revision not caught: %+v", res)
	}
	if held, _ := victim.Store.Held(pub); held != nil {
		t.Fatal("unverified material was stored")
	}
	ev, _ := victim.Store.Events(5)
	if len(ev) == 0 || ev[0].Verified {
		t.Fatal("unverified fetch was not logged")
	}

	staleHead, err := reader.Store.HeadEnv(pub)
	must(t, err)
	fetchOK(t, reader, pub, hostA.URL)
	rollback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/head") {
			w.Write(staleHead)
			return
		}
		http.Redirect(w, req, hostA.URL+req.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer rollback.Close()
	res, _ = reader.FetchFrom(ctx, pub, rollback.URL)
	if res == nil || res.Failure == nil || res.Failure.Check != "head sequence" {
		t.Fatalf("rollback not caught: %+v", res)
	}

	impostor := open(t)
	_, err = impostor.Init()
	must(t, err)
	fake, err := impostor.NewPublication("", "Notes", "Not the real one", nil)
	must(t, err)
	fakeHead, err := impostor.Store.HeadEnv(fake)
	must(t, err)
	swap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { w.Write(fakeHead) }))
	defer swap.Close()
	res, _ = open(t).FetchFrom(ctx, pub, swap.URL)
	if res == nil || res.Failure == nil || res.Failure.Check != "publication address" {
		t.Fatalf("key substitution not caught: %+v", res)
	}

	bundle, err := author.Export(pub)
	must(t, err)
	imp := open(t)
	if a, f, err := imp.Import(bundle, "bundle"); err != nil || f != nil || a != pub {
		t.Fatalf("bundle import: %v %v %v", a, f, err)
	}

	restored := open(t)
	must(t, restored.Restore(strings.Join(words, " ")))
	got, err := restored.AddressFor("", 0)
	must(t, err)
	if got != pub {
		t.Fatalf("restored seed derives %s, want %s", got, pub)
	}
	fetchOK(t, restored, pub, hostB.URL)
	_, err = restored.Adopt("", 0)
	must(t, err)
	_, err = restored.Publish(pub, node.Draft{Kind: obj.Post, Title: "After restore", Body: "Still mine."})
	must(t, err)
	p, err := restored.Store.Load(pub)
	must(t, err)
	if p.Head.Sequence != 5 || len(p.Head.Entries) != 3 {
		t.Fatalf("after restore: head %d with %d entries", p.Head.Sequence, len(p.Head.Entries))
	}
}

func TestCustomSaltPublicationsAreSeparate(t *testing.T) {
	n := open(t)
	_, err := n.Init()
	must(t, err)
	a, err := n.NewPublication("Café Poems", "Poems", "", nil)
	must(t, err)
	b, err := n.AddressFor("cafe  POEMS", 0)
	must(t, err)
	if a != b {
		t.Fatalf("normalized salts gave different addresses: %s vs %s", a, b)
	}
	c, err := n.NewPublication("", "Counter zero", "", nil)
	must(t, err)
	if c == a {
		t.Fatal("counter and custom salt collided")
	}
	if _, err := n.NewPublication("CAFÉ POEMS", "dup", "", nil); err == nil {
		t.Fatal("same normalized salt created twice")
	}
}

func hexs(b []byte) string {
	const h = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[2*i], out[2*i+1] = h[c>>4], h[c&15]
	}
	return string(out)
}
