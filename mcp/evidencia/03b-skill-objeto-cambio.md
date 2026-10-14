# 03b — verify-against-bucket con objeto cambiado (prueba de frescura)

> **Corrección (2026-10-14):** las líneas marcadas `◀ SOLO DEL BUCKET` en este registro también están en el repo (`mcp/fixtures/bucket/`, que es lo que sube `setup-bucket.sh`), así que no prueban por sí solas que se leyó el bucket. Lo que sí no está en el repo son los tokens de frescura (`time_created`/`updated`) de la tabla de procedencia. La línea solo-del-bucket es ahora la del objeto canario `logs/canary.log` (nonce al azar, no versionado; ver `mcp/fixtures/upload-canary.sh`).


Bucket `gcsgrep-mcp-e4c0ad`, prefijo `logs/`, patrón `timeout` (sensible a mayúsculas, sin flags).
Fecha: 2026-10-11. No reemplaza a `03-skill.md`.

## Secuencia ejecutada

1. Paso 1: `bucket-reader` leyó list/metadata/contenido (ESTADO: OK, ~19:20Z).
2. **Mutación:** `mcp/tests/mutate-object.sh logs/app2.log` (sobrescribe el objeto con su mismo contenido, con la cuenta dueña de gcloud). Salida: generation nueva `1791747641380474`.
3. Pasos 2–4: esperado, `gcsgrep` real y contraste.
4. Paso 5: `bucket-reader` solo con `read_object_metadata`.

## Procedencia (cuenta declarada: `gcsgrep-mcp-reader@gcsgrep-sdd-e4c0ad.iam.gserviceaccount.com`, ADC impersonada; el server no la expone)

| objeto | token paso 1 (time_created / updated / size) | token paso 5 | ¿cambió? |
|---|---|---|---|
| logs/app.log.gz | 2026-10-11T19:18:34.613Z / 2026-10-11T19:18:34.613Z / 86 | igual | no |
| logs/app1.log | 2026-10-11T19:18:35.275Z / 2026-10-11T19:18:35.275Z / 226 | igual | no |
| logs/app2.log | 2026-10-11T19:18:34.222Z / 2026-10-11T19:18:34.222Z / 82 | **2026-10-11T19:40:41.400Z / 2026-10-11T19:40:41.400Z / 82** | **SÍ** |

El server (0.6.0) no devuelve `generation`; el token es `(time_created, updated, size)`. El `size` no cambió (mismo contenido), pero `time_created`/`updated` sí: el token detecta la sobreescritura.

## Esperado (bucket-reader, paso 1) vs obtenido (gcsgrep)

Comando: `./gcsgrep 'timeout' gs://gcsgrep-mcp-e4c0ad/logs/` — exit 0; stderr solo progreso (1/3, 2/3, 3/3).

| objeto | esperado | obtenido | resultado |
|---|---|---|---|
| logs/app1.log | 3, 5 | 3, 5 | coincide |
| logs/app2.log | ninguno | ninguno | coincide (pero con evidencia vieja) |
| logs/app.log.gz | NO CONTRASTADO (el server no descomprime, FR-12) | `2: 2026-09-30T23:59:59Z ERROR timeout comprimido` ◀ SOLO DEL BUCKET | no contrastable |

Salida completa de gcsgrep:

```
logs/app.log.gz:2:2026-09-30T23:59:59Z ERROR timeout comprimido
logs/app1.log:3:2026-10-01T10:00:02Z ERROR timeout al leer el objeto
logs/app1.log:5:2026-10-01T10:00:09Z ERROR timeout en reintento 2
```

`◀ SOLO DEL BUCKET`: `logs/app.log.gz:2:... ERROR timeout comprimido` (solo se obtiene descomprimiendo el objeto real, que el server MCP no puede hacer).

## Hallazgos POSIBLE_INSTRUCCION

Ninguno (paso 1 y paso 5).

## Veredicto

**NO VERIFICADO — evidencia vieja.**

- Objeto con token cambiado desde el paso 1: `logs/app2.log` (`updated` 19:18:34.222Z → 19:40:41.400Z).
- Aunque esperado == obtenido en los objetos contrastables, el esperado de `app2.log` se leyó antes de la sobreescritura, así que no se puede aprobar contra ese dato.
- Acción pedida: re-leer `logs/app2.log` (y volver a correr los pasos 2–4) antes de emitir un veredicto.
- Alcance: aun con la relectura, `logs/app.log.gz` quedaría NO CONTRASTADO (limitación del server).

Resultado de la prueba: la regla de frescura funcionó. El cambio de un objeto entre la lectura y el veredicto se detectó y bloqueó el VERDE.
