import json
import struct
import sys

import unicodedata

import skein

TAG = "forum:v0:publication-key"
COUNTER, CUSTOM = b"\x00", b"\x01"


def framed(fields):
    return b"".join(struct.pack(">I", len(f)) + f for f in fields)


def derive(master, kind, value):
    return skein.skein512(framed([TAG.encode("ascii"), master, kind, value]), digest_bits=512).digest()[:32]


LETTERS = {"ø": "o", "ł": "l", "đ": "d", "ħ": "h", "ı": "i", "ŀ": "l", "ŧ": "t",
           "æ": "ae", "œ": "oe", "ß": "ss", "þ": "th", "ð": "d", "ŋ": "n", "ĸ": "k"}


def is_space(c):
    return c in "\t\n\v\f\r\x85" or unicodedata.category(c) in ("Zs", "Zl", "Zp")


def normalize(s):
    s = unicodedata.normalize("NFKD", s.casefold())
    out, space = [], False
    for c in s:
        if unicodedata.category(c) == "Mn":
            continue
        if is_space(c):
            space = bool(out)
            continue
        if space:
            out.append(" ")
            space = False
        out.append(LETTERS.get(c, c))
    return unicodedata.normalize("NFC", "".join(out))


def seq(start):
    return bytes((start + i) & 0xFF for i in range(32))


masters = [("seq(0x00)", seq(0x00)), ("fill(0x42)", b"\x42" * 32)]
cases = [
    ("counter 0", COUNTER, struct.pack(">Q", 0)),
    ("counter 1", COUNTER, struct.pack(">Q", 1)),
    ("counter 2^64-1", COUNTER, struct.pack(">Q", 2**64 - 1)),
    ("custom \"my poetry blog\"", CUSTOM, "my poetry blog"),
    ("custom \"secret01\"", CUSTOM, "secret01"),
    ("custom UTF-8 \"Grøstl → Skein\"", CUSTOM, "Grøstl → Skein"),
    ("custom whose bytes equal counter 1", CUSTOM, struct.pack(">Q", 1).decode()),
]

out = []
for mname, m in masters:
    for cname, kind, value in cases:
        e = {"id": f"drv-{len(out) + 1:03d}", "description": f"master {mname}, {cname}",
             "master_seed_hex": m.hex(), "salt_kind": "counter" if kind == COUNTER else "custom"}
        if kind == CUSTOM:
            e["salt_input"] = value
            value = normalize(value).encode()
        e["salt_value_hex"] = value.hex()
        e["mldsa65_seed_hex"] = derive(m, kind, value).hex()
        out.append(e)

seeds = [v["mldsa65_seed_hex"] for v in out]
assert out[6]["mldsa65_seed_hex"] != out[1]["mldsa65_seed_hex"]

groups = [
    ("cafe", ["café", "CAFÉ", "Cafe", "cafe\u0301", "  Café  ", "caf\u00e9\u00a0"]),
    ("my poetry blog", ["My Poetry Blog", "my  poetry\tblog", " MY POETRY BLOG\n", "Mý Pöetry Blög"]),
    ("smorrebrod", ["Smørrebrød", "SMØRREBRØD"]),
    ("strasse", ["Straße", "STRASSE", "strasse"]),
    ("istanbul", ["İstanbul", "ISTANBUL", "istanbul"]),
    ("fiancee", ["ﬁancée", "Fiancée"]),
    ("σοφια", ["ΣΟΦΙΑ", "σοφία", "Σοφίας"[:-1]]),
    ("東京", ["東京", " 東京 "]),
    ("aeon", ["Æon", "æon", "AEON"]),
]
norm_vecs = []
for want, inputs in groups:
    for raw in inputs:
        got = normalize(raw)
        assert got == want, (raw, got, want)
        norm_vecs.append({"id": f"nrm-{len(norm_vecs) + 1:03d}", "input": raw, "normalized": want,
                          "normalized_hex": want.encode().hex()})
for raw in ["", "   ", "\u0301", "\t\u00a0\u2003"]:
    assert normalize(raw) == ""
    norm_vecs.append({"id": f"nrm-{len(norm_vecs) + 1:03d}", "input": raw, "normalized": "",
                      "expect_error": "custom salt is empty after normalization"})
assert len(set(seeds)) == len(seeds)

json.dump({
    "suite": "forum-spec/publication-key-derivation",
    "rule": "mldsa65_seed = first 32 bytes of Skein-512-512(frame(\"forum:v0:publication-key\", master_seed, kind, value)); kind 0x00 counter (value u64 big-endian), 0x01 custom (value = UTF-8 bytes of the normalized salt, non-empty)",
    "normalization": "casefold, NFKD, drop Mn, map ø ł đ ħ ı ŀ ŧ æ œ ß þ ð ŋ ĸ, collapse runs of whitespace (Go unicode.IsSpace) to one space and trim, NFC",
    "vectors": out,
    "normalization_vectors": norm_vecs,
}, sys.stdout, indent=2, ensure_ascii=False)
sys.stdout.write("\n")
