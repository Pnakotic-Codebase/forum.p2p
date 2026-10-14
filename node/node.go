package node

import (
	"crypto/mldsa"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Pnakotic-Codebase/forum.p2p/addr"
	"github.com/Pnakotic-Codebase/forum.p2p/digest"
	"github.com/Pnakotic-Codebase/forum.p2p/keys"
	"github.com/Pnakotic-Codebase/forum.p2p/obj"
	"github.com/Pnakotic-Codebase/forum.p2p/seedphrase"
	"github.com/Pnakotic-Codebase/forum.p2p/store"
	"github.com/Pnakotic-Codebase/forum.p2p/verify"
)

var (
	ErrNoSeed     = errors.New("no master seed here yet: run `forum init` or `forum restore`")
	ErrSeedExists = errors.New("a master seed already exists here")
	ErrNotOwned   = errors.New("this publication is not one of yours on this machine")
	ErrNoDocument = errors.New("no such document in this publication")
)

type Node struct {
	Dir   string
	Store *store.Store
	Now   func() time.Time
}

func Open(dir string) (*Node, error) {
	s, err := store.Open(dir)
	if err != nil {
		return nil, err
	}
	return &Node{Dir: dir, Store: s, Now: time.Now}, nil
}

func (n *Node) Close() error { return n.Store.Close() }

func (n *Node) seedPath() string { return filepath.Join(n.Dir, "seed") }

func (n *Node) writeSeed(seed *[keys.MasterSize]byte) error {
	if _, err := os.Stat(n.seedPath()); err == nil {
		return ErrSeedExists
	}
	return os.WriteFile(n.seedPath(), []byte(hex.EncodeToString(seed[:])+"\n"), 0o600)
}

func (n *Node) Init() ([]string, error) {
	var seed [keys.MasterSize]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, err
	}
	defer clear(seed[:])
	if err := n.writeSeed(&seed); err != nil {
		return nil, err
	}
	return seedphrase.Encode(seed), nil
}

func (n *Node) Restore(text string) error {
	seed, err := seedphrase.RestoreText(text)
	defer clear(seed[:])
	if err != nil {
		return err
	}
	return n.writeSeed(&seed)
}

func (n *Node) loadSeed() (*[keys.MasterSize]byte, error) {
	b, err := os.ReadFile(n.seedPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoSeed
	}
	if err != nil {
		return nil, err
	}
	raw, err := hex.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(raw) != keys.MasterSize {
		return nil, errors.New("seed file is damaged")
	}
	var seed [keys.MasterSize]byte
	copy(seed[:], raw)
	clear(raw)
	return &seed, nil
}

func (n *Node) Phrase() ([]string, error) {
	seed, err := n.loadSeed()
	if err != nil {
		return nil, err
	}
	defer clear(seed[:])
	return seedphrase.Encode(*seed), nil
}

func saltFor(o *store.Owned) (keys.Salt, error) {
	if o.SaltKind == 0 {
		var v uint64
		for _, c := range o.SaltValue {
			v = v<<8 | uint64(c)
		}
		return keys.Counter(v), nil
	}
	return keys.Custom(string(o.SaltValue))
}

func (n *Node) key(o *store.Owned) (*mldsa.PrivateKey, error) {
	seed, err := n.loadSeed()
	if err != nil {
		return nil, err
	}
	defer clear(seed[:])
	s, err := saltFor(o)
	if err != nil {
		return nil, err
	}
	return keys.Derive(seed, s)
}

func (n *Node) ms() int64 { return n.Now().UnixMilli() }

func (n *Node) commit(sk *mldsa.PrivateKey, a addr.ID, h *obj.Head, extra map[digest.Digest][]byte) error {
	env, err := obj.Seal(sk, h.Encode())
	if err != nil {
		return err
	}
	held, err := n.Store.Held(a)
	if err != nil {
		return err
	}
	src := overlay{extra: extra, base: store.Local{Store: n.Store, Address: a}}
	p, f := verify.Full(a, env, held, src)
	if f != nil {
		return fmt.Errorf("refusing to publish, own output failed verification: %w", f)
	}
	return n.Store.Save(p, "self")
}

type overlay struct {
	extra map[digest.Digest][]byte
	base  verify.Source
}

func (o overlay) Revision(id digest.Digest) ([]byte, error) {
	if b, ok := o.extra[id]; ok {
		return b, nil
	}
	return o.base.Revision(id)
}

func (o overlay) Asset(id digest.Digest) ([]byte, error) { return o.base.Asset(id) }

