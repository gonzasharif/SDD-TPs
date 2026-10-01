#!/usr/bin/env bash
# Crea en GCS los datos de prueba de la sección "Datos de prueba" de
# gcsgrep-spec.md: los objetos de <bucket> y las service accounts
# <sa-viewer>, <sa-restringida> y <sa-sin-rol>.
#
# Uso (los valores reales están en gcsgrep/testenv.local.md, no versionado):
#
#   PROJECT=mi-proyecto BUCKET=mi-bucket ./testdata/setup-testdata.sh [objetos|cuentas|todo]
#
#   objetos   sube los objetos de <bucket> (alcanza con permisos sobre el bucket)
#   cuentas   crea las 3 service accounts y sus permisos (requiere
#             roles/iam.serviceAccountAdmin en el proyecto). Por defecto le da
#             permiso de impersonarlas a la cuenta activa de gcloud; con
#             IMPERSONATOR=usuario@dominio se lo da a otra persona.
#   todo      las dos cosas
#
# Requiere: gcloud autenticado con permisos de administración sobre el
# proyecto, bash, gzip, base64, openssl, yes y head. El bucket debe existir
# en us-central1 con uniform bucket-level access habilitado (lo necesita la
# IAM Condition de <sa-restringida>).
#
# Subir perf/ (500 MiB), mem/ (505 MiB) y l/ (100 MiB) tarda: a ~1 MB/s son
# unos 20 minutos.
set -euo pipefail

: "${PROJECT:?definí PROJECT}"
: "${BUCKET:?definí BUCKET}"
MODE="${1:-todo}"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
MIB=1048576

# put <nombre-de-objeto> : sube stdin como gs://$BUCKET/<nombre-de-objeto>.
put() {
  local tmp="$WORK/obj"
  cat > "$tmp"
  gcloud storage cp --quiet "$tmp" "gs://$BUCKET/$1"
}

# fill <línea> <bytes> : repite <línea> (con su \n) hasta completar <bytes>.
# `yes` termina con SIGPIPE cuando `head` corta; con `set -o pipefail` eso
# haría abortar el script en silencio, por eso el `|| true`.
fill() { yes "$1" | head -c "$2" || true; }

# chars <carácter> <bytes> : <bytes> repeticiones de <carácter>, sin \n.
chars() { head -c "$2" /dev/zero | tr '\0' "$1"; }

# PNG de 1x1 píxel (su cabecera tiene bytes nulos: es un objeto binario).
png() {
  echo 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==' | base64 -d
}

objetos() {
  echo "== <bucket>"
  printf 'INFO start\nERROR timeout\n' | put logs/a.log
  printf 'INFO ok\n' | put logs/b.log

  printf 'version 1.2\nversion 1x2\n' | put v/a.log
  printf 'TIMEOUT error\n' | put case/a.log

  { printf 'timeout\n'; fill 'INFO ok' $((100 * MIB - 8)); } | put l/big.log

  printf 'INFO ok\n' | put c/none.log
  printf 'timeout 1\nINFO\ntimeout 2\nINFO\ntimeout 3\n' | put c/three.log

  for i in 1 2 3 4 5; do printf 'timeout\n' | put "acl/$i.log"; done
  printf 'secreto timeout\n' | put acl/denied.log

  printf 'timeout\n' | put gzbad/ok.log
  printf 'esto no es gzip\n' | put gzbad/bad.gz

  printf 'timeout\n' | put x/ok.log
  printf 'timeout\n' > "$WORK/csek"
  gcloud storage cp --quiet --encryption-key="$(openssl rand -base64 32)" "$WORK/csek" "gs://$BUCKET/x/csek.log"

  for i in $(seq -w 0 49); do printf 'INFO ok\n' | put "prog/$i.log"; done

  png | put bin/icon.png
  printf 'timeout\n' | put bin/a.log

  printf 'INFO start\nERROR timeout\n' | gzip -c | put gz/app.log.gz
  png | gzip -c | put gzb/icon.png.gz

  for i in $(seq -w 0 19); do printf 'match 1\nmatch 2\nmatch 3\n' | put "conc/$i.log"; done
  for i in $(seq 0 9); do printf 'timeout\n' | put "max/0$i.log"; done

  fill 'INFO' $((2 * MIB)) | gzip -c | put big/app.log.gz
  printf 'timeout\n' | put big/ok.log

  for i in 1 2 3 4 5; do { printf 'timeout\n'; fill 'INFO' $((MIB - 8)); } | put "tot/$i.log"; done

  mkdir -p "$WORK/perf"
  fill 'INFO ok' "$MIB" > "$WORK/base"
  for i in $(seq -w 0 499); do cp "$WORK/base" "$WORK/perf/$i.log"; done
  { printf 'needle\n'; tail -c +9 "$WORK/base"; } > "$WORK/perf/000.log"
  gcloud storage cp --quiet -r "$WORK/perf" "gs://$BUCKET/"

  fill 'INFO ok' $((5 * MIB)) | put mem/small.log
  for i in $(seq 500); do cat "$WORK/base"; done | put mem/large.log

  {
    printf 'timeout antes\n'
    chars 'x' $((2 * MIB)); printf 'timeout'
    chars 'x' $((5 * MIB - 2 * MIB - 7)); printf '\n'
    printf 'timeout despues\n'
    chars 'y' $((2 * MIB)); printf '\n'
  } | put long/x.log

  for name in logs-old/b.log logs/a.log other/c.log; do printf 'timeout\n' | put "pfx/$name"; done
  for name in b a c; do printf 'match 1\nmatch 2\n' | put "ord/$name.log"; done

  : | put e/empty.log
  printf 'timeout\n' | put e/a.log
  printf 'uno\ndos timeout' | put n/last.log
}

cuentas() {
  local me viewer restringida sinrol
  me="${IMPERSONATOR:-$(gcloud config get-value account)}"
  viewer="gcsgrep-viewer@$PROJECT.iam.gserviceaccount.com"
  restringida="gcsgrep-restringida@$PROJECT.iam.gserviceaccount.com"
  sinrol="gcsgrep-sin-rol@$PROJECT.iam.gserviceaccount.com"

  for sa in gcsgrep-viewer gcsgrep-restringida gcsgrep-sin-rol; do
    gcloud iam service-accounts create "$sa" --project "$PROJECT" --quiet || true
  done

  gcloud storage buckets add-iam-policy-binding "gs://$BUCKET" \
    --member="serviceAccount:$viewer" --role=roles/storage.objectViewer
  gcloud storage buckets add-iam-policy-binding "gs://$BUCKET" \
    --member="serviceAccount:$restringida" --role=roles/storage.objectViewer \
    --condition="expression=resource.name != \"projects/_/buckets/$BUCKET/objects/acl/denied.log\",title=excluir-acl-denied"

  # Para correr un VC con una de estas cuentas sin crear claves:
  #   gcloud auth application-default login --impersonate-service-account=<cuenta>
  for sa in "$viewer" "$restringida" "$sinrol"; do
    gcloud iam service-accounts add-iam-policy-binding "$sa" --project "$PROJECT" \
      --member="user:$me" --role=roles/iam.serviceAccountTokenCreator
  done
}

case "$MODE" in
  objetos) objetos ;;
  cuentas) cuentas ;;
  todo) objetos; cuentas ;;
  *) echo "uso: $0 [objetos|cuentas|todo]" >&2; exit 2 ;;
esac
