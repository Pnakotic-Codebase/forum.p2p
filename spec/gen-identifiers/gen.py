import csv
import json
import struct
import sys

import skein

ALPHABET = "0123456789abcdefghjkmnpqrstvwxyz"


def framed(fields):
    return b"".join(struct.pack(">I", len(f)) + f for f in fields)


def tagged(tag, *fields):
    return skein.skein512(framed([tag.encode("ascii"), *fields]), digest_bits=512).digest()


def address_id(digest):
    v = int.from_bytes(digest, "big") >> (512 - 110)
    return "".join(ALPHABET[(v >> (5 * (21 - i))) & 31] for i in range(22))


def decode_address(s):
    s = s.replace("-", "").lower().replace("i", "1").replace("l", "1").replace("o", "0")
    if len(s) != 22 or any(c not in ALPHABET for c in s):
        return None
    v = 0
    for c in s:
        v = (v << 5) | ALPHABET.index(c)
    return v


def load_words(path):
    rows = list(csv.DictReader(open(path, encoding="utf-8"), delimiter="\t"))
    words = [r["word"] for r in rows]
    assert len(words) == 4096 and words == sorted(words)
    assert [int(r["index"]) for r in rows] == list(range(4096))
    return words


def phrase(seed, words):
    check = int.from_bytes(tagged("forum:v0:seed-checksum", seed)[:3], "big") >> 4
    v = (int.from_bytes(seed, "big") << 20) | check
    return [words[(v >> (12 * (22 - i))) & 0xFFF] for i in range(23)]


def fold(w):
    return "".join(chr(ord(c) | 0x20) if "A" <= c <= "Z" else c for c in w)


WS = " \t\n\v\f\r"


def restore(ws, words):
    ws = [fold(w.strip(WS)) for w in ws]
    if len(ws) != 23 or any(w not in words for w in ws):
        return None
    v = 0
    for w in ws:
        v = (v << 12) | words.index(w)
    seed = (v >> 20).to_bytes(32, "big")
    if phrase(seed, words) != ws:
        return None
    return seed


def seq(start):
    return bytes((start + i) & 0xFF for i in range(32))


words = load_words(sys.argv[1])
sig = json.load(open(sys.argv[2]))

addr = []
seen = set()
for v in sig["vectors"]:
    pk = bytes.fromhex(v["public_key_hex"])
    if pk in seen:
        continue
    seen.add(pk)
    pub = tagged("forum:v0:publication-id", pk)
    ident = tagged("forum:v0:identity-id", pk)
    addr.append({
        "id": f"adr-{len(addr) + 1:03d}",
        "description": f"public key from {v['id']}",
        "public_key_hex": pk.hex(),
        "publication_digest_hex": pub.hex(),
        "publication_id": address_id(pub),
        "identity_id_hex": ident.hex(),
    })

for d, raw in [("all-zero digest", bytes(64)), ("all-ones digest", b"\xff" * 64)]:
    addr.append({
        "id": f"adr-{len(addr) + 1:03d}",
        "description": d,
        "publication_digest_hex": raw.hex(),
        "publication_id": address_id(raw),
    })

canon = addr[0]["publication_id"]
canon_v = decode_address(canon)
dec = []


def dcase(desc, text, valid):
    got = decode_address(text)
    ok = got is not None and got == canon_v
    assert ok == valid, (desc, text)
    dec.append({"id": f"dec-{len(dec) + 1:03d}", "description": desc, "input": text,
                "expect_valid": valid, "expected_id": canon if valid else None})


dcase("canonical lowercase", canon, True)
dcase("uppercase", canon.upper(), True)
dcase("hyphenated in groups of 4", "-".join(canon[i:i + 4] for i in range(0, 22, 4)), True)
for ch, alias in [("1", "i"), ("1", "l"), ("0", "o")]:
    if ch in canon:
        k = canon.index(ch)
        dcase(f"'{alias}' read as '{ch}'", canon[:k] + alias + canon[k + 1:], True)
dcase("one character short", canon[:-1], False)
dcase("one character long", canon + "0", False)
dcase("letter u is not in the alphabet", "u" + canon[1:], False)

seeds = [
    ("seq(0x00)", seq(0x00)),
    ("fill(0x42)", b"\x42" * 32),
    ("all zero", bytes(32)),
    ("all 0xff", b"\xff" * 32),
    ("seq(0x20)", seq(0x20)),
]
phr = []
for d, s in seeds:
    p = phrase(s, words)
    assert restore(p, words) == s
    phr.append({"id": f"phr-{len(phr) + 1:03d}", "description": d, "seed_hex": s.hex(),
                "words": p, "expect_valid": True})

