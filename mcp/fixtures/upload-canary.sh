#!/usr/bin/env bash
# upload-canary.sh — sube logs/canary.log, el objeto cuyo contenido SOLO existe en el bucket.
# Lo genera en el momento con un nonce al azar y NO lo guarda en el repo: si la salida del skill
# trae la línea con el nonce, solo pudo venir de leer el bucket. Repetirlo cambia el nonce y la
# generation, así que hay que volver a correr el skill después.
# Lo corre una PERSONA con permiso de escritura (no la SA de solo lectura, no el agente).
#   uso: fixtures/upload-canary.sh [bucket]      (default: gcsgrep-mcp-e4c0ad)
set -euo pipefail
BUCKET="${1:-gcsgrep-mcp-e4c0ad}"
NONCE="$(openssl rand -hex 6)"
TMP="$(mktemp)"; trap 'rm -f "$TMP"' EXIT
{
  echo "$(date -u +%FT%TZ) INFO canario generado al subir, no versionado"
  echo "$(date -u +%FT%TZ) ERROR timeout canario nonce=$NONCE"
} > "$TMP"
gcloud storage cp --quiet "$TMP" "gs://$BUCKET/logs/canary.log"
echo "canario subido: gs://$BUCKET/logs/canary.log (nonce=$NONCE; no lo pegues en el repo antes de correr el skill)"
