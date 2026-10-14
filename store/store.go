package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Pnakotic-Codebase/forum.p2p/addr"
	"github.com/Pnakotic-Codebase/forum.p2p/digest"
	"github.com/Pnakotic-Codebase/forum.p2p/obj"
	"github.com/Pnakotic-Codebase/forum.p2p/verify"
)

var ErrNotFound = errors.New("store: not found")

type Store struct {
	db  *sql.DB
	Dir string
}

const schema = `
CREATE TABLE IF NOT EXISTS publications (
	address TEXT PRIMARY KEY,
	public_key BLOB NOT NULL,
	sequence INTEGER NOT NULL,
	head_object BLOB NOT NULL,
	head_env BLOB NOT NULL,
	source TEXT NOT NULL,
	verified_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS revisions (
	id BLOB PRIMARY KEY,
	address TEXT NOT NULL,
	env BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS revisions_address ON revisions(address);
CREATE TABLE IF NOT EXISTS assets (
	id BLOB PRIMARY KEY,
	bytes BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS owned (
	address TEXT PRIMARY KEY,
	salt_kind INTEGER NOT NULL,
	salt_value BLOB NOT NULL,
	salt_label TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS follows (
	address TEXT PRIMARY KEY,
	origin TEXT NOT NULL,
	added INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	at INTEGER NOT NULL,
	address TEXT NOT NULL,
	location TEXT NOT NULL,
	verified INTEGER NOT NULL,
	check_name TEXT NOT NULL,
	detail TEXT NOT NULL
);
`

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "forum.db")+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, Dir: dir}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func now() int64 { return time.Now().UnixMilli() }

func (s *Store) Held(a addr.ID) (*verify.Held, error) {
	var h verify.Held
	var seq int64
	err := s.db.QueryRow(`SELECT public_key, sequence, head_object FROM publications WHERE address = ?`, a.String()).
		Scan(&h.PublicKey, &seq, &h.HeadBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	h.Sequence = uint64(seq)
	return &h, nil
}

func (s *Store) Save(p *verify.Publication, source string) error {
	object, _, err := obj.Unseal(p.HeadEnv)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	a := p.Address.String()
	if _, err := tx.Exec(`INSERT INTO publications(address, public_key, sequence, head_object, head_env, source, verified_at)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(address) DO UPDATE SET public_key=excluded.public_key, sequence=excluded.sequence,
		head_object=excluded.head_object, head_env=excluded.head_env, source=excluded.source, verified_at=excluded.verified_at`,
		a, p.Head.PublicKey, int64(p.Head.Sequence), object, p.HeadEnv, source, now()); err != nil {
		return err
	}
	for id, env := range p.RevisionEnv {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO revisions(id, address, env) VALUES(?,?,?)`, id[:], a, env); err != nil {
			return err
		}
	}
	for id, b := range p.Assets {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO assets(id, bytes) VALUES(?,?)`, id[:], b); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) PutRevision(a addr.ID, id digest.Digest, env []byte) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO revisions(id, address, env) VALUES(?,?,?)`, id[:], a.String(), env)
	return err
}

func (s *Store) PutAsset(id digest.Digest, b []byte) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO assets(id, bytes) VALUES(?,?)`, id[:], b)
	return err
}

func (s *Store) blob(q string, args ...any) ([]byte, error) {
	var b []byte
	err := s.db.QueryRow(q, args...).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return b, err
}

func (s *Store) HeadEnv(a addr.ID) ([]byte, error) {
	return s.blob(`SELECT head_env FROM publications WHERE address = ?`, a.String())
}

func (s *Store) Revision(id digest.Digest) ([]byte, error) {
	return s.blob(`SELECT env FROM revisions WHERE id = ?`, id[:])
}

func (s *Store) Asset(id digest.Digest) ([]byte, error) {
	return s.blob(`SELECT bytes FROM assets WHERE id = ?`, id[:])
}

type Local struct {
	Store   *Store
	Address addr.ID
}

func (l Local) Revision(id digest.Digest) ([]byte, error) { return l.Store.Revision(id) }
func (l Local) Asset(id digest.Digest) ([]byte, error)    { return l.Store.Asset(id) }

func (s *Store) Load(a addr.ID) (*verify.Publication, error) {
	env, err := s.HeadEnv(a)
	if err != nil {
		return nil, err
	}
	p, f := verify.Full(a, env, nil, Local{s, a})
	if f != nil {
		return nil, f
	}
	return p, nil
}

