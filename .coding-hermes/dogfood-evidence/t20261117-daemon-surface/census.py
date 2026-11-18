import json
from collections import Counter

p = '/home/kara/wojons/muster/.coding-hermes/board/tasks.jsonl'
rows = []
bad = 0
for line in open(p, encoding='utf-8', errors='replace'):
    line = line.strip()
    if not line:
        continue
    try:
        rows.append(json.loads(line))
    except Exception:
        bad += 1
print("rows", len(rows), "unparsed", bad)
print(Counter(r.get('status') for r in rows))
term = ('complete', 'closed', 'done', 'abandoned', 'cancelled')
pend = [r for r in rows if r.get('status') not in term]
print("non-terminal:", len(pend))
for r in pend:
    print(r.get('id'), '|', r.get('status'), '|', r.get('priority'), '|', str(r.get('title'))[:100])