func (n *Node) NewPublication(saltText, title, description string, copies []string) (addr.ID, error) {
	var o store.Owned
	var s keys.Salt
	var err error
	if saltText == "" {
		c, err := n.Store.NextCounter()
		if err != nil {
			return addr.ID{}, err
		}
		s = keys.Counter(c)
		o.SaltKind, o.SaltValue, o.Label = 0, []byte{byte(c >> 56), byte(c >> 48), byte(c >> 40), byte(c >> 32), byte(c >> 24), byte(c >> 16), byte(c >> 8), byte(c)}, fmt.Sprintf("counter %d", c)
	} else {
		if s, err = keys.Custom(saltText); err != nil {
			return addr.ID{}, err
		}
		o.SaltKind, o.SaltValue, o.Label = 1, []byte(keys.NormalizeSalt(saltText)), "custom"
	}
	seed, err := n.loadSeed()
	if err != nil {
		return addr.ID{}, err
	}
	sk, err := keys.Derive(seed, s)
	clear(seed[:])
	if err != nil {
		return addr.ID{}, err
	}
	a := keys.Address(sk.PublicKey())
	o.Address = a
	if held, err := n.Store.Held(a); err != nil {
		return a, err
	} else if held != nil {
		return a, fmt.Errorf("publication %s already exists here", a)
	}
	h := &obj.Head{PublicKey: sk.PublicKey().Bytes(), Sequence: 1, Updated: n.ms(), Title: oneLine(title),
		Description: description, KnownCopies: copies}
	if err := n.commit(sk, a, h, nil); err != nil {
		return a, err
	}
	return a, n.Store.AddOwned(o)
}

func (n *Node) Adopt(saltText string, counter uint64) (addr.ID, error) {
	var o store.Owned
	var s keys.Salt
	var err error
	if saltText == "" {
		s = keys.Counter(counter)
		o.SaltKind, o.Label = 0, fmt.Sprintf("counter %d", counter)
		o.SaltValue = []byte{byte(counter >> 56), byte(counter >> 48), byte(counter >> 40), byte(counter >> 32), byte(counter >> 24), byte(counter >> 16), byte(counter >> 8), byte(counter)}
	} else {
		if s, err = keys.Custom(saltText); err != nil {
			return addr.ID{}, err
		}
		o.SaltKind, o.SaltValue, o.Label = 1, []byte(keys.NormalizeSalt(saltText)), "custom"
	}
	seed, err := n.loadSeed()
	if err != nil {
		return addr.ID{}, err
	}
	sk, err := keys.Derive(seed, s)
	clear(seed[:])
	if err != nil {
		return addr.ID{}, err
	}
	o.Address = keys.Address(sk.PublicKey())
	if _, err := n.Store.GetOwned(o.Address); err == nil {
		return o.Address, nil
	}
	return o.Address, n.Store.AddOwned(o)
}

func (n *Node) AddressFor(saltText string, counter uint64) (addr.ID, error) {
	var s keys.Salt
	var err error
	if saltText == "" {
		s = keys.Counter(counter)
	} else if s, err = keys.Custom(saltText); err != nil {
		return addr.ID{}, err
	}
	seed, err := n.loadSeed()
	if err != nil {
		return addr.ID{}, err
	}
	sk, err := keys.Derive(seed, s)
	clear(seed[:])
	if err != nil {
		return addr.ID{}, err
	}
	return keys.Address(sk.PublicKey()), nil
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func (n *Node) owned(a addr.ID) (*mldsa.PrivateKey, *verify.Publication, error) {
	o, err := n.Store.GetOwned(a)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, ErrNotOwned
	}
	if err != nil {
		return nil, nil, err
	}
	sk, err := n.key(o)
	if err != nil {
		return nil, nil, err
	}
	p, err := n.Store.Load(a)
	if err != nil {
		return nil, nil, err
	}
	return sk, p, nil
}

func nextHead(p *verify.Publication, now int64) *obj.Head {
	h := *p.Head
	h.Sequence++
	h.Updated = now
	h.Entries = append([]obj.Entry(nil), p.Head.Entries...)
	return &h
}

type Draft struct {
	Document  string
	Kind      obj.Kind
	Title     string
	Body      string
	Published int64
}

func (n *Node) findEntry(p *verify.Publication, doc string) (int, error) {
	want, err := addr.Parse(doc)
	if err != nil {
		return -1, err
	}
	for i, e := range p.Head.Entries {
		if addr.FromDigest(obj.DocumentDigest(p.Digest, e.DocumentNonce)) == want {
			return i, nil
		}
	}
	return -1, ErrNoDocument
}

