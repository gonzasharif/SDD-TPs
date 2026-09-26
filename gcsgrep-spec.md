# gcsgrep — Spec

> **Estado: revisada.** Sin preguntas abiertas (ver cierre del documento).
> Deriva del base context refinado,
> [`gcsgrep-requirements.md`](./gcsgrep-requirements.md) (qué y con qué
> vocabulario), con las decisiones de
> [`gcsgrep-design.md`](./gcsgrep-design.md) (cómo y por qué).
> Este documento es el contrato verificable: cada FR está
> en formato Dado/Cuando/Entonces, cada BR tiene fundamento, y cada FR, BR y
> NFR tiene un VC (criterio de verificación) concreto. El alcance por iteración
> (qué entra en la Iteración 1 vs. después) vive en `gcsgrep-plan.md`, no acá.

## Convenciones

- **Sub-ítems.** Un requerimiento con sub-ítems (como FR-8.1, FR-8.2 y
  FR-8.3) se cumple solo si se cumplen **todos** sus sub-ítems. Un VC con
  sub-ítems (como VC-8.1, VC-8.2 y VC-8.3) pasa solo si pasan **todos**: alcanza con que
  falle uno para que el VC falle.
- **Un Dado, una situación.** Cada "Dado" describe una sola situación. Si un
  comportamiento depende de más de una situación, se parte en sub-ítems.
- **Vocabulario.** Los términos (objeto fallido, objeto salteado, aviso,
  error, etc.) tienen el significado del glosario de
  `gcsgrep-requirements.md`.
- **Mensajes por `stderr`.** Un aviso empieza con el prefijo literal
  `gcsgrep: warning: `; un mensaje de error, con `gcsgrep: error: `.
- **Tamaños.** 1 KiB = 1.024 bytes, 1 MiB = 1.024 KiB, 1 GiB = 1.024 MiB.
  Los flags de tamaño reciben un entero en bytes.
- **Trazabilidad.** "*(deriva de X)*" usa los IDs de
  `gcsgrep-requirements.md` (base context refinado): FR-a a FR-g, BR-a a
  BR-g, NFR-a a NFR-c.

## Propósito

Permitir buscar texto dentro del contenido de objetos de Google Cloud Storage
sin descargarlos primero a disco, con la misma experiencia de uso que `grep`,
para gente de desarrollo/operaciones que ya tiene credenciales de GCP
configuradas.

## Alcance

**Incluido (v1, todas las iteraciones):**
- Búsqueda por patrón con sintaxis RE2 completa (sin backtracking) sobre
  objetos de texto bajo un bucket o prefijo de GCS.
- Autenticación vía Application Default Credentials (ADC) únicamente.
- Flags `-i`, `-n` (aceptado sin efecto: el número de línea siempre está en
  la salida), `-l`, `-c`.
- Salida `objeto:línea:texto`, con color condicional a TTY.
- Exit codes estilo `grep` (0/1/2).
- Lectura por streaming, solo lectura, sin amplificación de acceso.
- Descompresión de `.gz` al vuelo; detección y salteo de binarios.
- Guardrails de costo: cantidad de objetos, tamaño descomprimido por objeto,
  tamaño descomprimido acumulado.
- Concurrencia configurable (`--concurrency N`, tope 32).

**Fuera de alcance (v1 completa, no solo Iteración 1):**
- Regex completa tipo PCRE, o elegible por flag.
- Autenticación por archivo de service account key.
- Sintaxis de ubicación sin esquema (`bucket/prefijo`).
- Flags `-v`, `-r`, `--include` y cualquier otro flag de `grep` no listado
  arriba.
- Modo de salida JSON.
- Cualquier operación de escritura, copia, borrado o cambio de permisos sobre
  GCS.
- Interfaz web, API HTTP, o librería importable.
- Soporte para S3, Azure Blob u otro proveedor de object storage.
- Detección de generación/versión de objeto para consistencia ante escritura
  concurrente.

## Actores

- **Persona operadora/desarrolladora** (actor primario): invoca `gcsgrep`
  desde una terminal o script, con credenciales ADC ya configuradas. Espera
  semántica y flags compatibles con su experiencia previa de `grep`.
- **Google Cloud Storage** (sistema externo): fuente de datos. `gcsgrep` solo
  lo consume vía operaciones de lectura (list, get/read de objetos); nunca lo
  modifica (BR-1).
- **Proceso consumidor** (actor secundario, no humano): un script o pipeline
  que invoca `gcsgrep` y reacciona a su exit code y/o parsea su `stdout`
  (FR-8). No interactúa con `stderr` de forma estructurada — ahí solo van
  avisos, mensajes de error y progreso, pensados para un humano.

## Datos de prueba

Todos los VCs que corren contra GCS usan estos datos, en el proyecto de GCP de
prueba (región `us-central1`). Los nombres reales del proyecto, de los
buckets y de las service accounts no se versionan: están en
`gcsgrep/testenv.local.md`. En la spec se escriben como `<bucket>`,
`<bucket-completo>` y `<sa-…>`. En los contenidos, `\n` es un salto de línea.

### Credenciales

| Nombre | Permisos | Usada en |
|---|---|---|
| ADC del usuario | Lectura y escritura sobre ambos buckets | Todos los VCs que no nombran otra credencial |
| `<sa-viewer>` | `roles/storage.objectViewer` sobre `<bucket>` | VC-15.1 |
| `<sa-restringida>` | `roles/storage.objectViewer` sobre `<bucket>` con la IAM Condition `resource.name != "projects/_/buckets/<bucket>/objects/acl/denied.log"` | VC-8.3, VC-9.1, VC-16 |
| `<sa-sin-rol>` | Ningún rol sobre `<bucket>` | VC-25.3 |

### Bucket `<bucket-completo>`

Solo lo usa VC-2. Contiene exactamente 4 objetos, cada uno con el contenido
`timeout\n`: `a/1.log`, `b/2.log`, `c/d/3.log` y `raiz.log`.

### Bucket `<bucket>`

| Prefijo | Objetos y contenido | Usado en |
|---|---|---|
| `logs/` | `logs/a.log` = `INFO start\nERROR timeout\n`; `logs/b.log` = `INFO ok\n` | VC-1.1, VC-3.x, VC-8.1, VC-8.2, VC-25.x, VC-29 |
| `v/` | `v/a.log` = `version 1.2\nversion 1x2\n` | VC-1.3 |
| `case/` | `case/a.log` = `TIMEOUT error\n` | VC-4.x |
| `l/` | `l/big.log` = 100 MiB: línea 1 `timeout`, resto líneas `INFO ok` | VC-5 |
| `c/` | `c/none.log` = `INFO ok\n`; `c/three.log` = `timeout 1\nINFO\ntimeout 2\nINFO\ntimeout 3\n` | VC-6 |
| `acl/` | `acl/1.log` … `acl/5.log` = `timeout\n` cada uno; `acl/denied.log` = `secreto timeout\n` | VC-8.3, VC-9.1, VC-16 |
| `gzbad/` | `gzbad/ok.log` = `timeout\n`; `gzbad/bad.gz` = `esto no es gzip\n` (texto plano con nombre `.gz`) | VC-9.2 |
| `x/` | `x/ok.log` = `timeout\n`; `x/csek.log` = `timeout\n`, subido con una clave de cifrado provista por el cliente (CSEK) | VC-9.4 |
| `prog/` | `prog/00.log` … `prog/49.log` = `INFO ok\n` cada uno | VC-10.x |
| `bin/` | `bin/icon.png` = una imagen PNG; `bin/a.log` = `timeout\n` | VC-11 |
| `gz/` | `gz/app.log.gz` = gzip de `INFO start\nERROR timeout\n` | VC-12.1 |
| `gzb/` | `gzb/icon.png.gz` = gzip de una imagen PNG | VC-12.2 |
| `conc/` | `conc/00.log` … `conc/19.log` = `match 1\nmatch 2\nmatch 3\n` cada uno | VC-13.x, VC-28.2 |
| `max/` | `max/00.log` … `max/09.log` = `timeout\n` cada uno | VC-17.x |
| `big/` | `big/app.log.gz` = gzip de 2 MiB de líneas `INFO`; `big/ok.log` = `timeout\n` | VC-18 |
| `tot/` | `tot/1.log` … `tot/5.log` = 1 MiB cada uno: línea 1 `timeout`, resto líneas `INFO` | VC-19 |
| `perf/` | `perf/000.log` … `perf/499.log` = 1 MiB cada uno de líneas `INFO ok`; `perf/000.log` tiene además `needle` como línea 1 | VC-21.x |
| `mem/` | `mem/small.log` = 5 MiB de líneas `INFO ok`; `mem/large.log` = 500 MiB de líneas `INFO ok` | VC-22 |
| `long/` | `long/x.log` = línea 1 `timeout antes`; línea 2 de 5 MiB con `timeout` en el byte 2.097.152; línea 3 `timeout despues`; línea 4 de 2 MiB sin `timeout` | VC-24 |
| `pfx/` | `pfx/logs/a.log`, `pfx/logs-old/b.log`, `pfx/other/c.log` = `timeout\n` cada uno | VC-27 |
| `ord/` | `ord/b.log`, `ord/a.log`, `ord/c.log` (subidos en ese orden) = `match 1\nmatch 2\n` cada uno | VC-28.1 |
| `e/` | `e/empty.log` = 0 bytes; `e/a.log` = `timeout\n` | VC-30.1 |
| `n/` | `n/last.log` = `uno\ndos timeout` (sin `\n` final) | VC-30.2 |

