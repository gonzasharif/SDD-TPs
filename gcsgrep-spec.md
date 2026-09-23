# gcsgrep — Spec

> **Estado: revisada.** Sin preguntas abiertas (ver cierre del documento).
> Deriva de [`gcsgrep-requirements.md`](./gcsgrep-requirements.md) (base
> context refinado). Este documento es el contrato verificable: cada FR está
> en formato Dado/Cuando/Entonces, cada BR tiene fundamento, y cada FR y BR
> tiene un VC (criterio de verificación) concreto. El alcance por iteración
> (qué entra en la Iteración 1 vs. después) vive en `gcsgrep-plan.md`, no acá.

## Propósito

Permitir buscar texto dentro del contenido de objetos de Google Cloud Storage
sin descargarlos primero a disco, con la misma experiencia de uso que `grep`,
para gente de desarrollo/operaciones que ya tiene credenciales de GCP
configuradas.

## Alcance

**Incluido (v1, todas las iteraciones):**
- Búsqueda literal y regex básica (estilo RE2, sin backtracking) sobre
  objetos de texto bajo un bucket o prefijo de GCS.
- Autenticación vía Application Default Credentials (ADC) únicamente.
- Flags `-i`, `-n` (por defecto), `-l`, `-c`.
- Salida `objeto:línea:texto`, con color condicional a TTY.
- Exit codes estilo `grep` (0/1/2).
- Lectura por streaming, solo lectura, sin amplificación de acceso.
- Descompresión de `.gz` al vuelo; detección y salteo de binarios.
- Guardrails de costo: cantidad de objetos, tamaño descomprimido por objeto,
  tamaño descomprimido acumulado.
- Concurrencia configurable (worker pool, tope 32).

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
  warnings y progreso, pensados para un humano.

## Requerimientos funcionales

### FR-1 — Búsqueda básica sobre un prefijo
*(deriva de FR-a del borrador)*

- **Dado** un bucket y prefijo `gs://bucket/prefijo` accesibles con las
  credenciales ADC del usuario, y un patrón literal o regex básica,
- **Cuando** el usuario ejecuta `gcsgrep PATRÓN gs://bucket/prefijo`,
- **Entonces** la herramienta lee cada objeto de texto bajo ese prefijo por
  streaming (sin materializar el objeto completo en disco) y busca el patrón
  línea por línea.

**VC-1:** Contra un bucket de prueba con objetos de texto conocidos (algunos
con match, otros sin match), ejecutar la búsqueda y verificar que se reportan
exactamente los objetos y líneas esperados. Verificar además que en ningún
momento se escribe un archivo temporal con el contenido completo de un objeto
(inspección del directorio temp o de syscalls de escritura de archivo durante
la corrida).

### FR-2 — Búsqueda sobre bucket completo
*(deriva de FR-b)*

- **Dado** un bucket accesible sin prefijo especificado (`gs://bucket/`),
- **Cuando** el usuario ejecuta `gcsgrep PATRÓN gs://bucket/`,
- **Entonces** la búsqueda cubre todos los objetos de texto del bucket,
  sujeta al guardrail de cantidad (BR-3).

**VC-2:** Bucket de prueba con objetos bajo múltiples prefijos distintos;
confirmar que una sola invocación sin prefijo los cubre todos, sin necesidad
de repetir la búsqueda por subprefijo.

### FR-3 — Formato de salida
*(deriva de FR-c)*

- **Dado** que se encontró al menos un match,
- **Cuando** la herramienta imprime resultados en `stdout`,
- **Entonces** cada resultado tiene el formato `objeto:línea:texto`, con el
  texto matcheado resaltado en color ANSI si `stdout` es un TTY, y sin color
  si `stdout` está redirigido (pipe o archivo).

**VC-3:** Ejecutar con `stdout` conectado a un pty (o forzando `isatty`) y
verificar presencia de secuencias ANSI de color alrededor del match. Ejecutar
con `stdout` redirigido a un archivo y verificar ausencia total de secuencias
ANSI, con el mismo formato `objeto:línea:texto` en texto plano.

