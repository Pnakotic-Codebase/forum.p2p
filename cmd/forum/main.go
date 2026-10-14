package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/Pnakotic-Codebase/forum.p2p/addr"
	"github.com/Pnakotic-Codebase/forum.p2p/mcp"
	"github.com/Pnakotic-Codebase/forum.p2p/node"
	"github.com/Pnakotic-Codebase/forum.p2p/obj"
	"github.com/Pnakotic-Codebase/forum.p2p/web"
)

const usage = `forum — publish and read forum:// publications

  forum init                         make a new master seed and show its 23 words
  forum restore                      restore the master seed from its words (read from stdin)
  forum phrase                       show the 23 words again
  forum new --title T [--salt S] [--description D] [--copy URL ...]
                                     create a publication (salt empty: next counter)
  forum adopt [--salt S | --counter N] [--from URL]
                                     after restore: take back a publication you own
  forum list                         publications held here
  forum post [--pub P] [--page] [--title T] [--doc D] FILE.md
                                     publish a post or page (with --doc: a new revision of it)
  forum remove [--pub P] DOC         take a document out of the publication
  forum set [--pub P] [--title T] [--description D] [--copy URL ...]
                                     change publication details
  forum asset FILE                   store an image or file; prints the Markdown to paste
  forum serve [--listen ADDR] [--writer ADDR]
                                     host every held publication; writer UI on loopback
  forum fetch forum://P [--from URL] fetch and verify once
  forum follow forum://P --from URL  fetch, verify, keep in sync, and re-host
  forum sync                         refresh everything followed
  forum read forum://P[/DOC]         print a publication's documents or one document
  forum export forum://P FILE        write a bundle of the whole publication
  forum import FILE                  verify and store a bundle
  forum log                          recent verification results
  forum mcp [--read-only]            serve this node to an AI agent over MCP (stdio), with the same
                                     powers a person has here: read, publish, revise, follow, re-host

  --home DIR (or FORUM_HOME) selects the data directory; default ~/.forum
`

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(s string) error { *m = append(*m, s); return nil }

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "forum: "+format+"\n", a...)
	os.Exit(1)
}

func home(args []string) (string, []string) {
	dir := os.Getenv("FORUM_HOME")
	for i := 0; i < len(args); i++ {
		if args[i] == "--home" && i+1 < len(args) {
			dir = args[i+1]
			args = append(args[:i:i], args[i+2:]...)
			break
		}
		if v, ok := strings.CutPrefix(args[i], "--home="); ok {
			dir = v
			args = append(args[:i:i], args[i+1:]...)
			break
		}
	}
	if dir == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			die("no home directory; pass --home")
		}
		dir = filepath.Join(h, ".forum")
	}
	return dir, args
}

func parseAddr(s string) (addr.ID, string) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "forum://")
	pub, doc, _ := strings.Cut(s, "/")
	a, err := addr.Parse(pub)
	if err != nil {
		die("%q is not a forum:// address", s)
	}
	return a, doc
}

func pickPub(n *node.Node, flagVal string) addr.ID {
	if flagVal != "" {
		a, _ := parseAddr(flagVal)
		return a
	}
	owned, err := n.Store.OwnedList()
	if err != nil {
		die("%v", err)
	}
	switch len(owned) {
	case 0:
		die("no publications yet: run `forum new --title ...`")
	case 1:
		return owned[0].Address
	}
	die("you have %d publications here: choose one with --pub", len(owned))
	return addr.ID{}
}