`prefijo-sin-objetos/` (VC-26) no tiene ningún objeto. Los VCs que usan un
"cliente GCS simulado" (VC-1.4, VC-7, VC-9.3, VC-13.1, VC-14.x, VC-23.x,
VC-25.1, VC-31.x) no dependen de estos datos: el cliente simulado define sus propios
objetos.

## Requerimientos funcionales

### FR-1 — Búsqueda sobre un prefijo
*(deriva de FR-a)*

FR-1 se cumple solo si se cumplen FR-1.1, FR-1.2, FR-1.3 y FR-1.4.

#### FR-1.1 — Resultados de la búsqueda

- **Dado** un prefijo `gs://bucket/prefijo`, legible con las credenciales ADC
  del usuario, que contiene objetos de texto con y sin matches del patrón,
- **Cuando** el usuario ejecuta `gcsgrep PATRÓN gs://bucket/prefijo`,
- **Entonces** `stdout` contiene exactamente una línea con el formato de
  FR-3.1 por cada match de cada objeto de texto bajo el prefijo, y ninguna
  otra línea; el exit code sigue FR-8.

**VC-1.1:** Prefijo `logs/` con `logs/a.log` (contenido
`INFO start\nERROR timeout\n`) y `logs/b.log` (contenido `INFO ok\n`). Ejecutar
`gcsgrep timeout gs://<bucket>/logs/` con `stdout` redirigido a un archivo.
Verificar que el archivo contiene exactamente la línea
`logs/a.log:2:ERROR timeout` y que el exit code es `0`.

#### FR-1.2 — Sin escritura a disco

- **Dado** cualquier corrida,
- **Cuando** la herramienta lee el contenido de un objeto,
- **Entonces** lo lee por streaming y nunca escribe el contenido del objeto
  (ni completo ni en partes) a un archivo en disco.

**VC-1.2:** Búsqueda estática en el código de `gcsgrep/`, excluyendo los
archivos `_test.go`: cero apariciones de `os.Create`, `os.CreateTemp`,
`os.WriteFile` y `os.OpenFile`, es decir, ningún camino de código puede
escribir el contenido de un objeto a disco.

#### FR-1.3 — El patrón es una regex RE2

- **Dado** un patrón que contiene metacaracteres de RE2,
- **Cuando** la herramienta evalúa cada línea,
- **Entonces** interpreta el patrón con la sintaxis RE2 completa; un
  metacarácter precedido por `\` se busca de forma literal.

**VC-1.3:** Objeto `v/a.log` con las líneas `version 1.2` (línea 1) y
`version 1x2` (línea 2). Ejecutar `gcsgrep 'version 1\.[0-9]' gs://<bucket>/v/`.
Verificar que `stdout` contiene exactamente la línea `v/a.log:1:version 1.2`
y que el exit code es `0`.

#### FR-1.4 — Patrón inválido

- **Dado** un patrón que no es una regex RE2 válida,
- **Cuando** el usuario ejecuta `gcsgrep`,
- **Entonces** la herramienta no hace ninguna llamada a GCS, emite por
  `stderr` el mensaje de error
  `gcsgrep: error: invalid pattern "<patrón>": <detalle>` y termina con exit
  code `2` (error de uso).

**VC-1.4:** Ejecutar `gcsgrep '(' gs://<bucket>/logs/` con un cliente GCS
que cuenta llamadas. Verificar exit code `2`, `stdout` vacío, una línea de
`stderr` que empieza con `gcsgrep: error: invalid pattern "("` y cero
llamadas de listado o apertura.

### FR-2 — Búsqueda sobre bucket completo
*(deriva de FR-b)*

- **Dado** un bucket accesible sin prefijo especificado (`gs://bucket/`),
- **Cuando** el usuario ejecuta `gcsgrep PATRÓN gs://bucket/`,
- **Entonces** la búsqueda cubre todos los objetos de texto del bucket,
  sujeta al guardrail de cantidad (BR-3).

**VC-2:** Ejecutar `gcsgrep timeout gs://<bucket-completo>/`. Verificar que
`stdout` es exactamente, en este orden: `a/1.log:1:timeout`,
`b/2.log:1:timeout`, `c/d/3.log:1:timeout` y `raiz.log:1:timeout`, y que el
exit code es `0`.

### FR-3 — Formato de salida
*(deriva de FR-c)*

FR-3 se cumple solo si se cumplen FR-3.1, FR-3.2 y FR-3.3.

#### FR-3.1 — Formato de cada resultado

- **Dado** un match en la línea número `L` de un objeto,
- **Cuando** la herramienta lo imprime en `stdout`,
- **Entonces** imprime una línea con el formato `objeto:L:texto`, donde
  `objeto` es el nombre de objeto y `texto` es la línea completa sin su `\n`
  final.

**VC-3.1:** Objeto `logs/a.log` cuya línea 2 es `ERROR timeout`, patrón
`timeout`, `stdout` redirigido a un archivo. Verificar que el archivo
contiene exactamente la línea `logs/a.log:2:ERROR timeout`.

#### FR-3.2 — Color con `stdout` en una terminal

- **Dado** que `stdout` es una terminal (TTY),
- **Cuando** la herramienta imprime un resultado,
- **Entonces** cada porción del texto que matchea el patrón queda entre la
  secuencia ANSI de inicio `ESC[1;31m` y la de fin `ESC[0m` (`ESC` = byte
  `0x1b`); el resto del formato de FR-3.1 no cambia.

**VC-3.2:** Mismo objeto y patrón que VC-3.1, con `stdout` conectado a un
pseudo-terminal (pty). Verificar que la salida es exactamente
`logs/a.log:2:ERROR ESC[1;31mtimeoutESC[0m`.

#### FR-3.3 — Sin color con `stdout` redirigido

- **Dado** que `stdout` no es una terminal (está redirigido a un pipe o a un
  archivo),
- **Cuando** la herramienta imprime resultados,
- **Entonces** la salida no contiene ningún byte `0x1b`.

**VC-3.3:** Mismo objeto y patrón que VC-3.1, con `stdout` redirigido a un
archivo. Verificar que el archivo no contiene ningún byte `0x1b`.

### FR-4 — Búsqueda case-insensitive (`-i`)
*(deriva de FR-d)*

- **Dado** un objeto con una línea que contiene el patrón con otras
  mayúsculas y minúsculas,
- **Cuando** se ejecuta `gcsgrep -i PATRÓN ...`,
- **Entonces** se reporta el match sin distinguir mayúsculas de minúsculas.

**VC-4** (pasa solo si pasan VC-4.1 y VC-4.2), sobre el prefijo `case/`:
- **VC-4.1:** `gcsgrep -i timeout gs://<bucket>/case/`: `stdout` es
  exactamente `case/a.log:1:TIMEOUT error` y el exit code es `0`.
