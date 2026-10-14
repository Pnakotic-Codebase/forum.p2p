# forum:// Canonical Format — v0 draft

Status: DRAFT. Language-neutral byte-level contract for the forum:// Publishing MVP v0.1.
Any implementation in any language MUST be able to produce and verify the objects defined here
from this document and the test vectors alone, without reference to another implementation's code.

Keywords MUST, MUST NOT, SHOULD, MAY follow RFC 2119.

Each section is marked:

- **DECIDED**: fixed for v0, and test vectors exist or will exist for it.
- **OPEN**: not yet decided. The alternatives are listed, and nothing in this document presumes one of them.

## 1. Scope

In scope: identity keys, signatures, digests, identifiers, canonical encoding, object field sets,
signature coverage, and the verification procedure.

Out of scope: transport, peer discovery, replication protocol, WebAccess rendering, UI.

## 1.1 Readers and authors — DECIDED (Anja: "AI integration is first class, peer grade, and totally equal. Markdown is native.")

- The protocol makes no distinction between a person and an AI, as reader, author, publisher or host. No object, tag or
  field records which one made it, and no check treats them differently.
- Markdown is the native form. The canonical object is Markdown (§6.1); HTML is one rendering of it for browsers.
- Every human-facing view has a Markdown equal with the same content: append `.md` to the path, or ask with
  `Accept: text/markdown`. Nobody gets a lesser or separate channel.
- On purpose there is no `llms.txt` and no `robots.txt`: no side files that address machines apart from people, and no
  gatekeeping by kind of reader.
- An agent operating a node has the powers a person operating it has (read, publish, revise, remove, follow, re-host),
  through the same node. The reference implementation offers them over MCP (`forum mcp`) as well as the CLI and the
  browser writer.

## 2. Signature scheme — DECIDED

- Algorithm: ML-DSA-65, as specified in FIPS 204.
- Private key storage form: the 32-byte seed ξ. The full private key is re-derived through
  FIPS 204 KeyGen_internal(ξ). The seed is the identity backup material (MVP §19).
- Public key encoding: the FIPS 204 encoding, 1,952 bytes.
- Signature encoding: the FIPS 204 encoding, 3,309 bytes.
- Verification MUST reject any signature whose length is not 3,309 bytes (vector `sig-104`).

Vectors: `vectors/signature-mldsa65.json`. These were generated with Go 1.27 `crypto/mldsa` and
independently reproduced byte-for-byte by `cloudflare/circl` v1.6.5 `mldsa65`.

### 2.1 Signing mode — DECIDED: hedged for producers

Verification is identical in both modes. The choice affects only what producers emit.

- **Hedged** (FIPS 204 default, fresh randomness per signature). This resists fault and side-channel
  attacks. Signatures over the same message differ each time.
- **Deterministic** (rnd = 0^32). The same input always gives the same signature, which makes
  reproducible builds and diffing easier.

Producers sign in hedged mode. The vectors use deterministic mode so that they reproduce. They pass under either mode, because verifiers do not care.

### 2.2 Domain separation — DECIDED: tagged bytes

Signatures and digests over different object types MUST NOT be interchangeable. forum:// does this in its own bytes:
every encoded object begins with a domain tag as its first field (§5). The mechanism works the same for any signature
scheme and any digest, and needs nothing beyond plain Skein and plain ML-DSA.

Not used: Skein personalization (§3.2), which no tested Rust crate exposes. The ML-DSA context string is left empty,
since the tag already lives inside the signed message.

Tag shape — DECIDED: `forum:v0:<kind>`, ASCII, carried as field 0 under §5 framing. A tagged digest is
`H(tag ‖ x₁ ‖ … ‖ xₙ) = Skein-512-512(frame(tag, x₁, …, xₙ))`.