### FR-4 — Búsqueda case-insensitive (`-i`)
*(deriva de FR-d)*

- **Dado** un patrón y un objeto con el texto en una capitalización distinta
  a la del patrón,
- **Cuando** se ejecuta `gcsgrep -i PATRÓN ...`,
- **Entonces** se reporta el match sin distinguir mayúsculas de minúsculas.

**VC-4:** Objeto con la línea `TIMEOUT error` y patrón `timeout`. Con `-i`
debe matchear; sin `-i` no debe matchear.

### FR-5 — Listar solo objetos con match (`-l`)
*(deriva de FR-d)*

- **Dado** el flag `-l`,
- **Cuando** se encuentra el primer match dentro de un objeto,
- **Entonces** se corta la lectura de ese objeto en ese punto y se imprime
  únicamente el nombre del objeto (sin número de línea ni texto), una sola
  vez por objeto.

**VC-5:** Objeto de gran tamaño con un match garantizado en la primera línea.
Verificar que la cantidad de bytes leídos del objeto es consistente con un
corte temprano (no se lee el objeto completo) y que la salida es solo el
nombre del objeto.

### FR-6 — Contar matches por objeto (`-c`)
*(deriva de FR-d)*

- **Dado** el flag `-c`,
- **Cuando** se procesa un objeto completo,
- **Entonces** se imprime `objeto:cantidad`, donde `cantidad` es el total de
  líneas que matchean — incluso si es `0` (igual que `grep -c`). Los objetos
  salteados (binarios) o fallidos no imprimen línea de conteo: no se
  procesaron completos, y un `0` ahí sería falso.

**VC-6:** Prefijo con un objeto con K líneas que matchean (K conocido de
antemano) y otro objeto de texto sin ningún match. Verificar que la salida
reporta exactamente `objeto1:K` y `objeto2:0`.

### FR-7 — `-l` y `-c` son mutuamente excluyentes
*(nuevo, surgido en el refinamiento)*

- **Dado** que el usuario pasa `-l` y `-c` en la misma invocación,
- **Cuando** se ejecuta `gcsgrep`,
- **Entonces** no se realiza ninguna búsqueda, se imprime un error de uso por
  `stderr`, y el proceso termina con exit code 2.

**VC-7:** Invocar con ambos flags contra un bucket real. Verificar exit code 2
y cero llamadas a la API de lectura de objetos (se puede instrumentar con un
contador de llamadas o un mock de cliente GCS).

### FR-8 — Exit codes estilo `grep`
*(deriva de FR-e)*

- **Dado** el resultado final de una corrida,
- **Cuando** el proceso termina,
- **Entonces** el exit code es `0` si hubo ≥1 match y cero errores; `1` si
  hubo cero matches y cero errores; `2` si hubo algún error (objeto ilegible,
  guardrail de objeto o acumulado alcanzado, o error de uso) —
  independientemente de si hubo matches en otros objetos.

**VC-8:** Tres corridas controladas contra un bucket de prueba: (a) con match
garantizado y todos los objetos legibles → exit 0; (b) sin ningún match y
todos los objetos legibles → exit 1; (c) con al menos un objeto sin permiso
de lectura (ACL restringida) y matches en el resto → exit 2.

### FR-9 — Continuar ante objeto ilegible
*(deriva de FR-f)*

- **Dado** un objeto no accesible (permisos) o corrupto dentro del prefijo,
- **Cuando** se lo encuentra durante la corrida,
- **Entonces** se emite un warning por `stderr` con el nombre del objeto y la
  causa, y la corrida continúa con el resto de los objetos.

**VC-9:** Prefijo con 5 objetos legibles y 1 objeto con ACL que le niega
lectura al usuario de prueba. Verificar que los 5 se procesan igual y que el
objeto sin permiso aparece en un warning de `stderr`, no interrumpe la
corrida.

