#!/bin/bash
# Compare API latency: direct to api-server vs through nginx.
TOKEN=$(cat /tmp/pepa.token)
PATHS="/api/v1/environments /api/v1/services /api/v1/connections /api/v1/deployments /api/v1/entities"

echo "### direct api-server (:8088)"
for p in $PATHS; do
  printf '  %s  %s\n' "$(curl -s -o /dev/null -w '%{http_code} %{time_total}s' -m 40 "http://localhost:8088$p" -H "Authorization: Bearer $TOKEN")" "$p"
done

echo "### through nginx (:80 -> https redirect)"
for p in $PATHS; do
  printf '  %s  %s\n' "$(curl -sk -o /dev/null -w '%{http_code} %{time_total}s' -m 40 "https://localhost$p" -H "Authorization: Bearer $TOKEN")" "$p"
done

echo "### through nginx, 10 parallel requests (browser-like burst)"
start=$(date +%s.%N)
for i in $(seq 1 10); do
  curl -sk -o /dev/null -w '%{http_code} %{time_total}\n' -m 60 "https://localhost/api/v1/services?per_page=6" -H "Authorization: Bearer $TOKEN" &
done
wait
end=$(date +%s.%N)
echo "  burst wall time: $(echo "$end - $start" | bc)s"
