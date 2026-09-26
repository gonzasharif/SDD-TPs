# gcsgrep — diseño

> **Estado: vigente.** Complementa [`gcsgrep-requirements.md`](./gcsgrep-requirements.md)
> (el *qué*, en el lenguaje del dominio) con el *cómo* y el *por qué*:
> decisiones, modelo de dominio y arquitectura. Usa el lenguaje ubicuo
> definido en el glosario de `gcsgrep-requirements.md`.
>
> La spec ([`gcsgrep-spec.md`](./gcsgrep-spec.md)) no depende de este
> documento para ser verificable: define comportamiento observable. El plan
> ([`gcsgrep-plan.md`](./gcsgrep-plan.md)) y el código (`gcsgrep/`) sí se
> apoyan en él.

## Contexto delimitado

`gcsgrep` tiene un único contexto delimitado: **búsqueda de contenido en
object storage**. Todo el lenguaje del glosario pertenece a ese contexto.

Google Cloud Storage es un sistema externo. Se integra a través de
`gcsclient`, que funciona como **capa anticorrupción**: traduce el SDK de GCS
a las dos únicas operaciones que el dominio necesita (listar objetos y abrir
el contenido de un objeto) y no expone ninguna operación de escritura (BR-a).
Un proveedor futuro (S3, Azure Blob) entraría como otra implementación de esa
misma interfaz, sin cambiar el dominio.

## Modelo de dominio

| Concepto (glosario) | Tipo | Representación en el código | Invariantes |
|---|---|---|---|
| Ubicación | Value object | `cli.parseLocation` → `(bucket, prefix)` | Empieza con `gs://`; bucket no vacío |
| Patrón | Value object | `match.Matcher` | Sintaxis RE2; se compila una vez por corrida; `-i` lo vuelve case-insensitive |
| Objeto listado | Value object | `gcsclient.ObjectInfo{Name, Size}` | No lleva contenido: sale del listado |
| Match | Value object | `reader.LineMatch{LineNum, Text}` | `LineNum` ≥ 1 |
| Resultado de objeto | Value object | `reader.ObjectResult` | Exactamente un estado: con/sin matches, objeto salteado u objeto fallido |
| Corrida | Raíz de agregado | `scanner.Run` + `scanner.Config` | Aplica el guardrail de cantidad antes de leer contenido; agrega los resultados de objeto y decide un único exit code |
| Límites de la corrida | Value object | `scanner.Config` (`MaxObjects`, `MaxLineSize`) | `MaxObjects = 0` deshabilita el guardrail de cantidad |
| Exit code | Resultado de la corrida | `scanner.ExitMatch` / `ExitNoMatch` / `ExitError` | 0, 1 o 2 |

Servicios de dominio: **búsqueda en un objeto** (`reader.ProcessObject`, sin
estado, recibe un stream) y **escaneo de una ubicación** (`scanner.Run`,
orquesta la corrida).

## Decisiones de diseño

Cada decisión responde una pregunta abierta del borrador original (1 a 10) o
una que surgió en el refinamiento (11 a 13). Formato fijo: **Elegido** (qué
se decidió), **Fundamento** (por qué), **Descartado** (qué otras opciones se
consideraron y por qué no) y, cuando corresponde, **Trade-off aceptado** (qué
se pierde con lo elegido).

### 1. Sabor de regex