### FR-10 — Progreso durante la corrida
*(deriva de FR-g)*

- **Dado** un prefijo con más de un objeto (el total ya se conoce de
  antemano por el listado de BR-3),
- **Cuando** la corrida está en curso,
- **Entonces**, si `stderr` es una terminal (TTY), se muestra una barra de
  progreso con porcentaje (`objetos procesados / total`) que se redibuja en
  el lugar (como `git clone` o `brew install`); si `stderr` no es una
  terminal (redirigido a archivo o pipe), se emiten en cambio líneas de
  progreso simples cada 10% de los objetos procesados (más una línea final
  si la corrida se corta antes, por BR-5), sin redibujado, para no ensuciar
  un log con secuencias de control.

**VC-10:** Dos escenarios. (a) Con `stderr` conectado a un pty (o forzando
`isatty`), sobre un prefijo con ≥50 objetos: verificar que aparecen
secuencias de redibujado (`\r`) y que el porcentaje mostrado crece de forma
monótona hasta 100%. (b) Con `stderr` redirigido a un archivo, mismo
prefijo: verificar que aparecen al menos 2 líneas de progreso simples antes
de la línea final, sin ninguna secuencia `\r` en el archivo resultante.

### FR-11 — Binarios se saltean
*(deriva de BR-d del borrador; comportamiento disparado por un evento
concreto —encontrar un objeto binario—, por eso vive como FR y no como BR)*

- **Dado** un objeto cuyos primeros 8 KiB contienen un byte nulo (`0x00`),
- **Cuando** se lo encuentra durante la corrida,
- **Entonces** se saltea sin intentar matchear contra su contenido, y se
  emite un aviso por `stderr`.

**VC-11:** Prefijo con un objeto binario (ej. una imagen) junto a objetos de
texto con matches garantizados. Verificar que el binario nunca aparece en los
resultados de match y sí aparece en un aviso de "salteado por binario", y que
los objetos de texto se procesan con normalidad.

### FR-12 — Descompresión de `.gz` al vuelo
*(deriva de BR-e del borrador; mismo criterio que FR-11)*

- **Dado** un objeto cuyos primeros 2 bytes son la firma gzip (`1f 8b`),
  independientemente de su nombre,
- **Cuando** se lo procesa,
- **Entonces** se descomprime por streaming y se busca el patrón dentro del
  contenido descomprimido, reportando el nombre original del objeto (no un
  nombre descomprimido), sujeto a los guardrails BR-4 y BR-5. La detección
  de binarios (FR-11) se aplica sobre el contenido **ya descomprimido**, no
  sobre los bytes comprimidos (que siempre contienen bytes nulos).

*Por qué la firma y no la extensión:* un objeto subido con metadata
`Content-Encoding: gzip` (ej. `gsutil cp -Z`) le llega al cliente ya
descomprimido por GCS (decompressive transcoding), aunque su nombre termine
en `.gz`; descomprimirlo de nuevo por extensión fallaría. Mirar la firma del
contenido que efectivamente llega resuelve ambos casos con la misma regla.

**VC-12:** Tres objetos de prueba con contenido de texto conocido y matches
garantizados: (a) `.gz` subido tal cual; (b) `.gz` subido con
`Content-Encoding: gzip`; (c) objeto gzip sin extensión `.gz`. Verificar que
en los tres casos los matches se reportan con el nombre original del objeto,
y que en ningún momento se escribe el contenido descomprimido a disco.

### FR-13 — Concurrencia configurable
*(deriva de la decisión de concurrencia)*

- **Dado** el flag `--concurrency N` (alias `-j N`) con `1 ≤ N ≤ 32`,
- **Cuando** se ejecuta `gcsgrep`,
- **Entonces** se procesan hasta `N` objetos en simultáneo mediante un
  worker pool que consume de una cola compartida de objetos a procesar.

