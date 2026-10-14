package mcp

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/Pnakotic-Codebase/forum.p2p/addr"
	"github.com/Pnakotic-Codebase/forum.p2p/node"
	"github.com/Pnakotic-Codebase/forum.p2p/obj"
	"github.com/Pnakotic-Codebase/forum.p2p/web"
)

const protocolVersion = "2025-06-18"

type Server struct {
	Node     *node.Node
	ReadOnly bool
	Version  string
	mu       sync.Mutex
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type prop struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Items       *prop    `json:"items,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema struct {
		Type       string          `json:"type"`
		Properties map[string]prop `json:"properties"`
		Required   []string        `json:"required,omitempty"`
	} `json:"inputSchema"`
	write bool
	run   func(ctx context.Context, s *Server, args map[string]any) (string, error)
}

func str(args map[string]any, k string) string {
	v, _ := args[k].(string)
	return v
}

func newTool(name, desc string, write bool, props map[string]prop, required []string, run func(context.Context, *Server, map[string]any) (string, error)) tool {
	t := tool{Name: name, Description: desc, write: write, run: run}
	t.InputSchema.Type = "object"
	t.InputSchema.Properties = props
	if t.InputSchema.Properties == nil {
		t.InputSchema.Properties = map[string]prop{}
	}
	t.InputSchema.Required = required
	return t
}

func splitAddress(s string) (addr.ID, string, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "forum://")
	pub, doc, _ := strings.Cut(s, "/")
	a, err := addr.Parse(pub)
	if err != nil {
		return addr.ID{}, "", fmt.Errorf("%q is not a forum:// address", s)
	}
	return a, doc, nil
}

func docPart(s string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "forum://")
	if _, d, ok := strings.Cut(s, "/"); ok {
		return d
	}
	return s
}

func (s *Server) ownedPub(args map[string]any) (addr.ID, error) {
	if v := str(args, "publication"); v != "" {
		a, _, err := splitAddress(v)
		return a, err
	}
	owned, err := s.Node.Store.OwnedList()
	if err != nil {
		return addr.ID{}, err
	}
	if len(owned) == 1 {
		return owned[0].Address, nil
	}
	return addr.ID{}, fmt.Errorf("there are %d publications here: pass publication", len(owned))
}

var addrProp = prop{Type: "string", Description: "forum:// address, e.g. forum://<publication> or forum://<publication>/<document>"}
var pubProp = prop{Type: "string", Description: "forum://<publication>; may be omitted when this node owns exactly one"}

func tools() []tool {
	return []tool{
		newTool("list_publications", "List every publication this node holds, as Markdown: yours, followed, and replicas.", false, nil, nil,
			func(_ context.Context, s *Server, _ map[string]any) (string, error) {
				list, err := s.Node.Store.List()
				if err != nil {
					return "", err
				}
				var b strings.Builder
				b.WriteString(web.IndexMarkdown(list))
				b.WriteString("\n| address | role | head |\n|---|---|---|\n")
				for _, x := range list {
					role := "replica"
					if x.Owned {
						role = "yours"
					} else if x.Followed {
						role = "followed"
					}
					fmt.Fprintf(&b, "| forum://%s | %s | %d |\n", x.Address, role, x.Sequence)
				}
				return b.String(), nil
			}),
		newTool("read", "Read a publication (its index) or one document, as Markdown with a YAML header. Only verified material is returned.", false,
			map[string]prop{"address": addrProp}, []string{"address"},
			func(_ context.Context, s *Server, args map[string]any) (string, error) {
				a, doc, err := splitAddress(str(args, "address"))
				if err != nil {
					return "", err
				}
				p, err := s.Node.Store.Load(a)
				if err != nil {
					return "", fmt.Errorf("forum://%s is not held here or no longer verifies (%v); use fetch first", a, err)
				}
				if doc == "" {
					return web.PublicationMarkdown(p), nil
				}
				v, ok := web.LoadDocument(p, doc)
				if !ok {
					return "", errors.New("no such document in this publication")
				}
				return web.DocumentMarkdown(p, v), nil
			}),
		newTool("fetch", "Fetch a publication from a host, verify every byte, and store it if it verifies. Reports UNVERIFIED with the failing check otherwise.", true,
			map[string]prop{"address": addrProp, "from": {Type: "string", Description: "http://host:port to fetch from; optional when its known copies are already held"}}, []string{"address"},
			func(ctx context.Context, s *Server, args map[string]any) (string, error) {
				a, _, err := splitAddress(str(args, "address"))
				if err != nil {
					return "", err
				}
				r, errs := s.Node.Fetch(ctx, a, str(args, "from"))
				return web.Describe(a, r, errs), nil
			}),
		newTool("follow", "Fetch, verify and keep a publication in sync; this node then re-hosts it.", true,
			map[string]prop{"address": addrProp, "from": {Type: "string", Description: "http://host:port to fetch from"}}, []string{"address", "from"},
			func(ctx context.Context, s *Server, args map[string]any) (string, error) {
				a, _, err := splitAddress(str(args, "address"))
				if err != nil {
					return "", err
				}
				r, errs := s.Node.Follow(ctx, a, str(args, "from"))
				return web.Describe(a, r, errs), nil
			}),
		newTool("sync", "Refresh every followed publication from its known locations.", true, nil, nil,
			func(ctx context.Context, s *Server, _ map[string]any) (string, error) {
				reports, err := s.Node.Sync(ctx)
				if err != nil {
					return "", err
				}
				var out []string
				for _, r := range reports {
					out = append(out, "- "+web.Describe(r.Address, r.Result, r.Errors))
				}
				if len(out) == 0 {
					return "Nothing followed yet.", nil
				}
				return strings.Join(out, "\n"), nil
			}),
		newTool("new_publication", "Create a publication from this node's master seed. Salt empty: next counter. A custom salt ignores case, accents and spacing.", true,
			map[string]prop{"title": {Type: "string", Description: "title"}, "description": {Type: "string", Description: "description"},
				"salt": {Type: "string", Description: "custom salt (optional)"}}, []string{"title"},
			func(_ context.Context, s *Server, args map[string]any) (string, error) {
				a, err := s.Node.NewPublication(str(args, "salt"), str(args, "title"), str(args, "description"), nil)
				if err != nil {
					return "", err
				}
				return "forum://" + a.String(), nil
			}),
		newTool("publish", "Sign and publish Markdown as a new post or page, or as a new revision of an existing document. Returns its forum:// address.", true,
			map[string]prop{"publication": pubProp, "title": {Type: "string", Description: "title, one line"},
				"body":     {Type: "string", Description: "Markdown body; reference stored assets as asset:<id>"},
				"kind":     {Type: "string", Description: "post (default) or page", Enum: []string{"post", "page"}},
				"document": {Type: "string", Description: "document address to revise; omit for a new document"}}, []string{"title", "body"},
			func(_ context.Context, s *Server, args map[string]any) (string, error) {
				a, err := s.ownedPub(args)
				if err != nil {
					return "", err
				}
				kind := obj.Post
				if str(args, "kind") == "page" {
					kind = obj.Page
				}
				doc := docPart(str(args, "document"))
				d, err := s.Node.Publish(a, node.Draft{Document: doc, Kind: kind, Title: str(args, "title"), Body: str(args, "body")})
				if err != nil {
					return "", err
				}
				return "forum://" + a.String() + "/" + d.String(), nil
			}),
		newTool("remove", "Take a document out of a publication (a new signed head without it).", true,
			map[string]prop{"publication": pubProp, "document": {Type: "string", Description: "document address"}}, []string{"document"},
			func(_ context.Context, s *Server, args map[string]any) (string, error) {
				a, err := s.ownedPub(args)
				if err != nil {
					return "", err
				}
				doc := docPart(str(args, "document"))
				if err := s.Node.Remove(a, doc); err != nil {
					return "", err
				}
				return "removed", nil
			}),
		newTool("set_details", "Change a publication's title, description or known copies (signed as a new head).", true,
			map[string]prop{"publication": pubProp, "title": {Type: "string", Description: "new title"},
				"description":  {Type: "string", Description: "new description"},
				"known_copies": {Type: "array", Description: "http://host:port locations that carry it; replaces the list", Items: &prop{Type: "string", Description: "location"}}}, nil,
			func(_ context.Context, s *Server, args map[string]any) (string, error) {
				a, err := s.ownedPub(args)
				if err != nil {
					return "", err
				}
				var m node.Meta
				if v, ok := args["title"].(string); ok {
					m.Title = &v
				}
				if v, ok := args["description"].(string); ok {
					m.Description = &v
				}
				if v, ok := args["known_copies"].([]any); ok {
					m.SetCopies = true
					for _, x := range v {
						if s, ok := x.(string); ok {
							m.KnownCopies = append(m.KnownCopies, s)
						}
					}
				}
				if err := s.Node.SetMeta(a, m); err != nil {
					return "", err
				}
				return "signed and stored", nil
			}),
		newTool("add_asset", "Store an image or file by content; returns the Markdown to paste into a body.", true,
			map[string]prop{"media_type": {Type: "string", Description: "e.g. image/png"},
				"content_base64": {Type: "string", Description: "file bytes, base64"}, "name": {Type: "string", Description: "alt text"}},
			[]string{"media_type", "content_base64"},
			func(_ context.Context, s *Server, args map[string]any) (string, error) {
				b, err := base64.StdEncoding.DecodeString(str(args, "content_base64"))
				if err != nil {
					return "", errors.New("content_base64 is not valid base64")
				}
				id, err := s.Node.AddAsset(str(args, "media_type"), b)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("![%s](asset:%x)", str(args, "name"), id[:]), nil
			}),
		newTool("verification_log", "Recent verification results: what verified and what was refused, with the failing check.", false, nil, nil,
			func(_ context.Context, s *Server, _ map[string]any) (string, error) {
				ev, err := s.Node.Store.Events(50)
				if err != nil {
					return "", err
				}
				var out []string
				for _, e := range ev {
					out = append(out, "- "+e.String())
				}
				if len(out) == 0 {
					return "No fetches yet.", nil
				}
				return strings.Join(out, "\n"), nil
			}),
	}
}

func (s *Server) available() []tool {
	var out []tool
	for _, t := range tools() {
		if s.ReadOnly && t.write {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 128<<20)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		line := sc.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			enc.Encode(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
			continue
		}
		if len(req.ID) == 0 {
			continue
		}
		resp := response{JSONRPC: "2.0", ID: req.ID}
		result, rerr := s.handle(ctx, req)
		if rerr != nil {
			resp.Error = rerr
		} else {
			resp.Result = result
		}
		s.mu.Lock()
		err := enc.Encode(resp)
		s.mu.Unlock()
		if err != nil {
			return err
		}
	}
	return sc.Err()
}

func (s *Server) handle(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "forum", "version": s.Version},
			"instructions": "This is a forum:// node. Publications are signed Markdown. Agents and people use the same node with the same powers: " +
				"read, publish, revise, follow and re-host. Everything returned by read has been verified against its publication key.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": s.available()}, nil
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{-32602, "invalid params"}
		}
		for _, t := range s.available() {
			if t.Name != p.Name {
				continue
			}
			if p.Arguments == nil {
				p.Arguments = map[string]any{}
			}
			text, err := t.run(ctx, s, p.Arguments)
			if err != nil {
				return map[string]any{"content": []map[string]string{{"type": "text", "text": err.Error()}}, "isError": true}, nil
			}
			return map[string]any{"content": []map[string]string{{"type": "text", "text": text}}}, nil
		}
		return nil, &rpcError{-32602, "unknown tool: " + p.Name}
	}
	return nil, &rpcError{-32601, "method not found: " + req.Method}
}