func main() {
	dir, args := home(os.Args[1:])
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(usage)
		return
	}
	n, err := node.Open(dir)
	if err != nil {
		die("%v", err)
	}
	defer n.Close()
	cmd, rest := args[0], args[1:]
	fs := &flagSet{FlagSet: flag.NewFlagSet(cmd, flag.ExitOnError)}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	switch cmd {
	case "init":
		fs.Parse(rest)
		words, err := n.Init()
		if err != nil {
			die("%v", err)
		}
		fmt.Println("Your master seed, as 23 words. Write them down in order and keep them offline.")
		fmt.Println("Anyone holding them holds every publication you make from this seed.")
		fmt.Println()
		for i, w := range words {
			fmt.Printf("%2d  %s\n", i+1, w)
		}
	case "restore":
		fs.Parse(rest)
		fmt.Fprintln(os.Stderr, "Enter the 23 words, then end input (Ctrl-D):")
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			die("%v", err)
		}
		if err := n.Restore(string(b)); err != nil {
			die("%v", err)
		}
		fmt.Println("Master seed restored. Take back each publication with `forum adopt --counter N --from URL` or `forum adopt --salt S --from URL` (or import a bundle first).")
	case "phrase":
		fs.Parse(rest)
		words, err := n.Phrase()
		if err != nil {
			die("%v", err)
		}
		fmt.Println(strings.Join(words, " "))
	case "new":
		title := fs.String("title", "", "publication title")
		desc := fs.String("description", "", "description")
		salt := fs.String("salt", "", "custom salt (empty: next counter)")
		var copies multi
		fs.Var(&copies, "copy", "location of a host that carries it (repeatable)")
		fs.Parse(rest)
		if *title == "" {
			die("--title is required")
		}
		a, err := n.NewPublication(*salt, *title, *desc, copies)
		if err != nil {
			die("%v", err)
		}
		fmt.Printf("forum://%s\n", a)
	case "adopt":
		salt := fs.String("salt", "", "custom salt")
		counter := fs.Uint64("counter", 0, "counter number (when no salt)")
		from := fs.String("from", "", "http://host:port that carries the publication")
		fs.Parse(rest)
		a, err := n.AddressFor(*salt, *counter)
		if err != nil {
			die("%v", err)
		}
		if *from != "" {
			r, errs := n.Fetch(ctx, a, *from)
			fmt.Println(web.Describe(a, r, errs))
		}
		if held, err := n.Store.Held(a); err != nil || held == nil {
			die("forum://%s is not held here: fetch it with --from or import its bundle first", a)
		}
		if _, err := n.Adopt(*salt, *counter); err != nil {
			die("%v", err)
		}
		fmt.Printf("forum://%s is yours again on this machine\n", a)
	case "list":
		fs.Parse(rest)
		list, err := n.Store.List()
		if err != nil {
			die("%v", err)
		}
		for _, s := range list {
			role := "replica"
			if s.Owned {
				role = "yours"
			} else if s.Followed {
				role = "followed"
			}
			fmt.Printf("forum://%s  %-8s head %-4d %s\n", s.Address, role, s.Sequence, s.Title)
		}
	case "post":
		pub := fs.String("pub", "", "publication address")
		page := fs.Bool("page", false, "publish as a page, not a post")
		title := fs.String("title", "", "title (default: the file's first # heading)")
		doc := fs.String("doc", "", "document address to revise")
		fs.Parse(rest)
		if fs.NArg() != 1 {
			die("usage: forum post [--pub P] [--page] [--title T] [--doc D] FILE.md")
		}
		raw, err := os.ReadFile(fs.Arg(0))
		if err != nil {
			die("%v", err)
		}
		t, body := splitTitle(string(raw))
		if *title != "" {
			t = *title
		}
		if t == "" {
			die("no title: start the file with a # heading or pass --title")
		}
		a := pickPub(n, *pub)
		kind := obj.Post
		if *page {
			kind = obj.Page
		}
		_, d := parseAddrMaybe(*doc)
		docID, err := n.Publish(a, node.Draft{Document: d, Kind: kind, Title: t, Body: body})
		if err != nil {
			die("%v", err)
		}
		fmt.Printf("forum://%s/%s\n", a, docID)
	case "remove":
		pub := fs.String("pub", "", "publication address")
		fs.Parse(rest)
		if fs.NArg() != 1 {
			die("usage: forum remove [--pub P] DOC")
		}
		a := pickPub(n, *pub)
		_, d := parseAddrMaybe(fs.Arg(0))
		if err := n.Remove(a, d); err != nil {
			die("%v", err)
		}
		fmt.Println("removed")
	case "set":
		pub := fs.String("pub", "", "publication address")
		title := fs.String("title", "", "new title")
		desc := fs.String("description", "", "new description")
		var copies multi
		fs.Var(&copies, "copy", "known copy location (repeatable; replaces the list)")
		fs.Parse(rest)
		a := pickPub(n, *pub)
		m := node.Meta{}
		fs.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "title":
				m.Title = title
			case "description":
				m.Description = desc
			case "copy":
				m.SetCopies, m.KnownCopies = true, copies
			}
		})
		if err := n.SetMeta(a, m); err != nil {
			die("%v", err)
		}
		fmt.Println("signed and stored")
	case "asset":
		fs.Parse(rest)
		if fs.NArg() != 1 {
			die("usage: forum asset FILE")
		}
		b, err := os.ReadFile(fs.Arg(0))
		if err != nil {
			die("%v", err)
		}
		mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(fs.Arg(0))))
		if mt == "" {
			mt = http.DetectContentType(b)
		}
		mt = strings.SplitN(mt, ";", 2)[0]
		id, err := n.AddAsset(mt, b)
		if err != nil {
			die("%v", err)
		}
		fmt.Printf("![%s](asset:%x)\n", filepath.Base(fs.Arg(0)), id[:])
	case "serve":
		listen := fs.String("listen", "127.0.0.1:4433", "public address (use 0.0.0.0:4433 to reach other machines)")
		writer := fs.String("writer", "127.0.0.1:4434", "writer address, loopback only; empty to disable")
		every := fs.Duration("sync", 10*time.Minute, "how often to sync followed publications; 0 to disable")
		fs.Parse(rest)
		serve(ctx, n, *listen, *writer, *every)
	case "fetch", "follow":
		from := fs.String("from", "", "http://host:port to fetch from")
		fs.Parse(rest)
		if fs.NArg() != 1 {
			die("usage: forum %s forum://P [--from URL]", cmd)
		}
		a, _ := parseAddr(fs.Arg(0))
		if cmd == "follow" && *from == "" {
			die("--from is required to follow")
		}
		var r *node.FetchResult
		var errs []error
		if cmd == "follow" {
			r, errs = n.Follow(ctx, a, *from)
		} else {
			r, errs = n.Fetch(ctx, a, *from)
		}
		fmt.Println(web.Describe(a, r, errs))
		if r == nil || r.Failure != nil {
			os.Exit(2)
		}
	case "sync":
		fs.Parse(rest)
		reports, err := n.Sync(ctx)
		if err != nil {
			die("%v", err)
		}
		for _, rep := range reports {
			fmt.Println(web.Describe(rep.Address, rep.Result, rep.Errors))
		}
	case "read":
		fs.Parse(rest)
		if fs.NArg() != 1 {
			die("usage: forum read forum://P[/DOC]")
		}
		read(n, fs.Arg(0))
	case "export":
		fs.Parse(rest)
		if fs.NArg() != 2 {
			die("usage: forum export forum://P FILE")
		}
		a, _ := parseAddr(fs.Arg(0))
		b, err := n.Export(a)
		if err != nil {
			die("%v", err)
		}
		if err := os.WriteFile(fs.Arg(1), b, 0o644); err != nil {
			die("%v", err)
		}
		fmt.Printf("%d bytes written\n", len(b))
	case "import":
		fs.Parse(rest)
		if fs.NArg() != 1 {
			die("usage: forum import FILE")
		}
		b, err := os.ReadFile(fs.Arg(0))
		if err != nil {
			die("%v", err)
		}
		a, f, err := n.Import(b, "bundle "+filepath.Base(fs.Arg(0)))
		if err != nil {
			die("%v", err)
		}
		if f != nil {
			fmt.Printf("forum://%s is UNVERIFIED and was not stored: %s: %s\n", a, f.Check, f.Detail)
			os.Exit(2)
		}
		fmt.Printf("forum://%s verified and stored\n", a)
	case "mcp":
		ro := fs.Bool("read-only", false, "offer only reading tools")
		fs.Parse(rest)
		srv := &mcp.Server{Node: n, ReadOnly: *ro, Version: "v0"}
		if err := srv.Serve(ctx, os.Stdin, os.Stdout); err != nil {
			die("%v", err)
		}
	case "log":
		fs.Parse(rest)
		events, err := n.Store.Events(50)
		if err != nil {
			die("%v", err)
		}
		for _, e := range events {
			fmt.Println(e)
		}
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