**Elegido:** el patrón es siempre una expresión regular con sintaxis RE2
completa (paquete `regexp` de Go). No hay modo literal aparte: un texto sin
metacaracteres ya es una regex que se busca literalmente, y un metacarácter
se busca literal escapándolo con `\` (ej. `1\.2`). `-i` equivale a anteponer
`(?i)` al patrón.

**Fundamento:** RE2 garantiza tiempo de matching lineal en el largo de la
línea — sin backtracking catastrófico —, así que ningún patrón, por
patológico que sea, puede colgar una corrida que recorre objetos de cientos
de MiB. Es parte de la biblioteca estándar de Go (cero dependencias) y su
sintaxis es casi la misma que ya conoce quien usa `grep -E`.

**Descartado:**
- *PCRE (backreferences, lookaround):* el backtracking puede ser exponencial
  en el largo de la entrada, y requiere una dependencia externa con cgo.
- *Flag `-F` para modo literal:* suma un flag, un FR y un VC para algo que ya
  se resuelve escapando el metacarácter.
- *Subconjunto "básico" enumerado:* obliga a mantener y validar una gramática
  propia, y dos implementaciones podrían aceptar patrones distintos.
- *BRE/ERE de POSIX (como `grep`):* no hay implementación en la biblioteca
  estándar y habría que especificar sus diferencias con RE2.

**Trade-off aceptado:** en BRE (`grep` sin `-E`) `+`, `?` y `|` son
literales; en `gcsgrep` son metacaracteres, igual que en `grep -E`.

### 2. Autenticación

**Elegido:** solo Application Default Credentials (ADC), resueltas por el
SDK de GCS con su orden estándar. `gcsgrep` no acepta ningún flag para
indicar un archivo de credenciales. Sin credenciales resolubles, la corrida
termina con mensaje de error y exit code 2 sin leer nada.

**Fundamento:** ADC es el mecanismo que el público objetivo ya tiene
configurado (`gcloud auth application-default login`, o la identidad del
entorno en una VM, Cloud Run o CI). Usarlo como única vía hace que
`gcsgrep` opere exactamente con la identidad que el entorno ya resolvió
(BR-b), sin una segunda forma de elegir identidad y sin manejar secretos
propios.

**Descartado:**
- *Flag para archivo de service account key (`--credentials FILE`):* abre
  una segunda identidad posible por invocación, deja la ruta del secreto en
  el historial de la shell, y no agrega capacidad: quien necesita una service
  account ya puede usarla vía ADC (`GOOGLE_APPLICATION_CREDENTIALS`).
- *ADC y key file combinados:* suma reglas de precedencia a especificar y
  verificar sin un caso de uso de v1 que las requiera.

**Trade-off aceptado:** no se puede cambiar de identidad para una sola
invocación sin cambiar el entorno.

### 3. Sintaxis de ubicación

**Elegido:** solo `gs://bucket/prefijo`. Un prefijo sin `/` final es un
filtro por comienzo de nombre (`gs://b/logs` incluye `logs/a.log` y
`logs-old/b.log`), igual que la semántica de listado de GCS.

**Fundamento:** el esquema identifica al proveedor sin ambigüedad, así que
otro proveedor (`s3://`) podría sumarse sin romper nada; es la misma forma
que usan `gcloud storage` y `gsutil`. El prefijo como filtro de nombre
refleja que GCS no tiene carpetas y permite buscar por fecha en el nombre
(`gs://b/logs/2026-09`).

**Descartado:**
- *`bucket/prefijo` sin esquema:* ambiguo con rutas locales y con otros
  proveedores.
- *Agregar `/` al final automáticamente:* impediría filtrar por comienzo de
  nombre y se apartaría del comportamiento nativo de GCS.

### 4. Flags de `grep` en v1

**Elegido:** `-i`, `-n`, `-l` y `-c`. `-n` se acepta sin efecto (el formato ya
incluye el número de línea). `-l` y `-c` juntos son un error de uso (exit 2).

**Fundamento:** `-i` y `-n` están en la v1 sugerida por la consigna. `-l` y
`-c` reducen el volumen de salida y `-l` además reduce costo: corta la
lectura del objeto en el primer match, así que se leen menos bytes de GCS.
Los dos reutilizan el mismo pipeline de lectura; solo cambia la salida.
`-n` se acepta porque rechazarlo rompería la costumbre de escribir
`grep -n`. `-l` + `-c` es un error porque no hay un formato de salida único
para ambos, y elegir uno en silencio esconde un error de uso.

**Descartado:**
- *`-v`:* en objetos grandes imprime casi todo el contenido (costo sin caso
  de uso claro) y multiplica combinaciones con `-l`/`-c`.
- *`-r`:* no aplica: buscar bajo un prefijo ya es recursivo en GCS.
- *`--include`:* se aproxima con el prefijo; sumaría una sintaxis de globs a
  especificar y verificar.
- *Rechazar `-n` como error de uso:* rompe scripts y costumbres de `grep`.

### 5. Objetos binarios y comprimidos

**Elegido:**
- Un objeto binario (byte nulo en sus primeros 8 KiB) es un objeto salteado,
  con aviso por `stderr`.
