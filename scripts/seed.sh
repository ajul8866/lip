#!/usr/bin/env bash
# Seed a running LIP instance through the public API. Requires curl + python3.
# The script never writes to PostgreSQL directly and never bypasses audit/approval.
set -eu

LIP_URL="${LIP_URL:-http://127.0.0.1:8080}"
EMAIL="${LIP_SEED_EMAIL:-admin@example.com}"
PASSWORD="${LIP_SEED_PASSWORD:?set LIP_SEED_PASSWORD to an existing or new admin password}"
PROJECT_NAME="${LIP_SEED_PROJECT:-Demo Assessment}"
SETUP_TOKEN="${LIP_SEED_SETUP_TOKEN:-}"
ASSET_FILE="${LIP_SEED_ASSETS:-examples/assets.txt}"
COOKIE_JAR="$(mktemp)"
trap 'rm -f "$COOKIE_JAR"' EXIT

api() {
  method="$1"; path="$2"; body="${3:-}"
  if [ -n "$body" ]; then
    curl -fsS -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X "$method" \
      -H 'Content-Type: application/json' -H 'Accept: application/json' \
      --data "$body" "$LIP_URL/api/v1$path"
  else
    curl -fsS -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X "$method" \
      -H 'Accept: application/json' "$LIP_URL/api/v1$path"
  fi
}

json_field() {
  python3 -c 'import json,sys; print(json.load(sys.stdin)["'"$1"'"])'
}

setup_required="$(api GET /setup/status | json_field setupRequired)"
if [ "$setup_required" = "True" ]; then
  headers=()
  if [ -n "$SETUP_TOKEN" ]; then
    headers=(-H "X-LIP-Setup-Token: $SETUP_TOKEN")
  fi
  payload="$(python3 - "$EMAIL" "$PASSWORD" "$PROJECT_NAME" <<'PY'
import json, sys
print(json.dumps({"email": sys.argv[1], "password": sys.argv[2], "projectName": sys.argv[3]}))
PY
)"
  curl -fsS -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X POST \
    -H 'Content-Type: application/json' "${headers[@]}" \
    --data "$payload" "$LIP_URL/api/v1/setup" >/dev/null
  echo "created first administrator and project"
else
  payload="$(python3 - "$EMAIL" "$PASSWORD" <<'PY'
import json, sys
print(json.dumps({"email": sys.argv[1], "password": sys.argv[2]}))
PY
)"
  curl -fsS -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X POST \
    -H 'Content-Type: application/json' --data "$payload" "$LIP_URL/api/v1/auth/login" >/dev/null
  echo "signed in as $EMAIL"
fi

project_id="$(api GET '/projects?limit=200' | python3 -c '
import json, sys
items = json.load(sys.stdin)["items"]
print(items[0]["id"] if items else "")
')"
if [ -z "$project_id" ]; then
  payload="$(python3 - "$PROJECT_NAME" <<'PY'
import json, sys
print(json.dumps({"name": sys.argv[1], "description": "Seeded through the LIP API."}))
PY
)"
  project_id="$(api POST /projects "$payload" | python3 -c 'import json,sys; print(json.load(sys.stdin)["project"]["id"])')"
  echo "created project $project_id"
fi

values="$(python3 - "$ASSET_FILE" <<'PY'
import json, sys
lines = [line.strip() for line in open(sys.argv[1], encoding="utf-8") if line.strip() and not line.lstrip().startswith("#")]
print(json.dumps({"source": "inline", "values": lines}))
PY
)"
api POST "/projects/$project_id/assets/import" "$values" >/dev/null
echo "imported assets from $ASSET_FILE"

conversation_id="$(api POST "/projects/$project_id/conversations" '{"title":"Seeded scope review"}' | python3 -c 'import json,sys; print(json.load(sys.stdin)["conversation"]["id"])')"
api POST "/conversations/$conversation_id/messages" \
  '{"content":"Review only the declared assets. Do not scan outside the imported list; record uncertainty explicitly."}' >/dev/null
api POST "/projects/$project_id/tasks" \
  '{"title":"Summarize declared inventory","tool":"inventory_summary","description":"Local-only seed task."}' >/dev/null

echo "project: $project_id"
echo "conversation: $conversation_id"
echo "seed complete; open $LIP_URL/"
