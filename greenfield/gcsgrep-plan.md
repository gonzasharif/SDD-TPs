# gcsgrep — Plan de iteraciones

> Deriva de [`gcsgrep-spec.md`](gcsgrep-spec.md). La spec define **qué**
> tiene que hacer `gcsgrep` en su versión completa (v1); este documento
> define **en qué orden** se construye y **qué queda deliberadamente afuera**
> de cada iteración, y por qué. El alcance diferido vive acá — la spec no
> tiene iteraciones, tiene un contrato final.
>
> Los IDs (FR-X.Y, VC-X.Y) son los de la spec corregida. Un requerimiento o
> VC con sub-ítems se cumple solo si se cumplen todos sus sub-ítems
> (convención de la spec).

## Dónde vive el código

La implementación está en el subdirectorio `gcsgrep` de este repo, en Go
(módulo `gcsgrep`). Estructura: `cmd/gcsgrep/main.go` es el entry point;
`internal/{cli,scanner,reader,match,gcsclient,output}` son los módulos
descritos en la sección "Arquitectura" de `gcsgrep-design.md`.

```bash
cd gcsgrep
go build -o /tmp/gcsgrep ./cmd/gcsgrep   # binario
go vet ./...                              # sin warnings
go test ./...                             # tests unitarios, incluidos los
                                          # chequeos estáticos de VC-1.2 y
                                          # VC-15.2 (internal/invariants)

# VCs contra GCS real (datos de testdata/setup-testdata.sh):
GCSGREP_TEST_BUCKET=<bucket> GCSGREP_TEST_CREDS=<dir> \
  go test -tags integration -v -count=1 ./integration/
# + mediciones de NFR-1/NFR-2 (VC-21, VC-22):
GCSGREP_BENCH=1 GCSGREP_TEST_BUCKET=<bucket> \
  go test -tags integration -v -count=1 -timeout 60m -run 'VC21|VC22' ./integration/
```

El entorno de prueba real (proyecto de GCP, buckets, service accounts y
credenciales ADC) está descrito en la sección "Datos de prueba" de la spec.
Los nombres concretos no están versionados en el repo a propósito: están en
`gcsgrep/testenv.local.md` (gitignoreado); si no lo tenés, pedíselo a quien
armó el entorno en vez de crear uno nuevo. Los datos de prueba y las service
accounts se crean con `gcsgrep/testdata/setup-testdata.sh`.

## Resumen

| Iteración | Foco | FRs | BRs | NFRs |
|---|---|---|---|---|
| 1 | Búsqueda mínima viable, segura por diseño | FR-1, FR-2, FR-3.1, FR-3.3, FR-4, FR-8, FR-9.1, FR-9.3, FR-9.4, FR-11, FR-15, FR-16, FR-17, FR-18, FR-19.1, FR-20, FR-21, FR-22 | BR-1, BR-2, BR-3 | NFR-1 (parcial: VC-21.1, VC-21.3), NFR-2 |
| 2 | Costo en objetos comprimidos/grandes, UX, confiabilidad de red | FR-3.2, FR-5, FR-6, FR-7, FR-9.2, FR-10, FR-12 | BR-4, BR-5 | NFR-3 |
| 3 | Concurrencia y rendimiento | FR-13, FR-14, FR-19.2 | — | NFR-1 (completo: VC-21.2) |

Los 22 FRs (41 atómicos), 5 BRs y 3 NFRs de la spec quedan cubiertos entre
las tres iteraciones, y cada uno de los 63 VCs está en los criterios de
éxito de exactamente una iteración (41 + 16 + 6).

## Iteración 1 — Búsqueda mínima viable, segura por diseño

**Objetivo:** una `gcsgrep` que ya resuelve el caso de uso central —buscar un
patrón en objetos de texto bajo un prefijo, por streaming, sin bajar nada a
disco— y que es segura por diseño desde el primer commit: no amplía acceso,
no escribe nunca, no se cae por un objeto problemático, no puede reventar
memoria ni escanear un bucket entero por accidente, y falla de forma clara y
detectable por un script cuando la invocación o la ubicación no sirven.