- **VC-4.2:** `gcsgrep timeout gs://<bucket>/case/`: `stdout` queda vacío y
  el exit code es `1`.

### FR-5 — Listar solo objetos con match (`-l`)
*(deriva de FR-d)*

- **Dado** el flag `-l`,
- **Cuando** se encuentra el primer match dentro de un objeto,
- **Entonces** se corta la lectura de ese objeto en ese punto y se imprime
  únicamente el nombre del objeto (sin número de línea ni texto), una sola
  vez por objeto.

**VC-5:** Ejecutar `gcsgrep -l timeout gs://<bucket>/l/` con un cliente GCS
que cuenta los bytes leídos de cada stream. Verificar que `stdout` es
exactamente `l/big.log`, que el exit code es `0` y que se leyeron ≤ 1 MiB
(1.048.576 bytes) de los 100 MiB de `l/big.log`.

### FR-6 — Contar matches por objeto (`-c`)
*(deriva de FR-d)*

- **Dado** el flag `-c`,
- **Cuando** se procesa un objeto completo,
- **Entonces** se imprime `objeto:cantidad`, donde `cantidad` es el total de
  líneas que matchean, incluso si es `0`. Un objeto salteado o un objeto
  fallido no imprime línea de conteo (no se procesó completo).

**VC-6:** Ejecutar `gcsgrep -c timeout gs://<bucket>/c/`. Verificar que
`stdout` es exactamente `c/none.log:0` y `c/three.log:3`, en ese orden, y
que el exit code es `0`.

### FR-7 — `-l` y `-c` son mutuamente excluyentes
*(deriva de FR-d)*

- **Dado** que el usuario pasa `-l` y `-c` en la misma invocación,
- **Cuando** se ejecuta `gcsgrep`,
- **Entonces** no se hace ninguna llamada a GCS, se emite el mensaje de error
  `gcsgrep: error: -l and -c cannot be used together` y el exit code es `2`
  (error de uso).

**VC-7:** Ejecutar `gcsgrep -l -c timeout gs://<bucket>/logs/` con un
cliente GCS que cuenta llamadas. Verificar exit code `2`, `stdout` vacío, la
línea `gcsgrep: error: -l and -c cannot be used together` en `stderr` y cero
llamadas a GCS.

### FR-8 — Exit codes estilo `grep`
*(deriva de FR-e)*

FR-8 se cumple solo si se cumplen FR-8.1, FR-8.2 y FR-8.3.

Cuenta como **error** a efectos de FR-8: un objeto fallido (FR-9, NFR-3), el
guardrail de cantidad alcanzado (BR-3), un objeto cortado (BR-4), un escaneo
incompleto (BR-5), un error de uso (FR-1.4, FR-7, FR-14, FR-16.1, FR-22), la falta
de credenciales ADC (FR-16.2), el listado denegado (FR-16.3), un bucket
inexistente (FR-16.4) y un listado que falla tras agotar los reintentos
(NFR-3). Una ubicación sin objetos **no** es un error (FR-17).

#### FR-8.1 — Exit 0

- **Dado** una corrida con al menos un match y ningún error,
- **Cuando** el proceso termina,
- **Entonces** el exit code es `0`.

**VC-8.1:** `gcsgrep timeout gs://<bucket>/logs/`: exit code `0`.

#### FR-8.2 — Exit 1

- **Dado** una corrida sin ningún match y sin ningún error,
- **Cuando** el proceso termina,
- **Entonces** el exit code es `1`.

**VC-8.2:** `gcsgrep patron_inexistente_xyz gs://<bucket>/logs/`: `stdout`
vacío y exit code `1`.

#### FR-8.3 — Exit 2

- **Dado** una corrida en la que ocurrió al menos un error,
- **Cuando** el proceso termina,
- **Entonces** el exit code es `2`, aunque haya habido matches en otros
  objetos.

**VC-8.3:** Con las credenciales de `<sa-restringida>`,
`gcsgrep timeout gs://<bucket>/acl/`: `stdout` contiene las 5 líneas de
`acl/1.log` … `acl/5.log` y el exit code es `2`.

### FR-9 — Un objeto fallido no interrumpe la corrida
*(deriva de FR-f)*

FR-9 se cumple solo si se cumplen FR-9.1, FR-9.2, FR-9.3 y FR-9.4.

En todos los casos el aviso tiene el formato literal
`gcsgrep: warning: <objeto>: <causa>`, donde `<objeto>` es el nombre de
objeto, y el objeto cuenta como error para FR-8.3. En FR-9.1, FR-9.3 y FR-9.4
la falla ocurre al abrir el objeto, así que no aparece ninguna línea suya en
`stdout`; en FR-9.2, las líneas del objeto impresas antes de detectar la
corrupción se mantienen (mismo criterio que NFR-3).

#### FR-9.1 — Objeto sin permiso de lectura

- **Dado** un objeto listado cuya lectura GCS rechaza por falta de permiso
  (HTTP 403),
- **Cuando** la herramienta intenta leerlo,
- **Entonces** emite por `stderr` el aviso con causa `permission denied` y
  continúa con el resto de los objetos.

**VC-9.1:** Con las credenciales de `<sa-restringida>`, ejecutar
`gcsgrep timeout gs://<bucket>/acl/`. Verificar que `stdout` es exactamente
`acl/1.log:1:timeout` … `acl/5.log:1:timeout` (5 líneas),
que `stderr` contiene la línea
`gcsgrep: warning: acl/denied.log: permission denied` y que el exit code es
`2`.

#### FR-9.2 — Objeto corrupto

- **Dado** un objeto corrupto (objeto comprimido cuyo contenido gzip es
  inválido),
- **Cuando** la herramienta intenta descomprimirlo,
- **Entonces** emite por `stderr` el aviso con causa `corrupt gzip data` y
  continúa con el resto de los objetos.

**VC-9.2:** Ejecutar `gcsgrep timeout gs://<bucket>/gzbad/`. Verificar que
`stdout` es exactamente `gzbad/ok.log:1:timeout`, que `stderr` contiene la
línea `gcsgrep: warning: gzbad/bad.gz: corrupt gzip data` y que el exit code
es `2`.

#### FR-9.3 — Objeto que dejó de existir

- **Dado** un objeto listado que ya no existe al momento de abrirlo (HTTP
  404, porque se borró entre el listado y la lectura),
- **Cuando** la herramienta intenta abrirlo,
- **Entonces** emite por `stderr` el aviso con causa `object not found` y
  continúa con el resto de los objetos.

**VC-9.3:** Con un cliente GCS simulado cuyo prefijo `x/` lista
`x/gone.log` y `x/ok.log` (= `timeout\n`) y responde 404 al abrir
`x/gone.log`, ejecutar `gcsgrep timeout gs://<bucket>/x/`. Verificar que
`stdout` es exactamente `x/ok.log:1:timeout`, que `stderr` contiene la línea `gcsgrep: warning: x/gone.log: object not found` y que el
exit code es `2`.

#### FR-9.4 — Otro error permanente al abrir

- **Dado** un objeto listado cuya apertura falla con un error HTTP 4xx
  permanente distinto de 403 y 404 (408 y 429 son transitorios: NFR-3),
- **Cuando** la herramienta intenta abrirlo,
- **Entonces** emite por `stderr` el aviso con causa
  `read failed: <mensaje de error devuelto por GCS>` y continúa con el resto
  de los objetos.

**VC-9.4:** Ejecutar `gcsgrep timeout gs://<bucket>/x/` (`x/csek.log` está
cifrado con CSEK y GCS rechaza leerlo sin la clave con HTTP 400). Verificar
que `stdout` es exactamente `x/ok.log:1:timeout`, que `stderr` contiene una línea que
empieza con `gcsgrep: warning: x/csek.log: read failed: ` y que el exit
code es `2`.

### FR-10 — Progreso durante la corrida
*(deriva de FR-g)*

FR-10 se cumple solo si se cumplen FR-10.1 y FR-10.2.

