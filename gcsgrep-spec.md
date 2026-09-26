# gcsgrep — Spec

> **Estado: revisada.** Sin preguntas abiertas (ver cierre del documento).
> Deriva del base context refinado,
> [`gcsgrep-requirements.md`](./gcsgrep-requirements.md) (qué y con qué
> vocabulario), con las decisiones de
> [`gcsgrep-design.md`](./gcsgrep-design.md) (cómo y por qué).
> Este documento es el contrato verificable: cada FR está
> en formato Dado/Cuando/Entonces, cada BR tiene fundamento, y cada FR y BR
> tiene un VC (criterio de verificación) concreto. El alcance por iteración
> (qué entra en la Iteración 1 vs. después) vive en `gcsgrep-plan.md`, no acá.

## Convenciones

- **Sub-ítems.** Un requerimiento con sub-ítems (ej. FR-8.1, FR-8.2, FR-8.3)
  se cumple solo si se cumplen **todos** sus sub-ítems. Un VC con sub-ítems
  (ej. VC-8.1, VC-8.2, VC-8.3) pasa solo si pasan **todos**: alcanza con que
  falle uno para que el VC falle.
- **Un Dado, una situación.** Cada "Dado" describe una sola situación. Si un
  comportamiento depende de más de una situación, se parte en sub-ítems.
- **Vocabulario.** Los términos (objeto fallido, objeto salteado, aviso,
  error, etc.) tienen el significado del glosario de
  `gcsgrep-requirements.md`.
- **Mensajes por `stderr`.** Un aviso empieza con el prefijo literal
  `gcsgrep: warning: `; un mensaje de error, con `gcsgrep: error: `.

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

**VC-1** (pasa solo si pasan VC-1.1 y VC-1.2):
- **VC-1.1:** Contra un bucket de prueba con objetos de texto conocidos
  (algunos con match, otros sin match), ejecutar la búsqueda y verificar que
  se reportan exactamente los objetos y líneas esperados.
- **VC-1.2:** Búsqueda estática en el código de `gcsgrep/`, excluyendo los
  archivos `_test.go`: cero apariciones de `os.Create`, `os.CreateTemp`,
  `os.WriteFile` y `os.OpenFile`, es decir, ningún camino de código puede
  escribir el contenido de un objeto a disco.

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

- **Dado** un patrón y un objeto con el texto en una capitalización distinta
  a la del patrón,
- **Cuando** se ejecuta `gcsgrep -i PATRÓN ...`,
- **Entonces** se reporta el match sin distinguir mayúsculas de minúsculas.

**VC-4** (pasa solo si pasan VC-4.1 y VC-4.2), sobre un objeto cuya única
línea con la palabra es `TIMEOUT error` y el patrón `timeout`:
- **VC-4.1:** Con `-i`: la línea `TIMEOUT error` aparece en `stdout` y el
  exit code es `0`.
- **VC-4.2:** Sin `-i`: `stdout` queda vacío y el exit code es `1`.

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
  líneas que matchean.

**VC-6:** Objeto con K líneas que matchean (K conocido de antemano). Verificar
que la salida reporta exactamente K.

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

FR-8 se cumple solo si se cumplen FR-8.1, FR-8.2 y FR-8.3.

Cuenta como **error** a efectos de FR-8: un objeto fallido (FR-9), el
guardrail de cantidad alcanzado (BR-3), un objeto cortado (BR-4), un escaneo
incompleto (BR-5) y un error de uso (FR-7, FR-14).

#### FR-8.1 — Exit 0

- **Dado** una corrida con al menos un match y ningún error,
- **Cuando** el proceso termina,
- **Entonces** el exit code es `0`.

**VC-8.1:** Corrida contra un prefijo de prueba donde todos los objetos se
pueden leer y al menos uno tiene un match garantizado. Verificar exit code
`0`.

#### FR-8.2 — Exit 1

- **Dado** una corrida sin ningún match y sin ningún error,
- **Cuando** el proceso termina,
- **Entonces** el exit code es `1`.

**VC-8.2:** Corrida contra un prefijo de prueba donde todos los objetos se
pueden leer y ninguno contiene el patrón. Verificar exit code `1`.

#### FR-8.3 — Exit 2

- **Dado** una corrida en la que ocurrió al menos un error,
- **Cuando** el proceso termina,
- **Entonces** el exit code es `2`, aunque haya habido matches en otros
  objetos.

