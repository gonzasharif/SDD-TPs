# gcsgrep — requerimientos (borrador)

> **Estado: borrador.** Este documento está **deliberadamente subespecificado**.
> No es una spec. Es el punto de partida de la tarea de la Lección 1: tu trabajo
> es refinarlo hasta convertirlo en un contrato verificable.
>
> Los FRs de abajo son vagos a propósito, los BRs son candidatos sin decidir, los
> NFRs están en blanco, y hay una lista de preguntas abiertas al final. Si algo te
> parece impreciso, no es un error del documento: es el ejercicio.

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
credenciales de GCP configuradas. No es una herramienta para usuarios finales ni
para gente que no conoce la línea de comandos.

## Qué NO es

Esto importa tanto como lo de arriba:

- **No es un clon completo de `grep`.** No aspiramos a soportar todos los flags.
- **No es un gestor de GCS.** No copia, no mueve, no borra, no cambia permisos.
- **Solo CLI.** No hay interfaz web, ni API, ni librería para importar (por ahora).
- **Solo GCS en la v1.** S3 y Azure Blob quedan afuera, aunque el diseño no debería
  cerrarles la puerta para siempre.

## Requerimientos funcionales (borrador)

Están escritos como los dijo la persona que pidió la herramienta. Ninguno está en
forma Dado/Cuando/Entonces, ninguno tiene VC, y varios tienen más de un
comportamiento adentro. Eso hay que arreglarlo.

- **FR-a** — El usuario le pasa un patrón y una ubicación de GCS, y la herramienta
  busca el patrón dentro de los objetos que haya ahí.
- **FR-b** — Se tiene que poder buscar sobre todo un bucket o sobre un prefijo
  dentro del bucket.
- **FR-c** — La salida tiene que dejar claro en qué objeto apareció el match. Si se
  puede mostrar también la línea, mejor.
- **FR-d** — Debería poder buscar sin distinguir mayúsculas de minúsculas, como
  `grep -i`.
- **FR-e** — Si no encontró nada, la herramienta tiene que avisarlo de alguna forma
  que un script pueda detectar.
- **FR-f** — Si algún objeto no se puede leer (permisos, o está corrupto), que no se
  caiga toda la corrida por eso.
- **FR-g** — El usuario tiene que poder darse cuenta de que está progresando cuando
  hay muchos objetos, porque si no parece colgado.

## Reglas de negocio candidatas

Son candidatas: hay que decidir cuáles entran, con qué valores, y por qué. Las que
entren necesitan fundamento y excepciones.

- **BR-a?** — La herramienta nunca escribe en GCS. Solo lectura, siempre.
- **BR-b?** — La herramienta no amplía el acceso más allá de las credenciales de
  quien la invoca. Si el usuario no puede leer un bucket, `gcsgrep` tampoco.
- **BR-c?** — Debería haber algún límite para no escanear accidentalmente un bucket
  de varios terabytes. ¿Cantidad de objetos? ¿Bytes totales? ¿Un `--max` explícito?
  ¿Pedir confirmación? No está decidido.
- **BR-d?** — Los objetos que no son de texto se saltean en vez de imprimir basura
  binaria por pantalla. Falta definir cómo se decide si algo "es texto".

## Requerimientos no funcionales

<!-- Sin definir. Hay que completarlos con umbrales medibles: qué métrica,
     qué valor, bajo qué condición de carga. "Rápido" no es un NFR. -->

- **NFR-a** — Rendimiento: _pendiente_
- **NFR-b** — Uso de memoria con objetos grandes: _pendiente_
- **NFR-c** — Comportamiento ante fallos de red: _pendiente_

## Preguntas abiertas

Ninguna de estas está decidida. Cada una cambia el alcance, y si no las resolvés,
las va a resolver el agente por vos.

1. **¿Qué sabor de expresiones regulares?** ¿Solo búsqueda literal? ¿Regex básica?
   ¿La sintaxis completa del lenguaje que elijas? ¿Se puede elegir con un flag?
2. **¿Cómo se autentica?** ¿Application Default Credentials? ¿Un archivo de service
   account por variable de entorno? ¿Las dos? ¿Qué pasa si no hay credenciales?
3. **¿Cómo se escribe la ubicación?** ¿Solo `gs://bucket/prefijo`? ¿Se acepta
   `bucket/prefijo` sin esquema? ¿Qué significa exactamente un prefijo que no
   termina en `/`?
4. **¿Qué flags de `grep` se soportan en la v1?** `-i`, `-n`, `-l`, `-c`, `-v`,
   `-r`, `--include`… ¿Cuáles entran ahora y cuáles son iteraciones posteriores?
5. **¿Qué se hace con los binarios y con los `.gz`?** ¿Se saltean? ¿Se
   descomprimen al vuelo? ¿Cómo se detecta cada caso?
6. **¿Qué guardrails de costo?** Leer objetos de GCS se cobra. ¿Hay que avisar
   antes de una corrida cara? ¿Hay un tope por defecto?
7. **¿Cómo es exactamente la salida?** ¿Formato `objeto:línea:texto` como `grep`?
   ¿Hay un modo legible para máquina (JSON)? ¿Colores?
8. **¿Qué exit codes?** `grep` usa 0 si hubo match, 1 si no hubo, y 2 si hubo error.
   ¿Copiamos esa convención? ¿Qué cuenta como error?
9. **¿Concurrencia?** Leer objetos de a uno va a ser lento. ¿Cuántos en paralelo?
   ¿Es configurable? ¿Cómo afecta al orden de la salida?
10. **¿Qué pasa si un objeto cambia mientras lo estás leyendo?** ¿Importa?

## Cómo seguir

Esto es tu **base context**, no tu spec. El camino es:

1. Refinar este documento con el agente hasta que no queden preguntas abiertas.
2. Convertirlo en una spec: propósito, alcance, actores, FRs atómicos en
   Dado/Cuando/Entonces, BRs con fundamento, NFRs con umbral, y **un VC por cada
   FR y BR**.
3. Pasar la spec por una revisión que la habilite.
4. Partirla en un plan de iteraciones.
5. Implementar y verificar la Iteración 1.

La consigna completa está en [`enunciado.md`](./enunciado.md). Leela antes de empezar,
no después.

Si querés ver cómo se ven esos artefactos terminados, mirá el ejemplo guiado en
[`../ejemplo-guiado/`](../ejemplo-guiado/) — es otro proyecto (un CLI de tareas),
resuelto de punta a punta, justamente para que puedas usarlo de referencia sin que
te resuelva esta tarea.