El **texto de progreso** es
`gcsgrep: progress: <procesados>/<total> (<porcentaje>%)`, donde `<total>` es la cantidad de objetos listados,
`<procesados>` la cantidad de objetos ya procesados (con o sin matches,
salteados o fallidos) y `<porcentaje>` es `procesados × 100 / total`
redondeado hacia abajo. Si la corrida termina sin procesar ningún objeto
(FR-1.4, FR-7, FR-14, FR-16, FR-17, FR-22 o BR-3), no se emite progreso.

#### FR-10.1 — `stderr` en una terminal

- **Dado** que `stderr` es una terminal (TTY),
- **Cuando** termina de procesarse cada objeto,
- **Entonces** se redibuja en el lugar el texto de progreso (precedido por
  `\r`, sin `\n`), y al terminar la corrida se emite un único `\n`.

**VC-10.1:** `gcsgrep timeout gs://<bucket>/prog/` (50 objetos), con
`stderr` conectado a un pseudo-terminal (pty). Verificar que `stderr` contiene exactamente 50
redibujos (`\r` seguido del texto de progreso), con porcentajes 2, 4, …,
100 en orden creciente, y que termina en `\n`.

#### FR-10.2 — `stderr` redirigido

- **Dado** que `stderr` no es una terminal (está redirigido a un pipe o a un
  archivo),
- **Cuando** el porcentaje procesado alcanza o supera un nuevo múltiplo de 10
  (10, 20, …, 100),
- **Entonces** se emite una línea con el texto de progreso terminada en
  `\n`, sin `\r`. Si un mismo objeto hace superar más de un múltiplo, se
  emite una sola línea.

**VC-10.2:** `gcsgrep timeout gs://<bucket>/prog/`, con `stderr` redirigido
a un archivo. Verificar que el archivo contiene exactamente 10
líneas de progreso (10%, 20%, …, 100%, en ese orden) y ningún byte `\r`.

### FR-11 — Objetos binarios se saltean
*(deriva de BR-d; comportamiento disparado por un evento concreto —encontrar
un objeto binario—, por eso vive como FR y no como BR)*

- **Dado** un objeto binario (su contenido tiene un byte nulo `0x00` en los
  primeros 8 KiB),
- **Cuando** se lo encuentra durante la corrida,
- **Entonces** es un objeto salteado: no se busca el patrón en su contenido,
  se emite el aviso `gcsgrep: warning: <objeto>: skipped (binary object)` y
  la corrida continúa. Un objeto salteado no cuenta como error.

**VC-11:** Prefijo `bin/` con `bin/icon.png` (una imagen PNG, cuya cabecera
contiene bytes nulos) y `bin/a.log` (única línea `timeout`). Ejecutar
`gcsgrep timeout gs://<bucket>/bin/`. Verificar que `stdout` es exactamente
`bin/a.log:1:timeout`, que `stderr` contiene la línea
`gcsgrep: warning: bin/icon.png: skipped (binary object)` y que el exit code
es `0`.

### FR-12 — Descompresión de objetos comprimidos al vuelo
*(deriva de BR-e, nueva en el refinamiento; mismo criterio que FR-11)*

- **Dado** un objeto comprimido (su nombre termina en `.gz`),
- **Cuando** se lo procesa,
- **Entonces** se descomprime por streaming, se aplica la detección de objeto
  binario de FR-11 sobre los primeros 8 KiB del contenido **descomprimido**, y
  se busca el patrón en el contenido descomprimido, reportando los matches con
  el nombre de objeto original (terminado en `.gz`). Queda sujeto a BR-4 y
  BR-5, medidos sobre bytes descomprimidos.

**VC-12** (pasa solo si pasan VC-12.1 y VC-12.2):
- **VC-12.1:** Prefijo `gz/` con `gz/app.log.gz`, que descomprime a
  `INFO start\nERROR timeout\n`. Ejecutar `gcsgrep timeout gs://<bucket>/gz/`.
  Verificar que `stdout` es exactamente `gz/app.log.gz:2:ERROR timeout` y que
  el exit code es `0`.
- **VC-12.2:** Prefijo `gzb/` con solo `gzb/icon.png.gz` (una imagen PNG
  comprimida con gzip). Ejecutar `gcsgrep timeout gs://<bucket>/gzb/`.
  Verificar que `stdout` queda vacío, que `stderr` contiene la línea
  `gcsgrep: warning: gzb/icon.png.gz: skipped (binary object)` y que el exit
  code es `1`.

### FR-13 — Concurrencia configurable
*(deriva del requerimiento de concurrencia del base context)*

- **Dado** el flag `--concurrency N` (alias `-j N`) con `1 ≤ N ≤ 32`,
- **Cuando** se ejecuta `gcsgrep`,
- **Entonces** hay como máximo `N` objetos abiertos para lectura al mismo
  tiempo, y el conjunto de líneas de `stdout` es el mismo que en modo
  secuencial (el orden sigue FR-19).

**VC-13** (pasa solo si pasan VC-13.1 y VC-13.2), con el patrón `match`
sobre el prefijo `conc/` (20 objetos de 3 líneas que matchean):
- **VC-13.1:** Con `--concurrency 4` y un cliente GCS que registra cuántos
  streams hay abiertos a la vez: el máximo registrado es exactamente `4`.
- **VC-13.2:** El conjunto de líneas de `stdout` con `--concurrency 4` es
  idéntico al de la corrida con `--concurrency 1`.

### FR-14 — Tope de concurrencia
*(deriva del requerimiento de concurrencia del base context; absorbe la
antigua BR-6)*

- **Dado** `--concurrency N` con `N` fuera del rango `1–32`,
- **Cuando** se invoca `gcsgrep`,
- **Entonces** no se hace ninguna llamada a GCS, se emite el mensaje de error
  `gcsgrep: error: --concurrency must be between 1 and 32 (got <N>)` y el
  exit code es `2` (error de uso).

*Fundamento:* evitar que el flag se use como forma implícita de saltear los
guardrails de costo/carga, generando picos de tráfico contra la API de GCS
que un solo usuario no debería poder producir sin querer. *Excepciones:*
ninguna en v1 (no hay forma de levantar este tope).

**VC-14** (pasa solo si pasan VC-14.1 y VC-14.2), con un cliente GCS que
cuenta llamadas:
- **VC-14.1:** `gcsgrep --concurrency 100 timeout gs://<bucket>/logs/`: exit
  code `2`, la línea
  `gcsgrep: error: --concurrency must be between 1 and 32 (got 100)` en
  `stderr` y cero llamadas a GCS.
- **VC-14.2:** `gcsgrep --concurrency 0 timeout gs://<bucket>/logs/`: exit
  code `2`, la línea
  `gcsgrep: error: --concurrency must be between 1 and 32 (got 0)` en
  `stderr` y cero llamadas a GCS.

### FR-15 — Líneas de más de 1 MiB se saltean
*(deriva de NFR-b — corrige un riesgo de falso negativo: truncar una línea
para matchear podría partir un match real justo en el punto de corte y
perderlo silenciosamente)*

- **Dado** un objeto con una línea de más de 1 MiB (1.048.576 bytes, sin
  contar el `\n`),
- **Cuando** se la encuentra durante la lectura por streaming,
- **Entonces** es una línea salteada: no se busca el patrón en ninguna
  porción de ella, y se emite el aviso
  `gcsgrep: warning: <objeto>: skipped lines longer than 1 MiB` una única vez
  por objeto afectado, sin importar cuántas líneas largas tenga. Una línea
  salteada no cuenta como error.

**VC-24:** Objeto `long/x.log` con 4 líneas: línea 1 `timeout antes`; línea
2 de 5 MiB con `timeout` en el byte 2.097.152; línea 3 `timeout despues`;
línea 4 de 2 MiB sin `timeout`. Ejecutar `gcsgrep timeout gs://<bucket>/long/`.
Verificar que `stdout` es exactamente `long/x.log:1:timeout antes` y
`long/x.log:3:timeout despues`, que `stderr` contiene exactamente una línea
`gcsgrep: warning: long/x.log: skipped lines longer than 1 MiB` y que el
exit code es `0`.

### FR-16 — La ubicación no se puede usar
*(deriva de FR-b y de las decisiones de diseño 2 y 3)*

