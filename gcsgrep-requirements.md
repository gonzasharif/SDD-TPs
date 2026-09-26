# gcsgrep — requerimientos (base context refinado)

> **Estado: refinado.** Este es el *base context* del proyecto: describe el
> problema, para quién es y qué tiene que cumplir la herramienta, con el
> vocabulario del dominio. No describe cómo se construye.
>
> Arrancó como el borrador deliberadamente subespecificado de la cátedra y se
> refinó en este mismo archivo; la versión original queda en el historial de
> git (commit `c84efbc`). En todo el repo, **"borrador original"** se refiere
> a esa versión.
>
> - Cómo se construye y por qué (decisiones, modelo de dominio,
>   arquitectura): [`gcsgrep-design.md`](./gcsgrep-design.md).
> - Contrato verificable (FRs/BRs/NFRs con su VC):
>   [`gcsgrep-spec.md`](./gcsgrep-spec.md).
>
> **"Base context refinado"** es este documento. Las 10 preguntas abiertas del
> borrador original están resueltas; cada decisión, con su fundamento y las
> alternativas descartadas, vive en `gcsgrep-design.md`.

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

- **No es un clon completo de `grep`.** El patrón es una expresión regular con
  sintaxis RE2 (sin backtracking); no soporta PCRE (backreferences,
  lookaround) ni la sintaxis BRE/ERE de POSIX.
- **No es un gestor de GCS.** No copia, no mueve, no borra, no cambia permisos.
- **Solo CLI.** No hay interfaz web, ni API, ni librería para importar (por ahora).
- **Solo GCS en la v1.** S3 y Azure Blob quedan afuera, aunque el diseño no debería
  cerrarles la puerta para siempre.
- **No reintenta indefinidamente ni escala privilegios.** Nunca hace más de lo
  que las credenciales de quien invoca ya permiten.

## Glosario (lenguaje ubicuo)

Cada término se usa siempre con este significado, en este documento, en
`gcsgrep-design.md`, en `gcsgrep-spec.md` y en `gcsgrep-plan.md`. La columna
"No usar" lista los sinónimos que se reemplazan por el término.

| Término | Definición | No usar |
|---|---|---|
| **Ubicación** | Argumento `gs://bucket/prefijo` que indica dónde buscar. | ruta, path |
| **Bucket** | Contenedor de objetos de GCS nombrado en la ubicación. | — |
| **Prefijo** | Parte de la ubicación después del bucket. Filtra objetos por el comienzo de su nombre; en GCS no existen carpetas. | carpeta, directorio |
| **Objeto** | Unidad de almacenamiento de GCS, identificada por su nombre dentro del bucket. | archivo |
| **Nombre de objeto** | Nombre completo del objeto dentro del bucket, sin `gs://bucket/` (ej. `logs/a.log`). | ruta del objeto |
| **Patrón** | Expresión regular con sintaxis RE2 que se busca en cada línea. Un texto sin metacaracteres es un patrón válido que se busca literalmente; un metacarácter se busca literal escapándolo con `\` (ej. `1\.2`). | query, término |
| **Línea** | Secuencia de bytes de un objeto terminada en `\n` o en el fin del objeto. | — |
| **Match** | Línea de un objeto en la que el patrón aparece al menos una vez. | hit |
| **Corrida** | Una invocación completa de `gcsgrep`, desde el parseo de argumentos hasta el exit code. | ejecución, búsqueda (como sustantivo) |
| **Objeto binario** | Objeto cuyos primeros 8 KiB de contenido (descomprimido, si es un objeto comprimido) contienen un byte nulo (`0x00`). | — |
| **Objeto de texto** | Objeto que no es binario. | — |
| **Objeto comprimido** | Objeto cuyo nombre termina en `.gz`. | — |
| **Objeto corrupto** | Objeto comprimido cuyo contenido gzip es inválido (cabecera o CRC). El término no aplica a objetos no comprimidos. | dañado |
| **Objeto salteado** | Objeto que deliberadamente no se busca por una regla (hoy: objeto binario). **No** es un error. | ignorado, omitido |
| **Línea salteada** | Línea que supera el tamaño máximo de línea y no se busca. **No** es un error. | línea truncada |
| **Objeto fallido** | Objeto que no se pudo leer completo: sin permiso, no encontrado, objeto corrupto, otro error permanente al abrirlo, fallo de red tras agotar los reintentos, o lectura interrumpida a mitad. **Es** un error. | ilegible, no accesible, inaccesible |
| **Objeto cortado** | Objeto cuya lectura se detuvo al alcanzar el guardrail de tamaño por objeto. **Es** un error. | — |
| **Guardrail** | Límite de costo que impide o corta la lectura: cantidad de objetos, tamaño por objeto, tamaño acumulado de la corrida. | tope, límite (sueltos) |
| **Escaneo incompleto** | Corrida que terminó sin leer todos los objetos listados porque se alcanzó el guardrail acumulado. **Es** un error. | — |
| **Aviso** | Mensaje por `stderr` con prefijo `gcsgrep: warning:` sobre una situación de la que la corrida se recupera y continúa. | warning (en prosa), advertencia |
| **Mensaje de error** | Mensaje por `stderr` con prefijo `gcsgrep: error:` que acompaña el fin de la corrida por un error. | — |
| **Error** | Toda situación que obliga a terminar con exit code 2: objeto fallido, objeto cortado, escaneo incompleto, guardrail de cantidad alcanzado, error de uso, o fallo de acceso a la ubicación. | falla (como sinónimo) |
| **Error de uso** | Invocación inválida: flag desconocido, flags incompatibles, cantidad de argumentos incorrecta, valores no enteros o fuera de rango, patrón que no es una regex RE2 válida, ubicación sin `gs://`. | — |
| **Progreso** | Indicador por `stderr` de objetos procesados sobre el total listado. | — |
| **Credenciales ADC** | Credenciales de Application Default Credentials de quien invoca. | — |

