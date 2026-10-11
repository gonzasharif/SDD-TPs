#!/usr/bin/env bash
# setup-bucket.sh — sube los fixtures SINTÉTICOS al bucket de prueba de L4.
# Lo corre una PERSONA con permiso de escritura (no la service account de solo lectura,
# no el agente). Idempotente: sobrescribe, lo que genera una generation nueva por objeto.
#   uso: fixtures/setup-bucket.sh [bucket]      (default: gcsgrep-mcp-e4c0ad)
set -euo pipefail
BUCKET="${1:-gcsgrep-mcp-e4c0ad}"
cd "$(dirname "$0")/bucket"
gcloud storage cp --recursive . "gs://$BUCKET/"
gcloud storage ls -r --long "gs://$BUCKET/"