Kinds in use — DECIDED (names chosen by Claude on Anja's delegation, 2026-10-14):

| Tag | Used for |
|---|---|
| `forum:v0:identity-id` | §4.1 identity identifier over a public key |
| `forum:v0:publication-id` | §4.2 publication identifier over a public key |
| `forum:v0:seed-checksum` | §4.2 word-list checksum over the master seed |
| `forum:v0:publication-key` | §4.2 per-publication key derivation from the master seed |
| `forum:v0:document-id` | §4.3 document identifier |
| `forum:v0:revision` | §6.1 revision object |
| `forum:v0:revision-id` | §4.4 revision identifier |
| `forum:v0:asset` | §6.2 asset object |
| `forum:v0:asset-id` | §6.2 asset identifier |
| `forum:v0:head` | §6.3 publication head |
| `forum:v0:signed` | §6.4 signed envelope |
| `forum:v0:bundle` | §6.5 bundle |

## 3. Digest function — DECIDED: Skein v1.3

Used for identifiers and content addressing.

- Algorithm: Skein **version 1.3** (Ferguson et al., 1 Oct 2010), with the Skein-512 internal state.
  Earlier versions (1.0, 1.1, 1.2) produce different digests and MUST NOT be used.
- Anchor: an implementation MUST reproduce Appendix C.2 of the v1.3 paper for Skein-512-512
  (for example, message `FF` gives a digest beginning `71B7BCE6FE645222`).
- Reason for choosing it: a SHA-3 finalist with no full-round break. Standardization was not
  required for its own sake. This choice does not remove standardization elsewhere: §2 stays ML-DSA-65.

### 3.1 Output size — DECIDED: Skein-512-512 everywhere

Every digest in forum:// is Skein-512-512 with the full 64-byte output: revision identifiers, identity and
publication identifiers, and content addressing. Nothing is truncated.

Vectors: `vectors/digest-skein512-512.json`.

Background, kept for the record. The floor: any digest a signature relies on (revision identifiers, identity and publication identifiers) gives
collision resistance of half its output bits. 128 bits of output gives 2⁶⁴ collision resistance. That is within the range of
publicly demonstrated collision attacks (the SHA-1 collision took roughly 2⁶³ work), so 128-bit digests are only suitable for
labels nothing trusts.

- **Skein-512-256.** 32-byte digests. The designers' recommended general-purpose default.
- **Skein-512-512.** 64-byte digests, twice the margin, longer identifiers.
- **Skein-512-N with truncation for identifiers only** (for example, full 512 for content addressing, shorter in addresses).

### 3.2 Skein configuration — DECIDED: plain hashing only

forum:// uses plain Skein-512 with no personalization, key, or public-key binding in the configuration block. Domain
separation is done in the encoded bytes instead (§2.2). That keeps every tested implementation usable as-is: the reference C code,
the Go packages, and all three Rust crates.

Not considered further: MurmurHash (not cryptographic; seed-independent multicollisions are known), and Grøstl (sound, but its implementation ecosystem is thinner than Skein's).

## 4. Identifiers — DECIDED

### 4.1 Identity identifier

The 1,952-byte public key is too large to carry in an address, so the identity identifier is a digest of the key:
`identity_id = H(domain_tag ‖ public_key)`, the full 64-byte Skein-512-512 output (§3.1).

### 4.2 Publication identifier — DECIDED: one master secret, unlinkable publications, linking only by disclosure

Governing rule: nobody can tell that two publications share an owner unless the owner tells them.

- **Master seed — DECIDED.** 32 bytes carrying at least 256 bits of entropy. The spec requires the entropy, not any
  particular source: a physical process, dice, ambient measurement, or anything else that delivers it. A memorable phrase
  never stands in for the master seed.
- **Word-list form — DECIDED: forum://'s own list.** The master seed can be written as and restored from a list of words
  drawn from a list of forum://'s own, not a common or expectable one.
  - The list is part of the spec and ships in every implementation, so it is public. All of the seed's protection comes
    from its entropy; the words only spell it out.
  - Size: 4096 words, 12 bits per word. **Phrase length — DECIDED: 23 words**, 276 bits: the 256-bit seed followed by a 20-bit checksum.
  - **Checksum — DECIDED:** the first 20 bits of `H("forum:v0:seed-checksum" ‖ seed)` (tagged per §2.2). A wrong but valid word passes about once in 2²⁰.
  - **Packing:** seed bits then checksum bits, most-significant bit first; word *i* is bits 12i…12i+11, read as an index into the list in its a–z order (0–4095).
  - **Case — DECIDED: not case-sensitive.** Restore folds ASCII A–Z to a–z (in constant time) before matching; no other characters are folded. **Whitespace — DECIDED:** ASCII whitespace (space, tab, LF, VT, FF, CR) around each word is trimmed; a phrase given as one text splits on runs of ASCII whitespace. Non-ASCII spaces are neither trimmed nor treated as separators.
  - **Restore failure — DECIDED.** When the recomputed checksum does not match, the implementation reports "the key was not found". A phrase containing a non-list word, the wrong word count, or any other malformed input gets the same message. Restore failure never says what was wrong.
  - **Constant-time restore — DECIDED.** Restore takes the same path whatever the input: every word is looked up in full (no early exit at the first unknown word), the checksum hash always runs over a full-length seed buffer, and the comparison is constant-time. A failed restore must not be told apart from another failed restore, or from a checksum mismatch, by timing.
  - Draft v0 exists: `wordlist/forum-wordlist-v0.tsv` (sha256 e15a4c73…dd5229), archaic and lineage terms that later became science terms, each row recording that term. Frozen for v0.
  - Rules v0 meets: a–z only; every word unique within its first five letters; no word under five letters is the start of another; no two words one edit apart (insert, delete, substitute, adjacent swap); fixed a–z order. No length cap: the list is frozen as v0 (longest word 16 letters).
- **Custom salts — DECIDED.** Each publication's salt is chosen by its owner and may be anything memorable ("my poetry
  blog", "secret01"). All the security rests on the master seed, so salts need no strength of their own. When the owner
  chooses none, the salt defaults to a counter. Two publications under one master seed MUST NOT share a salt, since the
  same salt gives the same key. **Salt separation — DECIDED (Claude, on Anja's delegation):** every salt carries a one-byte kind,
  `0x00` counter or `0x01` custom, as its own framed field, so a counter and a custom salt with the same bytes can never
  give the same key. A counter value is a u64, big-endian, 8 bytes, starting at 0 (this fixes the counter field only, not
  the general integer encoding in §5.2). A custom salt is normalized, then used as its UTF-8 bytes; it must not be empty after normalization.
  **Normalization — DECIDED (Anja: "drop all accents - normalize"):** Unicode full case folding, then NFKD, then every
  nonspacing mark (Mn) is dropped, then the letters ø ł đ ħ ı ŀ ŧ æ œ ß þ ð ŋ ĸ become o l d h i l t ae oe ss th d n k,
  then every run of whitespace becomes one space and leading and trailing whitespace is removed, then NFC. So "Café",
  "CAFE" and "cafe" are the same salt on every device. Vectors: `normalization_vectors` in `vectors/derivation.json`.
  Characters assigned after Unicode 15.0 are not guaranteed to normalize the same way across implementations.
- **Derivation — DECIDED.** Each publication's ML-DSA-65 seed is the first 32 bytes of
  `H("forum:v0:publication-key" ‖ master_seed ‖ salt_kind ‖ salt_value)`, the four inputs framed per §5. That seed
  yields the publication's own key pair, and `publication_id = H("forum:v0:publication-id" ‖ public_key)`.
  Vectors: `vectors/derivation.json`.
- **Ruled out: phrase-only keys ("brain wallets").** Every address is public, so a key derived from a memorable phrase
  alone can be found by offline guessing against all published addresses.
  `publication_id = H(tag ‖ public_key_i)`. Every verifier recomputes it from the publication's public key alone; the
  master seed and the salt never leave the owner.
- **Unlinkability.** Without the master seed, the derived public keys look unrelated, so the protocol itself reveals
  nothing about common ownership.
- **Disclosure.** Linking is an explicit, signed act. To declare that publications A and B share an owner, each
  publication signs a link statement naming the other. Both signatures are required, so no third party can forge a link and
  neither publication can be claimed alone. The statement's encoding is deferred past the MVP.
- **Recovery.** The master seed (or its word list) plus each publication's salt re-creates every publication key.
  Counter salts need no separate record. Custom salts are remembered by their owner.
- Unlinkability holds as long as the master seed stays secret.
- **Limit, recorded so it is not mistaken for a guarantee:** this makes publications unlinkable *by key*. It does not hide
  shared hosting addresses, posting times, writing style, or other metadata. Those are outside the protocol.

Superseded: the public-nonce / shared-key form. Under the governing rule it would disclose ownership by default. The
committed-secret recovery idea moves to §19 (recovery and rotation), where it applies to any key.

### 4.3 Document identifiers — DECIDED

A post or page keeps one identifier across all its revisions. When the document is created, the producer draws a
16-byte `document_nonce` from its entropy source. Every revision of that document carries the same nonce.

`document_digest = H("forum:v0:document-id" ‖ publication_digest ‖ document_nonce)`, where `publication_digest` is the
full 64-byte publication digest (§4.2). The document's address identifier is the first 110 bits of `document_digest`
(§4.5). Binding the publication digest into it means another publication can never claim the same document address.

### 4.4 Revision identifier — DECIDED

`revision_id = H("forum:v0:revision-id" ‖ revision_bytes)`, the full 64 bytes, where `revision_bytes` is the encoded
revision (§6.1). Revision identifiers never appear in addresses.

### 4.5 Address identifiers — DECIDED: 110 bits, 22 characters

Identifiers that appear in addresses (publication identifiers and document identifiers) are the **first 110 bits** of
their full 64-byte Skein-512-512 digest, taken most-significant bit first. They are written as **22 lowercase base32
characters with no padding**. 110 = 22 × 5, so every character carries exactly 5 bits and none is partial.

- A verifier MUST fetch the full public key (or the document's inputs), recompute the full 64-byte digest, and check
  that its first 110 bits equal the address identifier. The short identifier is a commitment to be checked, never a
  source of trust by itself.
- Revision identifiers never appear in addresses. They stay at the full 64 bytes.
- Security of the address: a targeted forgery (a different key landing on one chosen address) costs a second-preimage
  search of 2¹¹⁰. An attacker who would take any address costs 2¹¹⁰ ÷ N for N existing publications (about 2⁸⁰ at N ≈ 2³⁰). Under
  Grover's algorithm the targeted search is about 2⁵⁵ sequential quantum iterations, each of which must run ML-DSA-65 key
  generation and Skein-512 over a 1,952-byte key inside the circuit. Collision resistance (2⁵⁵ classically) is not the relevant
  property: a colliding pair would be two keys held by the same attacker, which gives them nothing a single key does not.
- Address forgery posture, DECIDED as conditional stages, each one triggered by a condition rather than a date:
  1. **MVP:** rely on the 110-bit second-preimage cost alone. If the length ever has to grow, a later version takes more bits of the
     same 64-byte digest, so every existing 22-character address is a prefix of its longer form and nothing breaks.
  2. **When forum:// starts proliferating:** key pinning on follow. A client that follows a publication stores its full public
     key and MUST reject any different key for that address.
  3. **When there is outside interest:** first-seen attestations. Hosts and peers vouch for which key they first saw under an
     address, as a reputation layer.

  Ruled out: a global first-come-first-serve registry, since it requires consensus over global state (MVP §18).
- Alphabet, DECIDED: **Crockford base32**, `0123456789abcdefghjkmnpqrstvwxyz`.
  - Encoders emit lowercase.
  - Decoders accept either case, read `i` and `l` as `1` and `o` as `0`, and ignore hyphens.
  - Crockford's optional check symbol is not used.
  - The 110 bits are split most-significant first into 22 five-bit groups.

Constraint carried from the 64-byte discussion: in `forum://<publication_id>` the identifier sits in the RFC 3986 host
position, which is case-insensitive. A lowercase base32 identifier is unaffected by that.

### 4.6 Address shape

- Publication: `forum://<publication_id>`
- Post or page: `forum://<publication_id>/<document_id>`

Both are DECIDED: shape (MVP §7), length (§4.5, 22 characters each), and derivation (§4.2, §4.3).

## 5. Canonical encoding — DECIDED: length-prefixed fields

These are the bytes that are digested and signed. Every implementation MUST produce identical bytes for the same object.
Length-prefixing was chosen over deterministic CBOR and JCS JSON for compatibility: any language rebuilds it in a few
lines, with no library.

### 5.1 Framing

- A **field** is a 4-byte unsigned big-endian length L followed by exactly L bytes. L MAY be 0.
- An **encoded object** is the concatenation of its fields in the order this document fixes for that object type (§6).
  There are no separators, no padding, and no trailing bytes.
- **Field 0** of every encoded object is its domain tag (§2.2).
- A decoder MUST reject input that ends inside a length prefix, ends inside a field body, or has bytes left over after
  the last field the object type defines.
- A field longer than 2³² − 1 bytes is not representable. An object that would need one is invalid.

Vectors: `vectors/encoding-framing.json`.

### 5.2 Value encodings — DECIDED

| Value | Field bytes |
|---|---|
| unsigned integer (`u64`) | 8 bytes, big-endian |
| time (`ms`) | signed 64-bit, two's complement, big-endian: milliseconds since 1970-01-01T00:00:00Z |
| kind (`u8`) | 1 byte |
| text | UTF-8, MUST be valid. Producers write LF line endings. |
| digest | 64 bytes |
| optional | an empty field (L = 0) means absent; only fields marked optional may be empty |
| list | the items, each framed per §5.1, concatenated; the field's own length bounds the list; an empty list is an empty field |
| record (a list item with several parts) | its parts framed per §5.1 and concatenated, with no tag |

A decoder MUST reject a fixed-width field of the wrong length, a list whose items do not exactly fill the field, and
invalid UTF-8 in a text field.

## 6. Objects — DECIDED

### 6.1 Revision (post or page) — tag `forum:v0:revision`

| # | Field | Encoding |
|---|---|---|
| 0 | tag | `forum:v0:revision` |
| 1 | kind | u8: `0x01` post, `0x02` page |
| 2 | publication_digest | digest (§4.2) |
| 3 | document_nonce | 16 bytes (§4.3) |
| 4 | parent_revision_id | optional digest: the revision this one replaces; empty for a document's first revision |
| 5 | created | ms: when this revision was made |
| 6 | published | ms: the document's publication time, carried unchanged through edits unless the author changes it |
| 7 | title | text, no line breaks |
| 8 | body_markdown | text: the canonical Markdown source |
| 9 | assets | list of asset ids (digest each) |
| 10 | references | list of text: every `forum://` address the body links to, in order of first appearance, without duplicates |

The Markdown source is canonical. HTML is never part of a canonical object (MVP §6). Inside the body, an asset is
referenced as `asset:<asset id in lowercase hex>`; every asset so referenced MUST be listed in field 9.

In this version the publication key is the author: a revision is signed by the key whose digest is field 2. Several
authors per publication is a later version with its own tags.

### 6.2 Asset — tag `forum:v0:asset`

| # | Field | Encoding |
|---|---|---|
| 0 | tag | `forum:v0:asset` |
| 1 | media_type | text, e.g. `image/png` |
| 2 | content | the file's bytes |

`asset_id = H("forum:v0:asset-id" ‖ asset_bytes)`. Assets are not signed on their own; a signed revision that lists
the asset id covers them.

### 6.3 Publication head — tag `forum:v0:head`

| # | Field | Encoding |
|---|---|---|
| 0 | tag | `forum:v0:head` |
| 1 | public_key | the publication's ML-DSA-65 public key, 1,952 bytes |
| 2 | sequence | u64, strictly greater than the previous head's |
| 3 | updated | ms |
| 4 | title | text, no line breaks |
| 5 | description | optional text |
| 6 | entries | list of records: (kind u8, document_nonce 16 bytes, current revision_id digest) |
| 7 | known_copies | list of text: base locations where copies may be fetched, e.g. `http://203.0.113.5:4433`. Hints only: where to look, never who to trust |

`publication_digest = H("forum:v0:publication-id" ‖ public_key)`. Entries appear in the order the publisher chose;
readers sort posts by their `published` time.

### 6.4 Signed envelope — tag `forum:v0:signed`

| # | Field | Encoding |
|---|---|---|
| 0 | tag | `forum:v0:signed` |
| 1 | object | an encoded revision or head |
| 2 | signature | ML-DSA-65 signature over field 1's bytes, empty context, 3,309 bytes |

### 6.5 Bundle — tag `forum:v0:bundle`

A whole publication as one file, for export, backup and migration: tag, the signed head envelope, a list of signed
revision envelopes, and a list of asset objects. Importing a bundle runs the full verification of §8.

## 7. Signature coverage — DECIDED: every revision, every head, with a parent pointer

Each revision is signed. Each head is signed and lists the current revision of every document. Each revision names the
revision it replaces (§6.1 field 4), which gives a document its own lineage. There is no chain across documents, and no
chain across heads beyond the increasing `sequence`.

## 8. Verification procedure — DECIDED

A verifier given only public material (no seed) MUST do the following:

1. Decode the head envelope. Verify the signature with the head's own `public_key`. Recompute `publication_digest` and
   reject unless its first 110 bits equal the requested publication address.
2. If the verifier already holds a head for this publication: reject a different `public_key` (key pinning) and reject a
   `sequence` that is not greater than the held one, unless the bytes are identical.
3. For each entry: decode the revision envelope, verify its signature with the head's `public_key`, recompute
   `revision_id` and compare it with the entry, and check that `kind`, `publication_digest` and `document_nonce` match.
4. For each asset id a revision lists: recompute `asset_id` from the asset bytes and compare.
5. Report material as **verified** or **unverified**. Material obtained from a replica or intermediary that fails any
   check MUST be reported unverified, with the failing check, never silently dropped or silently accepted (MVP §10, §19).
   Unverified material never replaces verified material.

## 9. MVP transport — DECIDED for v0 (HTTP)

Transport stays outside the canonical format: the same objects verify the same way whatever carried them. v0 carries
them over plain HTTP, because every host can serve it and every client can fetch it. Any host is a replica; none is
the authority.

| Request | Response |
|---|---|
| `GET {base}/forum/v0/{pub}/head` | signed head envelope |
| `GET {base}/forum/v0/{pub}/revision/{revision_id hex}` | signed revision envelope |
| `GET {base}/forum/v0/{pub}/asset/{asset_id hex}` | asset object |
| `GET {base}/forum/v0/{pub}/bundle` | bundle |

`{pub}` is the 22-character publication address. Responses are `application/vnd.forum.v0`. A reader resolves
`forum://{pub}` by trying, in order, any location it was given, then the `known_copies` of the newest verified head it
holds.

## 10. Test vectors

| File | Covers | Status |
|---|---|---|
| `vectors/signature-mldsa65.json` | §2: key derivation from seed, signing, verification, 4 valid + 4 invalid | present, cross-checked |
| `vectors/encoding-framing.json` | §5.1 framing, 7 valid + 6 invalid | present, cross-checked (Python, Rust) |
| `vectors/digest-skein512-512.json` | §3, 13 digests incl. the three v1.3 paper C.2 anchors | present, cross-checked (2 Go, 2 Rust) |
| `vectors/identifiers.json` | §2.2 tagged digests, §4.1/§4.2 identity and publication ids, §4.5 address encode/decode, §4.2 23-word phrase (19 word-list and 7 text restores, valid and invalid) | present, cross-checked (Python generator, Go PoC) |
| `vectors/derivation.json` | §4.2 master seed + salt → ML-DSA-65 seed, 2 masters × 7 salts incl. a custom salt equal to counter 1's bytes, plus 32 custom-salt normalization cases | present, cross-checked (Python generator, Go PoC; full chain to address with circl + Python) |
| `vectors/objects.json` | §5.2, §6: asset, revision (first, edited with parent and asset, page with negative time and CRLF body) and head bytes, ids, document addresses | present, cross-checked (Python generator, Go PoC) |
| `vectors/verification.json` | §6.4, §8: one valid publication, re-fetch and lower held sequence, and failures at head signature, publication address, key pinning, head sequence, revision id, revision signature, asset id, revision fetch | present, signatures cross-checked with circl |

Vector file format: each vector gives `seed_hex`, `public_key_hex`, `message_hex`, `context`,
`signature_hex`, and the inputs actually verified (`verify_message_hex`, `verify_context`,
`verify_signature_hex`) with `expect_valid`. A conforming implementation MUST do the following:

- Derive `public_key_hex` from `seed_hex`.
- When producing in deterministic mode, reproduce `signature_hex` exactly.
- Return `expect_valid` for every verify tuple.