for d, ws in [("first word capitalised", [phr[0]["words"][0].capitalize()] + phr[0]["words"][1:]),
              ("all uppercase", [w.upper() for w in phr[0]["words"]]),
              ("mixed case", [w.swapcase() if i % 2 else w for i, w in enumerate(phr[0]["words"])])]:
    assert restore(ws, words) == seeds[0][1]
    phr.append({"id": f"phr-{len(phr) + 1:03d}", "description": d, "seed_hex": seeds[0][1].hex(),
                "words": ws, "expect_valid": True, "canonical_words": phr[0]["words"]})

ws0 = phr[0]["words"]
for d, ws in [("words with surrounding spaces and tabs", [f" {w}\t" if i % 3 == 0 else w for i, w in enumerate(ws0)]),
              ("last word with trailing newline", ws0[:-1] + [ws0[-1] + "\r\n"])]:
    assert restore(ws, words) == seeds[0][1]
    phr.append({"id": f"phr-{len(phr) + 1:03d}", "description": d, "seed_hex": seeds[0][1].hex(),
                "words": ws, "expect_valid": True, "canonical_words": ws0})

base = phr[0]["words"]
bad = []
w = list(base); w[3] = words[(words.index(w[3]) + 1) % 4096]
bad.append(("one word replaced by its list neighbour", w))
w = list(base); w[0], w[1] = w[1], w[0]
bad.append(("first two words swapped", w))
w = list(base); w[7] = w[7] + "x"
bad.append(("word not on the list", w))
bad.append(("22 words", base[:22]))
bad.append(("24 words", base + [base[0]]))
bad.append(("non-ASCII capital is not folded", ["\u0130" + base[0][1:]] + base[1:]))
bad.append(("empty phrase", []))
w = list(base); w[4] = "\u00a0" + w[4]
bad.append(("non-ASCII space is not trimmed", w))
w = list(base); w[4] = w[4][:3] + " " + w[4][3:]
bad.append(("space inside a word", w))
for d, w in bad:
    assert restore(w, words) is None, d
    phr.append({"id": f"phr-{len(phr) + 1:03d}", "description": d, "words": w,
                "expect_valid": False, "expected_error": "the key was not found"})

def restore_text(t, words):
    return restore([w for w in "".join(" " if c in WS else c for c in t).split(" ") if w], words)


txt = []
for d, t, valid in [
    ("single spaces", " ".join(base), True),
    ("newlines, tabs, repeated spaces, leading and trailing whitespace",
     "\n  " + "\t".join(base[:8]) + "\r\n" + "   ".join(base[8:16]) + "\n" + " ".join(base[16:]) + " \n", True),
    ("upper case, one line", " ".join(base).upper(), True),
    ("non-ASCII space between two words", " ".join(base[:5]) + "\u00a0" + " ".join(base[5:]), False),
    ("22 words", " ".join(base[:22]), False),
    ("empty text", "", False),
    ("whitespace only", " \t\n ", False),
]:
    got = restore_text(t, words)
    assert (got == seeds[0][1]) == valid and (valid or got is None), d
    e = {"id": f"txt-{len(txt) + 1:03d}", "description": d, "text": t, "expect_valid": valid}
    if valid:
        e["seed_hex"] = seeds[0][1].hex()
    else:
        e["expected_error"] = "the key was not found"
    txt.append(e)

json.dump({
    "suite": "forum-spec/identifiers",
    "rules": {
        "tagged_digest": "Skein-512-512(frame(tag, fields...)), frame per encoding-framing",
        "address_id": "first 110 bits of the 64-byte digest, MSB first, 22 lowercase Crockford base32 chars",
        "seed_phrase": "restore trims ASCII whitespace (space, tab, LF, VT, FF, CR) around each word and folds ASCII A-Z to a-z only; text form splits on runs of ASCII whitespace; 23 words; 256-bit seed then first 20 bits of tagged(forum:v0:seed-checksum, seed); 12 bits per word, MSB first",
    },
    "wordlist": "wordlist/forum-wordlist-v0.tsv",
    "address_vectors": addr,
    "address_decode_vectors": dec,
    "phrase_vectors": phr,
    "phrase_text_vectors": txt,
}, sys.stdout, indent=2, ensure_ascii=False)
sys.stdout.write("\n")
