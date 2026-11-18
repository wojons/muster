import json

p = '/home/kara/wojons/muster/.coding-hermes/board/tasks.jsonl'
ids = {'QA-MUSTER-052', 'QA-MUSTER-053', 'QA-MUSTER-054'}
found = {}
for line in open(p, encoding='utf-8', errors='replace'):
    line = line.strip()
    if not line:
        continue
    try:
        r = json.loads(line)
    except Exception:
        continue
    if r.get('id') in ids:
        found[r.get('id')] = r
for i in sorted(ids):
    r = found.get(i)
    if not r:
        print(i, "MISSING")
        continue
    print(i, "status=%s priority=%s complexity=%s type=%s" % (
        r.get('status'), r.get('priority'), r.get('complexity'), r.get('type')))
    print("   title:", str(r.get('title'))[:110])
    print("   keys:", ','.join(sorted(r.keys())))
    print("   ac_chars=%d reasoning_chars=%d tags=%s" % (
        len(str(r.get('acceptance_criteria') or '')), len(str(r.get('reasoning') or '')),
        r.get('capability_tags')))

ev = '/home/kara/wojons/muster/.coding-hermes/board/events.jsonl'
want = {'task_created'}
count = 0
for line in open(ev, encoding='utf-8', errors='replace'):
    line = line.strip()
    if not line:
        continue
    try:
        e = json.loads(line)
    except Exception:
        continue
    if not isinstance(e, dict):
        continue
    name = e.get('event') or e.get('event_type')
    if name in want and str(e.get('task_id') or e.get('id') or '') in ids:
        count += 1
        print("event", name, e.get('task_id') or e.get('id'), "event_id=", e.get('id'))
print("paired task_created events:", count)
