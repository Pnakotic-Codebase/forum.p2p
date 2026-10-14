# devlog

## 2026-10-07: signature layer

- `gen/` is the vector generator. It uses Go 1.27 stdlib only (`crypto/mldsa`). Run: `cd gen && GOTOOLCHAIN=auto go run . > ../vectors/signature-mldsa65.json`.
- Two consecutive runs produce identical output (sha256 6e93379116735155ab0a6ad4a1b7258e99b9eaae053c11f27e29d17297e27de4), because signing is deterministic and the seeds are fixed.
- Independent cross-check: `cloudflare/circl` v1.6.5 `mldsa65` re-derived all 8 public keys from the seeds, re-produced all 8 deterministic signatures byte-for-byte, and gave the same accept/reject on every verify tuple. The check harness lives outside the repo, so the generator stays stdlib-only.
- Seeds: `seq(0x00)` = bytes 0x00..0x1f; `seq(0x20)` = 0x20..0x3f; `fill(0x42)` = 32 × 0x42.
- Negative cases cover a one-byte message change, a flipped signature bit, a context mismatch, and a signature truncated by one byte.
- Everything past §2 waits on the OPEN items in SPEC.md.

## 2026-10-07: digest = Skein v1.3

- Anchor: the Skein v1.3 paper (1 Oct 2010), Appendix C, read from the PDF text itself. A page-summarizer pass had returned wrong values (31-byte "digests"), so summarized pages should not be trusted for test vectors.
- pyskein 1.0 (a binding over the reference C code) reproduces all three C.2 Skein-512-512 vectors (FF, 64-byte FF..C0, 128-byte FF..80), plus Skein-256-256 C.1 FF and the Wikipedia examples.
- 24 reference digests generated with pyskein: Skein-512-256 and Skein-512-512, 6 messages (empty, FF, 64 B, 128 B, the fox sentence, 10,240 B), each with and without personalization `forum:v0:revision`.
- Go candidates checked against those 24: `github.com/aead/skein` (MIT, ~2.5k lines with threefish, frozen since 2016-07-22, no dependencies) passes 24/24. `github.com/deatil/go-hash/skein` (Apache-2.0, same API and evidently a copy of aead's code, inside a 60-plus-hash module that depends on x/crypto, released 2025-04-27) also passes 24/24.
- Personalization works the same in both Go packages and in the reference C code.
- Rust: RustCrypto `skein` 0.2.0 (MIT OR Apache-2.0, github.com/RustCrypto/hashes) passes all 12 non-personalized reference digests (Skein-512-256 and Skein-512-512). It has no config-block API: no personalization, key, or public-key binding. That means the 12 personalized digests have no Rust implementation to check against.
- `skein-hash` 0.3.1 (cryptocorrosion/hashes, MIT/Apache-2.0): passes 12/12 plain digests on stable Rust. It builds a config block internally but exposes no personalization API. It sits on the older `digest` 0.9 / `threefish-cipher` 0.4 trait generation.
- `skein-ffi` 0.5.0 (GPL-3.0, bundles C Skein source, exposes only `hash(bits, data, out)`): does not build on stable Rust (`#![feature]` gates, error E0554). With `RUSTC_BOOTSTRAP=1` forced for testing only, it passes 12/12 plain digests. No personalization API. The library crate name is `skeinffi`.
- Every Rust option tested so far covers plain Skein-512 only. None exposes personalization.

## 2026-10-09: domain separation, Skein config, encoding

- Decided: domain separation by a tag in the bytes (field 0 of every encoded object). Skein runs with a plain configuration (no personalization). The ML-DSA context string is left empty.
- Decided: canonical encoding is length-prefixed fields (u32 big-endian length || bytes), chosen for compatibility.
- `gen-framing/` (Go 1.27, stdlib only) writes `vectors/encoding-framing.json`: 7 valid vectors (including an empty field, a body that looks like a length prefix, 300-byte and 70000-byte fields, and UTF-8) and 6 invalid ones (truncated prefix, truncated body, trailing bytes, too many fields, too few fields, a 0xFFFFFFFF length with no body). Two runs give identical output (sha256 32f9b7e5c2329dfab284a588afd638cfba227d1376e708b0501a85fc0e50104b).
- Independent cross-check: a Python encoder/decoder (stdlib struct) and a Rust one, both written separately from the generator, agree on all 13 vectors, including re-encoding the valid ones byte-for-byte.
- Still open inside the encoding: the tag strings, integer and time representation, and list framing.
- Output size decided: Skein-512-512 everywhere, full 64 bytes, no truncation. A 64-byte identifier is 86 characters in base64url, 88 in base58, 103 in base32, and 128 in hex.
- `gen-digest/gen.py` builds the vectors from pyskein 1.0, a binding over the designers' reference C code. Run: `cd gen-digest && uv run --no-project --with pyskein==1.0 python -I gen.py > ../vectors/digest-skein512-512.json`. The 13 vectors are the three v1.3 Appendix C.2 anchors, the empty input, block boundaries at 63/64/65/129 bytes, a 70000-byte input, and three framed objects that reuse the encoding vectors' bytes. Two runs give identical output (sha256 394f770eb7fd420c0df6837d1a4d98a41f62daa252616ba9496564a0e82cae87). The generator aborts if dig-001 drifts from the paper.
- Cross-check: aead/skein, deatil/go-hash/skein, RustCrypto skein 0.2.0, and skein-hash 0.3.1 all agree on all 13.
- Note for the address text form: forum://<id> puts the id in the URI host slot, which is case-insensitive (RFC 3986). Case-sensitive encodings (base64url, base58) can be corrupted there by normalization.
- Earlier, output size was raised as Skein-512-128.
- Decided: address identifiers (publication and document) are the first 110 bits of the 64-byte digest, written as 22 lowercase unpadded base32 characters (110 = 22 × 5, so no partial character). Revision identifiers stay at the full 64 bytes. Correction on record: the earlier claim that 128-bit digests suit only labels nothing trusts was too broad. Address identifiers face second-preimage attacks (2^n), not collisions (2^(n/2)), because a self-made collision gives an attacker nothing. The base32 alphabet (RFC 4648 or Crockford) is still open.
- Decided, as stages triggered by conditions: (1) MVP relies on the 110-bit cost, with prefix-extensible IDs as the upgrade path; (2) key pinning on follow once forum:// starts proliferating; (3) first-seen attestations once there is outside interest. A global FCFS registry is ruled out (consensus over global state, MVP §18).
- Decided: Crockford base32 for address IDs (encoders emit lowercase, decoders are case-insensitive with i/l→1 and o→0, no check symbol). Decided: one key holds many publications, with a toggle between a public nonce and a secret. Open: how the secret side works, since a raw secret in the ID breaks public recomputability (§8). Three forms are recorded in SPEC §4.2: a public nonce, a committed secret, or a secret-derived separate key.
- Resolved by Anja's rule "they should only be able to tell if you tell them." Each publication gets its own key, derived from one master seed. Linking requires a mutual signed link statement from both publications. The shared-key form is superseded. Open: salt form (index, so the master seed alone recovers everything, or random, so salts must be backed up too) and the link-statement encoding. Recorded limit: key-level unlinkability does not hide hosting or behavioral metadata.
- Decided: a high-entropy master seed (at least 256 bits of entropy, source not prescribed), a word-list form for it, and custom, memorable per-publication salts with a counter as the default. Phrase-only keys are ruled out. Open: which word list, and how counter and custom salts are kept apart. Word list: forum://'s own list, not a common or expectable one. Size (2048 or 4096) and contents are open; the list is public by necessity. Terminology: Anja does not equate entropy with randomness, so the spec says "entropy", never "random", for the master seed.
- Correction: the previous turn claimed that backing up the master seed backs up every publication. That only holds if salts are derived by index, not if they are random. That was a question, not a decision. A 128-bit digest gives 2^64 collision resistance, so it is not suitable for identifiers a signature relies on. The floor is recorded in SPEC §3.1.

## 2026-10-09: word list, first pass

- Brief: 4096 words, all archaic terms that later became science terms (example: alembic).
- `wordlist/candidates.tsv`: 163 candidates. Each row records the word, the modern science term it became, and its field (apparatus, substances, processes, astronomy, physics, mathematics, medicine, biology, geology). Assumption: plain a-z lowercase, so words can be typed anywhere.
- `wordlist/validate.py` checks a-z only, duplicates, first-four-letter uniqueness, and edit-distance-1 pairs.
- Result: 10 first-four-letter clashes (alkali/alkahest, azoth/azote, caloric/calomel, dioptra/dioptrics, galena/galenic, magnesia/magnes, plumbago/plumbum, quintessence/quinsy, thermoscope/theriac, versorium/versine) and 3 near-twins (azoth/azote, surd/sard, chyle/chyme). That leaves 153 usable of the 4096 needed.
- Yield concern: the pool of genuine archaic proto-science English terms looks far smaller than 4096. The ceiling is not known. Ways to widen it are listed in the reply to Anja.
- Rule widened at Anja's direction: alchemy is a science, and the goal is 4096 words. Sources now include alchemical vocabulary in Latin, Greek, Arabic, Sanskrit, and Chinese (pinyin), Greek and Latin roots of science terms, Galenic and Paracelsian medicine, materia medica, old chemistry names, old units, star names, and geology loanwords. Every row still records the science term it carried into. Star-name rows carry a generic gloss ("star (Arabic name)").
- Checkpoint: 1229 candidates. Usable count by rule: 1058 if words must differ in their first four letters, 1166 if in their first five, 1163 if whole words must differ by at least two letters. Target 4096.

## 2026-10-12: word list v0, 4096 words

- `wordlist/forum-wordlist-v0.tsv`: 4096 rows (index, word, later_science_term, field), sorted a–z, built by `build.py candidates.tsv 99` from 5669 candidates (4110 passed the rules; the first 4096 by field priority were kept).
- Rules held, re-checked on the output: a–z only; unique first five letters; no word under five letters is the start of another; no two words one edit apart (insert, delete, substitute, or adjacent swap).
- No length cap applied. Lengths 3–16; 51 words are over 12 letters (longest: prosthaphaeresis).
- Field mix leans biological: binomial 639, botany 582, zoology 527, greek-root 504; alchemy proper 47.
- sha256 e15a4c733d461f1730fb879e2b88bb4de18a4e4b845f0e1dc322b34394dd5229
- For Anja's review: length cap, generic glosses on star-name and epithet rows, field balance. Open: checksum construction for the 22-word phrase.

## 2026-10-14: restore failure message

- Decided by Anja: on checksum mismatch, report "the key was not found" (not "a word is wrong"). SPEC §4.2.
- Open: whether a non-list word gets the same message or names the word.
- Decided by Anja: every restore failure (non-list word, wrong count, checksum mismatch) gives the same "the key was not found". A public list is no reason to announce what was wrong.
- Decided by Anja: restore runs in constant time. No early exit on an unknown word, the hash always runs, the checksum comparison is constant-time. SPEC §4.2.

## 2026-10-14: implementation decisions

- Skein for the Go PoC: `github.com/deatil/go-hash/skein` (Apache-2.0) for now; a forum://-own Skein-512 implementation replaces it in a later version.
- Phrase: 23 words (276 bits = 256-bit seed + 20-bit checksum), superseding the 22-word / 8-bit layout. Checksum is tagged (`forum:v0:seed-checksum`), following §2.2 since every hash in forum:// is tagged.
- Tag shape: `forum:v0:<kind>`. Kind names in the SPEC §2.2 table are proposed, pending Anja's review.
- Go module path: `github.com/Pnakotic-Codebase/forum.p2p`.

## 2026-10-14: Go PoC, first slice

- Repo `forum.p2p/`, module `github.com/Pnakotic-Codebase/forum.p2p`, Go 1.27 (`GOTOOLCHAIN=auto`). One outside dependency: `github.com/deatil/go-hash` v1.0.3006 (skein only).
- Packages: `frame` (§5.1), `digest` (Skein-512-512 and tagged digests), `tag` (the three v0 tags), `addr` (110-bit Crockford IDs: encode, lenient decode, constant-time match), `seedphrase` (23-word encode/restore against the embedded v0 list, which is checked by its sha256 at startup).
- `gen-identifiers/gen.py` (pyskein 1.0, written separately from the Go code) writes `vectors/identifiers.json`. Two runs give identical output (sha256 dcbf06bf9f0411701673e4070f0bc60d97cbd760dd43afae040e868ee1e7fba8). Address vectors reuse the three ML-DSA-65 public keys from the signature vectors.
- `go test ./...` passes against framing (13), digest (13), and identifier vectors, plus a round trip through all 4096 word indices. `go vet` and `gofmt` are clean.
- Restore path: 23 slots always processed (missing slots count as unknown words); each word compared against all 4096 entries with `subtle.ConstantTimeCompare` on fixed 32-byte slots; the checksum hash always runs; the result is one flag. Every failure returns a zeroed seed and "the key was not found".
- Benchmarks (2-core sandbox): valid, unknown first word, unknown last word, checksum mismatch, and 22 words all land at about 1.6–1.9 ms per restore, inside run-to-run noise. Cost is the full table scan, about 94k slot compares per restore.
- Timing still varies with input length when a word is longer than 31 bytes. That length is the caller's own input, so it reveals nothing new.
- Not built yet, because each depends on an OPEN item: key derivation (counter vs custom salt separation), revision and head objects (integer/time encoding, list framing, field order), signing (signature coverage, signing mode).

- Decided by Anja: the phrase is not case-sensitive. Restore folds ASCII A–Z to a–z branch-free before lookup; non-ASCII capitals are not folded. identifiers.json regenerated with 3 new valid case vectors and a non-ASCII-capital invalid one (sha256 aeb912f22f6347f7ccfd23fe10d910571efa1da9372ac1d608f8def5970c9b0f). Go tests pass; restore timing unchanged across failure kinds.

## 2026-10-14: calls made on Anja's delegation ("ill leave those to your informed judgment")

- Tag names kept as proposed, plus `forum:v0:publication-key`.
- Phrase whitespace: trim ASCII whitespace per word; text form splits on runs of ASCII whitespace; non-ASCII spaces are not touched (a phrase with one fails like any off-list word). Reason: covers copy-paste and phone keyboards without widening what counts as a valid phrase. identifiers.json regenerated (sha256 f6d80bd0b3eb0bec7b102c94a4dd113fc73ca1b23302cc13576f5a7e05f8f38c), now 19 word-array and 7 text vectors.
- Order of blockers: salt separation first, because it is the smallest and completes master seed → publication address end to end. Encoding (integers, times, lists) and signing are next and are left for Anja.
- Salt separation: a one-byte kind field (0x00 counter, 0x01 custom). Counter is u64 big-endian from 0; custom is non-empty UTF-8, bytes as given. Unicode normalization of custom salts left OPEN, because it decides whether a salt retyped on another device still recovers the key.
- `gen-derivation/gen.py` (pyskein) writes `vectors/derivation.json`: 14 vectors, two runs identical (sha256 78e05d7b4473e2d665a63818f85a9610cfe37e8c56ab4a9b499fe9a28d4c707a). It asserts that a custom salt whose bytes equal counter 1 derives a different key.
- Go `keys` package: `Counter`, `Custom`, `Seed`, `Derive` (Go 1.27 crypto/mldsa), `IdentityID`, `PublicationDigest`, `Address`. Tests pass against all 14 vectors.
- Full-chain cross-check, harness outside the repo: circl v1.6.5 built the same public key as Go stdlib from all 14 derived seeds, and an independent Python computation of the address from circl's key matched the Go `keys.Address` 14/14.

## 2026-10-14: MVP push — Anja: "We have 3 days to get the working mvp, so please make the choices. drop all accents - normalize"

- Custom-salt normalization (her call): full case fold, NFKD, drop Mn, fixed map for ø ł đ ħ ı ŀ ŧ æ œ ß þ ð ŋ ĸ, collapse whitespace, NFC. Go uses x/text (cases.Fold, norm); Python generator uses str.casefold and unicodedata. 32 normalization vectors agree in both. derivation.json sha256 b330560a5a5004dbdba720b934d9c350b72bcb29e990cd85f156fec1685d2c63. New dependency: golang.org/x/text v0.43.0.
- Calls made by Claude under that instruction, all now DECIDED in SPEC:
  - Integers u64 big-endian; times signed 64-bit Unix milliseconds; kinds one byte; empty field means absent for optional fields; lists are framed items inside one field; records are framed parts with no tag.
  - Document id = H(document-id tag ‖ publication digest ‖ 16-byte nonce): stable across revisions and impossible for another publication to claim.
  - Objects and field order: revision, asset, head, signed envelope, bundle (SPEC §6).
  - Signature coverage: every revision and every head signed, plus a parent pointer in each revision.
  - Signing mode: hedged for producers.
  - Verification adds key pinning and sequence rollback rejection (stage-2 pinning comes free once a head is stored).
  - MVP transport: plain HTTP paths under /forum/v0/{pub}/; known_copies in the head are location hints, never authority.
  - Word list frozen as v0 with no length cap. Link statements deferred past the MVP.

## 2026-10-14: working MVP

- Object layer (`obj`): revision, asset, head, signed envelope, bundle; strict decoders (fixed widths, UTF-8, no line breaks in titles, list items must fill their field, no trailing bytes).
- `gen-objects/gen.py` (pyskein) writes `vectors/objects.json`, sha256 06d37fe8cbcf8e09f5d75a78e736bdd45d8964afa337186421c3318cd8271313. `gen-objects/sign` (Go stdlib, deterministic signing) writes `vectors/verification.json` from those bytes, sha256 ee9dcc4e5cfed8fd086ef7a17ffc9342a89ea4a66154f16e8a1d88cd9b7595d7: 3 valid and 8 failing cases, each naming the check that must fail. Both reproduce byte-for-byte across two runs. circl v1.6.5 verified the head and both revision signatures.
- `verify`: the §8 procedure; every failure names its check. Unverified material is logged (`forum log`) and never stored, and never replaces verified material.
- `store`: SQLite through modernc.org/sqlite v1.60.1 (pure Go, no cgo). Holds publications, revisions, assets, owned salts, follows, and the verification log.
- `node`: init/restore seed, new publication, publish/revise/remove, details, assets, export/import, fetch/follow/sync over HTTP, adopt after restore.
- `web`: public reading site (no JavaScript, CSP `default-src 'none'`, raw HTML in Markdown dropped, `javascript:` links dropped, forum:// and asset: links rewritten to local paths, Atom feed, `.md` source view, protocol endpoints). Writer UI on a separate loopback-only listener: Host and peer must be loopback, POSTs need a per-process token and a same-origin Origin/Referer.
- Revision `created` means when this revision was made; `published` stays fixed through edits. SPEC §6.1 says so.
- Seed storage: plain hex in `seed`, mode 0600. No passphrase encryption in the MVP.
- Tests: unit vectors in every package, plus `e2e`: author → host → replica → reader; edit + sync; a host that alters a revision (caught: revision id), serves an older head (caught: head sequence), or serves another key's head (caught: publication address); bundle round trip; restore from the 23 words + adopt + publish continuing at the next sequence; custom salts "Café Poems" and "cafe  POEMS" give one address. All pass.
- Manual smoke with the binary: three homes, two live servers, writer edits through curl with the token, replica syncs, cross-origin POST refused (403), writer refused on a non-loopback Host (403).
- Binaries built (CGO_ENABLED=0): linux amd64/arm64, darwin arm64/amd64, windows amd64.
- Not in this MVP: replies, follow objects, aliases, link statements, multi-author publications, seed passphrase, TLS (serve behind a reverse proxy for https), NAT traversal.

## 2026-10-14: agents as peers, Markdown native

- Anja: "AI integration is first class, peer grade, and totally equal. Markdown is native." Then: "there is no llms.txt" / "on purpose, no robots.txt either". SPEC §1.1 records it. A brief llms.txt route existed during this change and was removed before anything shipped; a test now asserts /llms.txt and /robots.txt are 404.
- Markdown parity: `/index.md`, `/{pub}.md`, `/{pub}/{doc}.md`, and `Accept: text/markdown` on any page path (Vary: Accept, plus Link rel=alternate headers and <link> tags). Document Markdown carries a YAML header (address, publication, kind, title, published, revised, revision, parent_revision, verified) then `# title` and the canonical body unchanged. CLI `forum read`, the web, and MCP `read` produce the same Markdown.
- `forum mcp`: stdio MCP server (JSON-RPC, protocol 2025-06-18, no SDK dependency) with the same powers as the writer and CLI: list_publications, read, fetch, follow, sync, new_publication, publish (new or revision), remove, set_details, add_asset, verification_log. `--read-only` offers only the reading tools. The seed phrase is not offered over MCP.
- Tests: `mcp` (an agent creates a publication with a custom salt, publishes, revises, reads both views; read-only mode refuses publish) and `e2e/markdown_test.go` (every view in Markdown by suffix and by Accept; HTML unchanged; no llms.txt, no robots.txt). Full suite passes. Binary smoke: `forum mcp` on the replica home returned the edited post with its parent revision.
- GitHub: not pushed. This session has no GitHub access to Pnakotic-Codebase/forum.p2p (the API answers that the repository is not enabled for the session).
