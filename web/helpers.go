package web

import (
	"fmt"
	"strings"

	"github.com/Pnakotic-Codebase/forum.p2p/addr"
	"github.com/Pnakotic-Codebase/forum.p2p/node"
	"github.com/Pnakotic-Codebase/forum.p2p/obj"
)

func nodeDraft(doc string, kind obj.Kind, title, body string) node.Draft {
	return node.Draft{Document: strings.TrimSpace(doc), Kind: kind, Title: title, Body: body}
}

func metaOf(title, desc *string, copies []string) node.Meta {
	return node.Meta{Title: title, Description: desc, KnownCopies: copies, SetCopies: true}
}

func describe(a addr.ID, r *node.FetchResult, errs []error) string {
	switch {
	case r != nil && r.Failure == nil && r.Unchanged:
		return fmt.Sprintf("forum://%s is up to date (head %d, verified, from %s).", a, r.Publication.Head.Sequence, r.Location)
	case r != nil && r.Failure == nil:
		return fmt.Sprintf("forum://%s verified and stored (head %d, from %s).", a, r.Publication.Head.Sequence, r.Location)
	case r != nil:
		return fmt.Sprintf("forum://%s from %s is UNVERIFIED and was not stored: %s: %s.", a, r.Location, r.Failure.Check, r.Failure.Detail)
	}
	var s []string
	for _, e := range errs {
		s = append(s, e.Error())
	}
	return fmt.Sprintf("forum://%s could not be fetched: %s", a, strings.Join(s, "; "))
}

func Describe(a addr.ID, r *node.FetchResult, errs []error) string { return describe(a, r, errs) }