type flagSet struct {
	*flag.FlagSet
	pos []string
}

func (f *flagSet) Parse(args []string) error {
	for {
		if err := f.FlagSet.Parse(args); err != nil {
			return err
		}
		args = f.FlagSet.Args()
		if len(args) == 0 {
			return nil
		}
		f.pos = append(f.pos, args[0])
		args = args[1:]
	}
}

func (f *flagSet) NArg() int        { return len(f.pos) }
func (f *flagSet) Arg(i int) string { return f.pos[i] }

func parseAddrMaybe(s string) (string, string) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "forum://")
	if pub, doc, ok := strings.Cut(s, "/"); ok {
		return pub, doc
	}
	return "", s
}

func splitTitle(s string) (string, string) {
	s = strings.TrimLeft(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	first, rest, _ := strings.Cut(s, "\n")
	if t, ok := strings.CutPrefix(first, "# "); ok {
		return strings.TrimSpace(t), strings.TrimLeft(rest, "\n")
	}
	return "", s
}

func read(n *node.Node, target string) {
	a, doc := parseAddr(target)
	p, err := n.Store.Load(a)
	if err != nil {
		die("not held here (or it no longer verifies): %v", err)
	}
	if doc == "" {
		fmt.Print(web.PublicationMarkdown(p))
		return
	}
	v, ok := web.LoadDocument(p, doc)
	if !ok {
		die("no such document")
	}
	fmt.Print(web.DocumentMarkdown(p, v))
}

func serve(ctx context.Context, n *node.Node, listen, writer string, every time.Duration) {
	pub := web.NewPublic(n)
	public := &http.Server{Addr: listen, Handler: pub, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		fmt.Printf("hosting on http://%s\n", listen)
		if err := public.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			die("%v", err)
		}
	}()
	var w *http.Server
	if writer != "" {
		host, _, err := net.SplitHostPort(writer)
		if ip := net.ParseIP(host); err != nil || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
			die("--writer must be a loopback address such as 127.0.0.1:4434")
		}
		w = &http.Server{Addr: writer, Handler: web.NewWriter(pub), ReadHeaderTimeout: 10 * time.Second}
		go func() {
			fmt.Printf("writer on http://%s/w (this machine only)\n", writer)
			if err := w.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				die("%v", err)
			}
		}()
	}
	if every > 0 {
		go func() {
			t := time.NewTicker(every)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					reports, _ := n.Sync(ctx)
					for _, rep := range reports {
						fmt.Println(web.Describe(rep.Address, rep.Result, rep.Errors))
					}
				}
			}
		}()
	}
	<-ctx.Done()
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	public.Shutdown(sctx)
	if w != nil {
		w.Shutdown(sctx)
	}
}
