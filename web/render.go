package web

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type linkRewriter struct{ pub string }

func (l linkRewriter) rewrite(dest []byte) []byte {
	s := string(dest)
	switch {
	case strings.HasPrefix(s, "forum://"):
		return []byte("/" + strings.TrimPrefix(s, "forum://"))
	case strings.HasPrefix(s, "asset:"):
		return []byte("/" + l.pub + "/asset/" + strings.TrimPrefix(s, "asset:"))
	}
	return dest
}

func (l linkRewriter) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Link:
			v.Destination = l.rewrite(v.Destination)
		case *ast.Image:
			v.Destination = l.rewrite(v.Destination)
		}
		return ast.WalkContinue, nil
	})
}

func renderMarkdown(pub, body string) (string, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Footnote),
		goldmark.WithParserOptions(parser.WithASTTransformers(util.Prioritized(linkRewriter{pub}, 100))),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(body), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}