func (n *Node) Publish(a addr.ID, d Draft) (addr.ID, error) {
	sk, p, err := n.owned(a)
	if err != nil {
		return addr.ID{}, err
	}
	now := n.ms()
	body := strings.ReplaceAll(strings.ReplaceAll(d.Body, "\r\n", "\n"), "\r", "\n")
	r := &obj.Revision{Kind: d.Kind, PublicationDigest: p.Digest, Created: now, Published: now,
		Title: oneLine(d.Title), Body: body, References: obj.ExtractReferences(body)}
	if d.Published != 0 {
		r.Published = d.Published
	}
	h := nextHead(p, now)
	idx := -1
	if d.Document != "" {
		if idx, err = n.findEntry(p, d.Document); err != nil {
			return addr.ID{}, err
		}
		e := h.Entries[idx]
		prev := p.Revisions[e.Revision]
		parent := e.Revision
		r.Parent, r.DocumentNonce, r.Kind = &parent, e.DocumentNonce, e.Kind
		if d.Published == 0 {
			r.Published = prev.Published
		}
	} else if _, err := rand.Read(r.DocumentNonce[:]); err != nil {
		return addr.ID{}, err
	}
	if !r.Kind.Valid() {
		return addr.ID{}, errors.New("kind must be post or page")
	}
	seen := map[string]bool{}
	for _, ref := range obj.AssetRefs(body) {
		if seen[ref] {
			continue
		}
		seen[ref] = true
		var id digest.Digest
		raw, _ := hex.DecodeString(ref)
		copy(id[:], raw)
		if _, err := n.Store.Asset(id); err != nil {
			return addr.ID{}, fmt.Errorf("body references asset %s… which is not stored here: add it with `forum asset add` first", ref[:16])
		}
		r.Assets = append(r.Assets, id)
	}
	revBytes := r.Encode()
	env, err := obj.Seal(sk, revBytes)
	if err != nil {
		return addr.ID{}, err
	}
	rid := obj.RevisionID(revBytes)
	entry := obj.Entry{Kind: r.Kind, DocumentNonce: r.DocumentNonce, Revision: rid}
	if idx >= 0 {
		h.Entries[idx] = entry
	} else {
		h.Entries = append([]obj.Entry{entry}, h.Entries...)
	}
	if err := n.commit(sk, a, h, map[digest.Digest][]byte{rid: env}); err != nil {
		return addr.ID{}, err
	}
	return addr.FromDigest(obj.DocumentDigest(p.Digest, r.DocumentNonce)), nil
}

func (n *Node) Remove(a addr.ID, doc string) error {
	sk, p, err := n.owned(a)
	if err != nil {
		return err
	}
	idx, err := n.findEntry(p, doc)
	if err != nil {
		return err
	}
	h := nextHead(p, n.ms())
	h.Entries = append(h.Entries[:idx], h.Entries[idx+1:]...)
	return n.commit(sk, a, h, nil)
}

type Meta struct {
	Title       *string
	Description *string
	KnownCopies []string
	SetCopies   bool
}

func (n *Node) SetMeta(a addr.ID, m Meta) error {
	sk, p, err := n.owned(a)
	if err != nil {
		return err
	}
	h := nextHead(p, n.ms())
	if m.Title != nil {
		h.Title = oneLine(*m.Title)
	}
	if m.Description != nil {
		h.Description = *m.Description
	}
	if m.SetCopies {
		h.KnownCopies = m.KnownCopies
	}
	return n.commit(sk, a, h, nil)
}

func (n *Node) AddAsset(mediaType string, content []byte) (digest.Digest, error) {
	a := obj.Asset{MediaType: oneLine(mediaType), Content: content}
	b := a.Encode()
	id := obj.AssetID(b)
	return id, n.Store.PutAsset(id, b)
}

func (n *Node) Export(a addr.ID) ([]byte, error) {
	p, err := n.Store.Load(a)
	if err != nil {
		return nil, err
	}
	b := &obj.Bundle{Head: p.HeadEnv}
	for _, e := range p.Head.Entries {
		b.Revisions = append(b.Revisions, p.RevisionEnv[e.Revision])
	}
	for _, x := range p.Assets {
		b.Assets = append(b.Assets, x)
	}
	return b.Encode(), nil
}

type bundleSource struct {
	revs   map[digest.Digest][]byte
	assets map[digest.Digest][]byte
}

func (b bundleSource) Revision(id digest.Digest) ([]byte, error) {
	if x, ok := b.revs[id]; ok {
		return x, nil
	}
	return nil, store.ErrNotFound
}

func (b bundleSource) Asset(id digest.Digest) ([]byte, error) {
	if x, ok := b.assets[id]; ok {
		return x, nil
	}
	return nil, store.ErrNotFound
}

func (n *Node) Import(raw []byte, label string) (addr.ID, *verify.Failure, error) {
	b, err := obj.DecodeBundle(raw)
	if err != nil {
		return addr.ID{}, nil, err
	}
	object, _, err := obj.Unseal(b.Head)
	if err != nil {
		return addr.ID{}, nil, err
	}
	h, err := obj.DecodeHead(object)
	if err != nil {
		return addr.ID{}, nil, err
	}
	a := addr.FromDigest(obj.PublicationDigest(h.PublicKey))
	src := bundleSource{revs: map[digest.Digest][]byte{}, assets: map[digest.Digest][]byte{}}
	for _, r := range b.Revisions {
		if o, _, err := obj.Unseal(r); err == nil {
			src.revs[obj.RevisionID(o)] = r
		}
	}
	for _, x := range b.Assets {
		src.assets[obj.AssetID(x)] = x
	}
	held, err := n.Store.Held(a)
	if err != nil {
		return a, nil, err
	}
	p, f := verify.Full(a, b.Head, held, src)
	n.Store.Log(a, label, f)
	if f != nil {
		return a, f, nil
	}
	return a, nil, n.Store.Save(p, label)
}
