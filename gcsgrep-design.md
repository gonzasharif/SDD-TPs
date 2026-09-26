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
| Patrón | Value object | `match.Matcher` | Se compila una vez por corrida; `-i` lo vuelve case-insensitive |
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

Las 10 preguntas abiertas del borrador original, resueltas:

1. **Sabor de regex** → literal + regex básica estilo RE2 (sin backtracking
   catastrófico). No configurable por flag en v1.
2. **Autenticación** → solo Application Default Credentials (ADC). Sin soporte
   de service account key file en v1.
3. **Sintaxis de ubicación** → solo `gs://bucket/prefijo`. Sin forma corta sin
   esquema, para no cerrarle la puerta a otros proveedores en el futuro.
4. **Flags de grep en v1** → `-i`, `-n` (por defecto en el formato de salida),
   `-l`, `-c`. `-v`, `-r`, `--include` quedan para iteraciones posteriores.
5. **Objetos binarios y comprimidos** → los objetos binarios se saltean
   (heurística de byte nulo); los objetos comprimidos se descomprimen al
   vuelo, sujetos a los guardrails de tamaño BR-f/BR-g.
6. **Guardrails de costo** → guardrail de cantidad de objetos (BR-c, 1000 por
   defecto) **más** guardrail de bytes descomprimidos por objeto (BR-f, 250
   MiB) **más** guardrail acumulado de toda la corrida (BR-g, 2 GiB) — este
   último surgió durante el refinamiento, no estaba en el borrador original.
7. **Formato de salida** → `objeto:línea:texto`, color si hay TTY, plano si
   hay pipe/redirección. Sin modo JSON en v1.
8. **Exit codes** → convención de `grep` (0/1/2), con la particularidad de que
   cualquier error parcial (objeto fallido, objeto cortado o escaneo
   incompleto) fuerza exit code 2 aunque haya habido matches en otros objetos.
9. **Concurrencia** → secuencial por defecto; paralelo opt-in con
   `--concurrency N` / `-j N`, tope máximo de 32. El orden de salida no está
   garantizado en modo concurrente.
10. **Objeto que cambia mid-lectura** → no se detecta ni se fija la
    generación al listar; se lee lo que GCS devuelva en el momento de la
    lectura. GCS ya garantiza que no se mezclan bytes de dos versiones dentro
    de una sola llamada de lectura.

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
match       → aplica el patrón (literal o regex básica, case-insensitive
              si corresponde) sobre una línea de texto. Sin I/O — es la
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
