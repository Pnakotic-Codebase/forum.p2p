package node

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Pnakotic-Codebase/forum.p2p/addr"
	"github.com/Pnakotic-Codebase/forum.p2p/digest"
	"github.com/Pnakotic-Codebase/forum.p2p/store"
	"github.com/Pnakotic-Codebase/forum.p2p/verify"
)

const (
	MediaType   = "application/vnd.forum.v0"
	maxObject   = 16 << 20
	maxAsset    = 64 << 20
	httpTimeout = 30 * time.Second
)

var client = &http.Client{Timeout: httpTimeout}

func get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", MediaType)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s: response larger than %d bytes", url, limit)
	}
	return b, nil
}

func base(location string) string {
	return strings.TrimRight(location, "/")
}

func ObjectURL(location string, a addr.ID, parts ...string) string {
	return base(location) + "/forum/v0/" + a.String() + "/" + strings.Join(parts, "/")
}

type remote struct {
	ctx      context.Context
	location string
	a        addr.ID
	local    *store.Store
}

func (r remote) Revision(id digest.Digest) ([]byte, error) {
	if b, err := r.local.Revision(id); err == nil {
		return b, nil
	}
	return get(r.ctx, ObjectURL(r.location, r.a, "revision", hex.EncodeToString(id[:])), maxObject)
}

func (r remote) Asset(id digest.Digest) ([]byte, error) {
	if b, err := r.local.Asset(id); err == nil {
		return b, nil
	}
	return get(r.ctx, ObjectURL(r.location, r.a, "asset", hex.EncodeToString(id[:])), maxAsset)
}

type FetchResult struct {
	Publication *verify.Publication
	Failure     *verify.Failure
	Location    string
	Unchanged   bool
}

func (n *Node) FetchFrom(ctx context.Context, a addr.ID, location string) (*FetchResult, error) {
	env, err := get(ctx, ObjectURL(location, a, "head"), maxObject)
	if err != nil {
		return nil, err
	}
	held, err := n.Store.Held(a)
	if err != nil {
		return nil, err
	}
	p, f := verify.Full(a, env, held, remote{ctx, location, a, n.Store})
	n.Store.Log(a, location, f)
	if f != nil {
		return &FetchResult{Failure: f, Location: location}, nil
	}
	unchanged := held != nil && held.Sequence == p.Head.Sequence
	if err := n.Store.Save(p, location); err != nil {
		return nil, err
	}
	return &FetchResult{Publication: p, Location: location, Unchanged: unchanged}, nil
}

func (n *Node) Locations(a addr.ID, first ...string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(l string) {
		l = base(strings.TrimSpace(l))
		if l != "" && !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	for _, l := range first {
		add(l)
	}
	if p, err := n.Store.Load(a); err == nil {
		for _, l := range p.Head.KnownCopies {
			add(l)
		}
	}
	return out
}

func (n *Node) Fetch(ctx context.Context, a addr.ID, first ...string) (*FetchResult, []error) {
	var errs []error
	var lastFail *FetchResult
	for _, l := range n.Locations(a, first...) {
		r, err := n.FetchFrom(ctx, a, l)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", l, err))
			continue
		}
		if r.Failure == nil {
			return r, errs
		}
		lastFail = r
		errs = append(errs, fmt.Errorf("%s: %w", l, r.Failure))
	}
	if lastFail != nil {
		return lastFail, errs
	}
	if len(errs) == 0 {
		errs = append(errs, errors.New("no location known for this publication: pass one with --from"))
	}
	return nil, errs
}

func (n *Node) Follow(ctx context.Context, a addr.ID, location string) (*FetchResult, []error) {
	r, errs := n.Fetch(ctx, a, location)
	if r != nil && r.Failure == nil {
		if err := n.Store.Follow(a, location); err != nil {
			errs = append(errs, err)
		}
	}
	return r, errs
}

type SyncReport struct {
	Address addr.ID
	Result  *FetchResult
	Errors  []error
}

func (n *Node) Sync(ctx context.Context) ([]SyncReport, error) {
	fs, err := n.Store.Follows()
	if err != nil {
		return nil, err
	}
	var out []SyncReport
	for _, f := range fs {
		r, errs := n.Fetch(ctx, f.Address, f.Origin)
		out = append(out, SyncReport{f.Address, r, errs})
	}
	return out, nil
}