FR-16 se cumple solo si se cumplen FR-16.1, FR-16.2, FR-16.3 y FR-16.4. En
todos los casos la corrida no abre ningún objeto, `stdout` queda vacío, se
emite un único mensaje de error por `stderr` y el exit code es `2`.

#### FR-16.1 — Ubicación sin esquema `gs://`

- **Dado** una ubicación que no empieza con `gs://` (como `bucket/prefijo`),
- **Cuando** el usuario ejecuta `gcsgrep`,
- **Entonces** la herramienta no hace ninguna llamada a GCS y emite
  `gcsgrep: error: invalid location "<ubicación>": must start with gs://`
  (error de uso).

**VC-25.1:** Ejecutar `gcsgrep timeout mybucket/logs/` con un cliente GCS
que cuenta llamadas. Verificar exit code `2`, `stdout` vacío, la línea
`gcsgrep: error: invalid location "mybucket/logs/": must start with gs://`
en `stderr` y cero llamadas a GCS.

#### FR-16.2 — Sin credenciales ADC

- **Dado** un entorno donde no se pueden resolver credenciales ADC,
- **Cuando** el usuario ejecuta `gcsgrep` con una ubicación válida,
- **Entonces** la herramienta emite
  `gcsgrep: error: no Application Default Credentials found: <detalle>`.

**VC-25.2:** En una máquina fuera de GCP (sin servidor de metadata), con la
variable `GOOGLE_APPLICATION_CREDENTIALS` sin definir y `HOME` apuntando a un
directorio vacío, ejecutar `gcsgrep timeout gs://<bucket>/logs/`. Verificar
exit code `2`, `stdout` vacío y una línea de `stderr` que empieza con
`gcsgrep: error: no Application Default Credentials found`.

#### FR-16.3 — Listado denegado

- **Dado** credenciales sin permiso para listar objetos del bucket (el
  listado responde HTTP 403),
- **Cuando** la herramienta lista la ubicación,
- **Entonces** emite
  `gcsgrep: error: permission denied listing gs://<bucket>/<prefijo>` y no
  reintenta el listado.

**VC-25.3:** Con las credenciales de `<sa-sin-rol>`, ejecutar
`gcsgrep timeout gs://<bucket>/logs/`. Verificar exit code `2`, `stdout`
vacío y la línea
`gcsgrep: error: permission denied listing gs://<bucket>/logs/` en
`stderr`.

#### FR-16.4 — Bucket inexistente

- **Dado** una ubicación cuyo bucket no existe (el listado responde HTTP
  404),
- **Cuando** la herramienta lista la ubicación,
- **Entonces** emite `gcsgrep: error: bucket <bucket> does not exist` y no
  reintenta el listado.

**VC-25.4:** Ejecutar `gcsgrep timeout gs://gcsgrep-bucket-inexistente-xyz/`.
Verificar exit code `2`, `stdout` vacío y la línea
`gcsgrep: error: bucket gcsgrep-bucket-inexistente-xyz does not exist` en
`stderr`.

### FR-17 — Ubicación sin objetos
*(deriva de la decisión de diseño 8)*

- **Dado** una ubicación válida y legible bajo la cual el listado no
  devuelve ningún objeto,
- **Cuando** la herramienta termina de listar,
- **Entonces** `stdout` queda vacío, emite por `stderr` el aviso
  `gcsgrep: warning: no objects under gs://<bucket>/<prefijo>` y el exit
  code es `1` (no es un error).

**VC-26:** Ejecutar `gcsgrep timeout gs://<bucket>/prefijo-sin-objetos/`
contra el bucket de prueba. Verificar exit code `1`, `stdout` vacío y
`stderr` exactamente igual a
`gcsgrep: warning: no objects under gs://<bucket>/prefijo-sin-objetos/`
(sin líneas de progreso).

### FR-18 — Prefijo sin `/` final
*(deriva de FR-b)*

- **Dado** una ubicación cuyo prefijo no termina en `/` (como
  `gs://bucket/logs`),
- **Cuando** la herramienta lista los objetos,
- **Entonces** incluye todos los objetos cuyo nombre empieza con ese prefijo,
  terminen o no en `/` después de él (`logs/a.log` y también
  `logs-old/b.log`).

**VC-27:** Ejecutar `gcsgrep timeout gs://<bucket>/pfx/logs` (sin `/`
final). Verificar que `stdout` es exactamente `pfx/logs-old/b.log:1:timeout`
y `pfx/logs/a.log:1:timeout`, en ese orden, sin ninguna línea de
`pfx/other/c.log`, y que el exit code es `0`.

### FR-19 — Orden de la salida
*(deriva del requerimiento de concurrencia del base context y de la decisión
de diseño 9)*

FR-19 se cumple solo si se cumplen FR-19.1 y FR-19.2.

#### FR-19.1 — Modo secuencial

- **Dado** una corrida en modo secuencial (`--concurrency` sin especificar,
  que equivale a `--concurrency 1`),
- **Cuando** la herramienta imprime resultados,
- **Entonces** los objetos aparecen en el orden del listado de GCS (orden
  lexicográfico por bytes del nombre de objeto) y, dentro de cada objeto, las
  líneas aparecen en orden ascendente de número de línea.

**VC-28.1:** Prefijo `ord/` con `ord/b.log`, `ord/a.log` y `ord/c.log`
(subidos en ese orden), cada uno con las líneas `match 1` y `match 2`.
Ejecutar `gcsgrep match gs://<bucket>/ord/`. Verificar que `stdout` es
exactamente, en este orden: `ord/a.log:1:match 1`, `ord/a.log:2:match 2`,
`ord/b.log:1:match 1`, `ord/b.log:2:match 2`, `ord/c.log:1:match 1`,
`ord/c.log:2:match 2`.

#### FR-19.2 — Modo concurrente

- **Dado** una corrida con `--concurrency N`, con `N ≥ 2`,
- **Cuando** la herramienta imprime resultados,
- **Entonces** las líneas de un mismo objeto aparecen juntas (sin líneas de
  otro objeto intercaladas) y en orden ascendente de número de línea; el
  orden entre objetos no está garantizado.

**VC-28.2:** Ejecutar 5 veces `gcsgrep --concurrency 8 match gs://<bucket>/conc/`
(20 objetos de 3 líneas que matchean). Verificar que en cada una de las 5
corridas las 3 líneas de cada objeto aparecen consecutivas y en orden
`:1:`, `:2:`, `:3:`, y que el conjunto de las 60 líneas es idéntico al de
`gcsgrep match gs://<bucket>/conc/` (modo secuencial).

### FR-20 — `-n` explícito
*(deriva de FR-d)*

- **Dado** el flag `-n`,
- **Cuando** el usuario ejecuta `gcsgrep -n PATRÓN UBICACIÓN`,
- **Entonces** `stdout`, `stderr` y el exit code son idénticos a los de la
  misma invocación sin `-n` (el número de línea ya está siempre en el formato
  de FR-3.1).

**VC-29:** Sobre el prefijo de VC-1.1, ejecutar
`gcsgrep timeout gs://<bucket>/logs/` y
`gcsgrep -n timeout gs://<bucket>/logs/`, ambas con `stdout` y `stderr`
redirigidos a archivos. Verificar que ambas corridas producen `stdout` y
`stderr` byte a byte idénticos y el mismo exit code (`0`).

### FR-21 — Bordes del contenido de un objeto
*(deriva de FR-a y de la definición de "línea" del glosario)*

FR-21 se cumple solo si se cumplen FR-21.1 y FR-21.2.

#### FR-21.1 — Objeto de 0 bytes

- **Dado** un objeto de texto de 0 bytes,
- **Cuando** la herramienta lo procesa,
- **Entonces** lo trata como un objeto sin líneas: no produce matches, no
  emite ningún aviso y no cuenta como objeto salteado ni como objeto fallido.

**VC-30.1:** Prefijo `e/` con `e/empty.log` (0 bytes) y `e/a.log` (única
línea `timeout`). Ejecutar `gcsgrep timeout gs://<bucket>/e/`. Verificar que
`stdout` es exactamente `e/a.log:1:timeout`, que `stderr` no contiene
`e/empty.log` y que el exit code es `0`.

#### FR-21.2 — Última línea sin `\n`