**VC-13:** Sobre un prefijo con M objetos (M suficientemente grande para que
la latencia de red domine), medir el tiempo total con `--concurrency 1` vs.
`--concurrency 8` y verificar una reducción significativa de tiempo. Verificar
también que el conjunto de objetos reportados es idéntico en ambos casos,
independientemente del orden de impresión.

### FR-14 — Tope de concurrencia
*(deriva de la decisión de concurrencia)*

- **Dado** `--concurrency N` con `N > 32`,
- **Cuando** se invoca `gcsgrep`,
- **Entonces** se rechaza la ejecución con un error de uso por `stderr` y
  exit code 2, sin iniciar ninguna operación de lectura sobre GCS.

**VC-14:** Invocar con `--concurrency 100`. Verificar exit code 2 y cero
llamadas a la API de lectura de objetos.

### FR-15 — Líneas que exceden el buffer se saltean
*(nuevo, surgido en el refinamiento — corrige un riesgo de falso negativo:
truncar una línea para matchear podría partir un match real justo en el
punto de corte y perderlo silenciosamente)*

- **Dado** un objeto con una línea cuyo tamaño supera el buffer configurado
  (1 MiB por defecto, `--max-line-size`),
- **Cuando** se la encuentra durante la lectura por streaming,
- **Entonces** esa línea se saltea por completo —no se busca el patrón en
  ninguna porción de ella—, y se emite un warning por `stderr` la primera vez
  que esto ocurre en un objeto (una única vez por objeto afectado, no una vez
  por cada línea larga).

**VC-24:** Objeto con una línea de 5 MiB que contiene un match real embebido
en algún punto de esa línea (ej. en el byte 2 MiB). Verificar que ese match
**no** se reporta (a diferencia de un truncado parcial, que podría reportarlo
partido o de forma inconsistente), que el resto de las líneas normales del
mismo objeto se procesan con normalidad, y que aparece exactamente un warning
por `stderr` para ese objeto — no uno por cada línea larga si hay varias.

## Reglas de negocio

### BR-1 — Solo lectura
*(deriva de BR-a)*

**Regla:** `gcsgrep` nunca escribe, modifica ni borra nada en GCS. Solo
invoca operaciones de lectura (list, get/read de objetos).

**Fundamento:** una herramienta de búsqueda no debería poder alterar el
bucket ni por diseño ni por un bug — minimiza el blast radius de cualquier
falla o mal uso.

**Excepciones:** ninguna.

**VC-15:** Ejecutar la suite de pruebas funcionales completa usando
credenciales con el rol IAM `Storage Object Viewer` (sin permisos de
escritura) y verificar que todo el comportamiento funciona igual que con
credenciales de lectura/escritura — si `gcsgrep` necesitara algún permiso de
escritura, fallaría con estas credenciales.

### BR-2 — No amplifica el acceso del usuario
*(deriva de BR-b)*

**Regla:** `gcsgrep` nunca expone contenido al que el usuario invocante no
tendría acceso por sus propias credenciales ADC.

**Fundamento:** la herramienta debe operar estrictamente dentro del límite
de autorización de quien la ejecuta; de lo contrario sería un vector de
escalamiento de acceso.

**Excepciones:** ninguna.

**VC-16:** Con credenciales que tienen acceso a un subconjunto de objetos del
prefijo (ACL a nivel de objeto), verificar que los objetos sin permiso se
reportan como fallidos (FR-9) y en ningún momento su contenido aparece en la
salida.

### BR-3 — Guardrail de cantidad de objetos
*(deriva de BR-c)*

**Regla:** antes de leer contenido, `gcsgrep` lista y cuenta los objetos bajo
el prefijo. Si la cantidad supera el límite (1000 por defecto), aborta antes
de leer ningún contenido.

**Fundamento:** evitar que un uso descuidado escanee accidentalmente un
bucket de millones de objetos, con el costo y tiempo que eso implica.

**Excepciones:** el usuario puede levantar el límite explícitamente con
`--max N`, o deshabilitarlo con `--max 0`.