**VC-8.3:** Corrida contra un prefijo de prueba con un objeto sin permiso de
lectura y matches garantizados en el resto de los objetos. Verificar exit
code `2`.

### FR-9 — Un objeto fallido no interrumpe la corrida
*(deriva de FR-f)*

FR-9 se cumple solo si se cumplen FR-9.1, FR-9.2, FR-9.3 y FR-9.4.

En todos los casos el aviso tiene el formato literal
`gcsgrep: warning: <objeto>: <causa>`, donde `<objeto>` es el nombre de
objeto; el objeto no aparece en `stdout`, y cuenta como error para FR-8.3.

#### FR-9.1 — Objeto sin permiso de lectura

- **Dado** un objeto listado cuya lectura GCS rechaza por falta de permiso
  (HTTP 403),
- **Cuando** la herramienta intenta leerlo,
- **Entonces** emite por `stderr` el aviso con causa `permission denied` y
  continúa con el resto de los objetos.

**VC-9.1:** Prefijo con 5 objetos legibles, cada uno con un match
garantizado, y el objeto `acl/denied.log` sin permiso de lectura para las
credenciales de la corrida. Verificar que `stdout` contiene los 5 matches,
que `stderr` contiene la línea
`gcsgrep: warning: acl/denied.log: permission denied` y que el exit code es
`2`.

#### FR-9.2 — Objeto corrupto

- **Dado** un objeto corrupto (objeto comprimido cuyo contenido gzip es
  inválido),
- **Cuando** la herramienta intenta descomprimirlo,
- **Entonces** emite por `stderr` el aviso con causa `corrupt gzip data` y
  continúa con el resto de los objetos.

**VC-9.2:** Prefijo con `gz/ok.log` (un match garantizado) y `gz/bad.gz`
(contenido de texto plano, sin formato gzip, con nombre `.gz`). Verificar
que `stdout` contiene el match de `gz/ok.log`, que `stderr` contiene la
línea `gcsgrep: warning: gz/bad.gz: corrupt gzip data` y que el exit code
es `2`.

#### FR-9.3 — Objeto que dejó de existir

- **Dado** un objeto listado que ya no existe al momento de abrirlo (HTTP
  404, porque se borró entre el listado y la lectura),
- **Cuando** la herramienta intenta abrirlo,
- **Entonces** emite por `stderr` el aviso con causa `object not found` y
  continúa con el resto de los objetos.

**VC-9.3:** Con un cliente de GCS simulado que lista `x/gone.log` y
`x/ok.log` (un match garantizado) y responde 404 al abrir `x/gone.log`.
Verificar que `stdout` contiene el match de `x/ok.log`, que `stderr`
contiene la línea `gcsgrep: warning: x/gone.log: object not found` y que el
exit code es `2`.

#### FR-9.4 — Otro error permanente al abrir

- **Dado** un objeto listado cuya apertura falla con un error HTTP 4xx
  distinto de 403 y 404,
- **Cuando** la herramienta intenta abrirlo,
- **Entonces** emite por `stderr` el aviso con causa
  `read failed: <mensaje de error devuelto por GCS>` y continúa con el resto
  de los objetos.

**VC-9.4:** Prefijo con `x/ok.log` (un match garantizado) y `x/csek.log`,
subido cifrado con una clave provista por el cliente (CSEK), que GCS
rechaza con HTTP 400 al leerlo sin la clave. Verificar que `stdout`
contiene el match de `x/ok.log`, que `stderr` contiene una línea que
empieza con `gcsgrep: warning: x/csek.log: read failed: ` y que el exit
code es `2`.

### FR-10 — Progreso durante la corrida
*(deriva de FR-g)*

FR-10 se cumple solo si se cumplen FR-10.1 y FR-10.2.

El **texto de progreso** es
`gcsgrep: progress: <procesados>/<total> (<porcentaje>%)`, donde `<total>` es la cantidad de objetos listados,
`<procesados>` la cantidad de objetos ya procesados (con o sin matches,
salteados o fallidos) y `<porcentaje>` es `procesados × 100 / total`
redondeado hacia abajo.

#### FR-10.1 — `stderr` en una terminal

- **Dado** que `stderr` es una terminal (TTY) y la corrida tiene más de un
  objeto listado,
