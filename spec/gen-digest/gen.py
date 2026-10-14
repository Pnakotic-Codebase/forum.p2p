import json
import struct
import sys

import skein


def framed(fields):
    return b"".join(struct.pack(">I", len(f)) + f for f in fields)


def pat(n):
    return bytes((i * 7) & 0xFF for i in range(n))


def desc_pat(n):
    return bytes((0xFF - i) & 0xFF for i in range(n))


cases = [
    ("dig-001", "paper v1.3 Appendix C.2: one byte FF", b"\xff"),
    ("dig-002", "paper v1.3 Appendix C.2: 64 bytes FF..C0", desc_pat(64)),
    ("dig-003", "paper v1.3 Appendix C.2: 128 bytes FF..80", desc_pat(128)),
    ("dig-004", "empty input", b""),
    ("dig-005", "ASCII forum://", b"forum://"),
    ("dig-006", "63 bytes, one short of a block", pat(63)),
    ("dig-007", "64 bytes, exactly one block", pat(64)),
    ("dig-008", "65 bytes, one past a block", pat(65)),
    ("dig-009", "129 bytes, one past two blocks", pat(129)),
    ("dig-010", "70000 bytes", pat(70000)),
    ("dig-011", "framed object: fields [\"forum\"] (same bytes as enc-001)", framed([b"forum"])),
    ("dig-012", "framed object: fields [\"\", \"\"] (same bytes as enc-002)", framed([b"", b""])),
    ("dig-013", "framed object: UTF-8 fields (same bytes as enc-007)", framed(["Grøstl → Skein".encode(), b"# Hello\n"])),
]

out = []
for cid, d, m in cases:
    out.append({
        "id": cid,
        "description": d,
        "message_hex": m.hex(),
        "digest_hex": skein.skein512(m, digest_bits=512).hexdigest(),
    })

anchors = {
    "dig-001": "71b7bce6fe6452227b9ced6014249e5bf9a9754c3ad618ccc4e0aae16b316cc8ca698d864307ed3e80b6ef1570812ac5272dc409b5a012df2a579102f340617a",
}
for v in out:
    if v["id"] in anchors and v["digest_hex"] != anchors[v["id"]]:
        sys.exit("anchor mismatch " + v["id"])

json.dump({
    "suite": "forum-spec/digest",
    "algorithm": "Skein-512-512, Skein v1.3, plain configuration (no personalization, key, or public-key binding)",
    "generator": "pyskein (binding over the Skein reference C code)",
    "vectors": out,
}, sys.stdout, indent=2)
sys.stdout.write("\n")
