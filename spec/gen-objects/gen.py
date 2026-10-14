import json
import struct
import sys

import skein

ALPHABET = "0123456789abcdefghjkmnpqrstvwxyz"


def fr(*fields):
    return b"".join(struct.pack(">I", len(f)) + f for f in fields)


def H(tag, *fields):
    return skein.skein512(fr(tag.encode(), *fields), digest_bits=512).digest()


def address(d):
    v = int.from_bytes(d, "big") >> (512 - 110)
    return "".join(ALPHABET[(v >> (5 * (21 - i))) & 31] for i in range(22))


def u64(v):
    return struct.pack(">Q", v)


def ms(v):
    return struct.pack(">q", v)


sig = json.load(open(sys.argv[1]))
pk = bytes.fromhex(sig["vectors"][0]["public_key_hex"])
pub_digest = H("forum:v0:publication-id", pk)

png = bytes.fromhex("89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c4890000000d4944415478da63f8cfc0f01f0005000201a5f6a5d70000000049454e44ae426082")
assets = []
for mt, content in [("image/png", png), ("text/plain", "plain words\n".encode())]:
    enc = fr(b"forum:v0:asset", mt.encode(), content)
    assets.append({"media_type": mt, "content_hex": content.hex(), "encoded_hex": enc.hex(),
                   "asset_id_hex": H("forum:v0:asset-id", enc).hex()})

a0 = assets[0]["asset_id_hex"]
other = "forum://" + address(H("forum:v0:publication-id", b"x"))
revs = []
specs = [
    dict(kind=1, nonce="00112233445566778899aabbccddeeff", parent=None, created=1760400000000, published=1760400000000,
         title="First post", body="# First post\n\nWritten without a domain.\n", assets=[], refs=[]),
    dict(kind=1, nonce="00112233445566778899aabbccddeeff", parent="REV0", created=1760400000000, published=1760403600000,
         title="First post", body=f"# First post\n\nEdited. ![dot](asset:{a0})\nSee {other} and {other}.\n",
         assets=[a0], refs=[other]),
    dict(kind=2, nonce="ffeeddccbbaa99887766554433221100", parent=None, created=-1, published=0,
         title="About", body="Ünïcödé — about this publication.\r\n", assets=[], refs=[]),
]
for s in specs:
    parent = bytes.fromhex(revs[0]["revision_id_hex"]) if s["parent"] == "REV0" else b""
    nonce = bytes.fromhex(s["nonce"])
    enc = fr(b"forum:v0:revision", bytes([s["kind"]]), pub_digest, nonce, parent, ms(s["created"]), ms(s["published"]),
             s["title"].encode(), s["body"].encode(), fr(*[bytes.fromhex(a) for a in s["assets"]]),
             fr(*[r.encode() for r in s["refs"]]))
    dd = H("forum:v0:document-id", pub_digest, nonce)
    revs.append({"kind": s["kind"], "publication_digest_hex": pub_digest.hex(), "document_nonce_hex": s["nonce"],
                 "parent_revision_id_hex": parent.hex(), "created": s["created"], "published": s["published"],
                 "title": s["title"], "body": s["body"], "assets": s["assets"], "references": s["refs"],
                 "encoded_hex": enc.hex(), "revision_id_hex": H("forum:v0:revision-id", enc).hex(),
                 "document_digest_hex": dd.hex(), "document_address": address(dd)})

entries = [(1, revs[1]), (2, revs[2])]
head_fields = dict(sequence=2, updated=1760403600000, title="Working notes", description="",
                   known_copies=["http://203.0.113.5:4433", "http://[2001:db8::1]:4433"])
enc = fr(b"forum:v0:head", pk, u64(2), ms(head_fields["updated"]), head_fields["title"].encode(), b"",
         fr(*[fr(bytes([k]), bytes.fromhex(r["document_nonce_hex"]), bytes.fromhex(r["revision_id_hex"])) for k, r in entries]),
         fr(*[c.encode() for c in head_fields["known_copies"]]))
head = dict(head_fields, public_key_hex=pk.hex(), signing_seed_hex=sig["vectors"][0]["seed_hex"],
            entries=[{"kind": k, "document_nonce_hex": r["document_nonce_hex"], "revision_id_hex": r["revision_id_hex"]} for k, r in entries],
            encoded_hex=enc.hex(), publication_digest_hex=pub_digest.hex(), publication_address=address(pub_digest))

json.dump({"suite": "forum-spec/objects", "assets": assets, "revisions": revs, "head": head},
          sys.stdout, indent=2, ensure_ascii=False)
sys.stdout.write("\n")
