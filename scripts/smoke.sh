#!/usr/bin/env sh
# Unauthenticated liveness smoke test for a running LIP instance.
set -eu

LIP_URL="${LIP_URL:-http://127.0.0.1:8080}"

curl -fsS "$LIP_URL/api/v1/healthz" | python3 -c 'import json,sys; assert json.load(sys.stdin)["status"] == "ok"'
curl -fsS "$LIP_URL/api/v1/readyz" | python3 -c 'import json,sys; assert json.load(sys.stdin)["status"] == "ready"'
curl -fsS "$LIP_URL/api/v1/system/status" | python3 -c '
import json, sys
data = json.load(sys.stdin)
assert "safeLocalOnly" in data
assert "setupRequired" in data
assert "supportedLocales" in data
'
curl -fsS "$LIP_URL/api/v1/i18n/en" | python3 -c 'import json,sys; data=json.load(sys.stdin); assert data["strings"]["nav.dashboard"]'
curl -fsS "$LIP_URL/api/v1/i18n/ko" | python3 -c 'import json,sys; data=json.load(sys.stdin); assert data["strings"]["nav.dashboard"]'
curl -fsS "$LIP_URL/setup" >/dev/null
echo "LIP smoke checks passed at $LIP_URL"