- **Cuando** termina de procesarse cada objeto,
- **Entonces** se redibuja en el lugar el texto de progreso (precedido por
  `\r`, sin `\n`), y al terminar la corrida se emite un único `\n`.

**VC-10.1:** Prefijo con 50 objetos, `stderr` conectado a un
pseudo-terminal (pty). Verificar que `stderr` contiene exactamente 50
redibujos (`\r` seguido del texto de progreso), con porcentajes 2, 4, …,
100 en orden creciente, y que termina en `\n`.

#### FR-10.2 — `stderr` redirigido

- **Dado** que `stderr` no es una terminal (está redirigido a un pipe o a un
  archivo) y la corrida tiene más de un objeto listado,
- **Cuando** el porcentaje procesado alcanza o supera un nuevo múltiplo de 10
  (10, 20, …, 100),
- **Entonces** se emite una línea con el texto de progreso terminada en
  `\n`, sin `\r`. Si un mismo objeto hace superar más de un múltiplo, se
  emite una sola línea.

**VC-10.2:** Mismo prefijo de 50 objetos que VC-10.1, con `stderr`
redirigido a un archivo. Verificar que el archivo contiene exactamente 10
líneas de progreso (10%, 20%, …, 100%, en ese orden) y ningún byte `\r`.

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

- **Dado** un objeto cuyo nombre termina en `.gz`,
- **Cuando** se lo procesa,
- **Entonces** se descomprime por streaming y se busca el patrón dentro del
  contenido descomprimido, reportando el nombre del objeto `.gz` original (no
  un nombre descomprimido), sujeto a los guardrails BR-4 y BR-5.

**VC-12:** Objeto `.gz` de prueba con contenido de texto conocido y matches
garantizados. Verificar que los matches se reportan con el nombre del objeto
`.gz`, y que en ningún momento se escribe el contenido descomprimido a disco.

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

**VC-17** (pasa solo si pasan VC-17.1, VC-17.2 y VC-17.3), sobre un prefijo
de prueba con 10 objetos:
- **VC-17.1:** Con `--max 5`: la corrida termina antes de leer contenido
  (solo la llamada de listado, 0 llamadas de apertura de objeto) y el exit
  code es `2`.
- **VC-17.2:** Con `--max 20`: se procesan los 10 objetos (10 llamadas de
  apertura).
- **VC-17.3:** Con `--max 0`: se procesan los 10 objetos (10 llamadas de
  apertura).

### BR-4 — Guardrail de tamaño por objeto
*(deriva de BR-f del borrador, nuevo en el refinamiento)*

**Regla:** si el contenido descomprimido leído de un objeto supera 250 MiB,
se corta la lectura de ese objeto y se continúa con el resto.

**Fundamento:** protección contra un objeto `.gz` que se expande de forma
desproporcionada (deliberada o accidentalmente) y consume tiempo, memoria o
costo fuera de proporción.

**Excepciones:** configurable con `--max-object-size`.

**VC-18:** Objeto `.gz` de prueba que descomprime a 2 MiB, con
`--max-object-size` fijado en 1 MiB, junto a otro objeto con un match
garantizado. Verificar que la lectura del `.gz` se corta en el límite, se
emite warning por `stderr`, y la corrida continúa con el resto de los objetos
del prefijo.

### BR-5 — Guardrail acumulado de la corrida
*(deriva de BR-g del borrador, nuevo en el refinamiento)*

**Regla:** si la suma de bytes descomprimidos leídos en toda la corrida
supera 2 GiB, se deja de leer objetos nuevos y se termina informando lo
encontrado hasta ese punto.

**Fundamento:** control de costo total de la corrida completa (no solo por
objeto individual) — leer de GCS se cobra por bytes.

**Excepciones:** configurable con `--max-total-size`.

**VC-19:** Prefijo con suficientes objetos para superar un
`--max-total-size` bajo (ej. 10 MiB, para que el test corra rápido).
Verificar que la herramienta deja de leer objetos nuevos al cruzar el límite,
reporta los matches encontrados hasta el corte, emite el aviso
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

**VC-21** (pasa solo si pasan VC-21.1, VC-21.2 y VC-21.3), con un benchmark
scripteado contra un bucket de prueba con ≥500 objetos de ~1 MiB:
- **VC-21.1:** Modo secuencial: throughput ≥ 0.5 objetos/seg.
- **VC-21.2:** Con `--concurrency 8`: throughput ≥ 15 objetos/seg.
- **VC-21.3:** Con el objeto con match entre los primeros 50 listados:
  tiempo hasta el primer resultado impreso ≤ 2 segundos.