## Requerimientos funcionales (resueltos)

Todos derivan del borrador original (FR-a a FR-g).

- **FR-a** — El usuario pasa un patrón (regex RE2) y una ubicación
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
  (objeto fallido, objeto cortado o escaneo incompleto) — incluso si hubo
  matches en otros objetos. Esto es deliberado: prioriza que un script detecte
  una corrida incompleta por sobre reportar éxito parcial silencioso.
- **FR-f** — Un objeto fallido (sin permiso, objeto corrupto, o falla de red
  persistente tras reintentos — ver NFR-c) genera un aviso por `stderr` con el
  nombre del objeto y la causa, y la corrida continúa con el resto de los
  objetos. No aborta la corrida completa.
- **FR-g** — Mientras hay objetos pendientes de procesar, la herramienta emite
  progreso por `stderr` (objetos procesados / total listado),
  independientemente de si corre en modo secuencial o concurrente.

## Reglas de negocio (decididas)

- **BR-a** — La herramienta nunca escribe en GCS.
  Solo lectura, siempre. Sin excepciones.
- **BR-b** — La herramienta no amplía el acceso más
  allá de las credenciales ADC de quien la invoca. Si el usuario no puede leer
  un bucket u objeto, `gcsgrep` tampoco — y lo reporta como error (objeto
  fallido, o fallo de acceso a la ubicación), no como crash.
- **BR-c** — Guardrail de cantidad de objetos: antes
  de leer contenido, `gcsgrep` lista y cuenta los objetos bajo el prefijo
  (operación de listado, barata en costo de GCS). Si la cantidad supera
  **1000 objetos** (valor por defecto),
  aborta antes de leer ningún contenido, con un mensaje de error que indica
  cómo levantar el límite. Se levanta con `--max N`, o `--max 0` para
  deshabilitarlo explícitamente.
- **BR-d** — Detección de binarios: un objeto
  binario se saltea, con aviso por `stderr` — heurística estándar equivalente
  a la que usa GNU grep.
- **BR-e** — Los objetos comprimidos se
  descomprimen al vuelo por streaming y se busca dentro del contenido
  descomprimido, tratado como texto (sujeto a la misma detección de binario de
  BR-d sobre el contenido ya descomprimido).
- **BR-f** — Guardrail de tamaño por objeto: si el contenido leído de un
  objeto (descomprimido, si es un objeto comprimido) supera **250 MiB** (valor
  por defecto), se corta la lectura de ese objeto (objeto cortado),
  se emite un aviso por `stderr` indicando que se alcanzó el límite, y se
  continúa con el resto de los objetos. Configurable con `--max-object-size`
  (en bytes; `0` lo deshabilita).
- **BR-g** — Guardrail de tamaño descomprimido
  acumulado: si la suma de bytes descomprimidos leídos en toda la corrida
  supera **2 GiB** (valor por defecto), la herramienta corta la
  lectura en ese punto —incluido el objeto en curso— y no abre objetos nuevos,
  informa por `stderr` que el guardrail acumulado se alcanzó y que hubo
  escaneo incompleto, y termina reportando los matches encontrados hasta ese
  punto. Configurable con `--max-total-size` (en bytes; `0` lo deshabilita).
  Cuenta como error a
  efectos de FR-e (exit code 2).