- Un objeto comprimido (nombre terminado en `.gz`) se descomprime al vuelo,
  por streaming, y se busca en el contenido descomprimido. La detección de
  objeto binario y los guardrails de tamaño se aplican sobre el contenido
  descomprimido. Un objeto comprimido con contenido gzip inválido es un
  objeto corrupto: objeto fallido.

**Fundamento:**
- *Binarios:* un byte nulo es la misma señal que usa GNU `grep` para
  considerar binario un archivo,
  tiene costo fijo (8 KiB por objeto, sin importar su tamaño) y evita volcar
  bytes de control a la terminal.
- *`.gz`:* el caso de uso principal son logs, que se rotan y archivan
  comprimidos; saltearlos dejaría afuera justamente los objetos históricos
  que más se buscan. Además, un `.gz` siempre tiene bytes nulos, así que sin
  descompresión se saltearía siempre como binario. Descomprimir por streaming
  mantiene la memoria constante (NFR-b) y no escribe nada a disco (FR-a).
  Detectar por extensión no requiere leer bytes extra.

**Descartado:**
- *Detectar binarios por `Content-Type`:* es un metadato opcional y muchas
  veces queda en el valor por defecto (`application/octet-stream`).
- *Detectar binarios por extensión:* en GCS los nombres son arbitrarios.
- *Saltear los `.gz` con aviso:* pierde el caso de uso principal.
- *Descargar y descomprimir a disco:* viola FR-a.
- *Otros formatos (zip, bz2, zstd):* cada uno suma detección y dependencias;
  zip además no se puede leer por streaming (su índice está al final).

**Trade-off aceptado:**
- Texto en UTF-16 o UTF-32 tiene bytes nulos, así que se saltea como objeto
  binario (falso positivo). Un binario sin bytes nulos en sus primeros 8 KiB
  se procesa como texto (falso negativo). Se acepta porque los logs, caso
  principal, son ASCII/UTF-8.
- Un objeto gzip cuyo nombre no termina en `.gz` no se descomprime: se
  saltea como objeto binario, con aviso.
- Un `.gz` chico puede expandirse muchísimo al descomprimirse; por eso
  existen los guardrails de tamaño (decisión 6).

### 6. Guardrails de costo

**Elegido:** tres guardrails, cada uno con valor por defecto y forma de
levantarlo:

| Guardrail | Valor por defecto | Actúa | Se levanta con |
|---|---|---|---|
| Cantidad de objetos (BR-c) | 1000 objetos | Antes de leer contenido, después de listar | `--max N` (`--max 0` lo deshabilita) |
| Tamaño por objeto (BR-f) | 250 MiB leídos (descomprimidos, si aplica) | Durante la lectura de cada objeto | `--max-object-size N` en bytes (`0` lo deshabilita) |
| Tamaño acumulado (BR-g) | 2 GiB leídos (descomprimidos, si aplica) | Durante toda la corrida; corta también el objeto en curso | `--max-total-size N` en bytes (`0` lo deshabilita) |

**Fundamento:** cada guardrail cubre un riesgo que los otros no ven:
- *Cantidad:* frena un escaneo de un bucket enorme antes de leer un solo
  byte de contenido, porque listar es barato y leer se cobra.
- *Por objeto:* frena un objeto que se expande al descomprimirse sin cortar
  el resto de la corrida.
- *Acumulado:* acota los bytes totales leídos aunque cada objeto sea chico.

Valores: 1000 objetos cubre un prefijo típico de logs (un servicio en un
rango de días) y corta un bucket entero de millones. 250 MiB son 250 veces
el objeto típico de 1 MiB de NFR-a. 2 GiB permite una corrida completa al
tope de cantidad con objetos típicos (1000 × 1 MiB ≈ 1 GiB) con margen 2×,
y acota a 8 los objetos que llegan al tope por objeto.

**Descartado:**
- *Un solo guardrail (cantidad):* no protege contra un objeto enorme ni
  contra un `.gz` que se expande.
- *Confirmación interactiva ("¿seguro?"):* rompe el uso desde scripts (el
  proceso consumidor no tiene TTY).
