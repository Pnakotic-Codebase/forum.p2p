package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Pnakotic-Codebase/forum.p2p/node"
)

type reply struct {
	ID     int `json:"id"`
	Result struct {
		Tools   []struct{ Name string } `json:"tools"`
		Content []struct{ Text string } `json:"content"`
		IsError bool                    `json:"isError"`
	} `json:"result"`
	Error *struct{ Message string } `json:"error"`
}

func session(t *testing.T, s *Server, calls ...string) []reply {
	t.Helper()
	var in bytes.Buffer
	in.WriteString(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}` + "\n")
	in.WriteString(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")
	for _, c := range calls {
		in.WriteString(c + "\n")
	}
	var out bytes.Buffer
	if err := s.Serve(context.Background(), &in, &out); err != nil {
		t.Fatal(err)
	}
	var rs []reply
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r reply
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("bad line %q: %v", line, err)
		}
		rs = append(rs, r)
	}
	return rs
}

func call(id int, name string, args map[string]any) string {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args}})
	return string(b)
}

func TestAgentIsAPeer(t *testing.T) {
	n, err := node.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if _, err := n.Init(); err != nil {
		t.Fatal(err)
	}
	s := &Server{Node: n, Version: "test"}
	rs := session(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		call(2, "new_publication", map[string]any{"title": "Agent desk", "salt": "Désk"}),
	)
	if len(rs) != 3 || len(rs[1].Result.Tools) != 11 {
		t.Fatalf("expected initialize, 11 tools, new_publication: %+v", rs)
	}
	pub := rs[2].Result.Content[0].Text
	if !strings.HasPrefix(pub, "forum://") || rs[2].Result.IsError {
		t.Fatalf("new_publication: %+v", rs[2])
	}
	rs = session(t, s, call(3, "publish", map[string]any{"title": "Written by an agent", "body": "Same powers.\n"}))
	doc := rs[1].Result.Content[0].Text
	if rs[1].Result.IsError || !strings.HasPrefix(doc, pub+"/") {
		t.Fatalf("publish: %+v", rs[1])
	}
	rs = session(t, s,
		call(4, "publish", map[string]any{"document": doc, "title": "Written by an agent", "body": "Same powers, revised.\n"}),
		call(5, "read", map[string]any{"address": doc}),
		call(6, "read", map[string]any{"address": pub}),
	)
	if rs[1].Result.IsError || rs[1].Result.Content[0].Text != doc {
		t.Fatalf("revise: %+v", rs[1])
	}
	md := rs[2].Result.Content[0].Text
	if !strings.Contains(md, "Same powers, revised.") || !strings.Contains(md, "parent_revision: ") || strings.Contains(md, "parent_revision: \n") {
		t.Fatalf("read document:\n%s", md)
	}
	if !strings.Contains(rs[3].Result.Content[0].Text, "Written by an agent") {
		t.Fatalf("read publication:\n%s", rs[3].Result.Content[0].Text)
	}

	ro := &Server{Node: n, ReadOnly: true}
	rs = session(t, ro, `{"jsonrpc":"2.0","id":7,"method":"tools/list"}`, call(8, "publish", map[string]any{"title": "x", "body": "y"}))
	for _, tl := range rs[1].Result.Tools {
		if tl.Name == "publish" {
			t.Fatal("read-only server offers publish")
		}
	}
	if rs[2].Error == nil {
		t.Fatal("read-only server accepted publish")
	}
}
