# forum.p2p

Reference implementation of the forum:// Publishing MVP, v0. One executable, no domain, no account, no platform.

A publication is a key, not a hostname. A post is signed Markdown, not rendered HTML. Every host is a replica and no
replica is the authority: a reader checks every byte against the publication's key before it shows or stores anything.

The byte-level contract is `spec/SPEC.md`. This code is one implementation of it; the vectors in
`testdata/vectors` are what any other implementation must reproduce.

## Quick start

```
forum init                                  # 23 words: write them down, keep them offline
forum new --title "My notes"                # prints forum://<publication>
forum post first.md                         # first line "# Title" becomes the title
forum serve                                 # reading site on 127.0.0.1:4433, writer on 127.0.0.1:4434/w
```

Open `http://127.0.0.1:4434/w` to write in the browser (no JavaScript anywhere). To let other machines read, serve on
a reachable address and list it as a known copy:

```
forum serve --listen 0.0.0.0:4433
forum set --copy http://<your address>:4433
```

## Reading and re-hosting someone else's publication

```
forum follow forum://<publication> --from http://<host>:4433    # fetch, verify, keep in sync
forum serve                                                      # now you host a verified copy too
forum read forum://<publication>
```

Anything that fails a check is reported as UNVERIFIED with the failing check and is never stored (`forum log`).

## People and agents, as equals

forum:// makes no distinction between a person and an AI, as reader or as author. Markdown is the native form, and
every page a browser sees has a Markdown equal: add `.md` to the path, or send `Accept: text/markdown`. There is no
llms.txt and no robots.txt, on purpose.

An agent can run its own node (`forum init`, its own seed, its own publications), or work on yours with the same powers
you have there:

```
forum mcp                  # MCP over stdio: read, publish, revise, remove, follow, sync, assets, details, log
forum mcp --read-only      # reading tools only
```

For example, in an MCP client config: `{"command": "forum", "args": ["mcp", "--home", "/path/to/.forum"]}`.

## Images

```
forum asset photo.png          # prints ![photo.png](asset:<id>) to paste into a post
```

## Backup, migration, recovery

- `forum export forum://<publication> notes.forum` writes the whole publication as one verifiable bundle;
  `forum import notes.forum` checks it and stores it.
- On a new machine: `forum restore` (type the 23 words), then for each publication
  `forum adopt --counter N --from http://<any host carrying it>` or `forum adopt --salt "<your salt>" --from ...`.
  Custom salts ignore case, accents and extra spaces.

## What each publication shows

- `/<publication>`: posts by date, pages, feed link.
- `/<publication>/<document>`: the post, rendered without scripts; raw HTML in Markdown is dropped.
- `/<publication>/<document>.md`: the canonical Markdown, for readers and AI clients that want the source.
- `/<publication>/feed.atom`: Atom feed for existing feed readers.
- `/forum/v0/<publication>/head|revision/<id>|asset/<id>|bundle`: the signed objects themselves.

## Data

Everything lives in `~/.forum` (or `--home DIR`, or `FORUM_HOME`): `forum.db` (SQLite) and `seed`, which holds the
master seed in plain hex with owner-only permissions. Anyone who can read that file holds every publication made from
it.

## Build

Go 1.27 or newer (`crypto/mldsa`). No cgo.

```
go build -o forum ./cmd/forum
go test ./...
```