El umbral secuencial (≥ 0.5 objetos/seg) ya
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

**Umbral:** un error transitorio (timeout, conexión reseteada, 5xx) se
reintenta hasta 3 intentos en total, con backoff exponencial (500ms, 1s, 2s
± 20% de jitter). Tras 3 fallos, el objeto se marca como fallido. Errores
permanentes (403, 404) no se reintentan.

**VC-23** (pasa solo si pasan VC-23.1, VC-23.2 y VC-23.3), con un proxy/mock
de la API de GCS:
- **VC-23.1:** 2 fallos transitorios seguidos de éxito → el objeto se procesa
  correctamente (recuperado, sin aparecer como fallido).
- **VC-23.2:** 3 fallos transitorios consecutivos → el objeto se marca como
  fallido (FR-9) tras exactamente 3 intentos.
- **VC-23.3:** un 403 → el objeto se marca como fallido inmediatamente, sin
  reintentos (1 solo intento).

## Cobertura de VCs

| Requisito | VC | Camino | Verificado por |
|---|---|---|---|
| FR-1 | VC-1.1 | feliz | Test de integración contra bucket de prueba |
| FR-1 | VC-1.2 | invariante (sin escritura a disco) | Búsqueda estática en el código |
| FR-2 | VC-2 | feliz | Test de integración |
| FR-3.1 | VC-3.1 | feliz | Test de integración (`stdout` a archivo) |
| FR-3.2 | VC-3.2 | borde (`stdout` en TTY) | Test de integración con pty |
| FR-3.3 | VC-3.3 | borde (`stdout` redirigido) | Test de integración (`stdout` a archivo) |
| FR-4 | VC-4.1, VC-4.2 | feliz | Test de integración |
| FR-5 | VC-5 | feliz | Test de integración + medición de bytes leídos |
| FR-6 | VC-6 | feliz | Test de integración |
| FR-7 | VC-7 | falla | Test unitario/CLI (validación de flags) |
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
| FR-12 | VC-12 | feliz | Test de integración con objeto .gz |
| FR-13 | VC-13 | medición | Benchmark de concurrencia |
| FR-14 | VC-14 | falla | Test unitario/CLI (validación de flags) |
| FR-15 | VC-24 | borde (línea extrema) | Test de integración con línea que excede el buffer |
| BR-1 | VC-15 | invariante | Test de integración con credenciales de solo lectura |
| BR-2 | VC-16 | invariante | Test de integración con ACL restringida |
| BR-3 | VC-17.1 | borde (límite de cantidad) | Test de integración, conteo de llamadas |
| BR-3 | VC-17.2, VC-17.3 | feliz (límite levantado / deshabilitado) | Test de integración, conteo de llamadas |
| BR-4 | VC-18 | borde (límite de tamaño) | Test de integración con .gz grande |
| BR-5 | VC-19 | borde (límite acumulado) | Test de integración con guardrail acumulado bajo |
| BR-6 | VC-20 | falla | = VC-14 |
| NFR-1 | VC-21.1, VC-21.2, VC-21.3 | medición | Benchmark de rendimiento |
| NFR-2 | VC-22 | medición | Benchmark de memoria (RSS) |
| NFR-3 | VC-23.1 | feliz (recuperación) | Test con proxy/mock de fallos de red |
| NFR-3 | VC-23.2, VC-23.3 | falla | Test con proxy/mock de fallos de red |

**15 FRs (23 FRs atómicos contando sub-ítems) + 6 BRs + 3 NFRs, 40 VCs
atómicos, 0 requerimientos sin VC.** De los 40 VCs, 19 ejercitan un camino
de falla o borde y 3 son invariantes — no es una tabla de puro camino feliz.

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

Ninguna. Las 10 preguntas abiertas del borrador original (commit `c84efbc`)
están resueltas en `gcsgrep-design.md`, y las que surgieron durante la atomización (mutua exclusión de `-l`/`-c`,
tope de concurrencia, riesgo de falso negativo en líneas largas) quedaron
resueltas en la revisión conversacional de este documento — no queda ningún
"a definir" pendiente.

## Qué sigue

El plan de iteraciones está en [`gcsgrep-plan.md`](./gcsgrep-plan.md).