**Entra:**
- **FR-1** — búsqueda sobre un prefijo: resultados (1.1), sin escritura a
  disco (1.2), patrón RE2 (1.3) y patrón inválido (1.4).
- **FR-2** — búsqueda sobre bucket completo.
- **FR-3.1, FR-3.3** — formato `objeto:línea:texto` en texto plano. El color
  con TTY (FR-3.2) se difiere a la Iteración 2: es una mejora de UX, no
  afecta la corrección del resultado.
- **FR-4** — `-i`.
- **FR-8** — exit codes 0/1/2, con todas las fuentes de error que existen en
  esta iteración.
- **FR-9.1, FR-9.3, FR-9.4** — un objeto fallido al abrirlo (sin permiso, 404,
  otro 4xx) no aborta la corrida. FR-9.2 (objeto comprimido corrupto) va con
  la descompresión, en la Iteración 2.
- **FR-11** — objetos binarios se saltean. Se incluye ya en la Iteración 1, y
  no se difiere, porque sin esto un solo objeto binario en el prefijo puede
  volcar basura sobre la terminal del usuario — es barato de implementar y
  protege la experiencia básica.
- **FR-15** — líneas de más de 1 MiB se saltean (no se truncan para
  matchear). Entra en la Iteración 1 porque es lo que sostiene la garantía de
  memoria constante de NFR-2.
- **FR-16, FR-17** — ubicación inválida, sin credenciales, listado denegado,
  bucket inexistente y ubicación sin objetos. Son los caminos de falla más
  comunes de un CLI de este tipo; sin ellos un script no puede distinguir un
  typo de una búsqueda sin resultados.
- **FR-18** — prefijo sin `/` final como filtro de nombre.
- **FR-19.1** — orden de salida en modo secuencial (el único modo de esta
  iteración).
- **FR-20** — `-n` aceptado sin efecto.
- **FR-21** — objeto de 0 bytes y última línea sin `\n`.
- **FR-22** — invocación inválida. En esta iteración el único flag numérico
  es `--max`; los que se agregan después (`--max-object-size`,
  `--max-total-size`, `--concurrency`) heredan el mismo comportamiento.
- **BR-1, BR-2** — solo lectura y sin amplificación de acceso. No son
  features que se "agreguen" después: son propiedades de cómo está
  construida la herramienta desde el primer commit.
