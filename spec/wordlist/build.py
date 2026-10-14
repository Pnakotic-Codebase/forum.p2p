import csv
import sys

PRIORITY = [
    "alchemy", "apparatus", "substance", "process", "heritage", "chemistry-old",
    "arabic", "sanskrit", "chinese", "mining", "pigment", "mineral",
    "materia-medica", "medicine", "pharmacopoeia", "astronomy", "physics",
    "mathematics", "instrument", "acoustics", "optics", "navigation",
    "meteorology", "physiology", "geology", "geologic-time", "metrology",
    "greek-root", "latin-root", "myth-root", "anatomy", "star-name",
    "microbiology", "biology", "botany", "zoology", "binomial",
]
rank = {f: i for i, f in enumerate(PRIORITY)}


def near_keys(w):
    return {w} | {w[:i] + w[i + 1:] for i in range(len(w))}


rows = list(csv.DictReader(open(sys.argv[1], encoding="utf-8"), delimiter="\t"))
maxlen = int(sys.argv[2])
order = sorted(range(len(rows)), key=lambda i: (rank.get(rows[i]["field"], len(PRIORITY)), i))

seen, near, prefixes, chosen = set(), set(), set(), []
for i in order:
    r = rows[i]
    w = r["word"]
    if w in seen or len(w) > maxlen:
        continue
    seen.add(w)
    p = w[:5]
    if p in prefixes or any(q.startswith(p) or p.startswith(q) for q in prefixes if len(q) < 5 or len(p) < 5):
        continue
    ks = near_keys(w)
    if ks & near:
        continue
    prefixes.add(p)
    near |= ks
    chosen.append(r)

print("accepted:", len(chosen), "max length:", maxlen, file=sys.stderr)
if len(chosen) >= 4096:
    final = sorted(chosen[:4096], key=lambda r: r["word"])
    w = csv.writer(sys.stdout, delimiter="\t", lineterminator="\n")
    w.writerow(["index", "word", "later_science_term", "field"])
    for n, r in enumerate(final):
        w.writerow([n, r["word"], r["later_science_term"], r["field"]])