- *Estimar el costo en dinero antes de correr:* depende de región y clase de
  almacenamiento, y el tamaño descomprimido no se conoce de antemano.
- *Límite por tiempo de corrida:* no se relaciona con el costo.

### 7. Formato de salida

**Elegido:** `objeto:línea:texto`, con el nombre de objeto sin
`gs://bucket/` (ej. `logs/a.log:2:ERROR timeout`). Si `stdout` es una
terminal, el texto matcheado se resalta con color ANSI; si está redirigido
(pipe o archivo), sale en texto plano.

**Fundamento:** es el formato de `grep -Hn`, que editores y herramientas
(`cut -d:`, quickfix) ya saben leer. El bucket se omite porque una corrida
es siempre sobre un único bucket. El color ayuda a ubicar el match en
líneas largas de log; condicionarlo a TTY (el mismo criterio que
`grep --color=auto`) garantiza que un proceso consumidor nunca reciba
secuencias ANSI.

**Descartado:**
- *Salida JSON:* duplica el formateo y los FRs; el formato de `grep` ya es
  parseable.
- *URI completa (`gs://b/logs/a.log`):* más larga sin información nueva.
- *Color siempre:* ensucia pipes y archivos.
- *Sin color nunca:* pierde legibilidad interactiva.
- *Flag `--color`:* suma un flag sin caso de uso en v1.

### 8. Exit codes

**Elegido:** `0` si hubo al menos un match y ningún error; `1` si no hubo
matches ni errores; `2` si hubo algún error (ver glosario), aunque haya
habido matches en otros objetos. Una ubicación sin objetos no es un error:
termina con `1` y un aviso.

**Fundamento:** es la convención de GNU `grep`, que los scripts ya usan. Que
cualquier error fuerce `2` prioriza que un script detecte una corrida
incompleta: un `0` con objetos fallidos le haría creer que se buscó todo.
Una ubicación vacía no impide buscar (no hay nada que buscar), pero el aviso
hace visible un prefijo mal escrito.

**Descartado:**
- *`0` si hubo matches aunque haya errores:* oculta corridas incompletas.
- *Un código por tipo de error (3, 4, …):* rompe la convención y los scripts
  que chequean `2`; el detalle va en el mensaje por `stderr`.
- *Ubicación vacía como `2`:* trataría como falla un prefijo legítimamente
  vacío (ej. los logs de hoy todavía no escritos).
- *Ubicación vacía como `1` sin aviso:* un typo en el prefijo pasaría
  inadvertido.

### 9. Concurrencia