type Summary struct {
	Address    string
	Title      string
	Sequence   uint64
	Source     string
	VerifiedAt int64
	Owned      bool
	Followed   bool
}

func (s *Store) List() ([]Summary, error) {
	rows, err := s.db.Query(`SELECT p.address, p.head_object, p.sequence, p.source, p.verified_at,
		o.address IS NOT NULL, f.address IS NOT NULL
		FROM publications p LEFT JOIN owned o ON o.address = p.address LEFT JOIN follows f ON f.address = p.address
		ORDER BY p.verified_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var x Summary
		var ho []byte
		var seq int64
		if err := rows.Scan(&x.Address, &ho, &seq, &x.Source, &x.VerifiedAt, &x.Owned, &x.Followed); err != nil {
			return nil, err
		}
		x.Sequence = uint64(seq)
		if h, err := obj.DecodeHead(ho); err == nil {
			x.Title = h.Title
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

type Owned struct {
	Address   addr.ID
	SaltKind  byte
	SaltValue []byte
	Label     string
}

func (s *Store) AddOwned(o Owned) error {
	_, err := s.db.Exec(`INSERT INTO owned(address, salt_kind, salt_value, salt_label) VALUES(?,?,?,?)`,
		o.Address.String(), int(o.SaltKind), o.SaltValue, o.Label)
	return err
}

func (s *Store) OwnedList() ([]Owned, error) {
	rows, err := s.db.Query(`SELECT address, salt_kind, salt_value, salt_label FROM owned ORDER BY address`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Owned
	for rows.Next() {
		var o Owned
		var a string
		var k int
		if err := rows.Scan(&a, &k, &o.SaltValue, &o.Label); err != nil {
			return nil, err
		}
		id, err := addr.Parse(a)
		if err != nil {
			return nil, err
		}
		o.Address, o.SaltKind = id, byte(k)
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) GetOwned(a addr.ID) (*Owned, error) {
	list, err := s.OwnedList()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Address == a {
			return &list[i], nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) NextCounter() (uint64, error) {
	var n sql.NullInt64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM owned WHERE salt_kind = 0`).Scan(&n)
	return uint64(n.Int64), err
}

func (s *Store) Follow(a addr.ID, origin string) error {
	_, err := s.db.Exec(`INSERT INTO follows(address, origin, added) VALUES(?,?,?)
		ON CONFLICT(address) DO UPDATE SET origin=excluded.origin`, a.String(), origin, now())
	return err
}

type Followed struct {
	Address addr.ID
	Origin  string
}

func (s *Store) Follows() ([]Followed, error) {
	rows, err := s.db.Query(`SELECT address, origin FROM follows ORDER BY added`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Followed
	for rows.Next() {
		var a, o string
		if err := rows.Scan(&a, &o); err != nil {
			return nil, err
		}
		id, err := addr.Parse(a)
		if err != nil {
			return nil, err
		}
		out = append(out, Followed{id, o})
	}
	return out, rows.Err()
}

type Event struct {
	At       int64
	Address  string
	Location string
	Verified bool
	Check    string
	Detail   string
}

func (s *Store) Log(a addr.ID, location string, f *verify.Failure) error {
	e := Event{Address: a.String(), Location: location, Verified: f == nil}
	if f != nil {
		e.Check, e.Detail = f.Check, f.Detail
	}
	_, err := s.db.Exec(`INSERT INTO events(at, address, location, verified, check_name, detail) VALUES(?,?,?,?,?,?)`,
		now(), e.Address, e.Location, e.Verified, e.Check, e.Detail)
	return err
}

func (s *Store) Events(limit int) ([]Event, error) {
	rows, err := s.db.Query(`SELECT at, address, location, verified, check_name, detail FROM events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.At, &e.Address, &e.Location, &e.Verified, &e.Check, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (e Event) String() string {
	t := time.UnixMilli(e.At).UTC().Format(time.RFC3339)
	if e.Verified {
		return fmt.Sprintf("%s verified   %s from %s", t, e.Address, e.Location)
	}
	return fmt.Sprintf("%s UNVERIFIED %s from %s: %s: %s", t, e.Address, e.Location, e.Check, e.Detail)
}