- **Dado** un objeto cuyo último byte no es `\n`,
- **Cuando** la herramienta llega al final del objeto,
- **Entonces** trata los bytes después del último `\n` como una línea más: se
  evalúa contra el patrón y, si matchea, se reporta con su número de línea.

**VC-30.2:** Objeto `n/last.log` con contenido `uno\ndos timeout` (sin `\n`
final). Ejecutar `gcsgrep timeout gs://<bucket>/n/`. Verificar que `stdout`
es exactamente `n/last.log:2:dos timeout` y que el exit code es `0`.

### FR-22 — Invocación inválida
*(deriva de FR-e y de la definición de "error de uso" del glosario)*

FR-22 se cumple solo si se cumplen FR-22.1, FR-22.2, FR-22.3 y FR-22.4. En
todos los casos la herramienta no hace ninguna llamada a GCS, `stdout` queda
vacío, `stderr` contiene únicamente el mensaje de error indicado y el exit
code es `2` (error de uso).

#### FR-22.1 — Flag desconocido

- **Dado** un flag que `gcsgrep` no reconoce (como `-v`),
- **Cuando** el usuario ejecuta `gcsgrep`,
- **Entonces** emite `gcsgrep: error: unknown flag <flag>`.

**VC-31.1:** Ejecutar `gcsgrep -v timeout gs://<bucket>/logs/` con un
cliente GCS que cuenta llamadas. Verificar exit code `2`, `stdout` vacío,
`stderr` exactamente igual a `gcsgrep: error: unknown flag -v` y cero
llamadas a GCS.

#### FR-22.2 — Cantidad de argumentos incorrecta

- **Dado** una invocación que no tiene exactamente dos argumentos
  posicionales (PATRÓN y UBICACIÓN),
- **Cuando** el usuario ejecuta `gcsgrep`,
- **Entonces** emite
  `gcsgrep: error: expected 2 arguments (PATTERN and LOCATION), got <cantidad>`.

**VC-31.2:** Ejecutar `gcsgrep timeout` con un cliente GCS que cuenta
llamadas. Verificar exit code `2`, `stdout` vacío, `stderr` exactamente igual
a `gcsgrep: error: expected 2 arguments (PATTERN and LOCATION), got 1` y cero
llamadas a GCS.

#### FR-22.3 — Valor no entero en un flag numérico

- **Dado** un valor que no es un número entero para `--max`,
  `--max-object-size`, `--max-total-size` o `--concurrency`,
- **Cuando** el usuario ejecuta `gcsgrep`,
- **Entonces** emite
  `gcsgrep: error: invalid value "<valor>" for <flag>: must be an integer`.

**VC-31.3:** Ejecutar
`gcsgrep --max-object-size 1MiB timeout gs://<bucket>/logs/` con un cliente
GCS que cuenta llamadas. Verificar exit code `2`, `stdout` vacío, `stderr`
exactamente igual a
`gcsgrep: error: invalid value "1MiB" for --max-object-size: must be an integer`
y cero llamadas a GCS.

#### FR-22.4 — Valor negativo en un flag de límite

- **Dado** un entero negativo para `--max`, `--max-object-size` o
  `--max-total-size`,
- **Cuando** el usuario ejecuta `gcsgrep`,
- **Entonces** emite
  `gcsgrep: error: invalid value "<valor>" for <flag>: must be >= 0`.

**VC-31.4:** Ejecutar `gcsgrep --max -5 timeout gs://<bucket>/logs/` con un
cliente GCS que cuenta llamadas. Verificar exit code `2`, `stdout` vacío,
`stderr` exactamente igual a
`gcsgrep: error: invalid value "-5" for --max: must be >= 0` y cero llamadas
a GCS.

## Reglas de negocio

### BR-1 — Solo lectura
*(deriva de BR-a)*

**Regla:** `gcsgrep` nunca escribe, modifica ni borra nada en GCS. Solo
invoca operaciones de lectura (list, get/read de objetos).

**Fundamento:** una herramienta de búsqueda no debería poder alterar el
bucket ni por diseño ni por un bug — minimiza el blast radius de cualquier
falla o mal uso.

**Excepciones:** ninguna.

**VC-15** (pasa solo si pasan VC-15.1 y VC-15.2):
- **VC-15.1:** Ejecutar los comandos de VC-1.1, VC-8.2 y VC-11 con las
  credenciales de `<sa-viewer>` (solo lectura) y con las ADC del usuario.
  Verificar que, para cada comando, `stdout` es byte a byte idéntico y el
  exit code es el mismo con ambas credenciales.
- **VC-15.2:** Búsqueda estática en el código de `gcsgrep/`, excluyendo los
  archivos `_test.go`: la interfaz `gcsclient.Client` declara solo `List` y
  `Open`, y hay cero apariciones de `NewWriter`, `Delete`, `Update`, `Copier`,
  `Compose` y `ACL(`.

### BR-2 — No amplifica el acceso del usuario
*(deriva de BR-b)*

**Regla:** `gcsgrep` nunca expone contenido al que el usuario invocante no
tendría acceso por sus propias credenciales ADC.

**Fundamento:** la herramienta debe operar estrictamente dentro del límite
de autorización de quien la ejecuta; de lo contrario sería un vector de
escalamiento de acceso.

**Excepciones:** ninguna.

**VC-16:** Con las credenciales de `<sa-restringida>` (que no pueden leer
`acl/denied.log`), ejecutar `gcsgrep secreto gs://<bucket>/acl/`. Verificar
que `stdout` queda vacío (la palabra `secreto` solo está en
`acl/denied.log`), que `stderr` contiene
`gcsgrep: warning: acl/denied.log: permission denied` y que el exit code es
`2`.

### BR-3 — Guardrail de cantidad de objetos
*(deriva de BR-c)*

**Regla:** antes de leer contenido, `gcsgrep` lista y cuenta los objetos bajo
el prefijo. Si la cantidad supera el límite (1000 por defecto), no abre
ningún objeto, emite el mensaje de error
`gcsgrep: error: the prefix has <N> objects, which exceeds the limit of <límite> (use --max to raise it, or --max 0 to disable it)`
y termina con exit code `2`.

**Fundamento:** evitar que un uso descuidado escanee accidentalmente un
bucket de millones de objetos, con el costo y tiempo que eso implica.

**Excepciones:** `--max N`, con `N` entero positivo, reemplaza el límite;
`--max 0` lo deshabilita.

**VC-17** (pasa solo si pasan VC-17.1, VC-17.2 y VC-17.3), sobre el prefijo
`max/` (10 objetos), con un cliente GCS que cuenta llamadas:
- **VC-17.1:** Con `--max 5`: solo la llamada de listado y 0 llamadas de
  apertura de objeto, `stdout` vacío, `stderr` exactamente igual a
  `gcsgrep: error: the prefix has 10 objects, which exceeds the limit of 5 (use --max to raise it, or --max 0 to disable it)`
  (sin líneas de progreso) y exit code `2`.
- **VC-17.2:** Con `--max 20`: se procesan los 10 objetos (10 llamadas de
  apertura).
- **VC-17.3:** Con `--max 0`: se procesan los 10 objetos (10 llamadas de
  apertura).

### BR-4 — Guardrail de tamaño por objeto
*(deriva de BR-f, nueva en el refinamiento)*

**Regla:** si el contenido leído de un objeto (descomprimido, si es un objeto
comprimido) supera el límite (250 MiB por defecto), se corta la lectura de
ese objeto en ese punto: el objeto queda como objeto cortado, se emite el
aviso
`gcsgrep: warning: <objeto>: object size limit of <límite> bytes reached, rest of the object not read`,
los matches del objeto ya impresos se mantienen y la corrida continúa con el
resto de los objetos. Un objeto cortado cuenta como error (FR-8.3).

**Fundamento:** protección contra un objeto que consume tiempo, memoria o
costo fuera de proporción — un objeto comprimido que se expande de forma
desproporcionada (deliberada o accidentalmente) o un objeto de texto
enorme.

**Excepciones:** `--max-object-size N`, con `N` en bytes, reemplaza el
límite; `--max-object-size 0` lo deshabilita.