## Requerimientos no funcionales

- **NFR-a — Rendimiento.** Sobre un prefijo de 500 objetos de 1 MiB, en un
  bucket de `us-central1`:
  - Throughput en modo secuencial (default, sin flag): **≥ 0,5 objetos/seg**.
    Medido en la Iteración 1: 0,59 objetos/seg desde un entorno de latencia
    alta, donde el throughput de una sola conexión queda acotado por la red y
    no por el código (se investigó un buffer de lectura más grande y lecturas
    por rango en paralelo sobre un mismo objeto; ninguno mejoró el número).
  - Throughput con `--concurrency 8`: **≥ 3 veces** el throughput secuencial
    medido en la misma sesión (medido en la Iteración 1: 4,6 veces). El umbral
    es relativo porque el valor absoluto depende de la red hacia GCS.
    **`--concurrency` es la recomendación operativa en redes de latencia
    alta.**
  - Latencia al primer resultado, en modo secuencial y con el match en la
    línea 1 del primer objeto listado: **≤ 2 segundos**.
- **NFR-b — Memoria con objetos grandes.** El uso de memoria por objeto en
  proceso es constante respecto de su tamaño total: lectura por streaming en
  chunks de 64 KiB, con un buffer de línea acotado a **1 MiB** (valor fijo en
  v1). Una línea que exceda ese tamaño es una
  **línea salteada** (no se busca el patrón en ninguna porción de ella) y se
  emite un aviso una única vez por objeto afectado — no se trunca para
  matchear, porque un truncado partiría un match real que cayera justo en el
  punto de corte y lo perdería en silencio. Memoria adicional: ≤ 5 MiB por
  objeto en procesamiento simultáneo (procesar un objeto de 500 MiB no usa
  más de 5 MiB de RSS por encima de procesar uno de 5 MiB), así que con
  concurrencia N el uso adicional es ≤ 5 × N MiB.
- **NFR-c — Comportamiento ante fallos de red.** Un error transitorio de red
  (timeout, conexión reseteada, HTTP 408, 429 o 5xx) al listar el prefijo o al abrir un objeto
  se reintenta hasta **3 intentos en total**, con backoff exponencial entre
  intentos (500ms antes del 2º, 1s antes del 3º, ± 20% de jitter). Si los 3
  intentos fallan al abrir, el objeto queda como objeto fallido (FR-f) y la
  corrida continúa; si fallan al listar, la corrida termina con error.
  Errores permanentes (HTTP 4xx distinto de 408 y 429, como 403 Forbidden o
  404 Not Found) no se reintentan. Un
  error a mitad de la lectura de un objeto no se reintenta: el objeto queda
  como objeto fallido y las líneas ya impresas se mantienen, sin duplicarse.

## Concurrencia

- Por defecto, `gcsgrep` lee los objetos **secuencialmente** (equivalente a
  `--concurrency 1`).
- Se puede pedir lectura en paralelo con `--concurrency N` (alias `-j N`).
- `N` está acotado al rango **1–32**; valores fuera de ese rango son un error de uso
  (exit code 2) antes de arrancar la corrida, para que el flag no se use como
  forma implícita de saltear los guardrails de costo/carga sobre la API de
  GCS.
- En modo secuencial, los resultados salen en el orden del listado de
  objetos. En modo concurrente, el orden entre objetos **no** coincide
  necesariamente con el orden de listado — cada objeto imprime sus resultados
  cuando termina de procesarse —, pero las líneas de un mismo objeto salen
  juntas y en orden ascendente. Esto es comportamiento esperado, no un bug.

Cómo se implementa (modelo de ejecución, contadores compartidos): ver
`gcsgrep-design.md`.

## Cómo sigue

El pipeline de documentos del proyecto:

1. **Este documento** — qué hay que resolver y con qué vocabulario.
2. [`gcsgrep-design.md`](./gcsgrep-design.md) — decisiones, modelo de dominio y
   arquitectura.
3. [`gcsgrep-spec.md`](./gcsgrep-spec.md) — contrato verificable.
4. [`gcsgrep-plan.md`](./gcsgrep-plan.md) — iteraciones.
5. [`gcsgrep-cobertura-vc.md`](./gcsgrep-cobertura-vc.md) — evidencia de
   verificación.