- **BR-3** — guardrail de cantidad de objetos (1000 por defecto, `--max`).
  Entra en la Iteración 1 y no se difiere porque la consigna lo marca como
  restricción dura de entrega ("no escanees un bucket enorme sin guardrail de
  costo").
- **NFR-2** — memoria constante (streaming + FR-15).
- **NFR-1**, parcial — throughput secuencial (VC-21.1) y latencia al primer
  resultado (VC-21.3). El umbral con concurrencia (VC-21.2) se valida en la
  Iteración 3, cuando existe la concurrencia.

**Explícitamente afuera (y por qué):**
- Color en la salida (FR-3.2) — cosmético, no bloquea el caso de uso.
- `-l`, `-c` (FR-5, FR-6, FR-7) — el flujo de streaming ya funciona; agregar
  dos modos de salida distintos y su validación de mutua exclusión es trabajo
  incremental, no fundacional. En esta iteración `-l` y `-c` son flags
  desconocidos (FR-22.1).
- Progreso (FR-10) — relevante recién con prefijos grandes; con el guardrail
  de 1000 objetos de BR-3, una corrida de Iteración 1 nunca es tan larga como
  para "parecer colgada".
- Objetos comprimidos (FR-12, FR-9.2) y los guardrails de tamaño (BR-4,
  BR-5) — la descompresión agrega una fuente de complejidad, y el guardrail
  de bytes descomprimidos no tiene sentido sin descompresión primero.
- Reintentos de red (NFR-3) — sin reintentos, un error transitorio deja un
  objeto fallido (vía FR-9), lo cual es correcto pero pesimista. Aceptable
  para una primera versión funcional.
- Concurrencia (FR-13, FR-14, FR-19.2) — el modo secuencial ya es correcto y
  verificable; la concurrencia es una optimización de rendimiento, no una
  corrección funcional.

**Criterios de éxito** (41 VCs)

- [x] VC-1.1, VC-1.2, VC-1.3, VC-1.4 — búsqueda, sin escritura a disco, RE2,
      patrón inválido
- [x] VC-2.1, VC-2.2 — bucket completo
- [x] VC-3.1, VC-3.3 — formato en texto plano, sin bytes ANSI
- [x] VC-4.1, VC-4.2 — `-i`
- [x] VC-8.1, VC-8.2, VC-8.3 — exit codes 0/1/2
- [x] VC-9.1, VC-9.3, VC-9.4 — objeto fallido al abrirlo (403, 404, CSEK)
- [x] VC-11 — objetos binarios se saltean
- [x] VC-15.1, VC-15.2 — solo lectura
- [x] VC-16 — no se expone contenido fuera del acceso del usuario
- [x] VC-17.1, VC-17.2, VC-17.3 — guardrail de cantidad
- [x] VC-21.1 — throughput secuencial ≥ 0,5 objetos/seg
- [x] VC-21.3 — primer resultado en ≤ 2 segundos
- [x] VC-22 — memoria constante (en esta iteración se corre **sin**
      `--max-object-size 0 --max-total-size 0`: esos flags todavía no
      existen y no hay guardrails de tamaño que desactivar)
- [x] VC-24 — líneas de más de 1 MiB se saltean
- [x] VC-25.1, VC-25.2, VC-25.3, VC-25.4 — ubicación no usable
- [x] VC-26 — ubicación sin objetos
- [x] VC-27 — prefijo sin `/` final
- [x] VC-28.1 — orden en modo secuencial
- [x] VC-29 — `-n` sin efecto
- [x] VC-30.1, VC-30.2 — objeto de 0 bytes, última línea sin `\n`
- [x] VC-31.1, VC-31.2, VC-31.3, VC-31.4 — invocación inválida

**Demostrable así** (datos de la sección "Datos de prueba" de la spec):

```bash
gcsgrep timeout gs://<bucket>/logs/
# logs/a.log:2:ERROR timeout — exit 0

gcsgrep patron_inexistente_xyz gs://<bucket>/logs/
# sin resultados, exit 1

gcsgrep -i timeout gs://<bucket>/case/
# case/a.log:1:TIMEOUT error — exit 0

gcsgrep --max 1 timeout gs://<bucket>/
# "the prefix has N objects, ..." con N = todos los objetos del bucket, exit 2

gcsgrep timeout gs://<bucket>/bin/
# bin/a.log:1:timeout; aviso "skipped (binary object)" para bin/icon.png, exit 0

gcsgrep timeout gs://<bucket>/prefijo-sin-objetos/
# aviso "no objects under ...", exit 1

gcsgrep timeout gs://gcsgrep-bucket-inexistente-xyz/
# "gcsgrep: error: bucket ... does not exist", exit 2

gcsgrep timeout mybucket/logs/
# "gcsgrep: error: invalid location ...", exit 2, sin llamadas a GCS
```

## Iteración 2 — Costo en objetos grandes/comprimidos, UX y confiabilidad de red

**Objetivo:** cubrir el caso de uso real de logs (que suelen estar
comprimidos), cerrar los guardrails de costo que solo importan una vez que
hay descompresión de por medio, sumar los modos de salida que reducen el
volumen de resultados (`-l`, `-c`), mejorar la experiencia interactiva
(color, progreso), y dejar de ser pesimista ante errores de red transitorios.

**Entra:**
- **FR-12** — descompresión de objetos comprimidos al vuelo, con la
  detección de binario sobre el contenido descomprimido.
- **FR-9.2** — objeto comprimido corrupto.
- **BR-4, BR-5** — guardrails de tamaño por objeto y acumulado, con
  `--max-object-size` y `--max-total-size`. Entran junto con FR-12 porque su
  caso principal (un objeto comprimido que se expande) no existe antes.
- **FR-5, FR-6, FR-7** — `-l`, `-c`, y su mutua exclusión.
- **FR-3.2** — color condicional a TTY.
- **FR-10** — progreso (redibujo en TTY, una línea cada 10% sin TTY). Entra
  acá porque recién con objetos comprimidos y guardrails más permisivos las
  corridas empiezan a ser lo bastante largas como para que el usuario
  necesite la señal de que sigue viva.
- **NFR-3** — reintentos con backoff ante errores transitorios al listar y al
  abrir, y objeto fallido ante un corte a mitad de la lectura.

**Explícitamente afuera (y por qué):**
- Concurrencia (FR-13, FR-14, FR-19.2) — todavía no es necesaria para que la
  herramienta sea correcta; se deja para la iteración de rendimiento, así la
  introducción de paralelismo no se mezcla con la introducción de
  descompresión y sus guardrails (dos fuentes de bugs a la vez es peor que
  una por vez).

**Criterios de éxito** (16 VCs, más la regresión)

- [x] Todos los VCs de la Iteración 1 siguen pasando (ver nota de regresión)
- [x] VC-3.2 — color con TTY
- [x] VC-5 — `-l` corta la lectura en el primer match
- [x] VC-6 — `-c` cuenta matches, incluso `0`
- [x] VC-7 — `-l` y `-c` juntas se rechazan con exit 2
- [x] VC-9.2 — objeto comprimido corrupto
- [x] VC-10.1, VC-10.2 — progreso en TTY y sin TTY
- [x] VC-12.1, VC-12.2 — descompresión y binario comprimido
- [x] VC-18 — guardrail de tamaño por objeto
- [x] VC-19 — guardrail acumulado
- [x] VC-23.1, VC-23.2, VC-23.3, VC-23.4, VC-23.5 — reintentos y corte a mitad
      de lectura

**Nota de regresión:**
- VC-22 se vuelve a correr **con** `--max-object-size 0 --max-total-size 0`
  (el objeto de 500 MiB supera el límite por defecto de BR-4).
- VC-8.3 cubre una sola fuente de error; las nuevas de esta iteración
  (objeto cortado, escaneo incompleto, objeto corrupto, reintentos agotados)
  las verifican VC-18, VC-19, VC-9.2 y VC-23.x, todos con exit code 2.
- Con progreso activo, `stderr` tiene líneas nuevas: los VCs de la
  Iteración 1 que comparan `stderr` exacto (VC-26, VC-17.1, VC-29, VC-31.x)
  tienen que seguir pasando porque en esos casos no se procesa ningún
  objeto o la comparación es entre dos corridas iguales.
- VC-31.3 y VC-31.4 se extienden con tests unitarios para los flags nuevos
  (`--max-object-size`, `--max-total-size`).

## Iteración 3 — Concurrencia y validación de rendimiento

**Objetivo:** acelerar la corrida sobre prefijos con muchos objetos mediante
lectura paralela, con un tope explícito para no convertir el flag de
concurrencia en una forma de saltear los guardrails de costo, y validar el
umbral de NFR-1 con concurrencia.

**Entra:**
- **FR-13** — `--concurrency N` / `-j N`, con como máximo `N` objetos
  abiertos a la vez.
- **FR-14** — rechazo de `N` fuera de `1–32`.
- **FR-19.2** — orden de salida en modo concurrente (líneas de un objeto
  juntas y en orden).
- **NFR-1**, completo — throughput con `--concurrency 8` ≥ 3 × secuencial
  (VC-21.2).
- Revisión de thread-safety de los contadores compartidos entre workers:
  bytes acumulados de BR-5 y contador de progreso de FR-10, que hasta esta
  iteración corrían en un solo hilo y ahora necesitan sincronización
  (atomic/lock) para no producir una condición de carrera que deje pasar más
  bytes de los que BR-5 permite.

**Explícitamente afuera:** nada de la spec — con esta iteración se cierra el
contrato completo (22 FRs, 5 BRs, 3 NFRs).

**Criterios de éxito** (6 VCs, más la regresión)

- [ ] Todos los VCs de las Iteraciones 1 y 2 siguen pasando, corridos además
      con `--concurrency 8` (salvo los que fijan el modo secuencial: VC-19,
      VC-21.1, VC-21.3, VC-28.1)
- [ ] VC-13.1, VC-13.2 — como máximo `N` objetos abiertos, mismo resultado
      que en secuencial
- [ ] VC-14.1, VC-14.2 — `--concurrency` fuera de `1–32` se rechaza
- [ ] VC-21.2 — throughput con `-j 8` ≥ 3 × secuencial
- [ ] VC-28.2 — orden en modo concurrente

**Nota de regresión:** esta es la iteración de mayor riesgo de regresión
silenciosa, porque introduce concurrencia sobre comportamiento que hasta acá
solo corrió en un único hilo. En particular, BR-5 (guardrail acumulado) y
FR-10 (progreso) dependen de contadores compartidos — si no se sincronizan
correctamente, VC-19 y VC-10.x podrían seguir "pasando" en una corrida
puntual y fallar de forma intermitente bajo carga. No alcanza con correr la
suite una vez con `--concurrency 8`: se corre 5 veces seguidas (el mismo
criterio que VC-28.2) para exponer condiciones de carrera que no aparecen
siempre.

## Alcance diferido (después de v1)

Comportamiento que se consideró y se dejó para después de v1, sin estar
rechazado:

| Idea | Por qué se difiere |
|---|---|
| Flag `--max-line-size` para cambiar el tamaño máximo de línea | v1 usa 1 MiB fijo (decisión de diseño 12): no hay un caso de uso que requiera cambiarlo, y un flag más es un FR, un VC y una validación más |
| Flags `-v`, `-r`, `--include` | Decisión de diseño 4: sin caso de uso claro en v1 (`-r` ya es implícito en un prefijo) |

## Lo que quedó afuera del plan entero

Esto no es "todavía no lo hicimos": es alcance rechazado en la spec (sección
Alcance → Fuera), y vive acá también para que no vuelva a aparecer en cada
conversación de implementación.

| Idea | Decisión |
|---|---|
| Regex completa tipo PCRE, o elegible por flag | Descartado en v1 (decisión de diseño 1) |
| Autenticación por archivo de service account key | Descartado en v1 (decisión de diseño 2) |
| Sintaxis de ubicación sin esquema (`bucket/prefijo`) | Descartado en v1 (decisión de diseño 3) |
| Modo de salida JSON | Descartado en v1 (decisión de diseño 7) |
| Cualquier escritura/copia/borrado sobre GCS | Descartado — viola BR-1 |
| Interfaz web, API HTTP, librería importable | Descartado en v1 |
| Soporte para S3, Azure Blob u otro proveedor | Descartado en v1 |
| Detección de generación/versión de objeto ante escritura concurrente | Riesgo conocido, aceptado (decisión de diseño 10) |

## Cómo se usa este plan

Cada iteración es un incremento entregable y verificable por sí mismo: al
final de la Iteración 1 ya hay una `gcsgrep` usable en el caso de uso
central, no un prototipo descartable. Las iteraciones 2 y 3 no reabren
decisiones de la spec — solo agregan comportamiento que la spec ya
contempla, en el orden que minimiza la superficie de riesgo por iteración
(primero correctitud y seguridad, después cobertura de formatos reales,
después rendimiento).

## Qué sigue

Tras implementar y verificar cada iteración, la evidencia va en
**`gcsgrep-cobertura-vc.md`** — un único documento acumulativo para todo el
proyecto, no uno nuevo por iteración. Cada iteración agrega su propia
sección con, para cada VC de su lista de criterios de éxito, con qué se lo
ejercitó y qué se observó, sin borrar ni reescribir las secciones de
iteraciones anteriores: el documento completo es el historial de qué se
verificó y cuándo, no solo el estado actual.

La Iteración 1 se implementó contra la versión anterior de la spec (commit
`3337575`, tag `01.1-Greenfield`) y se re-verificó contra la spec corregida
(41/41 VCs, sección de re-verificación de `gcsgrep-cobertura-vc.md`).