**VC-18:** Prefijo `big/` con `big/app.log.gz`, que descomprime a 2 MiB de
líneas `INFO`, y `big/ok.log` (única línea `timeout`). Ejecutar
`gcsgrep --max-object-size 1048576 timeout gs://<bucket>/big/`. Verificar que
`stderr` contiene la línea
`gcsgrep: warning: big/app.log.gz: object size limit of 1048576 bytes reached, rest of the object not read`,
que `stdout` es exactamente `big/ok.log:1:timeout` y que el exit code es `2`.

### BR-5 — Guardrail acumulado de la corrida
*(deriva de BR-g, nueva en el refinamiento)*

**Regla:** si la suma de bytes leídos en toda la corrida (descomprimidos, en
los objetos comprimidos) supera el límite (2 GiB por defecto), se corta la
lectura en ese punto —incluido el objeto en curso—, no se abre ningún objeto
más, se emite el mensaje de error
`gcsgrep: error: total size limit of <límite> bytes reached, scan incomplete`,
los matches ya impresos se mantienen y el exit code es `2` (escaneo
incompleto).

**Fundamento:** control de costo total de la corrida completa (no solo por
objeto individual) — leer de GCS se cobra por bytes.

**Excepciones:** `--max-total-size N`, con `N` en bytes, reemplaza el
límite; `--max-total-size 0` lo deshabilita.

**VC-19:** Prefijo `tot/` con 5 objetos de 1 MiB (`tot/1.log` … `tot/5.log`),
cada uno con `timeout` como línea 1 y relleno sin `timeout` en el resto.
Ejecutar en modo secuencial
`gcsgrep --max-total-size 2621440 timeout gs://<bucket>/tot/` (2,5 MiB) con
un cliente GCS que cuenta aperturas. Verificar que `stdout` es exactamente
`tot/1.log:1:timeout`, `tot/2.log:1:timeout` y `tot/3.log:1:timeout`, que
hubo exactamente 3 aperturas, que `stderr` contiene la línea
`gcsgrep: error: total size limit of 2621440 bytes reached, scan incomplete`
y que el exit code es `2`.

## Requerimientos no funcionales