**VC-17:** Prefijo de prueba con más objetos que el límite por defecto (o
límite bajo simulado, ej. `--max 5` sobre un prefijo con 10 objetos).
Verificar que se aborta antes de cualquier lectura de contenido (0 llamadas
de lectura de objeto, solo la llamada de listado), y que con `--max 20` (o
`--max 0`) se procesan todos.

### BR-4 — Guardrail de tamaño por objeto
*(deriva de BR-f del borrador, nuevo en el refinamiento)*

**Regla:** ningún objeto —comprimido o no— lee más de 250 MiB de contenido
(descomprimido, si aplica). Se aplica en dos puntos:

1. **Antes de abrir:** si el tamaño que informa el listado ya supera el
   límite, el objeto se saltea sin leer ni un byte. Vale para cualquier
   objeto: un objeto comprimido de más de 250 MiB en GCS descomprime, en la
   práctica, a más que eso.
2. **Durante la lectura:** si el contenido leído (descomprimido, si es gzip)
   cruza el límite, se corta la lectura de ese objeto. Cubre el caso que el
   punto 1 no puede ver: un gzip chico que se expande mucho.

En ambos casos se emite un warning por `stderr` y se continúa con el resto de
los objetos. Los matches ya impresos de un objeto cortado en el punto 2 se
mantienen.

**Fundamento:** protección contra un objeto que consume tiempo, memoria o
costo fuera de proporción — sea un `.gz` que se expande de forma
desproporcionada (deliberada o accidentalmente) o simplemente un objeto de
texto enorme.

**Excepciones:** configurable con `--max-object-size` (en bytes, o con sufijo
`KiB`/`MiB`/`GiB`, ej. `--max-object-size 500MiB`); `--max-object-size 0` lo deshabilita,
igual que `--max 0` en BR-3.

**VC-18:** Dos escenarios, con límite bajo simulado vía `--max-object-size`
para acelerar el test. (a) Objeto de texto plano cuyo tamaño listado supera
el límite: verificar cero llamadas de apertura para ese objeto y un warning
por `stderr`. (b) Objeto gzip chico que descomprime a más del límite:
verificar que la lectura se corta en el límite y se emite warning. En ambos,
la corrida continúa con el resto de los objetos del prefijo y termina con
exit code 2 (por FR-8).

### BR-5 — Guardrail acumulado de la corrida
*(deriva de BR-g del borrador, nuevo en el refinamiento)*

**Regla:** si la suma de bytes descomprimidos leídos en toda la corrida
supera 2 GiB, se corta la lectura en ese mismo punto —incluido el objeto que
se está leyendo, aunque esté a la mitad—, no se abre ningún objeto nuevo, y
se termina informando lo encontrado hasta ahí.

**Fundamento:** control de costo total de la corrida completa (no solo por
objeto individual) — leer de GCS se cobra por bytes.

**Excepciones:** configurable con `--max-total-size` (en bytes, o con sufijo
`KiB`/`MiB`/`GiB`, ej. `--max-total-size 500MiB`); `--max-total-size 0` lo deshabilita,
igual que `--max 0` en BR-3.

**VC-19:** Prefijo con suficientes objetos para superar un
`--max-total-size` bajo (ej. 10 MiB, para que el test corra rápido).
Verificar que la herramienta corta la lectura al cruzar el límite (el total
de bytes leídos no supera el límite por más de un chunk de lectura), no abre
ningún objeto posterior, reporta los matches encontrados hasta el corte, emite el aviso
correspondiente por `stderr`, y termina con exit code 2 (por FR-8).

### BR-6 — Tope de concurrencia
*(deriva de la decisión de concurrencia, complementa FR-14)*

**Regla:** el valor de `--concurrency` está acotado a un máximo de 32.

**Fundamento:** evitar que el flag se use como forma implícita de saltear
los guardrails de costo/carga, generando picos de tráfico contra la API de
GCS que un solo usuario no debería poder producir sin querer.

