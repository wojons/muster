import json, re
p = '/home/kara/wojons/muster/.coding-hermes/board/tasks.jsonl'
pat = re.compile(r'daemon|metrics|/health|tenant|socket|9110', re.I)
rows = []
for line in open(p, encoding='utf-8', errors='replace'):
    line = line.strip()
    if not line:
        continue
    try:
        rows.append(json.loads(line))
    except Exception:
        pass
for r in rows:
    blob = ' '.join(str(r.get(k, '')) for k in ('id', 'title', 'reasoning', 'detail', 'foreman_note', 'acceptance_criteria'))
    if pat.search(blob):
        print(r.get('status'), '|', r.get('id'), '|', str(r.get('title'))[:150])
