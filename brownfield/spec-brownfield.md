# Spec — `new-ssh-window`: cliente SSH nativo en `tmux` (solo Linux)

> Spec **brownfield**, construida sobre [`notas-exploracion.md`](./notas-exploracion.md).
> **No se implementa nada**: esta spec es el contrato desde el cual otro equipo podría
> construir sin hablar con nosotros y sin romper los builds de macOS y BSD.

**Repo:** [`tmux/tmux`](https://github.com/tmux/tmux) · commit base **`5a820e63`** (2026-09-30)

## Propósito

Agregar un comando `new-ssh-window` que abre una ventana cuyo pane es una sesión SSH remota,
hablada por un cliente **nativo** (`libssh`), sin invocar nunca el binario `ssh`. Existe
solo en Linux y solo si se compiló con `--enable-native-ssh`.

## El límite solo-Linux

Es el eje de la spec. Se aplica en **tres capas**, de modo que ninguna depende de la otra:

| Capa | Mecanismo | Resultado fuera de Linux |
|---|---|---|
| **Configure** | `--enable-native-ssh`, **opt-in** (apagado por default). Si `PLATFORM` (`configure.ac:1006-1118`) no es `linux`, `configure` **falla con un mensaje claro** | el flag no se puede activar por accidente |
| **Make** | `AM_CONDITIONAL(ENABLE_NATIVE_SSH, …)`; las fuentes nuevas se agregan solo en esa rama (patrón de `ENABLE_SIXEL`, `Makefile.am:252-255`) | las fuentes nuevas no se compilan ni se enlazan |
| **Código** | `AC_DEFINE(ENABLE_NATIVE_SSH)`; todo código nuevo en archivos existentes va dentro de `#ifdef ENABLE_NATIVE_SSH` | el binario es el mismo que hoy |

Un build sin el flag es **idéntico** al actual, en cualquier plataforma, incluido Linux.
`libssh` **nunca** es dependencia obligatoria.

## Alcance

### Dentro

| Archivo | Cambio |
|---|---|
| `cmd-new-ssh-window.c` (**nuevo**) | `cmd_new_ssh_window_entry`: parsea argumentos, arma el `spawn_context` y llama a `spawn_window()` |
| `ssh-client.c` (**nuevo**) | El cliente nativo: conectar, verificar host, autenticar, abrir canal con pty, copiar datos. Una sola entrada pública, `ssh_client_run()` |
| `spawn.c` | **Un** bloque en el camino del hijo de `spawn_pane()`, después de `environ_push` (`spawn.c:544`) y antes de los `exec` (`:552-574`) |
| `tmux.h` | Flag `SPAWN_SSH 0x2000` (el `0x1000` ya es `SPAWN_FLOATOVERZOOM`, `tmux.h:2531`) y prototipo de `ssh_client_run()` |
| `cmd.c` | `extern` + entrada en `cmd_table[]` (`cmd.c:123`), ambos bajo `#ifdef` |
| `configure.ac` | Flag, detección de `libssh` con `pkg-config`, guarda de plataforma, línea en el resumen final |
| `Makefile.am` | `if ENABLE_NATIVE_SSH` agregando las dos fuentes nuevas |
| `tmux.1` | Entrada del comando |
| `regress/new-ssh-window.sh` (**nuevo**) | Prueba end-to-end con un `sshd` local de prueba; se saltea si el feature no está compilado |
| `.github/workflows/regress.yml` | Entrada **nueva** de matriz solo Linux con `--enable-native-ssh`, e instalar `libssh-dev` |

### Fuera de alcance

Explícito, por path:

- **`window.c`, `server.c`, `server-fn.c`, `window-*.c`** — el modelo de pane (fd + bufferevent
  + pid) no cambia. Ese es el motivo de la opción A (ver "Preguntas abiertas").
- **`job.c`, `input*.c`, `screen*.c`, `tty*.c`, `layout*.c`** — sin cambios.
- **`format.c`, `window-tree.c`, `osdep-*.c`, `cmd-find.c`** — `pane_current_command`,
  `pane_current_path` y búsqueda por tty siguen igual; no se parchan.
- **`cmd-split-window.c`** — **no hay variante con split** (`split-window` tiene 424 líneas
  contra 196 de `new-window` por el manejo de layout). Es una spec posterior.
- **`cmd-respawn-pane.c`** — `respawn-pane` sobre un pane SSH **no se soporta** (ver
  "Limitaciones conocidas").
- **`options-table.c`, `key-bindings.c`** — sin opciones nuevas ni binding por default.
- **`compat/`** — no se agrega nada: esto es una feature, no un reemplazo de portabilidad.
- **El job de macOS de `regress.yml`** — no se toca; es la prueba del invariante INV-2.
- **Autenticación por password o keyboard-interactive, TOFU de host keys, agent forwarding,
  port forwarding, SFTP, ProxyJump, `~/.ssh/config`** — fuera de alcance.

## Invariantes

Cosas que tienen que seguir siendo verdad **después** del cambio:

| # | Invariante | Cómo se comprueba |
|---|---|---|
| **INV-1** | Sin `--enable-native-ssh`, el binario es el de hoy | `ldd tmux \| grep -ci ssh` da `0`; `tmux list-commands` no lista `new-ssh-window`; las dos fuentes nuevas no se compilan |
| **INV-2** | Los builds no-Linux siguen compilando | el job `macos-26-arm64` de `regress.yml` (sin `libssh` instalado) queda verde; `./configure --enable-native-ssh` en no-Linux **sale con error en configure**, no en compilación |
| **INV-3** | Los comandos existentes no cambian | `cmd_table[]` pasa de **92** a **93** entradas con el flag y queda en **92** sin él; `regress/` (**172** scripts) sigue en verde |
| **INV-4** | El modelo de PTY/panes no cambia | `git diff --stat` **no incluye** `window.c`, `server.c`, `server-fn.c`, `job.c`, `format.c`, `input.c`, `cmd-find.c`, `osdep-*.c`; `git diff --numstat spawn.c` muestra **0 líneas eliminadas** |
| **INV-5** | No se invoca el binario `ssh` | durante una sesión, `pgrep -x ssh` no devuelve nada; el pane SSH nunca llama a `execvp`/`execl` (el hijo sale por `ssh_client_run`) |
| **INV-6** | El server sigue siendo de un solo hilo y no se bloquea | `ls /proc/<pid-server>/task \| wc -l` es `1` antes y después; ver NFR-1 |
| **INV-7** | El bit nuevo no pisa uno existente | `grep -c 'define SPAWN_.* 0x2000' tmux.h` devuelve `1`, y ninguna otra constante `SPAWN_*` repite un valor (`grep -o 'SPAWN_[A-Z]* 0x[0-9a-f]*' tmux.h \| awk '{print $2}' \| sort \| uniq -d` no imprime nada) |

## Línea de base de regresión

Medida **antes** de tocar una línea, sobre `5a820e63` y **sin** el flag:

```bash
sh autogen.sh && ./configure --enable-utf8proc && make
(cd regress && make)         # 172 scripts: todos en verde
./tmux -f/dev/null list-commands | wc -l      # número a registrar como base
```

Valores ya medidos por lectura estática: **92** entradas en `cmd_table[]` y **172** scripts
en `regress/`. El resultado de la corrida (tiempo y verde/rojo) lo registra quien
implemente, antes de empezar. Al cerrar cada iteración, la misma corrida **sin** el flag
tiene que dar lo mismo. Cualquier diferencia es una regresión, no un efecto colateral.

## Requerimientos

### FR-1 · El comando existe solo cuando el feature está compilado

**Dado** un build con `--enable-native-ssh` en Linux,
**cuando** se ejecuta `tmux new-ssh-window [-dP] [-F format] [-n window-name] [-p port]
[-i identity-file] [-t target-window] [usuario@]host`,
**entonces** el comando se parsea y aparece en `list-commands` con alias `sshw`.

**VC-1:** `tmux list-commands | grep -c '^new-ssh-window'` devuelve `1` con el flag y `0`
sin él. El alias `sshw` no existe en ningún otro comando (`grep -n '\.alias = "sshw"' cmd-*.c`
devuelve una sola línea).

### FR-2 · La ventana se crea por el camino de spawn existente

**Dado** el comando de FR-1,
**cuando** se ejecuta,
**entonces** se llama a `spawn_window()` con `SPAWN_SSH` y el destino en `argv`; el pane se
crea con **`fdforkpty`** (`spawn.c:478`) igual que cualquier otro, y el hijo, en vez de
`exec`, ejecuta `ssh_client_run()`.

**VC-2:** con el feature activo, `tmux new-ssh-window -d host` crea una ventana cuyo pane
tiene `#{pane_pid}` distinto de cero y `#{pane_tty}` no vacío (hereda el modelo pty).

### FR-3 · El nombre de la ventana no es `tmux: server`

**Dado** que el hijo es un fork del server, `osdep_get_name` (`osdep-linux.c:30`) leería
`/proc/<pgrp>/cmdline` y devolvería el título del server,
**cuando** no se pasa `-n`,
**entonces** el comando usa el **destino** como nombre (pasar `sc.name` apaga
`automatic-rename`, `spawn.c:216-224`).

**VC-3:** `tmux new-ssh-window -d -P -F '#{window_name}' u@h` imprime `u@h`.

### FR-4 · Verificación estricta del host

**Dado** un destino cuyo host key no está en `~/.ssh/known_hosts`, o cambió,
**cuando** se intenta conectar,
**entonces** la conexión se **rechaza**, se imprime el motivo en el pane y el hijo sale con
código distinto de cero. No se pregunta, no se agrega la clave automáticamente.

**VC-4:** contra el `sshd` de prueba con una `known_hosts` vacía, el pane termina con
estado `1` y su pantalla contiene el texto del rechazo (`capture-pane -p`).

### FR-5 · Autenticación por agent y por clave

**Dado** un host conocido,
**cuando** se conecta,
**entonces** se intenta primero el **agent** (`SSH_AUTH_SOCK` del entorno de la sesión;
llega bien porque `environ_push` corre antes, `spawn.c:544`), y después el `-i` o las claves
por defecto de `~/.ssh`. Una clave con passphrase se pide **en el tty del pane**. No se
ofrece password.

**VC-5:** con una clave sin passphrase autorizada en el `sshd` de prueba y **sin** agent, se
llega a un prompt remoto; con `SSH_AUTH_SOCK` apuntando a un agent con la clave, también.
**VC-6:** con un servidor que solo acepta password, el pane termina con estado `1` y un
mensaje de método no soportado.

### FR-6 · Sesión interactiva con pty remoto

**Dado** una conexión autenticada,
**cuando** se abre la sesión,
**entonces** se pide un pty remoto con el `TERM` del pane y el tamaño actual, se abre una
shell y los bytes se copian en ambos sentidos.

**VC-7:** `send-keys 'echo ok' Enter` seguido de `capture-pane -p` muestra `ok` producido
por el host remoto.

### FR-7 · El resize llega al remoto

**Dado** una sesión abierta,
**cuando** cambia el tamaño del pane (el hijo recibe `SIGWINCH` porque `fdforkpty` lo deja
como líder de sesión),
**entonces** se actualiza el tamaño del pty remoto.

**VC-8:** tras `resize-window -x 100 -y 30`, `stty size` remoto devuelve `30 100`.

### FR-8 · El fin de la sesión usa el mecanismo normal de `tmux`

**Dado** que el hijo es un proceso real,
**cuando** la sesión remota termina (o la conexión se corta),
**entonces** el hijo sale con el **código de salida remoto** (o `1` si se cortó) y el pane
sigue el camino habitual: `SIGCHLD` → `server_child_exited` (`server.c:491`) → `PANE_EXITED`.
No se agrega ninguna ruta de destrucción nueva.

**VC-9:** `exit 3` en la shell remota deja `#{pane_dead_status}` en `3` con
`remain-on-exit on`. Cortar el `sshd` deja el pane muerto con estado `1` y el server vivo.

### FR-9 · Fallas de conexión no cuelgan el server

**Dado** un host inexistente, inalcanzable (tráfico descartado) o con puerto cerrado,
**cuando** se intenta conectar,
**entonces** el error se imprime en el pane y el hijo sale con `1`.

**VC-10:** con un destino que descarta paquetes, `tmux display-message -p ok` desde otro
cliente responde en **menos de 1 s** mientras la conexión está pendiente, y el pane termina
con estado `1` antes de **20 s**.

### BR-1 · No se registran secretos

Passphrases, contenido de claves y respuestas del agent **nunca** van a `log_debug`.

**VC-11:** con `tmux -vv`, tras una sesión con passphrase, `grep -c` de la passphrase de
prueba en `tmux-server-*.log` devuelve `0`.

### BR-2 · Sin binding por default

El comando no se asocia a ninguna tecla.

**VC-12:** `tmux list-keys | grep -c new-ssh-window` devuelve `0` recién compilado.

### NFR-1 · Latencia del server

El server no hace I/O de red: toda la conexión ocurre en el hijo. **VC-10** mide esto
(< 1 s de respuesta con una conexión colgada). **INV-6** verifica que no se agregaron hilos.

### NFR-2 · Documentación

El comando figura en `tmux.1` con su sinopsis y las limitaciones.

**VC-13:** `grep -c 'new-ssh-window' tmux.1` devuelve al menos `1`.

## Plan de iteraciones

Cada iteración termina con la línea de base **sin** el flag igual a la inicial.

| Iteración | Alcance | Cierra |
|---|---|---|
| **1** | Guardas de build: `configure.ac`, `Makefile.am`, nada de código | INV-1, INV-2, INV-7 |
| **2** | Comando + `SPAWN_SSH` + bloque en `spawn.c` + conexión, host key y auth | FR-1…FR-5, INV-3, INV-4, INV-5, VC-1…VC-6 |
| **3** | Sesión interactiva, resize, fin de sesión y fallas | FR-6…FR-9, BR-1, BR-2, VC-7…VC-12, INV-6 |
| **4** | `tmux.1`, `regress/new-ssh-window.sh` y CI | NFR-2, VC-13 |

La Iteración 1 es el camino más angosto que se puede verificar solo: no hay código C nuevo
y prueba el límite solo-Linux completo.

## Preguntas abiertas — resueltas

| Pregunta | Decisión | Por qué |
|---|---|---|
| ¿Entrada nueva en la tabla de comandos? | **Sí**: `new-ssh-window`, alias `sshw` | no hay campo "tipo de pane" en `spawn_context` (`tmux.h:2500`); un comando nuevo no toca ninguno existente |
| ¿Ventana o split? | **Ventana** | reusa `spawn_window` y evita el manejo de layout de `split-window`; el split es una spec posterior |
| ¿Dónde engancha en el spawn? | **En el hijo, tras `fdforkpty` (`spawn.c:478`) y `environ_push` (`:544`)**, donde hoy están los `exec` | conserva `pid`, `fd`, `tty`, `SIGCHLD` y `TIOCSWINSZ` intactos; el riesgo de `window.c:612-622` (`fatal` si el `ioctl` falla) desaparece porque el fd sigue siendo un pty |
| ¿`libssh` u OpenSSH? | **`libssh`** | OpenSSH se usa como binario, que es justo lo que el enunciado excluye |
| ¿Cómo se integra con el event loop? | **No se integra**: el cliente corre en el hijo con su propio bucle | el server es de un solo hilo con libevent (hallazgo 6): una conexión bloqueante dentro congelaría a todos |
| ¿Auth por claves o por agent? | **Ambas**; sin password | cubre el uso real y evita manejar secretos en el pane |
| ¿Host desconocido? | **Se rechaza** | evita TOFU silencioso; el mensaje indica cómo agregar la clave |
| ¿Guarda de build? | **Opt-in** `--enable-native-ssh`, solo Linux, con `AM_CONDITIONAL` y `#ifdef` | ya hay dos precedentes: `--enable-systemd` y `ENABLE_SIXEL` (hallazgo 8) |

## Limitaciones conocidas

- **`respawn-pane` sobre un pane SSH** relanza `argv` como un comando común: como `argv[0]`
  es el destino, ejecutaría `$SHELL -c <destino>` y fallaría con "command not found". Es
  inofensivo, pero confuso. Soportarlo exige guardar en el pane que es SSH
  (`struct window_pane`, `tmux.h:1306`), fuera del alcance.
- `pane_current_command` / `pane_current_path` no reflejan al host remoto: muestran el
  proceso local (hallazgo 4).
- **Dependencia de la versión de `libssh`**: los nombres exactos de las funciones y la
  versión mínima **no se verificaron**; los fija quien implemente (nota abierta de las
  notas de exploración).