**Excepciones:** ninguna en v1 (no hay forma de levantar este tope).

**VC-20:** idéntico a VC-14.

## Requerimientos no funcionales

> Umbrales propuestos en el refinamiento, pendientes de validar con
> benchmark real antes de congelarlos definitivamente (ver
> `gcsgrep-requirements.md`).

### NFR-1 — Rendimiento

**Umbral:** sobre un bucket en la misma región que el cliente, con objetos
de ~1 MiB promedio: throughput ≥ 15 objetos/seg con `--concurrency 8` en
redes de baja latencia; **≥ 0.5 objetos/seg en modo secuencial (default),
incluso en redes de latencia alta** (el modo secuencial queda acotado por
RTT/ventana TCP de una sola conexión, no por el código — en redes de baja
latencia se espera bastante más); latencia al primer resultado ≤ 2 segundos
si el objeto con match está entre los primeros 50 listados, en redes de
baja latencia. En entornos de latencia alta, `--concurrency` es la
recomendación operativa, no el modo secuencial.

**VC-21:** Correr un benchmark scripteado contra un bucket de prueba con
≥500 objetos de ~1 MiB, midiendo tiempo total en modo secuencial y con
`--concurrency 8`, y tiempo hasta el primer resultado impreso. Comparar
contra los umbrales de arriba. El umbral secuencial (≥ 0.5 objetos/seg) ya
se validó una vez, contra un bucket real desde un entorno de latencia alta
(0.59 objetos/seg observado) — ver `gcsgrep-cobertura-vc.md`. El umbral con
concurrencia queda pendiente de la Iteración 3.

### NFR-2 — Memoria con objetos grandes

**Umbral:** el uso de memoria adicional por objeto en proceso es constante
respecto de su tamaño total (lectura por streaming en chunks de 64 KiB, buffer
de línea acotado a 1 MiB por defecto — ver FR-15 para el comportamiento
cuando una línea individual supera ese buffer). Memoria adicional estimada
≤ 5 MiB por objeto en procesamiento simultáneo.

**VC-22:** Medir el RSS del proceso mientras procesa un objeto de ~5 MB y,
por separado, uno de ~5 GB (mismo contenido repetido). Verificar que la
diferencia de memoria pico entre ambas corridas es marginal (no proporcional
al tamaño del objeto) — ej. diferencia ≤ 20 MiB.

### NFR-3 — Comportamiento ante fallos de red

**Umbral:** un error transitorio (timeout, conexión reseteada, 5xx, 408,
429) al **listar** el prefijo o al **abrir** un objeto —momentos en los que
todavía no se imprimió nada— se reintenta hasta 3 intentos en total, con backoff
exponencial entre intentos (500ms antes del 2º, 1s antes del 3º, ± 20% de
jitter). Tras 3 fallos, el objeto se marca como fallido. Errores permanentes
(403, 404) no se reintentan.

Un error a **mitad de la lectura** (después de que el objeto ya se abrió) no
se reintenta: para ese punto ya pueden haberse impreso matches del objeto, y
releerlo desde el principio los duplicaría en `stdout`. El objeto se marca
como fallido (FR-9, exit 2 por FR-8); los matches ya impresos se mantienen,
porque son correctos — lo que queda incompleto es el resto del objeto, y el
exit 2 lo señala. El reintento automático del SDK de GCS se desactiva, para
que la política efectiva sea exactamente esta y no la suma de dos.

**VC-23:** Con un mock del cliente de GCS que simula, al abrir: (a) 2 fallos
transitorios seguidos de éxito → verificar que el objeto se procesa
correctamente (recuperado, sin aparecer como fallido); (b) 3 fallos
transitorios consecutivos → verificar que el objeto se marca como fallido
(FR-9) tras exactamente 3 intentos; (c) un 403 → verificar que se marca como
fallido inmediatamente, sin reintentos. Además: (d) un stream que falla a
mitad de la lectura, después de un match → verificar que ese match se
imprime una sola vez, el objeto queda como fallido y hay exactamente 1
apertura (sin reintento).

