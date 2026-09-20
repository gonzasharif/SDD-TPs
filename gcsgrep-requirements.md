# gcsgrep — requerimientos (refinado)

> **Estado: refinado.** Este documento arrancó como un borrador deliberadamente
> subespecificado (ver historial de git para la versión original). Las 10
> preguntas abiertas que traía se resolvieron en sesión de refinamiento con el
> agente; las decisiones y su fundamento están en la sección
> [Decisiones tomadas](#decisiones-tomadas). Los valores numéricos concretos de
> los NFRs y guardrails son una **propuesta inicial razonable**, no medida con
> benchmarks reales todavía — están marcados como tal.
>
> Este sigue siendo el *base context*, no la spec formal. El paso siguiente es
> convertir cada FR en Dado/Cuando/Entonces con su VC, y cada BR con su
> fundamento formal — eso vive en un documento de spec aparte.

## La idea

Queremos la experiencia de `grep`, pero apuntada a Google Cloud Storage.

Hoy, para buscar un texto dentro de objetos de un bucket, hay que bajarlos primero
—con `gsutil cp` o similar— y recién ahí correr `grep`. Es lento, gasta ancho de
banda, y llena el disco de archivos que no querés.

Queremos una herramienta de línea de comandos que busque dentro del **contenido**
de los objetos remotos, sin bajarlos a disco:

```
gcsgrep "timeout" gs://logs/
```

y que muestre qué objeto matcheó, y dónde.

## Para quién es

Gente de desarrollo y de operaciones que ya usa `grep` todos los días y ya tiene
credenciales de GCP configuradas (vía `gcloud auth application-default login` u
otro mecanismo de ADC). No es una herramienta para usuarios finales ni para
gente que no conoce la línea de comandos.

## Qué NO es

- **No es un clon completo de `grep`.** Soporta literal y regex básica (estilo
  RE2, sin backtracking), no PCRE ni la sintaxis completa de ningún lenguaje.
- **No es un gestor de GCS.** No copia, no mueve, no borra, no cambia permisos.
- **Solo CLI.** No hay interfaz web, ni API, ni librería para importar (por ahora).
- **Solo GCS en la v1.** S3 y Azure Blob quedan afuera, aunque el diseño no debería
  cerrarles la puerta para siempre.
- **No reintenta indefinidamente ni escala privilegios.** Nunca hace más de lo
  que las credenciales de quien invoca ya permiten.

## Requerimientos funcionales (resueltos)

- **FR-a** — El usuario pasa un patrón (literal o regex básica) y una ubicación
  `gs://bucket/prefijo`, y la herramienta busca el patrón línea por línea dentro
  del contenido de los objetos de texto bajo ese prefijo, leyendo por streaming
  (sin bajar el objeto completo a disco).
- **FR-b** — La ubicación acepta tanto un bucket completo (`gs://bucket/`) como
  un prefijo dentro del bucket (`gs://bucket/prefijo`). Un prefijo sin `/` final
  se trata como filtro de nombre (matchea `prefijoX` además de `prefijo/Y`,
  igual que la semántica nativa de list de GCS) — no hay carpetas reales en GCS.
- **FR-c** — La salida por defecto tiene el formato `objeto:línea:texto`
  (análogo a `grep -Hn`). Si `stdout` es una terminal (TTY), el patrón matcheado
  se resalta con color ANSI; si `stdout` está redirigida (pipe o archivo), sale
  en texto plano sin color.
- **FR-d** — Soporta `-i` (case-insensitive), `-n` (número de línea — activado
  por defecto en el formato de FR-c), `-l` (listar solo objetos con al menos un
  match, cortando la lectura del objeto en el primer match) y `-c` (contar
  matches por objeto, leyendo el objeto completo). `-l` y `-c` son mutuamente
  excluyentes: pasarlas juntas es un error de uso (exit code 2).
- **FR-e** — El exit code sigue la convención de `grep`: `0` si hubo al menos un
  match, `1` si no hubo ningún match y no hubo errores, `2` si hubo algún error
  (objeto ilegible, guardrail de tamaño superado en algún objeto, o guardrail
  acumulado alcanzado) — incluso si hubo matches en otros objetos. Esto es
  deliberado: prioriza que un script detecte una corrida incompleta por sobre
  reportar éxito parcial silencioso.
- **FR-f** — Si un objeto individual no se puede leer (permisos, corrupto, o
  falla de red persistente tras reintentos — ver NFR-c), se emite un warning
  por `stderr` con el nombre del objeto y la razón, y la corrida continúa con
  el resto de los objetos. No aborta la corrida completa.
- **FR-g** — Mientras hay objetos pendientes de procesar, la herramienta emite
  indicadores de progreso por `stderr` (objetos procesados / total listado),
  independientemente de si corre en modo secuencial o concurrente.

## Reglas de negocio (decididas)

- **BR-a** — La herramienta nunca escribe en GCS. Solo lectura, siempre. Sin
  excepciones.
- **BR-b** — La herramienta no amplía el acceso más allá de las credenciales de
  quien la invoca (ADC). Si el usuario no puede leer un bucket u objeto,
  `gcsgrep` tampoco — y lo reporta como objeto/bucket ilegible (FR-f), no como
  crash.
- **BR-c** — Guardrail de cantidad de objetos: antes de leer contenido,
  `gcsgrep` lista y cuenta los objetos bajo el prefijo (operación de listado,
  barata en costo de GCS). Si la cantidad supera **1000 objetos** (valor por
  defecto propuesto, sin validar con benchmark), aborta antes de leer ningún
  contenido, con un mensaje que indica cómo levantar el límite. Se levanta con
  `--max N`, o `--max 0` para deshabilitarlo explícitamente.
- **BR-d** — Detección de binarios: un objeto se trata como binario (y se
  saltea, con aviso por `stderr`) si sus primeros 8 KiB contienen un byte nulo
  (`0x00`) — heurística estándar equivalente a la que usa GNU grep.
- **BR-e** — Los objetos `.gz` (detectados por extensión de nombre) se
  descomprimen al vuelo por streaming y se busca dentro del contenido
  descomprimido, tratado como texto (sujeto a la misma detección de binario de
  BR-d sobre el contenido ya descomprimido).
- **BR-f** — Guardrail de tamaño descomprimido por objeto: si el contenido
  descomprimido leído de un objeto supera **250 MiB** (valor por defecto
  propuesto), se corta la lectura de ese objeto, se emite un warning por
  `stderr` indicando que se alcanzó el límite, y se continúa con el resto de
  los objetos. Configurable con `--max-object-size`.
- **BR-g** — Guardrail de tamaño descomprimido acumulado: si la suma de bytes
  descomprimidos leídos en toda la corrida supera **2 GiB** (valor por defecto
  propuesto), la herramienta deja de leer objetos nuevos, informa por `stderr`
  que el guardrail acumulado se alcanzó y que el escaneo quedó incompleto, y
  termina reportando los matches encontrados hasta ese punto. Configurable con
  `--max-total-size`. Cuenta como error a efectos de FR-e (exit code 2).

## Requerimientos no funcionales

> Los umbrales de esta sección son una propuesta inicial de partida.
> Corresponde validarlos con un benchmark real (bucket de prueba, objetos de
> tamaño representativo) antes de congelarlos en la spec formal.

- **NFR-a — Rendimiento.** Sobre un bucket en la misma región que el cliente,
  con objetos de ~1 MiB promedio:
  - Throughput con `--concurrency 8`: ≥ 15 objetos/seg.
  - Throughput en modo secuencial (default, sin flag): ≥ 3 objetos/seg (cota
    inferior dominada por la latencia de red por objeto, ~150–300ms).
  - Latencia al primer resultado: ≤ 2 segundos, si el primer objeto con match
    está entre los primeros 50 objetos listados.
- **NFR-b — Memoria con objetos grandes.** El uso de memoria por objeto en
  proceso es constante respecto de su tamaño total: lectura por streaming en
  chunks de 64 KiB, con un buffer de línea acotado a **1 MiB** por defecto
  (configurable con `--max-line-size`). Una línea que exceda ese tamaño se
  trunca para el matching (se busca el patrón solo dentro de la porción
  bufferizada) y se emite un warning una única vez por objeto afectado. Memoria
  adicional estimada: ≤ 5 MiB por objeto en procesamiento simultáneo, por lo
  que con concurrencia N el uso adicional escala como ~5×N MiB.
- **NFR-c — Comportamiento ante fallos de red.** Un error transitorio de red
  (timeout, conexión reseteada, 5xx) al leer un objeto se reintenta hasta
  **3 intentos en total**, con backoff exponencial (500ms, 1s, 2s ± 20% de
  jitter). Si los 3 intentos fallan, el objeto se marca como fallido (FR-f) y
  la corrida continúa. Errores permanentes (403 Forbidden, 404 Not Found) no
  se reintentan: se marcan como fallidos de inmediato.

## Guardrail de concurrencia

- Por defecto, `gcsgrep` lee los objetos **secuencialmente** (equivalente a
  `--concurrency 1`).
- Se puede pedir lectura en paralelo con `--concurrency N` (alias `-j N`).
- `N` está acotado a un máximo de **32**; valores mayores son rechazados con un
  error de uso (exit code 2) antes de arrancar la corrida, para que el flag no
  se use como forma implícita de saltear los guardrails de costo/carga sobre
  la API de GCS.
- En modo concurrente, el orden de salida **no** coincide necesariamente con el
  orden de listado de objetos — cada objeto imprime sus resultados cuando
  termina de procesarse. Esto es comportamiento esperado, no un bug.
- **Modelo de ejecución: worker pool con cola compartida.** Se lanzan `N`
  workers (threads/goroutines/tareas, según la implementación) que consumen de
  una cola común con la lista de objetos a procesar. Cada worker toma un
  objeto, lo procesa de punta a punta, y cuando termina toma el siguiente de
  la cola — no hay reparto en bloques fijos por adelantado, así que un objeto
  grande no bloquea a los demás workers, que siguen sacando objetos chicos
  mientras tanto.
- Los contadores compartidos entre workers —bytes acumulados de BR-g, cantidad
  de objetos procesados para el progreso de FR-g— deben ser **thread-safe**
  (atomic o con lock). En particular, el corte del guardrail acumulado (BR-g)
  tiene que evaluarse de forma segura entre los `N` workers para no permitir
  que, por una condición de carrera, se lean más bytes de los que el límite
  permite antes de que el corte surta efecto en todos los workers.

## Decisiones tomadas

Las 10 preguntas abiertas del borrador original, resueltas:

1. **Sabor de regex** → literal + regex básica estilo RE2 (sin backtracking
   catastrófico). No configurable por flag en v1.
2. **Autenticación** → solo Application Default Credentials (ADC). Sin soporte
   de service account key file en v1.
3. **Sintaxis de ubicación** → solo `gs://bucket/prefijo`. Sin forma corta sin
   esquema, para no cerrarle la puerta a otros proveedores en el futuro.
4. **Flags de grep en v1** → `-i`, `-n` (por defecto en el formato de salida),
   `-l`, `-c`. `-v`, `-r`, `--include` quedan para iteraciones posteriores.
5. **Binarios y `.gz`** → binarios se saltean (heurística de byte nulo);
   `.gz` se descomprime al vuelo, sujeto a los guardrails de tamaño BR-f/BR-g.
6. **Guardrails de costo** → guardrail de cantidad de objetos (BR-c, 1000 por
   defecto) **más** guardrail de bytes descomprimidos por objeto (BR-f, 250
   MiB) **más** guardrail acumulado de toda la corrida (BR-g, 2 GiB) — este
   último surgió durante el refinamiento, no estaba en el borrador original.
7. **Formato de salida** → `objeto:línea:texto`, color si hay TTY, plano si
   hay pipe/redirección. Sin modo JSON en v1.
8. **Exit codes** → convención de `grep` (0/1/2), con la particularidad de que
   cualquier error parcial (objeto ilegible, guardrail de objeto o acumulado
   alcanzado) fuerza exit code 2 aunque haya habido matches en otros objetos.
9. **Concurrencia** → secuencial por defecto; paralelo opt-in con
   `--concurrency N` / `-j N`, tope máximo de 32. El orden de salida no está
   garantizado en modo concurrente.
10. **Objeto que cambia mid-lectura** → no se detecta ni se fija la
    generación al listar; se lee lo que GCS devuelva en el momento de la
    lectura. GCS ya garantiza que no se mezclan bytes de dos versiones dentro
    de una sola llamada de lectura.

## Cómo seguir

Con las preguntas resueltas, el paso siguiente ya no es refinar este
documento, sino convertirlo en la spec formal:

1. Atomizar cada FR en formato Dado/Cuando/Entonces.
2. Escribir el fundamento (y las excepciones) de cada BR.
3. Escribir un VC (criterio de verificación) por cada FR y cada BR — incluidos
   los guardrails nuevos (BR-f, BR-g) y el criterio de exit code de FR-e.
4. Correr esos umbrales de NFR contra un benchmark real antes de congelarlos.
5. Partir el resultado en `gcsgrep-plan.md` con ≥ 2 iteraciones — la Iteración
   1 sugerida es: búsqueda literal/regex básica sobre un prefijo, `-i`/`-n`,
   salida con nombre de objeto, streaming, solo lectura, exit codes estilo
   grep. Concurrencia, `-l`/`-c`, `.gz` y los guardrails de tamaño pueden ir en
   Iteración 1 si el tiempo lo permite, o pasar a Iteración 2 — es una decisión
   de scoping, no de spec.
