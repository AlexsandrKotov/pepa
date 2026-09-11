import subprocess, json, collections, sys

out = subprocess.run(["docker", "logs", "pepa-api"], capture_output=True, text=True).stdout
slow = collections.Counter()
count = collections.Counter()
tot_ms = collections.Counter()
windows = collections.Counter()
n = 0
for line in out.splitlines():
    line = line.strip()
    if not line.startswith("{"):
        continue
    try:
        d = json.loads(line)
    except Exception:
        continue
    path = d.get("path")
    dur = d.get("duration")
    if not path or dur is None:
        continue
    n += 1
    ms = dur / 1e6
    count[path] += 1
    tot_ms[path] += ms
    if ms > 2000:
        slow[path] += 1
        t = d.get("time", "")[11:16]
        windows[t] += 1

print(f"total access-log lines with duration: {n}")
print("\n=== paths with requests slower than 2s ===")
for p, c in slow.most_common(25):
    print(f"  {c:5d} slow /{count[p]:5d} total  {p}  (avg all={tot_ms[p]/count[p]:.0f}ms)")
print("\n=== per-minute distribution of >2s requests ===")
for t in sorted(windows):
    print(f"  {t}  {'#'*min(windows[t],60)}  {windows[t]}")
print("\n=== top paths by total time spent ===")
for p, ms in tot_ms.most_common(15):
    print(f"  {ms/1000:9.1f}s over {count[p]:5d} reqs  {p}")
