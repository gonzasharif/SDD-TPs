# Tarea Lección 1 — `gcsgrep`

**En equipo de trabajo.** Se entrega antes de la Lección 2 · [cómo se entrega](../../entrega.md)

## Qué hay que hacer

Llevá la experiencia de `grep` a Google Cloud Storage: buscar dentro del **contenido**
de objetos remotos sin bajarlos primero.

```
gcsgrep "timeout" gs://logs/
```

Un CLI que haga grep sobre los objetos de texto de un bucket o prefijo, y muestre qué
objeto matcheó y dónde.

## Lo que te damos

[`gcsgrep-requirements.md`](./gcsgrep-requirements.md) — un borrador **deliberadamente
subespecificado**: FRs flojos, BRs candidatos, NFRs en blanco y una lista de preguntas
abiertas al final.

Dice qué queremos, no cómo. **Convertir ese borrador vago en un contrato verificable es
el ejercicio.** Ese documento es tu base context: refinalo, no lo implementes tal cual.

## Cómo encararlo

### 1 · Refiná el base context

Usá el agente para resolver las preguntas abiertas. Las decisiones son tuyas:

- ¿qué sabor de regex?
- ¿auth por ADC o por key de service account?
- ¿cómo se direcciona `gs://`?
- ¿qué flags de `grep` soportás?
- ¿qué hacés con binarios y `.gz`?
- ¿qué guardrails de costo?

### 2 · Corré el pipeline en serio

**Especificar → Revisar → Planificar → Implementar → Verificar**, con umbrales reales
en los NFRs y una tabla de cobertura de VCs.

**v1 sugerida, discutila:** búsqueda literal o regex básica sobre objetos de texto bajo
un prefijo de un bucket, con `-i`, `-n`, salida con nombre de objeto, lectura por
streaming, solo lectura, y exit codes estilo `grep`. Todo lo demás va a iteraciones
posteriores del plan, no a la Iteración 1.

## Qué se entrega

| Artefacto | Qué tiene que contener |
|---|---|
| **Spec revisada** | ≥ 3 FRs con sus VCs, umbrales reales en los NFRs, preguntas abiertas resueltas |
| **`gcsgrep-plan.md`** | ≥ 2 iteraciones; el alcance diferido vive acá, no en la spec |
| **Código de la Iteración 1** | Corre una búsqueda real y sus chequeos de verificación pasan |

## Restricciones

- **Solo lectura.** La herramienta nunca amplía el acceso más allá de las credenciales
  de quien la invoca.
- **No escanees un bucket enorme sin un guardrail de costo.**
- Vas a necesitar una cuenta de Google Cloud con un bucket de prueba y `gcloud`
  configurado, o credenciales de una service account.

## Antes de entregar

Mirá [`../ejemplo-guiado/README.md`](../ejemplo-guiado/README.md): el mismo pipeline
resuelto de punta a punta sobre otro proyecto (`taskcli`). Te muestra la forma sin
resolverte la tarea.
