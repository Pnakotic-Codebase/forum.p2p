package e2e

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Pnakotic-Codebase/forum.p2p/node"
	"github.com/Pnakotic-Codebase/forum.p2p/obj"
)

func get(t *testing.T, url, accept string) (int, string, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := http.DefaultClient.Do(req)
	must(t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Content-Type"), string(b)
}

func TestMarkdownParity(t *testing.T) {
	n := open(t)
	_, err := n.Init()
	must(t, err)
	pub, err := n.NewPublication("", "Field notes", "Same words for every reader.", nil)
	must(t, err)
	doc, err := n.Publish(pub, node.Draft{Kind: obj.Post, Title: "Alembic", Body: "The **still** is the instrument.\n"})
	must(t, err)
	s := host(t, n)
	p, d := "/"+pub.String(), "/"+pub.String()+"/"+doc.String()

	for _, c := range []struct{ path, accept, want string }{
		{"/index.md", "", "Field notes"},
		{"/", "text/markdown", "Field notes"},
		{p + ".md", "", "- 20"},
		{p, "text/markdown, text/html;q=0.5", "## Posts"},
		{d + ".md", "", "The **still** is the instrument."},
		{d, "text/markdown", "address: \"forum://" + pub.String() + "/" + doc.String() + "\""},
	} {
		code, ct, body := get(t, s.URL+c.path, c.accept)
		if code != 200 || !strings.HasPrefix(ct, "text/markdown") || !strings.Contains(body, c.want) {
			t.Errorf("%s (Accept %q): %d %s, missing %q in:\n%s", c.path, c.accept, code, ct, c.want, body)
		}
	}
	code, ct, body := get(t, s.URL+d, "text/html")
	if code != 200 || !strings.HasPrefix(ct, "text/html") || !strings.Contains(body, "<strong>still</strong>") {
		t.Errorf("browser view broke: %d %s", code, ct)
	}
	for _, path := range []string{"/llms.txt", "/robots.txt"} {
		if code, _, _ := get(t, s.URL+path, ""); code != 404 {
			t.Errorf("%s answered %d; forum:// keeps no separate files for machines", path, code)
		}
	}
}