Los umbrales están congelados. Todas las mediciones usan los datos de prueba
de la sección [Datos de prueba](#datos-de-prueba), 3 corridas por medición y
el valor mediano.

### NFR-1 — Rendimiento

**Umbral:** sobre el prefijo `perf/` (500 objetos de 1 MiB, bucket en
`us-central1`):
- Throughput en modo secuencial: **≥ 0,5 objetos/seg**.
- Throughput con `--concurrency 8`: **≥ 3 veces** el throughput en modo
  secuencial medido en la misma sesión y desde la misma red.
- Latencia al primer resultado en modo secuencial, con el match en la línea 1
  del primer objeto listado: **≤ 2 segundos** desde el arranque del proceso
  hasta el primer byte de `stdout`.

El umbral con concurrencia es relativo al secuencial porque el throughput
absoluto depende del ancho de banda de la red hacia GCS, no del código (ver
decisión de diseño 9).

**VC-21** (pasa solo si pasan VC-21.1, VC-21.2 y VC-21.3), sobre el prefijo
`perf/` (500 objetos de 1 MiB) del bucket de `us-central1`, con 3 corridas
por medición y el valor mediano. Throughput = 500 / tiempo total, con el
tiempo total medido con `/usr/bin/time` (tiempo real):
- **VC-21.1:** `gcsgrep needle gs://<bucket>/perf/`: throughput ≥ 0,5
  objetos/seg (tiempo total ≤ 1000 segundos).
- **VC-21.2:** `gcsgrep --concurrency 8 needle gs://<bucket>/perf/`:
  throughput ≥ 3 × el throughput de VC-21.1 medido en la misma sesión.
- **VC-21.3:** `gcsgrep needle gs://<bucket>/perf/`, con un script que
  registra el instante de arranque del proceso y el instante en que lee el
  primer byte de su `stdout`: diferencia ≤ 2 segundos.

### NFR-2 — Memoria con objetos grandes

**Umbral:** el pico de memoria residente (RSS) al procesar un objeto de 500
MiB supera al de procesar un objeto de 5 MiB con el mismo contenido
repetido en **≤ 5 MiB**. La memoria por objeto no depende de su tamaño:
lectura por streaming en chunks de 64 KiB y buffer de línea de 1 MiB (FR-15).

**VC-22:** Ejecutar
`gcsgrep --max-object-size 0 --max-total-size 0 needle gs://<bucket>/mem/small.log`
y
`gcsgrep --max-object-size 0 --max-total-size 0 needle gs://<bucket>/mem/large.log`,
cada una 3 veces bajo `/usr/bin/time` (campo de RSS máximo: `-l` en macOS,
`-v` en Linux), tomando el RSS máximo mediano de cada una. Verificar que
todas terminan con exit code `1` y que
RSS máximo de `large.log` − RSS máximo de `small.log` ≤ 5 MiB
(5.242.880 bytes).

### NFR-3 — Comportamiento ante fallos de red

**Umbral:**
- Un error transitorio (timeout, conexión reseteada, HTTP 408, 429 o 5xx) al **listar**
  la ubicación o al **abrir** un objeto se reintenta hasta **3 intentos en
  total**. Espera antes del 2º intento: 500 ms ± 20% (400–600 ms); antes del
  3º: 1 s ± 20% (800–1200 ms).
- Un error permanente (HTTP 4xx distinto de 408 y 429) no se reintenta
  (FR-9, FR-16).
- Si se agotan los 3 intentos al abrir un objeto, el objeto queda como objeto
  fallido: aviso
  `gcsgrep: warning: <objeto>: network error after 3 attempts: <detalle>`, la
  corrida continúa y termina con exit code `2`.
- Si se agotan los 3 intentos al listar, no se abre ningún objeto: mensaje de
  error
  `gcsgrep: error: could not list gs://<bucket>/<prefijo> after 3 attempts: <detalle>`
  y exit code `2`.
- Un error **a mitad de la lectura** de un objeto ya abierto no se
  reintenta: el objeto queda como objeto fallido, con el aviso
  `gcsgrep: warning: <objeto>: read interrupted: <detalle>`; las líneas del
  objeto ya impresas se mantienen y no se vuelven a imprimir; exit code `2`.

**VC-23** (pasa solo si pasan VC-23.1 a VC-23.5), con un cliente GCS
simulado que inyecta errores y registra el instante de cada intento. En
VC-23.1 a VC-23.4 el prefijo simulado `r/` contiene solo `r/a.log` =
`timeout\n`; en VC-23.5 contiene solo `r/mid.log`:
- **VC-23.1:** Abrir `r/a.log` falla 2 veces con HTTP 503 y la 3ª tiene
  éxito: `stdout` es exactamente `r/a.log:1:timeout`, `stderr` no tiene
  avisos, exit code `0`, 3 intentos de apertura, espera entre el 1º y el 2º
  de 400–600 ms y entre el 2º y el 3º de 800–1200 ms.
- **VC-23.2:** Abrir `r/a.log` falla 3 veces con HTTP 503: exactamente 3
  intentos, `stderr` contiene una línea que empieza con
  `gcsgrep: warning: r/a.log: network error after 3 attempts: `, exit code
  `2`.
- **VC-23.3:** Abrir `r/a.log` falla con HTTP 403: exactamente 1 intento,
  `stderr` contiene `gcsgrep: warning: r/a.log: permission denied`, exit
  code `2`.
- **VC-23.4:** Listar `r/` falla 3 veces con HTTP 503: exactamente 3 intentos
  de listado, 0 aperturas, `stderr` contiene una línea que empieza con
  `gcsgrep: error: could not list gs://<bucket>/r/ after 3 attempts: `, exit
  code `2`.
- **VC-23.5:** `r/mid.log` = `timeout uno\nINFO\ntimeout tres\n`, y su stream
  se corta con "connection reset" después de entregar la línea 2: `stdout`
  es exactamente `r/mid.log:1:timeout uno` (una sola vez), `stderr` contiene
  una línea que empieza con `gcsgrep: warning: r/mid.log: read interrupted: `,
  hubo exactamente 1 apertura y el exit code es `2`.

## Cobertura de VCs

| Requisito | VC | Camino | Verificado por |
|---|---|---|---|
| FR-1.1 | VC-1.1 | feliz | Test de integración contra bucket de prueba |
| FR-1.2 | VC-1.2 | invariante (sin escritura a disco) | Búsqueda estática en el código |
| FR-1.3 | VC-1.3 | feliz (sintaxis RE2) | Test de integración |
| FR-1.4 | VC-1.4 | falla (error de uso) | Test CLI con cliente que cuenta llamadas |
| FR-2 | VC-2 | feliz | Test de integración |
| FR-3.1 | VC-3.1 | feliz | Test de integración (`stdout` a archivo) |
| FR-3.2 | VC-3.2 | borde (`stdout` en TTY) | Test de integración con pty |
| FR-3.3 | VC-3.3 | borde (`stdout` redirigido) | Test de integración (`stdout` a archivo) |
| FR-4 | VC-4.1, VC-4.2 | feliz | Test de integración |
| FR-5 | VC-5 | feliz | Test de integración + medición de bytes leídos |
| FR-6 | VC-6 | feliz | Test de integración |
| FR-7 | VC-7 | falla (error de uso) | Test CLI con cliente que cuenta llamadas |
| FR-8.1 | VC-8.1 | feliz | Test de integración |
| FR-8.2 | VC-8.2 | feliz (sin resultados) | Test de integración |
| FR-8.3 | VC-8.3 | falla | Test de integración con objeto sin permiso |
| FR-9.1 | VC-9.1 | falla (recuperable) | Test de integración con objeto sin permiso |
| FR-9.2 | VC-9.2 | falla (recuperable) | Test de integración con `.gz` inválido |
| FR-9.3 | VC-9.3 | falla (recuperable) | Test con cliente GCS simulado (404) |
| FR-9.4 | VC-9.4 | falla (recuperable) | Test de integración con objeto CSEK |
| FR-10.1 | VC-10.1 | borde (`stderr` en TTY) | Test de integración con pty |
| FR-10.2 | VC-10.2 | borde (`stderr` redirigido) | Test de integración, captura de `stderr` a archivo |
| FR-11 | VC-11 | borde (tipo de contenido) | Test de integración con objeto binario |
| FR-12 | VC-12.1 | feliz | Test de integración con objeto .gz |
| FR-12 | VC-12.2 | borde (binario comprimido) | Test de integración con .gz de una imagen |
| FR-13 | VC-13.1, VC-13.2 | feliz | Test con cliente que registra streams abiertos |
| FR-14 | VC-14.1, VC-14.2 | falla (error de uso) | Test CLI con cliente que cuenta llamadas |
| FR-15 | VC-24 | borde (línea extrema) | Test de integración con línea que excede el buffer |
| FR-16.1 | VC-25.1 | falla (error de uso) | Test CLI con cliente que cuenta llamadas |
| FR-16.2 | VC-25.2 | falla (sin credenciales) | Corrida real con entorno sin ADC |
| FR-16.3 | VC-25.3 | falla (listado denegado) | Corrida real con `<sa-sin-rol>` |
| FR-16.4 | VC-25.4 | falla (bucket inexistente) | Corrida real |
| FR-17 | VC-26 | borde (ubicación vacía) | Corrida real |
| FR-18 | VC-27 | borde (prefijo sin `/`) | Test de integración |
| FR-19.1 | VC-28.1 | feliz (orden secuencial) | Test de integración |
| FR-19.2 | VC-28.2 | borde (orden concurrente) | Test de integración, 5 corridas |
| FR-20 | VC-29 | feliz | Comparación byte a byte de dos corridas |
| FR-21.1 | VC-30.1 | borde (objeto de 0 bytes) | Test de integración |
| FR-21.2 | VC-30.2 | borde (última línea sin `\n`) | Test de integración |
| FR-22.1 | VC-31.1 | falla (error de uso) | Test CLI con cliente que cuenta llamadas |
| FR-22.2 | VC-31.2 | falla (error de uso) | Test CLI con cliente que cuenta llamadas |
| FR-22.3 | VC-31.3 | falla (error de uso) | Test CLI con cliente que cuenta llamadas |
| FR-22.4 | VC-31.4 | falla (error de uso) | Test CLI con cliente que cuenta llamadas |
| BR-1 | VC-15.1 | invariante | Corridas con credenciales de solo lectura vs. ADC del usuario |
| BR-1 | VC-15.2 | invariante | Búsqueda estática en el código |
| BR-2 | VC-16 | invariante | Corrida real con `<sa-restringida>` |
| BR-3 | VC-17.1 | borde (límite de cantidad) | Test de integración, conteo de llamadas |
| BR-3 | VC-17.2, VC-17.3 | feliz (límite levantado / deshabilitado) | Test de integración, conteo de llamadas |
| BR-4 | VC-18 | borde (límite de tamaño) | Test de integración con .gz grande |
| BR-5 | VC-19 | borde (límite acumulado) | Test de integración con guardrail acumulado bajo |
| NFR-1 | VC-21.1, VC-21.2, VC-21.3 | medición | `/usr/bin/time` + script de latencia sobre `perf/` |
| NFR-2 | VC-22 | medición | `/usr/bin/time` (RSS máximo) sobre `mem/` |
| NFR-3 | VC-23.1 | feliz (recuperación) | Test con cliente GCS simulado que inyecta errores |
| NFR-3 | VC-23.2, VC-23.3, VC-23.4, VC-23.5 | falla | Test con cliente GCS simulado que inyecta errores |

**22 FRs (41 FRs atómicos contando sub-ítems) + 5 BRs + 3 NFRs, 62 VCs
atómicos, 0 requerimientos sin VC.** De los 62 VCs, 36 ejercitan un camino
de falla o borde y 4 son invariantes — no es una tabla de puro camino feliz.

## Trazabilidad al base context refinado

La columna derecha usa los IDs de `gcsgrep-requirements.md`. FR-a a FR-g y
BR-a a BR-d ya estaban en el borrador original (commit `c84efbc`); BR-e,
BR-f y BR-g son nuevas en el refinamiento.

| Spec | Base context refinado (`gcsgrep-requirements.md`) |
|---|---|
| FR-1, FR-2 | FR-a, FR-b |
| FR-3 | FR-c |
| FR-4, FR-5, FR-6, FR-7, FR-20 | FR-d |
| FR-8 | FR-e |
| FR-9 | FR-f |
| FR-10 | FR-g |
| FR-11 | BR-d (promovida a FR) |
| FR-12 | BR-e (nueva en el refinamiento; promovida a FR) |
| FR-13, FR-14 | Requerimiento de concurrencia (FR-14 absorbe la antigua BR-6) |
| FR-15 | NFR-b (memoria) — promovido a FR, corrige riesgo de falso negativo detectado en revisión |
| FR-16 | FR-b + decisiones de diseño 2 (autenticación) y 3 (ubicación) |
| FR-17 | Decisión de diseño 8 (ubicación sin objetos) |
| FR-18 | FR-b (prefijo sin `/` final) |
| FR-19 | Requerimiento de concurrencia (orden de salida) + decisión de diseño 9 |
| FR-21 | FR-a + definición de "línea" del glosario |
| FR-22 | FR-e + definición de "error de uso" del glosario |
| BR-1 | BR-a |
| BR-2 | BR-b |
| BR-3 | BR-c |
| BR-4 | BR-f (nueva en el refinamiento) |
| BR-5 | BR-g (nueva en el refinamiento) |
| NFR-1, NFR-2, NFR-3 | NFR-a, NFR-b, NFR-c |

## Preguntas abiertas

Ninguna. Las 10 preguntas abiertas del borrador original (commit `c84efbc`)
y las que surgieron en el refinamiento (reintentos, tamaño máximo de línea,
progreso) están resueltas en `gcsgrep-design.md`; las que surgieron durante
la atomización (mutua exclusión de `-l`/`-c`, tope de concurrencia, riesgo de
falso negativo en líneas largas, casos de falla de la ubicación) quedaron
resueltas en este documento. Los umbrales de los NFRs están congelados.

## Qué sigue

El plan de iteraciones está en [`gcsgrep-plan.md`](./gcsgrep-plan.md).
