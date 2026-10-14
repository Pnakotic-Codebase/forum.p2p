import collections
import csv
import sys


def keys(w):
    return {w} | {w[:i] + w[i + 1:] for i in range(len(w))}


rows = list(csv.DictReader(open(sys.argv[1], encoding="utf-8"), delimiter="\t"))
seen = set()
words = []
dupes = []
for r in rows:
    w = r["word"]
    if w in seen:
        dupes.append(w)
        continue
    seen.add(w)
    words.append(w)

bad = [w for w in words if not (w.isascii() and w.isalpha() and w.islower())]
print("rows:", len(rows), "distinct words:", len(words))
print("duplicates dropped:", dupes)
print("non a-z:", bad)
print("fields:", dict(collections.Counter(r["field"] for r in rows)))
for k in (4, 5):
    print(f"usable, one word per first-{k}-letter prefix:", len({w[:k] for w in words}))
index = set()
greedy = []
for w in sorted(words, key=len):
    ks = keys(w)
    if ks & index:
        continue
    greedy.append(w)
    index |= ks
print("usable, whole words differing by >= 2 letters:", len(greedy))
print("target: 4096")
