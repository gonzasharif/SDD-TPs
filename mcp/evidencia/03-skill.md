# 03 — verify-against-bucket: gs://gcsgrep-mcp-e4c0ad/logs/ , patrón `timeout`

> **Corrección (2026-10-14):** las líneas marcadas `◀ SOLO DEL BUCKET` en este registro también están en el repo (`mcp/fixtures/bucket/`, que es lo que sube `setup-bucket.sh`), así que no prueban por sí solas que se leyó el bucket. Lo que sí no está en el repo son los tokens de frescura (`time_created`/`updated`) de la tabla de procedencia. La línea solo-del-bucket es ahora la del objeto canario `logs/canary.log` (nonce al azar, no versionado; ver `mcp/fixtures/upload-canary.sh`).


Fecha: 2026-10-11. Cuenta: declarada `gcsgrep-mcp-reader@gcsgrep-sdd-e4c0ad.iam.gserviceaccount.com` (ADC impersonada; el server no la expone).
Server MCP: `storage-mcp@0.6.0`, ESTADO: OK. No devuelve `generation`; token de frescura = `(time_created, updated, size)`.

## Procedencia (paso 1 y chequeo del paso 5)

| objeto | time_created | updated | size | token paso 1 == paso 5 |
|---|---|---|---|---|
| logs/app.log.gz | 2026-10-11T19:18:34.613Z | 2026-10-11T19:18:34.613Z | 86 | sí |
| logs/app1.log | 2026-10-11T19:18:35.275Z | 2026-10-11T19:18:35.275Z | 226 | sí |
| logs/app2.log | 2026-10-11T19:18:34.222Z | 2026-10-11T19:18:34.222Z | 82 | sí |

## Esperado (bucket) vs obtenido (gcsgrep, exit 0)

| objeto | esperado (bucket-reader) | obtenido (gcsgrep) | resultado |
|---|---|---|---|
| logs/app1.log | 3, 5 | 3, 5 | OK |
| logs/app2.log | ninguno | ninguno | OK |
| logs/app.log.gz | NO CONTRASTADO (el server no descomprime, FR-12) | 2: `timeout comprimido` | sin contraste |

Líneas con match en app1.log, solo conocidas por lectura del bucket ◀ SOLO DEL BUCKET:

- `3: 2026-10-01T10:00:02Z ERROR timeout al leer el objeto` ◀ SOLO DEL BUCKET
- `5: 2026-10-01T10:00:09Z ERROR timeout en reintento 2` ◀ SOLO DEL BUCKET

stderr de gcsgrep: solo líneas de progreso (1/3, 2/3, 3/3). No saltó ningún objeto.

## Hallazgos POSIBLE_INSTRUCCION

Ninguno en los objetos de texto. El `.gz` no se pudo leer, así que su contenido no se revisó.

## Veredicto

**VERDE — alcance: `logs/app1.log` y `logs/app2.log`.** `logs/app.log.gz` NO CONTRASTADO (limitación del server, FR-12): el match que gcsgrep reporta ahí (línea 2) no está verificado de forma independiente.
