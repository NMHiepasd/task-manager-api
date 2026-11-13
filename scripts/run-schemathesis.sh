#!/usr/bin/env bash
# scripts/run-schemathesis.sh
# Execute Schemathesis property-based and contract fuzz testing against the running API container.

set -euo pipefail

NETWORK_NAME="task-manager-api_task_network"
SCHEMA_URL="http://api:8080/swagger/doc.json"

echo "=== Running Schemathesis API Contract & Fuzz Testing ==="
echo "Target Network: ${NETWORK_NAME}"
echo "Schema URL:     ${SCHEMA_URL}"

docker run --rm \
  --network="${NETWORK_NAME}" \
  -v "${PWD}/schemathesis.toml:/schemathesis.toml:ro" \
  schemathesis/schemathesis:stable \
  --config-file=/schemathesis.toml \
  run "${SCHEMA_URL}" \
  --max-examples=10 \
  --checks=not_a_server_error,status_code_conformance,content_type_conformance

echo "=== Schemathesis testing completed successfully! ==="
