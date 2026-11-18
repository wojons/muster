import json, re
p = '/home/kara/wojons/muster/.coding-hermes/board/tasks.jsonl'
pat = re.compile(r'health/ready|not_ready|readiness|X-Forwarded|Forwarded-For|WARN\] metrics|metrics listener', re.I)
for line in open(p, encoding='utf-8', errors='replace'):
    line = line.strip()
    if not line:
        continue
    try:
        r = json.loads(line)
    except Exception:
        continue
    blob = ' '.join(str(r.get(k, '')) for k in ('id', 'title', 'reasoning', 'detail', 'foreman_note', 'acceptance_criteria'))
    if pat.search(blob):
        print(r.get('status'), '|', r.get('id'), '|', str(r.get('title'))[:160])