**Elegido:** secuencial por defecto; lectura en paralelo con
`--concurrency N` / `-j N`, con `1 ≤ N ≤ 32`. Orden de salida: en modo
secuencial, el orden del listado (lexicográfico); en modo concurrente, el
orden entre objetos no está garantizado, pero las líneas de un mismo objeto
salen juntas y en orden ascendente. Implementación: worker pool con cola
compartida (ver [Modelo de ejecución](#modelo-de-ejecución-concurrencia)).

**Fundamento:** el default secuencial da salida determinística y la menor
carga posible sobre la API. El paralelismo es opt-in porque el techo del
modo secuencial es de red, no de código: en la Iteración 1 se midió
0,59 objetos/seg en secuencial y 2,69 con 8 workers (4,6×; ver nota 2 de
`gcsgrep-cobertura-vc.md`). El tope de 32 deja margen 4× sobre esos 8
workers sin permitir que un typo (`-j 1000`) abra cientos de conexiones.
Imprimir cada objeto al terminarlo evita bufferear la salida de toda la
corrida (NFR-b) y no demora el primer resultado (NFR-a).

**Descartado:**
- *Paralelo por defecto:* salida no determinística y más carga sin pedirla.
- *Sin tope:* un valor alto por error genera picos de tráfico contra GCS.
- *Reparto en bloques fijos por worker:* un objeto grande deja workers
  ociosos.
- *Orden global garantizado en modo concurrente:* obliga a bufferear
  resultados (memoria) y retrasa el primer resultado.
- *Paralelismo dentro de un objeto (range reads):* en la Iteración 1 no
  mostró mejora (nota 2 de `gcsgrep-cobertura-vc.md`).

### 10. Objeto que cambia durante la lectura

**Elegido:** no se fija la generación del objeto al listar; se lee lo que
GCS devuelva al abrirlo.

**Fundamento:** GCS garantiza que una misma lectura no mezcla bytes de dos
versiones del objeto, y el caso de uso principal (logs) agrega objetos
nuevos en vez de sobreescribir los existentes.

**Descartado:**
- *Leer con la generación del listado como precondición:* si el objeto
  cambió, la lectura falla y el objeto queda como objeto fallido — más exit
  2 para un caso que no afecta al uso principal. Queda como riesgo conocido
  (ver [Riesgo conocido](#riesgo-conocido)).

### 11. Reintentos ante fallos de red

**Elegido:** un error transitorio (timeout, conexión reseteada, 5xx) al
listar o al abrir un objeto se reintenta hasta 3 intentos en total, con
esperas de 500ms antes del 2º intento y 1s antes del 3º, ± 20% de jitter.
403 y 404 no se reintentan. Un error a mitad de la lectura de un objeto no
se reintenta: el objeto queda como objeto fallido y las líneas ya impresas
se mantienen.

**Fundamento:** 3 intentos absorben un error puntual (un 503 aislado) sin que
un objeto caído demore la corrida más de ~1,5 s de espera. El backoff
exponencial con jitter es la estrategia que recomienda Google para GCS y
evita que varios workers reintenten sincronizados. 403 y 404 son permanentes:
reintentarlos no cambia el resultado. A mitad de la lectura ya pueden haberse
impreso matches del objeto; releerlo desde el inicio los duplicaría en
`stdout`.

**Descartado:**
- *Sin reintentos:* un error puntual deja un objeto fallido y la corrida en
  exit 2.
- *Reintentos indefinidos:* la corrida podría no terminar nunca.
- *Esperas de 500ms, 1s y 2s con 3 intentos (valor anterior):* 3 intentos
  tienen solo 2 esperas; la tercera nunca ocurría.
- *Releer desde el inicio y suprimir líneas ya impresas:* vuelve a leer (y
  pagar) bytes y obliga a llevar registro de lo impreso.
- *Retomar desde el último byte con una lectura por rango:* hay que manejar
  una línea partida entre dos lecturas, y un `.gz` no se puede retomar a
  mitad del stream.

### 12. Tamaño máximo de línea

**Elegido:** 1 MiB, valor fijo en v1. Una línea más larga es una línea
salteada: no se busca el patrón en ninguna parte de ella, y se emite un
único aviso por objeto afectado.

**Fundamento:** acota la memoria por objeto (NFR-b) sin afectar logs
normales, cuyas líneas están órdenes de magnitud por debajo de 1 MiB.
Saltear la línea entera, en vez de truncarla, evita reportar un match
partido o perder uno que cae justo en el punto de corte.

**Descartado:**
- *Truncar la línea y buscar en lo que entra:* un match que cruza el corte se
  pierde en silencio o se reporta mal.
- *Buffer sin límite:* una línea de varios GiB (ej. un JSON minificado)
  rompería NFR-b.
- *Flag `--max-line-size` en v1:* diferido al plan (alcance posterior); en
  v1 no hay un caso de uso que requiera cambiar el valor.

### 13. Progreso

**Elegido:** si `stderr` es una terminal, una línea de progreso (objetos
procesados / total listado y porcentaje) que se redibuja en el lugar cada vez
que termina un objeto. Si no es una terminal, una línea simple cada 10% de objetos
procesados, sin secuencias de redibujado.

**Fundamento:** redibujar al terminar cada objeto es el mínimo que refleja
avance real, sin temporizadores. En un log, una línea cada 10% acota la
salida a 10 líneas sin importar la cantidad de objetos.

**Descartado:**
- *Actualizar por tiempo (cada 1 s):* en corridas cortas no aparece ninguna
  línea y el resultado no es determinístico, lo que complica su VC.
- *Cada N objetos fijos:* con miles de objetos llena el log.

## Modelo de ejecución (concurrencia)

- **Worker pool con cola compartida.** Se lanzan `N` workers
  (threads/goroutines/tareas, según la implementación) que consumen de una
  cola común con la lista de objetos a procesar. Cada worker toma un objeto,
  lo procesa de punta a punta, y cuando termina toma el siguiente de la cola —
  no hay reparto en bloques fijos por adelantado, así que un objeto grande no
  bloquea a los demás workers, que siguen sacando objetos chicos mientras
  tanto.
- Los contadores compartidos entre workers —bytes acumulados de BR-g, cantidad
  de objetos procesados para el progreso de FR-g— deben ser **thread-safe**
  (atomic o con lock). En particular, el corte del guardrail acumulado (BR-g)
  tiene que evaluarse de forma segura entre los `N` workers para no permitir
  que, por una condición de carrera, se lean más bytes de los que el límite
  permite antes de que el corte surta efecto en todos los workers.

## Arquitectura

### Módulos

```
cli         → parsea argv/flags, valida combinaciones inválidas (-l + -c,
              --concurrency fuera de rango), despacha al scanner, traduce
              el resultado final a exit code (FR-8)
scanner     → lista objetos bajo el prefijo vía gcsclient, aplica el
              guardrail de cantidad (BR-3) antes de leer nada, arma el
              worker pool (FR-13) y reparte objetos de una cola compartida,
              agrega los contadores compartidos (bytes acumulados de BR-5,
              progreso de FR-10) de forma thread-safe, y decide el
              resultado global (¿hubo match? ¿hubo algún error?)
reader      → por objeto individual: abre el stream vía gcsclient, detecta
              objeto binario (FR-11), descomprime objetos comprimidos al
              vuelo (FR-12), aplica el guardrail de tamaño por objeto
              (BR-4), aplica el límite de línea (FR-15), reintenta ante
              fallos de red transitorios (NFR-3), y usa match para evaluar
              cada línea
match       → aplica el patrón (regex RE2, case-insensitive si
              corresponde) sobre una línea de texto. Sin I/O — es la
              pieza más fácil de testear unitariamente
gcsclient   → capa anticorrupción: única puerta de entrada a la API de
              GCS. Lista objetos y abre su stream de lectura, con las
              credenciales ADC del usuario. No expone ningún método de
              escritura, copia o borrado — eso hace estructuralmente
              imposible que el resto del código viole BR-1, no depende de
              que nadie se acuerde de no llamar a un método de escritura
output      → formatea resultados para stdout (objeto:línea:texto, color
              si hay TTY — FR-3) y escribe avisos/progreso a stderr
              (FR-9, FR-10), también con su propia detección de TTY
```

La dependencia va en una sola dirección: `cli → scanner → reader → gcsclient`,
con `match` y `output` como hojas sin dependencias de negocio. `gcsclient` es
el único módulo que conoce el SDK de GCS; `match` no sabe que existe una red,
y eso es lo que permite testear el matching y el parseo de flags sin tocar
un bucket real.

### Flujo de datos

```
argv → cli (parsea + valida)
         → scanner: lista objetos vía gcsclient, aplica BR-3
             → worker pool (N workers, cola compartida)
                 → por cada objeto: reader (gcsclient + match)
                                        ↓
                resultado del objeto: matches / salteado / fallido
                                        ↓
                              output (stdout: matches · stderr: avisos)
         ← scanner agrega el resultado global
       → cli traduce a exit code (FR-8)
```

### Actores

| Actor | Interacción |
|---|---|
| Persona operadora/desarrolladora | Ejecuta `gcsgrep` en una shell o script, lee stdout/stderr |
| Script consumidor | Ejecuta `gcsgrep` y decide en base al **exit code**, no al texto |
| Google Cloud Storage | Fuente de datos vía `gcsclient`; puede fallar por permisos, red, o no existir el bucket/objeto |

### Riesgo conocido

Un objeto puede sobreescribirse entre el momento en que `scanner` lo lista y
el momento en que `reader` lo abre para leer. **Decisión: fuera de alcance
en v1** — no se fija la generación del objeto al listar (ver decisión #10
en [Decisiones de diseño](#decisiones-de-diseño)). Se acepta porque GCS ya
garantiza que no se mezclan bytes de dos versiones dentro de una sola llamada
de lectura, y el caso de uso principal (logs) tiende a agregar objetos nuevos
en vez de sobreescribir los existentes. Queda anotado acá para que sea una
decisión consciente, y aparece como no-objetivo explícito en la spec.
