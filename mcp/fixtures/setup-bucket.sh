#!/usr/bin/env bash
# setup-bucket.sh — sube los fixtures SINTÉTICOS al bucket de prueba de L4.
# Lo corre una PERSONA con permiso de escritura (no la service account de solo lectura,
# no el agente). Idempotente: sobrescribe, lo que genera una generation nueva por objeto.
#   uso: fixtures/setup-bucket.sh [bucket]      (default: gcsgrep-mcp-e4c0ad)
set -euo pipefail
BUCKET="${1:-gcsgrep-mcp-e4c0ad}"
cd "$(dirname "$0")/bucket"
gcloud storage cp --recursive . "gs://$BUCKET/"
# Objeto que la SA lista pero NO puede leer: cifrado con clave del cliente (CSEK) cuya clave se
# descarta acá mismo. No toca IAM (el rol objectViewer queda limpio, sin condiciones) y nadie —ni
# el dueño— puede leerlo: es el "objeto sin permisos" de los VCs de FR-9.1.
KEYFILE="$(mktemp)"; printf 'secreto timeout\n' > "$KEYFILE"
gcloud storage cp --quiet --encryption-key="$(openssl rand -base64 32)" "$KEYFILE" "gs://$BUCKET/sin-permiso/csek.log"
rm -f "$KEYFILE"
gcloud storage ls -r --long "gs://$BUCKET/"