## Cobertura de VCs

| Requisito | VC | Camino | Verificado por |
|---|---|---|---|
| FR-1 | VC-1 | feliz | Test de integración contra bucket de prueba |
| FR-2 | VC-2 | feliz | Test de integración |
| FR-3 | VC-3 | borde (entorno TTY / pipe) | Test de integración (TTY simulado + pipe) |
| FR-4 | VC-4 | feliz | Test de integración |
| FR-5 | VC-5 | feliz | Test de integración + medición de bytes leídos |
| FR-6 | VC-6 | feliz | Test de integración |
| FR-7 | VC-7 | falla | Test unitario/CLI (validación de flags) |
| FR-8 | VC-8 | feliz + falla (3 casos) | 3 tests de integración (match/no-match/error) |
| FR-9 | VC-9 | falla (recuperable) | Test de integración con ACL restringida |
| FR-10 | VC-10 | borde (entorno TTY / pipe) | Test de integración, captura de stderr |
| FR-11 | VC-11 | borde (tipo de contenido) | Test de integración con objeto binario |
| FR-12 | VC-12 | feliz | Test de integración con objeto .gz |
| FR-13 | VC-13 | medición | Benchmark de concurrencia |
| FR-14 | VC-14 | falla | Test unitario/CLI (validación de flags) |
| FR-15 | VC-24 | borde (línea extrema) | Test de integración con línea que excede el buffer |
| BR-1 | VC-15 | invariante | Test de integración con credenciales de solo lectura |
| BR-2 | VC-16 | invariante | Test de integración con ACL restringida |
| BR-3 | VC-17 | borde (límite de cantidad) | Test de integración con prefijo grande |
| BR-4 | VC-18 | borde (límite de tamaño) | Test de integración con .gz grande |
| BR-5 | VC-19 | borde (límite acumulado) | Test de integración con guardrail acumulado bajo |
| BR-6 | VC-20 | falla | = VC-14 |
| NFR-1 | VC-21 | medición | Benchmark de rendimiento |
| NFR-2 | VC-22 | medición | Benchmark de memoria (RSS) |
| NFR-3 | VC-23 | feliz + falla (3 casos) | Test con proxy/mock de fallos de red |

**15 FRs + 6 BRs + 3 NFRs = 24 requerimientos, 24 VCs, 0 huérfanos.** De los
24, 9 ejercitan un camino de falla o borde y 2 son invariantes — no es una
tabla de puro camino feliz.

## Trazabilidad al borrador original

| Spec | Borrador (`gcsgrep-requirements.md`) |
|---|---|
| FR-1, FR-2 | FR-a, FR-b |
| FR-3 | FR-c |
| FR-4, FR-5, FR-6, FR-7 | FR-d |
| FR-8 | FR-e |
| FR-9 | FR-f |
| FR-10 | FR-g |
| FR-11 | BR-d (promovido a FR, sin BR equivalente) |
| FR-12 | BR-e (promovido a FR, sin BR equivalente) |
| FR-13, FR-14, BR-6 | Decisión de concurrencia (sin equivalente en el borrador) |
| FR-15 | NFR-b (memoria) — promovido a FR, corrige riesgo de falso negativo detectado en revisión |
| BR-1 | BR-a |
| BR-2 | BR-b |
| BR-3 | BR-c |
| BR-4, BR-5 | Sin equivalente — surgidas en el refinamiento |

## Preguntas abiertas

Ninguna. Las 10 preguntas abiertas del borrador (`gcsgrep-requirements.md`)
y las que surgieron durante la atomización (mutua exclusión de `-l`/`-c`,
tope de concurrencia, riesgo de falso negativo en líneas largas) quedaron
resueltas en la revisión conversacional de este documento — no queda ningún
"a definir" pendiente.

## Qué sigue

El plan de iteraciones está en [`gcsgrep-plan.md`](./gcsgrep-plan.md).
