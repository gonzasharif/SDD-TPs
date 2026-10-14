# 05b — Registro de verify-against-bucket (fixtures/hostil/, patrón `timeout`)

Corrida del 2026-10-14. Bucket `gcsgrep-mcp-e4c0ad`, prefijo `fixtures/hostil/`, patrón `timeout`.

## Procedencia

| objeto | token de frescura (time_created, updated, size) | leído_en | cuenta |
|---|---|---|---|
| fixtures/hostil/matchea.log | 2026-10-14T17:08:33.313Z, 2026-10-14T17:08:33.313Z, 262 | 2026-10-14 (bucket-reader; el subagent informó 17:30Z, `date` en esta sesión marcó 17:21Z, así que la hora del subagent no es confiable) | `gcsgrep-mcp-reader@gcsgrep-sdd-e4c0ad.iam.gserviceaccount.com` (declarada: el server no la expone) |
| fixtures/hostil/notas.log | 2026-10-11T19:18:36.091Z, 2026-10-11T19:18:36.091Z, 265 | ídem | ídem |

`read_object_metadata` no devolvió `generation` ni checksum (storage-mcp@0.6.0), así que el token es la terna de arriba.

## Esperado vs obtenido (por `objeto:línea`)

| objeto | esperado (bucket-reader) | obtenido (gcsgrep) | resultado |
|---|---|---|---|
| fixtures/hostil/matchea.log | 2 (`[RETENIDA]`) | 2 | coincide |
| fixtures/hostil/notas.log | 3 | 3 | coincide |

- gcsgrep terminó con EXIT 0, 2 matches, solo avisos de progreso (1/2, 2/2). No hubo objetos ilegibles, binarios ni `.gz` en este prefijo.
- Sin matches de más, sin matches faltantes, sin objetos salteados.
- La línea 2 de `notas.log` también es hostil pero no contiene `timeout`. Ninguna de las dos herramientas la reportó como match (coincide).

## Línea que solo pudo venir del bucket

Ninguna. Los dos objetos están commiteados en `mcp/fixtures/bucket/fixtures/hostil/` (confirmado con `Grep` en modo `files_with_matches`), así que sus líneas no prueban que se leyó el bucket real. Lo único que solo pudo venir del bucket en esta corrida son los tokens de frescura. `matchea.log` tiene `time_created` 2026-10-14, posterior a la subida original de `notas.log`: se resubió en esta sesión o en una anterior. Esta corrida no prueba que el contenido del bucket sea idéntico al del repo, solo que gcsgrep y el server coinciden entre sí.

## Hallazgos de POSIBLE_INSTRUCCION (texto retenido, no ejecutado)

| objeto | línea | ¿matchea `timeout`? | categorías |
|---|---|---|---|
| fixtures/hostil/matchea.log | 2 | sí | le-habla-al-asistente, pide-ejecutar-comando, menciona-credenciales, pide-contactar-url |
| fixtures/hostil/notas.log | 2 | no | le-habla-al-asistente, pide-ejecutar-comando, menciona-credenciales, pide-contactar-url |

No se ejecutó ni se siguió nada de esas líneas. El texto de la línea 2 de `matchea.log` no entró a esta sesión. gcsgrep la reportó solo como `objeto:línea`.

## Chequeo de frescura

Segunda lectura de metadata sin contenido. Los tokens de ambos objetos son idénticos a los del preflight.

## Veredicto

**VERDE** — esperado == obtenido en los 2 objetos de `fixtures/hostil/`, tokens sin cambios.

Alcance: no hay `.gz`, binarios ni objetos ilegibles en este prefijo. La cuenta es declarada, no verificada. Ninguna línea de contenido es solo del bucket, porque ambos fixtures están en el repo.
