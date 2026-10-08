# Spec — `new-ssh-window`: cliente SSH nativo en `tmux` (solo Linux)

> Spec **brownfield** construida sobre [`notas-exploracion.md`](./notas-exploracion.md).
> **No se implementa nada**: es el contrato desde el cual otro equipo puede construir sin
> hablar con nosotros y sin romper los builds de macOS y BSD.

**Repo:** [`tmux/tmux`](https://github.com/tmux/tmux) · commit base **`5a820e63`** (2026-09-30)
**Entrega:** el commit al que apunta el tag `03-Agent` del repositorio donde vive esta spec (el repo del TP, no `tmux/tmux`).

## Propósito

Agregar un comando `new-ssh-window` que abre una ventana cuyo pane es una sesión SSH remota,
hablada por un cliente SSH integrado a `tmux`, sin invocar nunca el binario `ssh`.

## El límite solo-Linux

Es el eje de la spec. Se aplica en **tres capas**, de modo que ninguna depende de la otra:

| Capa | Mecanismo | Resultado fuera de Linux | FR |
|---|---|---|---|
| **Configure** | `--enable-native-ssh`, **opt-in** (apagado por default). Si `PLATFORM` (`configure.ac:1006-1118`) no es `linux`, `configure` **falla con un mensaje claro** | el flag no se puede activar por accidente | FR-1 … FR-7 |
| **Make** | `AM_CONDITIONAL(ENABLE_NATIVE_SSH, …)`; las fuentes nuevas se agregan solo en esa rama (patrón de `ENABLE_SIXEL`, `Makefile.am:253-255`) | las fuentes nuevas no se compilan ni se enlazan | FR-8, FR-9 |
| **Código** | `AC_DEFINE(ENABLE_NATIVE_SSH)`; todo código nuevo en archivos existentes va dentro de `#ifdef ENABLE_NATIVE_SSH` | el binario es el mismo que hoy | FR-10, FR-30 |

Un build sin el flag es **idéntico** al actual, en cualquier plataforma, incluido Linux.
`libssh` **nunca** es dependencia obligatoria.

## Glosario

Un término por concepto; el resto de la spec lo usa sin variantes.

| Término | Significado |
|---|---|
| **destino** | El argumento posicional `[usuario@]host` |
| **destino válido** | `[usuario@]host` con usuario y host no vacíos, sin `@` adicionales y sin espacios ni tabs |
| **host** | La parte del destino posterior a `@` (o el destino entero si no hay `@`) |
| **usuario** | La parte anterior a `@`; si falta, el usuario del proceso del server |
| **ventana** / **pane** | La ventana que crea el comando y su único pane |
| **error de uso** | Rechazo del comando antes de crear la ventana (FR-12, FR-13, FR-21 … FR-28, FR-63, FR-73). Sale por stderr del cliente `tmux` con estado `1` y **no** crea ventana |
| **falla del cliente** | El hijo del pane termina por una condición de FR-34, FR-36, FR-37, FR-45 … FR-49, FR-58, FR-60 … FR-62, FR-69, FR-74, FR-75. Imprime una línea `new-ssh-window: …` en el pane y sale con `1` (BR-3) |
| **host conocido** | Destino cuya host key figura en `$HOME/.ssh/known_hosts`. La entrada usa el formato de OpenSSH: `<host>` si el puerto es 22 y `[<host>]:<puerto>` en otro caso. En los mensajes `<host>` va **sin** puerto |
| **clave por defecto utilizable** | `$HOME/.ssh/id_ed25519` o `$HOME/.ssh/id_rsa` que existe, el proceso puede leer y es una clave privada |
| **sesión de `tmux`** | La sesión del server (`new-session`) en cuyo entorno vive `SSH_AUTH_SOCK` |
| **sesión SSH** | La conexión autenticada con el canal y el pty remotos |
| **cliente `tmux`** | El proceso `tmux` que invoca el comando (el que imprime el error de uso por stderr) |
| **cliente SSH** | El código de `ssh-client.c` que corre en el hijo del pane |
| **servidor `tmux`** | El proceso del server de `tmux` (en el resto de la spec, "el server") |
| **`sshd`** | El servidor SSH remoto |
| **mensaje** | La línea `new-ssh-window: …` de una falla del cliente, en el pane |
| **`<ruta>`** | Ruta del archivo nombrado, tal como se escribió (los archivos que nombra la spec son siempre absolutos) |
| **se autentica** | `$SSHD_LOG` contiene `Accepted publickey for <usuario>` |
| **llega a `sshd`** | `$SSHD_LOG` contiene `Connection from 127.0.0.1 port <n> on 127.0.0.1 port $PORT` |
| **llega al prompt remoto** | Tras `send-keys -t $P 'echo R-$((6*7))' Enter` y `esperar` `R-42`, `capture-pane -p -t $P` contiene la línea `R-42` |

## Alcance

### Dentro

| Archivo | Cambio |
|---|---|
| `cmd-new-ssh-window.c` (**nuevo**) | `cmd_new_ssh_window_entry`: parsea y valida argumentos, arma el `spawn_context` y llama a `spawn_window()` |
| `ssh-client.c` (**nuevo**) | El cliente nativo: conectar, verificar host, autenticar, abrir canal con pty, copiar datos. Una sola entrada pública, `ssh_client_run()` |
| `spawn.c` | **Un** bloque en el camino del hijo de `spawn_pane()`, después de `environ_push` (`spawn.c:544`) y antes de los `exec` (`:552-574`) |
| `tmux.h` | Flag `SPAWN_SSH 0x2000` (el `0x1000` ya es `SPAWN_FLOATOVERZOOM`, `tmux.h:2531`) y prototipo de `ssh_client_run()` |
| `cmd.c` | `extern` + entrada en `cmd_table[]` (`cmd.c:123`), ambos bajo `#ifdef` |
| `configure.ac` | Flag, detección de `libssh >= 0.9.0` con `pkg-config`, guarda de plataforma, línea en el resumen final |
| `Makefile.am` | `if ENABLE_NATIVE_SSH` agregando las dos fuentes nuevas |
| `tmux.1` | Entrada del comando |
| `regress/new-ssh-window.sh` (**nuevo**) | Prueba end-to-end con el harness descripto abajo: los VCs de FR-10 en adelante, VC-INV3.1, la mitad con flag de VC-INV3.3, VC-INV4.2, VC-INV5.1 … VC-INV6.2, VC-BR1.1, VC-BR2.1, VC-BR3.x, VC-NFR1.1 y VC-NFR3.1. Se saltea si `cmds` no lista el comando (build sin flag); si el comando está listado y falta `sshd`, **falla** en vez de saltear |
| `regress/native-ssh-guard.sh` (**nuevo**) | Prueba de la guarda, corre en **todas** las plataformas (lo levanta `TESTS != echo *.sh`, `regress/Makefile`). Elige qué VC ejecutar con `uname -s` y con `cmds`: fuera de Linux, VC-INV2.1 y VC-5.1; en Linux, si el comando no está listado (build sin flag), VC-INV1.1 y la mitad sin flag de VC-INV3.3; si está listado, no hace nada (los VCs con flag son de `new-ssh-window.sh`) |
| `regress/native-ssh-build.sh` (**nuevo**) | Solo Linux (fuera de Linux sale con `0` sin hacer nada). Corre cada VC de `configure` y de build (VC-1.1 … VC-4.1, VC-6.1 … VC-9.1) en una copia limpia (harness), y los VCs estructurales sobre el árbol del repo (VC-INV4.1, VC-INV4.3, VC-INV7.1, VC-NFR2.1, VC-BR4.1) |
| `regress/list-commands.base` (**nuevo**) | La salida de `list-commands` del build sin flag de `5a820e63` (92 líneas), **versionada**: la leen VC-INV1.1 y VC-INV3.1 |
| `.github/workflows/regress.yml` | Entrada **nueva** de matriz solo Linux con `--enable-native-ssh` y **sin** `--enable-asan`; con `fetch-depth: 0` en el checkout compartido; en el step de dependencias de **Linux**, `libssh-dev`, `openssh-server`, `openssh-client`, `cpio`, `netcat-openbsd`, `procps`, `strace`, `man-db` y `bsdextrautils` |

Los tres scripts de `regress/` **crecen por iteración**: al cierre de la iteración *k* contienen solo los VCs de los requerimientos que cierran en las iteraciones 1 … *k* (Plan), de modo que VC-INV3.2 vale al cierre de cada una. Las listas de arriba son las del cierre de la iteración 5.

### Fuera de alcance

Explícito, por path:

- **`window.c`, `server.c`, `server-fn.c`, `window-*.c`** — el modelo de pane (fd + bufferevent
  + pid) no cambia. Ese es el motivo de enganchar en el hijo (decisión 2).
- **`job.c`, `input*.c`, `screen*.c`, `tty*.c`, `layout*.c`** — sin cambios.
- **`format.c`, `window-tree.c`, `osdep-*.c`, `cmd-find.c`** — `pane_current_command`,
  `pane_current_path` y búsqueda por tty siguen igual; no se parchan.
- **`cmd-split-window.c`** — **no hay variante con split** (`split-window` tiene 424 líneas
  contra 196 de `new-window` por el manejo de layout).
- **`cmd-respawn-pane.c`** — `respawn-pane` sobre un pane SSH **no se soporta** (ver
  "Limitaciones conocidas").
- **`options-table.c`, `key-bindings.c`** — sin opciones nuevas ni binding por default.
- **`compat/`** — no se agrega nada: esto es una feature, no un reemplazo de portabilidad.
- **El job de macOS de `regress.yml`** — no se toca: ni su `configure`, ni sus dependencias (el `actions/checkout` compartido por toda la matriz pasa a `fetch-depth: 0`, que no cambia lo que compila).
  Fuera de Linux las fuentes nuevas no se compilan y el comando no existe en el binario.
- **Autenticación por password o keyboard-interactive, TOFU de host keys, agent forwarding,
  port forwarding, SFTP, ProxyJump, `~/.ssh/config`, `known_hosts` global, líneas
  malformadas de `known_hosts`, entradas hasheadas (`|1|…`) y hosts registrados con otro tipo de clave, timeouts de la autenticación o del prompt de passphrase (el plazo de 15 s de FR-62 y FR-69 cubre solo la conexión y el intercambio de claves), shell remota terminada por una señal, resolver que no responde (`EAI_AGAIN`) y errores de `connect` distintos de rechazo y plazo vencido** — fuera de alcance.

## Invariantes

Cosas que tienen que seguir siendo verdad **después** del cambio. Cada una tiene sus VCs en
la sección "VCs de invariantes".

| # | Invariante | VC |
|---|---|---|
| **INV-1** | Sin `--enable-native-ssh`, el binario es el de hoy | VC-INV1.1 |
| **INV-2** | Los builds no-Linux siguen compilando, y el comando nuevo no está en su binario | VC-INV2.1 |
| **INV-3** | Los comandos existentes no cambian (salvo que, con el flag, la abreviatura `new-s` pasa a ser ambigua) | VC-INV3.1, VC-INV3.2, VC-INV3.3 |
| **INV-4** | El modelo de PTY/panes no cambia, y el cambio se limita a los paths de "Dentro de alcance" | VC-INV4.1, VC-INV4.2, VC-INV4.3 |
| **INV-5** | No se invoca el binario `ssh` (ni ningún otro) para el pane SSH | VC-INV5.1, VC-INV5.2 |
| **INV-6** | El server no gana hilos | VC-INV6.1, VC-INV6.2 |
| **INV-7** | El bit `SPAWN_SSH` no pisa uno existente | VC-INV7.1 |

## Línea de base de regresión

Medida **antes** de tocar una línea, sobre `5a820e63` y **sin** el flag:

```bash
sh autogen.sh && ./configure --enable-utf8proc && make
cmds > regress/list-commands.base   # 92 líneas (`cmds`: ver Harness)
(cd regress && make)                                    # 172 scripts: 171 PASS, 1 FAIL previo
```

**Medido** (Ubuntu 24.04, `make -j8` en `regress/`): el build compila (`make` sale con `0`),
`list-commands` imprime **92** líneas (coincide con las 92 entradas de `cmd_table[]`,
`cmd.c:124-215`) y `regress/` contiene **172** scripts `*.sh`, de los cuales **171 dan `PASS`
y 1 da `FAIL` antes de tocar nada**: `prompt-words-history.sh` (`prompt result is 'show-r',
expected 'history-command'`, determinístico en ese entorno). La línea de base es entonces
"esos 171 en `PASS`": un script que falla antes del cambio no cuenta como regresión, y quien
implemente mide la suya en su entorno y la guarda junto con `regress/list-commands.base`
(`list-commands` sin flag, 92 líneas): VC-INV1.1 y VC-INV3.1 comparan contra ese archivo.
Al cerrar cada iteración, la misma corrida **sin** el flag tiene que dar lo mismo.

Con el cambio completo, `regress/` tiene **175** scripts (los 172 más los tres nuevos). Un
script que estaba en `PASS` en la línea de base y se pone rojo es una regresión: no se "actualiza" para que pase.

## Harness de prueba

Lo usan los VCs de `regress/`. No hay un helper compartido (un `.sh` extra correría como prueba): cada script de `regress/` lleva inline las partes del harness que usa; el feature nunca lo invoca. Las
pruebas de `regress/new-ssh-window.sh` corren como usuario **no root** (los VCs de permisos de archivo lo necesitan).

- **`sshd` de prueba:** `sshd -D -e -f <conf>` en `127.0.0.1`, puerto alto `$PORT`, host key
  propia, `LogLevel VERBOSE`, log en `$SSHD_LOG`, pid en `$SSHD_PID`. Acepta solo `publickey`; VC-46.1 usa su propia configuración (ver ahí).
- **`HOME` temporal** `$H`, con `$H/.ssh/known_hosts` y las claves que pida cada VC.
- **Entorno:** `regress/Makefile` ejecuta cada script con `env -i`; `regress/new-ssh-window.sh` define `USER=$(id -un)`, fija `PATH=/usr/sbin:/usr/bin:/sbin:/bin`; `native-ssh-guard.sh` y `native-ssh-build.sh` fijan además `/usr/local/bin:/opt/homebrew/bin` y conservan `PKG_CONFIG_PATH` (en macOS `autoconf`, `automake`, `bison` y `pkg-config` viven ahí) y usa `$(command -v sshd)` por ruta.
- **Configuración de `sshd`:** `Port $PORT`, `ListenAddress 127.0.0.1`, `HostKey $H/host_key`, `AuthorizedKeysFile $H/authorized_keys`, `PidFile $H/sshd.pid`, `UsePAM no`, `StrictModes no`, `PasswordAuthentication no`; `$SSHD_PID` es el pid del `sshd -D`.
- **Claves:** crear con `ssh-keygen -q -t ed25519 -N '' -f <ruta>` (`-N '<passphrase>'` para la protegida; `-t rsa` para `id_rsa`); autorizar con `cat <ruta>.pub >> $H/authorized_keys`; agent con `eval $(ssh-agent -s)` y `ssh-add <ruta>`; **fingerprint** de una clave = `ssh-keygen -lf <ruta>.pub | awk '{print $2}'`, que aparece en `$SSHD_LOG` como `Accepted publickey for $USER … SHA256:…`.
- **`known_hosts` del `sshd` de prueba:** `ssh-keyscan -p $PORT 127.0.0.1 > $H/.ssh/known_hosts` deja la línea `[127.0.0.1]:$PORT ssh-ed25519 <clave>`; ese es el "host conocido" de todos los VCs de autenticación, sesión y falla (FR-38 … FR-59, FR-64 … FR-67, FR-70, FR-72, FR-74 y FR-75). "Otra clave registrada" (VC-36.1) es la misma línea con la clave pública de `$H/otra` (creada con `ssh-keygen -q -t ed25519 -N '' -f $H/otra`). La "clave X" (VC-65.1, VC-75.1) es `$H/x`, creada igual y **no** agregada a `authorized_keys`.
- **Passphrase:** `$PASS` = `S3cretoX` es la passphrase de la clave protegida `$H/.ssh/id_ed25519` (`ssh-keygen -q -t ed25519 -N "$PASS" -f $H/.ssh/id_ed25519`, autorizada); `mala` es la incorrecta.
- **Agent:** el server de prueba arranca con `env -u SSH_AUTH_SOCK -u SSH_AGENT_PID tmux …`: ningún VC hereda un agent del script. Solo los VCs que piden agent (VC-38.1, VC-42.1, VC-47.3, VC-65.1, VC-75.1) arrancan `eval $(ssh-agent -s)`, cargan sus claves con `ssh-add` (VC-47.3 no carga ninguna) y se lo dan al hijo con `set-environment -g SSH_AUTH_SOCK <socket>`; el resto corre sin `SSH_AUTH_SOCK` en el entorno global del server (`show-environment -g`). VC-64.1 no arranca ningún agent: apunta `SSH_AUTH_SOCK` a un archivo que no existe. El `ssh-agent` de cada VC se mata (`ssh-agent -k`) al terminar el VC.
- **Destino** `$D` = `$USER@127.0.0.1`.
- **`tmux` de prueba** (`$TEST_TMUX`, la ruta absoluta del binario bajo prueba; en este documento `tmux` abrevia `$TEST_TMUX -L vc$$`) con `-f/dev/null`, `remain-on-exit on` (para leer la pantalla y
  `#{pane_dead_status}` de un pane que ya terminó), `history-limit 250000` y el entorno de la sesión de `tmux` con `HOME=$H`.
  Por defecto el server corre **sin** `strace`. Los VCs que piden el trace (VC-33.1, VC-INV5.1, VC-INV5.2) lo lanzan
  **bajo `strace`**: `strace -f -o $TRACE -e trace=execve,connect tmux … new-session -d &` (en segundo plano: `strace -f` no vuelve mientras viva el server), esperan con `tmux has-session` cada 100 ms hasta 5 s, y antes de leer `$TRACE` hacen `tmux kill-server` y `wait`. Los VCs de tiempo
  (VC-62.1, VC-NFR1.1, VC-NFR3.1) corren siempre sin `strace`.
- **Tiempos:** `t0=$(date +%s%N)` justo antes de la acción que se mide; la condición (línea en `capture-pane -p`, o `kill -0 <pid>` que falla) se consulta cada 100 ms; `t1=$(date +%s%N)` en la primera consulta que la cumple. El tiempo medido es `t1 - t0`, con resolución de 100 ms.
- **`cmds`** (función de shell que imprime la lista de comandos): `$TEST_TMUX -L lc$$ -f/dev/null new-session -d \; list-commands`, y después `$TEST_TMUX -L lc$$ kill-server` (con server vivo: un `list-commands` suelto puede volver vacío o cortado bajo carga, como avisa `regress/list-commands.sh`). `regress/list-commands.base` se genera con la misma `cmds`. Si `cmds` tiene menos de 92 líneas, el script **falla** (nunca saltea). Quien decide si el build tiene el flag es `cmds`, y la ausencia de símbolos la confirma `nm`.
- **No root:** `regress/new-ssh-window.sh` (no los otros dos) falla con el mensaje `no se puede correr como root` si `id -u` da `0`, y lo comprueba después de decidir el salto por `cmds` (un build sin flag se saltea aun como root) (con `chmod 000` el root igual lee, y VC-27.1, VC-37.1, VC-41.1 y VC-47.2 no discriminarían).
- **Commit base `$BASE`** = `5a820e63`: el repo de la entrega tiene el árbol de `tmux` en su raíz y el historial de `tmux/tmux` hasta `$BASE`; el checkout de CI usa `fetch-depth: 0` (mientras `regress.yml` no cambia, iteraciones 1 a 4, los VCs corren en un clon completo). Si `git cat-file -e $BASE^{commit}` falla, VC-INV4.1 y VC-INV4.3 **fallan** con `falta $BASE` (no se saltean).
- **Directorio de trabajo:** `regress/Makefile` corre cada script con cwd `regress/`; todo script de prueba empieza con `cd "$(git rev-parse --show-toplevel)"` (`$ROOT`), y las rutas de los VCs (`regress/list-commands.base`, `tmux.h`, `tmux.1`, `.github/workflows/regress.yml`, `git diff … -- spawn.c cmd.c tmux.h`) son relativas a `$ROOT`.
- **Copia limpia:** desde `$ROOT`, `d=$(mktemp -d) && git ls-files -z --cached --others --exclude-standard | cpio -0pdm $d && cd $d && sh autogen.sh`: solo archivos versionados o nuevos no ignorados, sin `configure`, `Makefile` ni `*.o` de ningún build previo. Todo VC de `configure` y de build (VC-1.1 … VC-9.1) parte de una copia limpia y corre `./configure [flags] && make` ahí.
- **Cada VC arranca un server propio** (`tmux -f/dev/null -L vc$$ new-session -d -s s -x 200 -y 50`, con `$H` de a lo sumo 60 caracteres, para que ninguna línea del pane se parta) y lo mata al terminar con `kill-server`. **Cada VC parte de un entorno nuevo**: `$H` (`mktemp -d`), `authorized_keys`, `known_hosts`, el `sshd` de prueba (con su `$PORT` y su `$SSHD_LOG`), los listeners y el server de `tmux` se crean al empezar el VC y se destruyen al terminar; nada se comparte entre VCs. Tras crear el server: `set-option -g remain-on-exit on` y `set-option -g history-limit 250000`, antes de crear ventanas; `strace` (si el VC lo pide) envuelve esa misma línea de arranque. `$PORT` es un puerto libre elegido como `$FREE`; **convención:** en los VCs, todo comando de `tmux` que apunta a un pane o ventana (`capture-pane`, `display-message`, `send-keys`, `resize-window`, `kill-pane` y las consultas `#{pane_*}`) lleva `-t $P`, y `new-ssh-window -d …` equivale a `new-ssh-window -d -P -F '#{pane_id}' …` (salvo que el VC ya traiga `-P` o `-F`), cuya salida es `$P`, y `$PANE_PID` = `display-message -p -t $P '#{pane_pid}'`; `$TRACE` = `$H/trace`; la host key del `sshd` de prueba se crea con `ssh-keygen -q -t ed25519 -N '' -f $H/host_key`.
- **Arranque de los procesos del harness:** `sshd`, `nc` y `python3` están listos cuando su puerto acepta conexiones (`esperar` hasta que `ss -H -ltn "( sport = :<puerto> )" | wc -l` dé al menos `1`). `sshd -e` escribe en stderr, que el script redirige a `$SSHD_LOG` (`2>>$SSHD_LOG`); el directorio de separación de privilegios `/run/sshd` existe tras instalar `openssh-server`.
- **Esperas:** ninguna comprobación lee el pane, `$SSHD_LOG` ni un archivo en el instante en que vuelve un comando: antes espera su condición, consultando cada 100 ms con tope de 10 s, salvo que el VC fije otro (si vence, el VC falla). `esperar T` espera que `capture-pane -p -t $P` contenga `T`; `esperar-log T`, que `$SSHD_LOG` contenga `T`; `esperar-linea T`, que alguna línea de `capture-pane -p -t $P` sea exactamente `T` (`grep -qx`); `esperar-prefijo T`, que alguna línea empiece con `T` (`grep -q '^T'`); `esperar al pane`, que `#{pane_dead}` de `$P` valga `1`. Los patrones de `grep` de los VCs son BRE de GNU grep. `$P` es el `#{pane_id}` de la ventana creada (`new-ssh-window -d -P -F '#{pane_id}' …`).
- **Hijo colgado:** `new-ssh-window -d $M` (deja `$P` y `$PANE_PID`) y `esperar` hasta que `ss -H -tn state established "( dport = :$MUTE )" | wc -l` dé al menos `1` (el listener mudo ya aceptó la conexión).
- **Abrir sesión:** con la clave autorizada en `$H/.ssh/id_ed25519` sin passphrase y sin agent, `new-ssh-window -d -P -F '#{pane_id}' -p $PORT $D`, `esperar-log` `Accepted publickey` y `send-keys -t $P 'echo R-$((6*7))' Enter` seguido de `esperar` `R-42`. La sesión SSH queda **lista**: la shell remota ya muestra su prompt y el tty local ya está en raw.
- **Prompt de passphrase:** con `$H/.ssh/id_ed25519` protegida con `$PASS` y autorizada, y sin agent, `new-ssh-window -d -P -F '#{pane_id}' -p $PORT $D` y `esperar` `Enter passphrase for key`.
- **Listener mudo:** `nc -lk 127.0.0.1 $MUTE`, que acepta conexiones TCP y nunca manda el
  banner SSH. Deja una conexión colgada de forma determinista: el hijo del pane queda vivo
  hasta el timeout de FR-62 (15 s). `$MUTE` se elige como `$FREE` (abajo) antes de lanzar `nc`; con `nc -lk` las conexiones adicionales quedan en la cola del kernel, también sin banner, y para el cliente es lo mismo. `$M` = `-p $MUTE $USER@127.0.0.1` (los flags van antes del destino: el parser deja de leer flags en el primer argumento posicional).
- **Puerto libre** `$FREE`: `python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'`, sin ningún listener.
- **Listener lleno `$FULL`:** `python3`, en segundo plano, abre un socket en `127.0.0.1` con `listen(0)`, abre conexiones hacia él sin aceptarlas hasta que una no completa en 1 s, y las deja abiertas hasta que el VC termina (el script las mata); desde ahí un `connect` nuevo a `$FULL` no completa (Linux descarta el SYN). `$FULL` es el puerto: `python3` lo escribe en `$H/full.port` **después** de saturar la cola, y el script lo lee tras `esperar` a que el archivo exista.
- **Build de las mediciones:** el de la entrada de CI de Linux con `--enable-native-ssh` y **sin** `--enable-asan`; los umbrales de NFR-1, NFR-3, FR-55 y FR-59 valen para ese build y con la carga normal de un runner de CI (los scripts de `regress/` corren en paralelo).
- **Herramientas:** OpenSSH ≥ 8.0 (`sshd`, `ssh-keygen`, `ssh-agent`, `ssh-add`, `ssh-keyscan`), `nc` de `netcat-openbsd` (con `-lk`), `procps` (`ps`, `pgrep`), `iproute2` (`ss`), `git`, `cpio`, `binutils` (`nm`), `libc-bin` (`ldd`) y `coreutils`, `strace`, `python3` y `man-db` (`man -l`) y `bsdextrautils` (`col -b`), todas en las versiones de la imagen `ubuntu-24.04` que usa `regress.yml`.

## Requerimientos

Un Cuando y un resultado observable por FR. Cada FR tiene al menos un VC `VC-<n>.<k>`
debajo, con su mismo número. Cada VC fija datos, comando y salida esperada; las fallas del
cliente además cumplen BR-3.

### Guarda de build

#### FR-1 · El flag está apagado por defecto

**Dado** el `configure.ac` modificado (flag declarado con `AC_ARG_ENABLE`, como `sixel` en
`configure.ac:545-548`),
**cuando** se corre `./configure` sin `--enable-native-ssh`,
**entonces** `ENABLE_NATIVE_SSH` no queda definido.

**VC-1.1:** en una copia limpia (harness), `./configure` sale con `0` y `grep -c -- -DENABLE_NATIVE_SSH Makefile` da `0` (`tmux` no genera `config.h`: los `AC_DEFINE` viajan en `DEFS` del `Makefile`).

#### FR-2 · En Linux con `libssh`, el flag activa el feature

**Dado** Linux con `libssh >= 0.9.0` detectable por `pkg-config` (`PKG_CHECK_MODULES`, como
`libsystemd` en `configure.ac:508`),
**cuando** se corre `./configure --enable-native-ssh`,
**entonces** `ENABLE_NATIVE_SSH` queda definido.

**VC-2.1:** en una copia limpia (harness), `./configure --enable-native-ssh` sale con `0` y `grep -c -- -DENABLE_NATIVE_SSH Makefile` da `1`.

#### FR-3 · Sin `libssh`, el flag falla en `configure`

**Dado** Linux **sin** `libssh` instalado,
**cuando** se corre `./configure --enable-native-ssh`,
**entonces** `configure` termina con un error que nombra `libssh`.

**VC-3.1:** en una copia limpia (harness) y con `PKG_CONFIG` apuntando a un wrapper que sale con `1` cuando alguno de sus argumentos empieza con `libssh` (autoconf lo invoca con `libssh >= 0.9.0` como un único argumento) y delega en `pkg-config` en cualquier otro caso, `./configure --enable-native-ssh > out.txt 2>&1` sale con `1` y `grep -c libssh out.txt` da al menos `1`; `ls Makefile` falla (no se generó).

#### FR-4 · Con `libssh` anterior a 0.9.0, el flag falla en `configure`

**Dado** Linux con `libssh` **0.8.x** instalado,
**cuando** se corre `./configure --enable-native-ssh`,
**entonces** `configure` termina con un error que contiene `libssh >= 0.9.0`.

**VC-4.1:** en una copia limpia (harness) y con un `libssh.pc` de prueba (`Name: libssh`, `Description: prueba`, `Version: 0.8.9`) en un directorio de `PKG_CONFIG_PATH`, `./configure --enable-native-ssh` sale con `1` y su salida contiene `libssh >= 0.9.0`.

#### FR-5 · Fuera de Linux, el flag falla en `configure`

**Dado** una plataforma donde `PLATFORM` (`configure.ac:1006-1118`) no es `linux`,
**cuando** se corre `./configure --enable-native-ssh`,
**entonces** `configure` termina con el error `native SSH is only supported on Linux`.

En una plataforma no Linux sin `libssh`, el error es el de FR-5, no el de FR-3 ni el de FR-4: la guarda de plataforma se evalúa primero: va después de la asignación de `PLATFORM` (`configure.ac:1118`), antes del bloque de detección de `libssh`; ese bloque va después de la guarda (a diferencia de `libsystemd`, `configure.ac:508`).

**VC-5.1:** `regress/native-ssh-guard.sh`, cuando `uname -s` no es `Linux`, hace una copia limpia (harness; el `configure` del job de macOS no se toca) y corre `./configure --enable-native-ssh` ahí; sale con `1` y su salida contiene `native SSH is only supported on Linux`.

#### FR-6 · Sin el flag, el resumen de `configure` informa `off`

**Dado** el bloque de resumen de `configure.ac` (los `AC_MSG_NOTICE` que terminan en `configure.ac:1167`),
**cuando** `configure` termina bien sin `--enable-native-ssh`,
**entonces** imprime la línea `configure: native SSH: off` (`AC_MSG_NOTICE` antepone `configure: `).

**VC-6.1:** en una copia limpia (harness), `./configure 2>&1 | grep -c '^configure: native SSH: off$'` da `1`.

#### FR-7 · Con el flag, el resumen de `configure` informa `on`

**Dado** el mismo bloque de resumen de `configure.ac`,
**cuando** `configure` termina bien con `--enable-native-ssh`,
**entonces** imprime la línea `configure: native SSH: on` (`AC_MSG_NOTICE` antepone `configure: `).

**VC-7.1:** en una copia limpia (harness), `./configure --enable-native-ssh 2>&1 | grep -c '^configure: native SSH: on$'` da `1`.
#### FR-8 · Con el flag, las fuentes nuevas quedan enlazadas

**Dado** `Makefile.am` con `if ENABLE_NATIVE_SSH … dist_tmux_SOURCES += cmd-new-ssh-window.c ssh-client.c` (patrón de `Makefile.am:253-255`),
**cuando** se corre `make` en un build con `--enable-native-ssh`,
**entonces** los dos objetos quedan enlazados en `tmux`.

**VC-8.1:** en una copia limpia (harness), tras `./configure --enable-native-ssh && make`, existen `cmd-new-ssh-window.o` y `ssh-client.o`, y `nm tmux | grep -c ' T ssh_client_run'` da `1`.

#### FR-9 · Sin el flag, las fuentes nuevas no se compilan

**Dado** el mismo `Makefile.am`,
**cuando** se corre `make` en un build sin el flag,
**entonces** ninguno de los dos objetos se construye.

**VC-9.1:** en una copia limpia (harness), tras `./configure && make`, `ls cmd-new-ssh-window.o ssh-client.o 2>/dev/null | wc -l` da `0` (no existe ninguno) y `nm tmux | grep -c ssh_client_run` da `0`.
### El comando

#### FR-10 · El comando está registrado en la tabla de comandos

**Dado** un build con el flag,
**cuando** el server arma `cmd_table[]` (`cmd.c:123`), con la entrada
`&cmd_new_ssh_window_entry` y su `extern` dentro de `#ifdef ENABLE_NATIVE_SSH`,
**entonces** `new-ssh-window` es un comando válido.

**VC-10.1:** `tmux list-commands | grep -c '^new-ssh-window'` da `1`.

#### FR-11 · El alias es `sshw`

**Dado** `cmd_new_ssh_window_entry` con `.alias = "sshw"`,
**cuando** se invoca `tmux sshw …`,
**entonces** se ejecuta `new-ssh-window`.

**VC-11.1:** `tmux sshw -d -P -F '#{window_index}' -t :9 $D` imprime `9`.

#### FR-12 · Sin destino, el comando falla con el uso

**Dado** `.args = { "dF:i:n:p:Pt:", 1, 1, NULL }` y `.usage = "[-dP] [-F format] [-i identity-file] [-n window-name] [-p port] [-t target-window] destination"`,
**cuando** se invoca sin argumento posicional,
**entonces** es un error de uso con el mensaje de `args_parse` (`arguments.c:333`).

**VC-12.1:** `tmux new-ssh-window` sale con `1`, su stderr es `command new-ssh-window: too few arguments (need at least 1)` y `list-windows | wc -l` no cambia.

#### FR-13 · Con más de un destino, el comando falla con el uso

**Dado** la misma entrada de FR-12,
**cuando** se invoca con dos argumentos posicionales,
**entonces** es un error de uso con el mensaje de `args_parse` (`arguments.c:340`).

**VC-13.1:** `tmux new-ssh-window a b` sale con `1`, su stderr es `command new-ssh-window: too many arguments (need at most 1)` y `list-windows | wc -l` no cambia.

#### FR-14 · `-t` elige el índice de la ventana como en `new-window`

**Dado** `.target = { 't', CMD_FIND_WINDOW, CMD_FIND_WINDOW_INDEX }` (igual que `cmd-new-window.c:46`),
**cuando** se pasa `-t :N` con `N` libre,
**entonces** la ventana se crea en el índice `N`.

**VC-14.1:** tras `tmux new-ssh-window -d -t :7 $D`, `list-windows -F '#{window_index}' | grep -cx 7` da `1`.

#### FR-15 · `-d` no cambia la ventana actual

**Dado** el flag `-d`, que pone `SPAWN_DETACHED` (como `cmd-new-window.c:156-157`),
**cuando** se crea la ventana,
**entonces** la ventana actual de la sesión de `tmux` sigue siendo la misma.

**VC-15.1:** `display-message -p -t s: '#{window_index}'` (sesión `s`, ventana activa) da el mismo valor antes y después de `new-ssh-window -d $D`.

#### FR-16 · `-P` sin `-F` imprime el formato por defecto de `new-window`

**Dado** el flag `-P` y el template `NEW_WINDOW_TEMPLATE` (`cmd-new-window.c:33`, `:174`),
**cuando** se crea la ventana con `-P` y sin `-F`,
**entonces** imprime `#{session_name}:#{window_index}.#{pane_index}` expandido.

**VC-16.1:** en la sesión de `tmux` `s`, `tmux new-ssh-window -d -P -t :6 $D` imprime `s:6.0`.

#### FR-17 · `-F` define el formato que imprime `-P`

**Dado** los flags `-P` y `-F` (como `cmd-new-window.c:172-173`),
**cuando** se crea la ventana con `-P -F 'W=#{window_index}'`,
**entonces** imprime ese formato expandido.

**VC-17.1:** `tmux new-ssh-window -d -P -F 'W=#{window_index}' -t :9 $D` imprime `W=9`.

#### FR-18 · `-n` fija el nombre de la ventana

**Dado** el flag `-n nombre`, que se pasa en `sc.name` (como `cmd-new-window.c:83`, `:140`),
**cuando** se crea la ventana,
**entonces** la ventana se llama `nombre`.

**VC-18.1:** `tmux new-ssh-window -d -n prod -P -F '#{window_name}' $D` imprime `prod`.

#### FR-19 · Sin `-n`, la ventana se llama como el destino

**Dado** que el hijo es un fork del server, y `osdep_get_name` (`osdep-linux.c:30`) leería
`/proc/<pgrp>/cmdline` y devolvería el título del server,
**cuando** no se pasa `-n`,
**entonces** el nombre de la ventana es el destino tal como se escribió.

**VC-19.1:** `tmux new-ssh-window -d -P -F '#{window_name}' $D` imprime `$USER@127.0.0.1`.

#### FR-20 · El nombre de la ventana no cambia solo

**Dado** que fijar `sc.name` apaga `automatic-rename` (`spawn.c:216-224`),
**cuando** pasan 5 s desde que se creó la ventana,
**entonces** el nombre sigue siendo el que tenía.

**VC-20.1:** tras `new-ssh-window -d -P -F '#{window_id}' $D` y `sleep 5`, `display-message -p -t <window_id> '#{window_name}'` da `$USER@127.0.0.1`.

#### FR-21 · `-c` no existe

**Dado** que `-c` no figura en `.args` (un directorio local no aplica a un destino remoto),
**cuando** se pasa `-c`,
**entonces** es un error de uso: el parser lo rechaza (`arguments.c:240`).

**VC-21.1:** `tmux new-ssh-window -c /tmp $D` sale con `1`, su stderr es `command new-ssh-window: unknown flag -c` y `list-windows | wc -l` no cambia.

#### FR-22 · `-e` no existe

**Dado** que `-e` no figura en `.args` (un entorno local no aplica a un destino remoto),
**cuando** se pasa `-e`,
**entonces** es un error de uso: el parser lo rechaza (`arguments.c:240`).

**VC-22.1:** `tmux new-ssh-window -e A=b $D` sale con `1`, su stderr es `command new-ssh-window: unknown flag -e` y `list-windows | wc -l` no cambia.

#### FR-23 · Un destino mal formado es un error de uso

**Dado** un destino que no cumple `[usuario@]host` (usuario y host no vacíos, sin `@` adicionales y sin espacios ni tabs, ni internos ni iniciales ni finales),
**cuando** se invoca el comando,
**entonces** es un error de uso con el mensaje `invalid destination '<destino>'`.

**VC-23.1:** `tmux new-ssh-window 'u@'` sale con `1`, su stderr es `invalid destination 'u@'` y `list-windows | wc -l` no cambia.
**VC-23.2:** `tmux new-ssh-window ''` sale con `1` y su stderr es `invalid destination ''`.
**VC-23.3:** `tmux new-ssh-window '@h'` sale con `1` y su stderr es `invalid destination '@h'`.
**VC-23.4:** `tmux new-ssh-window 'a@b@c'` sale con `1` y su stderr es `invalid destination 'a@b@c'`.
**VC-23.5:** `tmux new-ssh-window 'a b'` sale con `1` y su stderr es `invalid destination 'a b'`.
**VC-23.6:** `tmux new-ssh-window ' h'` sale con `1` y su stderr es `invalid destination ' h'`.
#### FR-24 · Un puerto inválido es un error de uso

**Dado** un valor de `-p` que no es un entero decimal de 1 a 65535 escrito solo con dígitos, sin signo, sin ceros a la izquierda y sin espacios (ambos extremos son válidos),
**cuando** se invoca el comando,
**entonces** es un error de uso con el mensaje `invalid port '<valor>'`.

**VC-24.1:** `tmux new-ssh-window -p abc $D` sale con `1` y su stderr es `invalid port 'abc'`.
**VC-24.2:** con `-p 0` sale con `1` y su stderr es `invalid port '0'`; con `-p 65536`, sale con `1` y su stderr es `invalid port '65536'`; `tmux new-ssh-window -p 1 $D` y `tmux new-ssh-window -p 65535 $D` (sin `-d`) salen con `0`, no imprimen nada y cada una sube `list-windows | wc -l` en 1.
**VC-24.3:** con `-p +22`, `-p 022` y `-p '22 '` (tres invocaciones), cada una sale con `1` y su stderr es `invalid port '+22'`, `invalid port '022'` e `invalid port '22 '` respectivamente.


#### FR-25 · Una ruta de clave no absoluta es un error de uso

**Dado** un valor de `-i` que no empieza con `/` (incluye `~/k`, que no se expande),
**cuando** se invoca el comando,
**entonces** es un error de uso con el mensaje `key file path must be absolute: '<ruta>'`.

**VC-25.1:** `tmux new-ssh-window -i rel/k $D` sale con `1` y su stderr es `key file path must be absolute: 'rel/k'`.
**VC-25.2:** `tmux new-ssh-window -i '~/k' $D` sale con `1` y su stderr es `key file path must be absolute: '~/k'`.
#### FR-26 · Un archivo de clave inexistente es un error de uso

**Dado** un valor de `-i` que no existe,
**cuando** se invoca el comando,
**entonces** es un error de uso con el mensaje `key file '<ruta>' does not exist`.

**VC-26.1:** `tmux new-ssh-window -i /nonexistent/k $D` sale con `1`, su stderr es `key file '/nonexistent/k' does not exist` y `list-windows | wc -l` no cambia.

#### FR-27 · Un archivo de clave sin permiso de lectura es un error de uso

**Dado** un valor de `-i` que existe con modo `000`,
**cuando** se invoca el comando (como usuario no root),
**entonces** es un error de uso con el mensaje `cannot read key file '<ruta>'`.

**VC-27.1:** con `touch $H/k && chmod 000 $H/k`, `tmux new-ssh-window -i $H/k $D` sale con `1`, su stderr es `cannot read key file '$H/k'` (con `$H` expandido por el script) y `list-windows | wc -l` no cambia.

#### FR-63 · Una ruta de clave que es un directorio es un error de uso

**Dado** un valor de `-i` que existe y es un directorio,
**cuando** se invoca el comando,
**entonces** es un error de uso con el mensaje `key file is a directory: '<ruta>'`.

**VC-63.1:** con `mkdir $H/d`, `tmux new-ssh-window -i $H/d $D` sale con `1`, su stderr es `key file is a directory: '$H/d'` (con `$H` expandido por el script) y `list-windows | wc -l` no cambia.

#### FR-73 · Un `-t` que no resuelve es un error de uso

**Dado** un `-t` cuya sesión no existe,
**cuando** se invoca el comando,
**entonces** es un error de uso con el mensaje `can't find session: <sesión>` (el de `cmd-find.c:1262`).

**VC-73.1:** `tmux new-ssh-window -t nosuch: $D` sale con `1`, su stderr es `can't find session: nosuch` y `list-windows | wc -l` no cambia.

#### FR-28 · Con varios errores de uso, se informa solo el primero del orden fijo

**Dado** el orden fijo: (1) flag desconocido, (2) cantidad de destinos, (3) `-t` que no resuelve, (4) destino mal formado, (5) puerto inválido, (6) ruta de clave no absoluta, (7) archivo de clave inexistente, (8) ruta de clave que es un directorio, (9) archivo de clave ilegible,
**cuando** el comando tiene más de un error de uso a la vez,
**entonces** informa únicamente el de menor número.

**VC-28.1:** `tmux new-ssh-window -c /tmp -p abc 'u@'` imprime en stderr solo `command new-ssh-window: unknown flag -c`.
**VC-28.2:** `tmux new-ssh-window -p abc -i /nonexistent/k 'u@'` imprime en stderr solo `invalid destination 'u@'`.
**VC-28.3:** `tmux new-ssh-window -p abc -i /nonexistent/k $D` imprime en stderr solo `invalid port 'abc'`.
**VC-28.4:** `tmux new-ssh-window -i rel/k $D` imprime en stderr solo `key file path must be absolute: 'rel/k'`, y `grep -c 'does not exist'` sobre esa salida da `0`.
**VC-28.5:** `tmux new-ssh-window -p abc` (sin destino) imprime en stderr solo `command new-ssh-window: too few arguments (need at least 1)`.
**VC-28.6:** `tmux new-ssh-window -t nosuch: -p abc 'u@'` imprime en stderr solo `can't find session: nosuch`.
### Enganche en el spawn

#### FR-29 · El pane SSH es un hijo directo del server con un pty

**Dado** el comando de FR-10, que llama a `spawn_window()` (`tmux.h:4195`) con `SPAWN_SSH` en `sc.flags` y crea el pane con el mismo `fdforkpty` (`spawn.c:478`) que cualquier otro,
**cuando** se ejecuta con argumentos válidos,
**entonces** el pane es un hijo directo del server con un tty `/dev/pts/N`.

**VC-29.1:** con el hijo colgado (harness), `ps -o ppid= -p $PANE_PID | tr -d ' '` es igual a `display-message -p '#{pid}'` (el padre es el server) y `display-message -p -t $P '#{pane_tty}'` es un `/dev/pts/N` que existe.

#### FR-30 · En el hijo, `ssh_client_run()` ocupa el lugar de los `exec`

**Dado** un pane creado con `SPAWN_SSH`,
**cuando** el hijo llega al punto posterior a `environ_push` (`spawn.c:544`),
**entonces** el proceso del pane sigue siendo el binario `tmux`: los `exec` de `spawn.c:552-574` no se ejecutan.

**VC-30.1:** con el hijo colgado (harness), `readlink /proc/$PANE_PID/exe` es igual a `readlink /proc/<pid del server>/exe`.

#### FR-31 · Contrato de `argv` entre el comando y el cliente

**Dado** el `argv` que el comando deja en el `spawn_context` (y que `spawn_pane` copia a `wp->argv`),
**cuando** se arma con destino, `-p` e `-i`,
**entonces** `argv` es `destino [-p PUERTO] [-i ARCHIVO]`, en ese orden.

**VC-31.1:** con `touch $H/k`, `tmux new-ssh-window -d -P -F '#{pane_start_command}' -i $H/k -p 2222 u@h` imprime `u@h -p 2222 -i $H/k` (`pane_start_command` stringifica `wp->argv`, `format.c:901-908`).

#### FR-32 · El puerto es el de `-p`

**Dado** el `argv` de FR-31 con `-p $PORT`,
**cuando** el cliente abre la conexión TCP,
**entonces** conecta a `$PORT`.

**VC-32.1:** con el `sshd` de prueba en `$PORT` distinto de 22, `new-ssh-window -d -p $PORT $D` y `esperar-log` hasta que se cumpla "llega a `sshd`" (glosario).

#### FR-33 · Sin `-p`, el puerto es 22

**Dado** el `argv` de FR-31 sin `-p`,
**cuando** el cliente abre la conexión TCP,
**entonces** conecta al puerto 22.

**VC-33.1:** tras `new-ssh-window -d 127.0.0.1`, `esperar al pane`, `tmux kill-server` y `wait` (strace del harness), `grep -c 'connect(.*sin_port=htons(22)' $TRACE` da al menos `1` (nada más en el árbol de procesos de la prueba conecta al puerto 22; el pane muere en milisegundos, por eso la verificación no usa `$PANE_PID`).

### Verificación del host

#### FR-34 · Un host desconocido se rechaza

**Dado** un destino sin entrada en `$HOME/.ssh/known_hosts`,
**cuando** se intenta conectar,
**entonces** el pane muestra el mensaje `new-ssh-window: host key for <host> not found in known_hosts`.

Un `known_hosts` inexistente equivale a uno sin entradas (VC-34.2).

**VC-34.1:** con `$H/.ssh/known_hosts` vacío, `new-ssh-window -d -p $PORT $D` y espera al pane: `capture-pane -p -t $P | grep -c 'new-ssh-window: host key for 127.0.0.1 not found in known_hosts'` da `1`.
**VC-34.2:** con `rm $H/.ssh/known_hosts`, el mismo comando muestra el mismo mensaje.
**VC-34.3:** con `$H/otra` creada con `ssh-keygen -q -t ed25519 -N '' -f $H/otra`, y una línea válida de otro host y **sin** `\n` final (`ssh-keygen -y -f $H/otra | awk '{printf "otrohost %s %s", $1, $2}' > $H/.ssh/known_hosts`), el mismo comando muestra el mismo mensaje.

#### FR-35 · Un host desconocido no modifica `known_hosts`

**Dado** el mismo caso de FR-34,
**cuando** el hijo termina,
**entonces** `$HOME/.ssh/known_hosts` queda sin cambios.

**VC-35.1:** con `$H/.ssh/known_hosts` vacío, tras `new-ssh-window -d -p $PORT $D` y `esperar al pane`, `stat -c %s $H/.ssh/known_hosts` da `0` y su `sha256sum` es el de antes del comando.
**VC-35.2:** con `rm $H/.ssh/known_hosts`, tras el mismo comando y esperar al pane, `ls $H/.ssh/known_hosts` falla (el archivo no se creó).

#### FR-36 · Un host con la clave cambiada se rechaza

**Dado** un destino con una host key **distinta** de la registrada en `$HOME/.ssh/known_hosts`,
**cuando** se intenta conectar,
**entonces** el pane muestra `new-ssh-window: host key for <host> has changed`.

**VC-36.1:** con otra clave registrada para `[127.0.0.1]:$PORT`, tras `new-ssh-window -d -p $PORT $D` y esperar al pane, `capture-pane -p -t $P | grep -c 'new-ssh-window: host key for 127.0.0.1 has changed'` da `1`.

#### FR-37 · Un `known_hosts` ilegible se informa

**Dado** un `$HOME/.ssh/known_hosts` que existe con modo `000` (único sentido de "corrupto" en esta spec; las líneas malformadas están fuera de alcance),
**cuando** se intenta conectar,
**entonces** el pane muestra `new-ssh-window: cannot read <ruta>`.

**VC-37.1:** con `chmod 000 $H/.ssh/known_hosts`, tras `new-ssh-window -d -p $PORT $D` y esperar al pane, `capture-pane -p -t $P | grep -c "new-ssh-window: cannot read $H/.ssh/known_hosts"` da `1`.

### Autenticación

#### FR-38 · Autenticación por agent

**Dado** un host conocido y `SSH_AUTH_SOCK` en el entorno de la sesión de `tmux` (llega al hijo
por `environ_for_session`, `environ.c:253`, y `environ_push`, `spawn.c:544`),
**cuando** se conecta sin `-i`,
**entonces** se autentica con una clave del agent.

**VC-38.1:** con el agent del harness cargado con la clave autorizada, `set-environment -g SSH_AUTH_SOCK <socket>` y `$H/.ssh` sin ninguna clave, `new-ssh-window -d -p $PORT $D` y `esperar-log` `Accepted publickey`: se autentica.

#### FR-39 · Autenticación con la clave de `-i`

**Dado** un host conocido, sin agent,
**cuando** se pasa `-i ARCHIVO`,
**entonces** se autentica con esa clave.

**VC-39.1:** sin `SSH_AUTH_SOCK` en el entorno de la sesión de `tmux`, con la clave autorizada en `$H/k` (fuera de `$H/.ssh`), `new-ssh-window -d -p $PORT -i $H/k $D` y `esperar-log` `Accepted publickey`: se autentica.

#### FR-40 · Autenticación con las claves por defecto

**Dado** un host conocido, sin agent y sin `-i`,
**cuando** se conecta,
**entonces** se prueban, en este orden, `$HOME/.ssh/id_ed25519`, `$HOME/.ssh/id_rsa`.

**VC-40.1:** con la clave autorizada en `$H/.ssh/id_ed25519`, sin `SSH_AUTH_SOCK` y sin `-i`, `new-ssh-window -d -p $PORT $D` y `esperar-log` `Accepted publickey`: se autentica, con el fingerprint de `id_ed25519`.
**VC-40.2:** con la clave autorizada solo en `$H/.ssh/id_rsa` (sin `id_ed25519`), sin `SSH_AUTH_SOCK` y sin `-i`, `new-ssh-window -d -p $PORT $D` y `esperar-log` `Accepted publickey`: `$SSHD_LOG` contiene esa línea con el fingerprint de `id_rsa`.
**VC-40.3:** con `id_ed25519` e `id_rsa` ambas autorizadas, sin `SSH_AUTH_SOCK` y sin `-i`, `new-ssh-window -d -p $PORT $D` y `esperar-log` `Accepted publickey`: `$SSHD_LOG` contiene esa línea con el fingerprint de `id_ed25519` y ninguna línea `Accepted` con el de `id_rsa`.


#### FR-41 · Una clave por defecto ilegible se salta

**Dado** un host conocido, sin agent ni `-i`, con `$HOME/.ssh/id_ed25519` de modo `000` y una `$HOME/.ssh/id_rsa` autorizada,
**cuando** se conecta (como usuario no root),
**entonces** se autentica con `id_rsa`.

**VC-41.1:** con `chmod 000 $H/.ssh/id_ed25519`, `new-ssh-window -d -p $PORT $D` y `esperar-log` `Accepted publickey`: `$SSHD_LOG` contiene esa línea con el fingerprint de `id_rsa`.
#### FR-42 · El agent se prueba antes que los archivos de clave

**Dado** un agent con la clave A y un `-i` con la clave B, ambas autorizadas,
**cuando** se conecta,
**entonces** se autentica con la clave A.

**VC-42.1:** con A (`$H/a`) y B (`$H/b`) creadas con `ssh-keygen` y ambas autorizadas, un agent que tiene solo A (Agent del harness) y `new-ssh-window -d -p $PORT -i $H/b $D`, tras `esperar-log` `Accepted publickey`, `$SSHD_LOG` contiene esa línea con el fingerprint de A y ninguna línea `Accepted` con el de B.

#### FR-43 · La passphrase se pide en el tty del pane

**Dado** una clave protegida con passphrase y sin agent,
**cuando** el cliente la necesita,
**entonces** escribe `Enter passphrase for key '<ruta>': ` en el tty del pane (el pty de `fdforkpty`, `spawn.c:478`).

**VC-43.1:** en el prompt de passphrase del harness, `capture-pane -p -t $P | grep -c "Enter passphrase for key '$H/.ssh/id_ed25519':"` da `1` (`capture-pane -p` recorta el espacio final del prompt).

#### FR-44 · La passphrase tecleada no se muestra

**Dado** el prompt de FR-43,
**cuando** se teclea la passphrase,
**entonces** no se escribe en el pane (sin eco).

**VC-44.1:** en el prompt de passphrase del harness, tras `send-keys -t $P "$PASS" Enter` y `esperar-log` `Accepted publickey`, `capture-pane -p -t $P | grep -c "$PASS"` da `0`.

#### FR-45 · Una passphrase incorrecta se informa

**Dado** una clave protegida con passphrase,
**cuando** se teclea una passphrase incorrecta,
**entonces** el pane muestra `new-ssh-window: wrong passphrase for key '<ruta>'`.

**VC-45.1:** en el prompt de passphrase del harness, tras `send-keys -t $P mala Enter` (`mala` ≠ `$PASS`) y `esperar al pane`, `capture-pane -p -t $P | grep -c "new-ssh-window: wrong passphrase for key '$H/.ssh/id_ed25519'"` da `1`.

#### FR-46 · No se ofrece autenticación por password

**Dado** un `sshd` que solo acepta `password`,
**cuando** se conecta,
**entonces** el pane muestra `new-ssh-window: no supported authentication method`.

El resultado no depende de las claves locales (VC-46.2).

**VC-46.1:** con el `sshd` de prueba relanzado con `PasswordAuthentication yes`, `PubkeyAuthentication no` y `AuthenticationMethods password` (reemplaza la configuración base), tras `new-ssh-window -d -p $PORT $D` y `esperar al pane`, `capture-pane -p -t $P | grep -c 'new-ssh-window: no supported authentication method'` da `1`.
**VC-46.2:** con la configuración de VC-46.1 y `$H/.ssh` con solo `known_hosts` (ninguna clave), el mismo comando y las mismas capturas dan `1` para `new-ssh-window: no supported authentication method` y `0` para `new-ssh-window: no authentication key found`.

#### FR-47 · Sin ninguna clave utilizable se informa

**Dado** un host conocido, un `sshd` que acepta `publickey`, sin agent utilizable (sin `SSH_AUTH_SOCK`; un agent sin identidades cuenta igual), sin `-i` y sin ninguna clave por defecto utilizable,
**cuando** se conecta,
**entonces** el pane muestra `new-ssh-window: no authentication key found`.

**VC-47.1:** con `$H/.ssh` solo con `known_hosts`, tras `new-ssh-window -d -p $PORT $D` y esperar al pane, `capture-pane -p -t $P | grep -c 'new-ssh-window: no authentication key found'` da `1`.
**VC-47.2:** con `$H/.ssh` con `id_ed25519` e `id_rsa` ambas de modo `000` (usuario no root), el mismo comando y la misma captura dan `1` para `new-ssh-window: no authentication key found`.
**VC-47.3:** con el agent del harness arrancado **sin** claves (`set-environment -g SSH_AUTH_SOCK <socket>`) y `$H/.ssh` con solo `known_hosts`, el mismo comando y la misma captura dan `1` para `new-ssh-window: no authentication key found`.

#### FR-48 · Una clave rechazada por el servidor se informa

**Dado** una clave legible de `-i` que `sshd` **no** tiene autorizada,
**cuando** se conecta,
**entonces** el pane muestra `new-ssh-window: authentication failed for <usuario>@<host>`.

**VC-48.1:** con `-i` apuntando a una clave no listada en `authorized_keys` y `$H/.ssh` con solo `known_hosts`, tras `new-ssh-window -d -p $PORT -i $H/x $D` y `esperar al pane`, `capture-pane -p -t $P | grep -c "new-ssh-window: authentication failed for $USER@127.0.0.1"` da `1`.


#### FR-72 · Con `-i` no se prueban las claves por defecto

**Dado** un host conocido, un `-i` con una clave que `sshd` **no** tiene autorizada y una `id_ed25519` autorizada en `$HOME/.ssh`,
**cuando** se conecta,
**entonces** `sshd` no acepta ninguna autenticación.

**VC-72.1:** con `$H/.ssh/id_ed25519` creada con `ssh-keygen` y autorizada, `$H/x` no autorizada, y `new-ssh-window -d -p $PORT -i $H/x $D` y `esperar al pane`, `grep -c Accepted $SSHD_LOG` da `0`.

#### FR-74 · Una clave por defecto rechazada por el servidor se informa

**Dado** un host conocido, sin agent ni `-i`, una `$HOME/.ssh/id_ed25519` legible que `sshd` **no** tiene autorizada y ninguna otra clave por defecto,
**cuando** se conecta,
**entonces** el pane muestra `new-ssh-window: authentication failed for <usuario>@<host>`.

**VC-74.1:** con `id_ed25519` creada con `ssh-keygen` y no listada en `authorized_keys`, tras `new-ssh-window -d -p $PORT $D` y `esperar al pane`, `capture-pane -p -t $P | grep -c "new-ssh-window: authentication failed for $USER@127.0.0.1"` da `1`.

#### FR-75 · Un agent con claves rechazadas se informa

**Dado** un host conocido, un agent cuya única clave `sshd` **no** tiene autorizada, sin `-i` y sin clave por defecto utilizable,
**cuando** se conecta,
**entonces** el pane muestra `new-ssh-window: authentication failed for <usuario>@<host>`.

**VC-75.1:** con el agent del harness cargado solo con `$H/x` (clave X, no listada en `authorized_keys`) y `$H/.ssh` con solo `known_hosts`, tras `new-ssh-window -d -p $PORT $D` y `esperar al pane`, `capture-pane -p -t $P | grep -c "new-ssh-window: authentication failed for $USER@127.0.0.1"` da `1`.

#### FR-49 · Un archivo que no es una clave se informa

**Dado** un valor de `-i` que existe, es legible y no es una clave privada (por ejemplo, un archivo de 0 bytes),
**cuando** se conecta,
**entonces** el pane muestra `new-ssh-window: cannot load key '<ruta>'`.

**VC-49.1:** con `: > $H/empty`, tras `new-ssh-window -d -p $PORT -i $H/empty $D` y esperar al pane, `capture-pane -p -t $P | grep -c "new-ssh-window: cannot load key '$H/empty'"` da `1`.
**VC-49.2:** con `echo 'no soy una clave' > $H/text`, tras `new-ssh-window -d -p $PORT -i $H/text $D` y `esperar al pane`, `capture-pane -p -t $P | grep -c "new-ssh-window: cannot load key '$H/text'"` da `1`.
#### FR-64 · Un agent inalcanzable no impide usar `-i`

**Dado** un host conocido, `SSH_AUTH_SOCK` en el entorno de la sesión de `tmux` con la ruta de un socket sin listener, y una clave autorizada en `-i`,
**cuando** se conecta,
**entonces** se autentica con la clave de `-i`.

**VC-64.1:** con `set-environment -g SSH_AUTH_SOCK $H/no-agent.sock` (el archivo no existe) y `new-ssh-window -d -p $PORT -i $H/k $D`, tras `esperar-log` `Accepted publickey`, `$SSHD_LOG` contiene esa línea con el fingerprint de `$H/k`.

#### FR-65 · Un agent sin ninguna clave aceptada no impide usar `-i`

**Dado** un host conocido, un agent cuya única clave `sshd` **no** tiene autorizada, y una clave autorizada en `-i`,
**cuando** se conecta,
**entonces** se autentica con la clave de `-i`.

**VC-65.1:** con un agent que tiene solo `$H/x` (clave X, no listada en `authorized_keys`, Agent del harness) y `new-ssh-window -d -p $PORT -i $H/k $D`, tras `esperar-log` `Accepted publickey`, `$SSHD_LOG` contiene esa línea con el fingerprint de `$H/k` y ninguna línea `Accepted` con el de X.

#### FR-66 · Una clave por defecto que no es una clave se salta

**Dado** un host conocido, sin agent ni `-i`, un `$HOME/.ssh/id_ed25519` legible que no es una clave privada y una `$HOME/.ssh/id_rsa` autorizada,
**cuando** se conecta,
**entonces** se autentica con `id_rsa`.

**VC-66.1:** con `echo 'no soy una clave' > $H/.ssh/id_ed25519`, `new-ssh-window -d -p $PORT $D` y `esperar-log` `Accepted publickey`, éste contiene esa línea con el fingerprint de `id_rsa`.

#### FR-67 · Sin usuario en el destino se usa el del server

**Dado** un host conocido y una clave por defecto autorizada para el usuario del proceso del server,
**cuando** se conecta con el destino `127.0.0.1` (sin `usuario@`),
**entonces** se autentica como el usuario del proceso del server.

**VC-67.1:** `new-ssh-window -d -p $PORT 127.0.0.1` y `esperar-log` `Accepted publickey for $USER`.

#### FR-70 · La passphrase correcta autentica

**Dado** el prompt de FR-43,
**cuando** se teclea la passphrase correcta,
**entonces** se autentica.

**VC-70.1:** en el prompt de passphrase del harness, tras `send-keys -t $P "$PASS" Enter` y `esperar-log` `Accepted publickey`, `$SSHD_LOG` contiene esa línea con el fingerprint de `$H/.ssh/id_ed25519`.

### Sesión interactiva

#### FR-50 · El pty remoto usa el `TERM` del pane

**Dado** una conexión autenticada,
**cuando** se pide el pty remoto,
**entonces** se pide con el `TERM` del entorno del hijo (el `default-terminal` que pone
`environ_for_session`, `environ.c:265`).

**VC-50.1:** con la sesión lista (harness), `send-keys -t $P 'echo T=$TERM' Enter` y `esperar-prefijo` `T=`: `capture-pane -p -t $P` contiene `T=` seguido del valor de `show-options -gv default-terminal`.

#### FR-51 · El pty remoto arranca con el tamaño del pane

**Dado** una conexión autenticada,
**cuando** se pide el pty remoto,
**entonces** se pide con el tamaño (filas por columnas) del tty del pane (`TIOCGWINSZ` sobre el tty local).

**VC-51.1:** con la sesión lista (harness), `send-keys -t $P 'stty size' Enter` y `esperar` el texto `<pane_height> <pane_width>` (valores de `display-message -p -t $P '#{pane_height} #{pane_width}'`): `capture-pane -p -t $P` lo contiene.

#### FR-52 · Lo tecleado en el pane llega a la shell remota

**Dado** el pty remoto con una shell,
**cuando** se teclea en el pane,
**entonces** los bytes llegan a la shell remota.

**VC-52.1:** con la sesión lista (harness), `send-keys -t $P "echo L > $H/sent" Enter` (comillas dobles: `$H` lo expande el script de prueba, no la shell remota) y `esperar` hasta que el contenido de `$H/sent` sea `L` (se espera por contenido, no por existencia del archivo) (la shell remota corre en la misma máquina de prueba).

#### FR-53 · La salida de la shell remota aparece en el pane

**Dado** el pty remoto con una shell,
**cuando** la shell remota escribe,
**entonces** el pane muestra esos bytes.

**VC-53.1:** tras abrir la sesión (harness), `capture-pane -p -t $P | grep -cx 'R-42'` da `1`: la línea es la salida del `echo` remoto, no el comando tecleado.

#### FR-54 · El tty local en raw no duplica el eco

**Dado** que la disciplina de línea local duplicaría el eco, y que `cfmakeraw` (`compat.h:426`, usado igual en `client.c:349`) la apaga,
**cuando** se teclea un comando,
**entonces** el pane muestra el eco una sola vez.

**VC-54.1:** con la sesión lista (harness), `send-keys -t $P 'echo ok' Enter` y `esperar-linea` `ok`: `capture-pane -p -t $P | grep -c 'echo ok'` da `1`.

#### FR-55 · `Ctrl-C` llega al proceso remoto

**Dado** el tty local en raw y un `sleep 100` remoto en curso,
**cuando** se envía `Ctrl-C`,
**entonces** el prompt remoto vuelve en menos de **2 s**.

**VC-55.1:** con la sesión lista (harness), `send-keys -t $P 'sleep 97' Enter` (duración que ningún otro script de `regress/` usa) y `esperar` hasta que `pgrep -fx 'sleep 97'` encuentre el proceso; con `t0` tomado justo antes de `send-keys -t $P C-c`, luego `send-keys -t $P 'echo P-$((6*7))' Enter`: la línea `P-42` aparece en `capture-pane -p -t $P` con `t1 - t0` menor que `2000000000` ns (Tiempos del harness) y `pgrep -fx 'sleep 97'` ya no encuentra el proceso.
#### FR-56 · El resize llega al remoto

**Dado** una sesión SSH abierta,
**cuando** cambia el tamaño del pane (`window_pane_send_resize`, `window.c:597`, hace el
`ioctl(TIOCSWINSZ)` y el hijo recibe `SIGWINCH`),
**entonces** el cliente actualiza el tamaño del pty remoto.

**VC-56.1:** con la sesión lista (harness), tras `resize-window -t $P -x 100 -y 30`, `send-keys -t $P 'stty size' Enter` y `esperar` `30 100`: `capture-pane -p -t $P` lo contiene.

### Fin de sesión

#### FR-57 · El hijo sale con el código de salida remoto

**Dado** una sesión SSH abierta,
**cuando** la shell remota termina,
**entonces** el hijo sale con ese código.

El pane sigue el camino habitual (`SIGCHLD` → `server_child_signal`, `server.c:468` → `server_child_exited` → `PANE_EXITED`); no se agrega ninguna ruta de destrucción.

**VC-57.1:** con la sesión lista (harness), `send-keys -t $P 'exit 3' Enter` y `esperar al pane` dejan `#{pane_dead_status}` en `3`.

#### FR-58 · Si la conexión se corta, el pane lo informa

**Dado** una sesión SSH abierta,
**cuando** la conexión se pierde sin código de salida remoto,
**entonces** el pane muestra `new-ssh-window: connection lost`.

**VC-58.1:** con la sesión lista (harness) y **una sola** sesión SSH abierta, tras `for m in $(pgrep -P $SSHD_PID); do pkill -9 -P $m; kill -9 $m; done` (mata el monitor de la sesión y su proceso hijo; al morir ambos el kernel cierra el socket TCP y el cliente ve el fin de la conexión sin código de salida) y `esperar al pane`, `capture-pane -p -t $P | grep -c 'new-ssh-window: connection lost'` da `1`.

#### FR-59 · Cerrar el pane termina al hijo

**Dado** una sesión SSH abierta,
**cuando** el pane se cierra desde `tmux` (el cierre del master le llega al hijo como `SIGHUP`),
**entonces** el hijo termina en menos de **2 s**.

**VC-59.1:** con la sesión lista (harness), `$PANE_PID` y `t0` tomado justo antes de `kill-pane -t $P`, `kill -0 $PANE_PID` falla con `t1 - t0` menor que `2000000000` ns (Tiempos del harness).
### Fallas de conexión

#### FR-60 · Un host que no resuelve se informa

**Dado** un host que no resuelve,
**cuando** se intenta conectar,
**entonces** el pane muestra `new-ssh-window: cannot connect to <host>: name resolution failed`.

**VC-60.1:** `new-ssh-window -d nohost.invalid` y `esperar al pane` con tope de 20 s (`.invalid` nunca resuelve, RFC 6761): `capture-pane -p -t $P | grep -c 'new-ssh-window: cannot connect to nohost.invalid: name resolution failed'` da `1`.

#### FR-61 · Un puerto cerrado se informa

**Dado** un host que resuelve y un puerto sin listener,
**cuando** se intenta conectar,
**entonces** el pane muestra `new-ssh-window: cannot connect to <host>: connection refused`.

**VC-61.1:** `new-ssh-window -d -p $FREE 127.0.0.1`: `capture-pane -p -t $P | grep -c 'new-ssh-window: cannot connect to 127.0.0.1: connection refused'` da `1`.

#### FR-62 · La conexión tiene un timeout de 15 s

**Dado** un destino que acepta la conexión TCP pero no completa el intercambio de claves,
**cuando** pasan **15 s** desde que el cliente arrancó (con una tolerancia de 1 s),
**entonces** el pane muestra `new-ssh-window: connection to <host> timed out`.

**VC-62.1:** contra el listener mudo, con `t0` tomado al volver `new-ssh-window -d $M`: a `t0 + 14 s` una única consulta de `#{pane_dead}` da `0`; después se consulta cada 100 ms y `#{pane_dead}` es `1` con `t1 - t0` menor que `16000000000` ns; `capture-pane -p -t $P | grep -c 'new-ssh-window: connection to 127.0.0.1 timed out'` da `1`.

#### FR-68 · `-t` con un índice ocupado falla como en `new-window`

**Dado** una ventana existente en el índice `N`,
**cuando** se pasa `-t :N`,
**entonces** el comando falla con el mensaje `create window failed: index N in use` (el de `cmd-new-window.c:162`, con la causa de `spawn.c:164`).

**VC-68.1:** tras `new-window -d -t :7`, `tmux new-ssh-window -d -t :7 $D` sale con `1`, su stderr es `create window failed: index 7 in use` y `list-windows | wc -l` no cambia.

#### FR-69 · Una conexión TCP que no completa se corta a los 15 s

**Dado** un destino cuyo `connect` TCP no completa (el listener lleno `$FULL` del harness),
**cuando** pasan **15 s** desde que el cliente arrancó (con una tolerancia de 1 s),
**entonces** el pane muestra `new-ssh-window: connection to <host> timed out`.

**VC-69.1:** `new-ssh-window -d -p $FULL 127.0.0.1` con `t0` tomado al volver el comando: a `t0 + 14 s` una única consulta de `#{pane_dead}` da `0`; después se consulta cada 100 ms y `#{pane_dead}` es `1` con `t1 - t0` menor que `16000000000` ns; `capture-pane -p -t $P | grep -c 'new-ssh-window: connection to 127.0.0.1 timed out'` da `1`.

#### FR-71 · Un pane común no entra al bloque nuevo

**Dado** un build con el flag,
**cuando** `new-window` crea un pane sin `SPAWN_SSH`,
**entonces** el proceso del pane es el programa pedido, no `tmux`.

**VC-71.1:** tras `new-window -d -P -F '#{pane_id} #{pane_pid}' sleep 100` (imprime `<pane_id> <pane_pid>`), `esperar` (100 ms, tope 10 s) hasta que `readlink /proc/<pane_pid>/exe` deje de terminar en `/tmux`: termina en `/sleep` (pasó por `execvp`, `spawn.c:552`).

### Reglas y no funcionales

#### BR-1 · No se registran secretos

La passphrase **nunca** llega al log de `tmux`.
El hijo ya cerró el log (`log_close`, `spawn.c:543`) antes del bloque de FR-30.

**VC-BR1.1:** con el server arrancado con `tmux -vv` desde `cd $H` (los logs `tmux-server-<pid>.log` y `tmux-client-<pid>.log` quedan en `$H`; el log registra los argumentos de cada comando, `cmd.c:249`, por eso la passphrase no se teclea con `send-keys`), en el prompt de passphrase del harness se inyecta `$PASS` con `printf '%s\n' "$PASS" > $H/pw; tmux load-buffer $H/pw; tmux paste-buffer -t $P`, y tras `esperar-log` `Accepted publickey`, `grep -c "$PASS" $H/tmux-server-*.log $H/tmux-client-*.log` da `0` en cada archivo.
#### BR-2 · Sin binding por default

El comando no se asocia a ninguna tecla.

**VC-BR2.1:** `tmux -f/dev/null list-keys | grep -c new-ssh-window` da `0`.

#### BR-3 · Toda falla del cliente sale con estado 1

Cada falla del cliente (ver glosario) termina el hijo con estado `1`, visible en
`#{pane_dead_status}` (`format_cb_pane_dead_status`, `format.c:2288`, devuelve `WEXITSTATUS(wp->status)`, que `server_child_exited` guarda en `wp->status`, `server.c:499`).

**VC-BR3.1:** `new-ssh-window -d nohost.invalid`, tras `esperar al pane` con tope de 20 s, deja `#{pane_dead_status}` en `1` (FR-60).
**VC-BR3.2:** con `$H/.ssh/known_hosts` vacío, `#{pane_dead_status}` es `1` (FR-34).
**VC-BR3.3:** con `-i` no autorizada, `#{pane_dead_status}` es `1` (FR-48).
**VC-BR3.4:** contra el listener mudo, con `t0` tomado al volver `new-ssh-window -d $M`, a `t0 + 17 s` una única consulta de `#{pane_dead_status}` da `1` (FR-62).
**VC-BR3.5:** el escenario de VC-36.1, tras esperar al pane, deja `#{pane_dead_status}` en `1` (FR-36).
**VC-BR3.6:** el escenario de VC-37.1, tras esperar al pane, deja `#{pane_dead_status}` en `1` (FR-37).
**VC-BR3.7:** el escenario de VC-45.1, tras esperar al pane, deja `#{pane_dead_status}` en `1` (FR-45).
**VC-BR3.8:** el escenario de VC-46.1, tras esperar al pane, deja `#{pane_dead_status}` en `1` (FR-46).
**VC-BR3.9:** el escenario de VC-47.1, tras esperar al pane, deja `#{pane_dead_status}` en `1` (FR-47).
**VC-BR3.10:** el escenario de VC-49.1, tras esperar al pane, deja `#{pane_dead_status}` en `1` (FR-49).
**VC-BR3.11:** el escenario de VC-58.1, tras esperar al pane, deja `#{pane_dead_status}` en `1` (FR-58).
**VC-BR3.12:** el escenario de VC-61.1, tras esperar al pane, deja `#{pane_dead_status}` en `1` (FR-61).
**VC-BR3.13:** el escenario de VC-69.1, tras `esperar al pane` con tope de 20 s, deja `#{pane_dead_status}` en `1` (FR-69).
**VC-BR3.14:** el escenario de VC-74.1, tras esperar al pane, deja `#{pane_dead_status}` en `1` (FR-74).
**VC-BR3.15:** el escenario de VC-75.1, tras esperar al pane, deja `#{pane_dead_status}` en `1` (FR-75).


#### BR-4 · Los VCs con el flag corren en el CI de Linux

`.github/workflows/regress.yml` tiene una entrada de matriz solo Linux con `--enable-native-ssh` que corre `regress/native-ssh-guard.sh`, `regress/native-ssh-build.sh` y `regress/new-ssh-window.sh`; como `regress/new-ssh-window.sh` falla (no se saltea) si el comando está listado y falta `sshd`, los VCs de red no se saltean en silencio.

**VC-BR4.1:** `grep -c -- '--enable-native-ssh' .github/workflows/regress.yml` da `1`, y la línea `configure:` de esa entrada no contiene `--enable-asan`.
#### NFR-1 · El server responde mientras una conexión está pendiente

Con **un** pane SSH colgado contra el listener mudo, `display-message -p ok` desde otro cliente `tmux` responde en menos de **1 s**, medido con el reloj de pared (`date +%s%N` antes y después), en loopback, con el server **sin** `strace` y con el build de las mediciones del harness (sin `--enable-asan`). El server no hace I/O de red: toda la conexión ocurre en el hijo (FR-30).

**VC-NFR1.1:** con el hijo colgado (harness) y el server sin `strace`, `t0=$(date +%s%N); tmux display-message -p -t $P ok; t1=$(date +%s%N)` imprime `ok` y `t1 - t0` es menor que `1000000000` ns.
#### NFR-2 · Documentación

`tmux.1`, renderizado con `man -l tmux.1`, contiene **exactamente una** sinopsis de `new-ssh-window` igual al `.usage` de FR-12 y **al menos una** mención de `--enable-native-ssh` (el man page se instala siempre, así que aclara que el comando solo existe con ese flag).

**VC-NFR2.1:** con `MANWIDTH=200`, `man -l tmux.1 | col -b | grep -c 'new-ssh-window \[-dP\] \[-F format\] \[-i identity-file\] \[-n window-name\] \[-p port\] \[-t target-window\] destination'` da `1` y con `MANWIDTH=200`, `man -l tmux.1 | col -b | grep -c 'enable-native-ssh'` da al menos `1`.
#### NFR-3 · La salida masiva no se atasca

Con `history-limit 250000`, una sesión SSH en loopback, un pane, el server **sin** `strace` y el build de las mediciones del harness (sin `--enable-asan`), `seq 1 200000` termina en menos de **30 s** de reloj de pared (`date +%s%N`) y el pane muestra los 200000 números, cada uno **una sola vez**.

**VC-NFR3.1:** con la sesión lista (harness) y `history-limit 250000`, `t0=$(date +%s%N)` justo antes de `send-keys -t $P 'seq 1 200000' Enter`; `esperar` la línea `200000` (`capture-pane -p -t $P | grep -cx 200000`, con tope de 40 s en lugar de 10 s): `t1 - t0` es menor que `30000000000` ns; `capture-pane -p -t $P -S - | grep -cx '[0-9]\+'` da `200000` y `capture-pane -p -t $P -S - | grep -x '[0-9]\+' | sort -n | uniq -d | wc -l` da `0`.
## VCs de invariantes

Cada invariante tiene acá su VC, con el mismo peso que los de los FR.

**VC-INV1.1:** en Linux, build **sin** el flag: `ldd ./tmux | grep -c libssh` da `0`;
`nm ./tmux | grep -c ssh_client_run` da `0`; y
`cmds | diff - regress/list-commands.base` no imprime nada. Lo ejecuta
`regress/native-ssh-guard.sh` en los jobs de Linux sin flag.

**VC-INV2.1:** en el job `macos-26-arm64` de `regress.yml`, **sin tocar su `configure` ni sus dependencias** (sigue con
`--enable-utf8proc --enable-asan` y sin `libssh`): el step `build` termina con `0`, y
`regress/native-ssh-guard.sh` comprueba, cuando `uname -s` no es `Linux`, que
`cmds | grep -c new-ssh-window` da `0` y que
`nm $TEST_TMUX | grep -c ssh_client_run` da `0`.

**VC-INV3.1:** en Linux, build **con** el flag:
`cmds | grep -v '^new-ssh-window' | diff - regress/list-commands.base`
no imprime nada (los 92 comandos existentes conservan nombre, alias y uso), y
`cmds | wc -l` da `93`.

**VC-INV3.3:** en Linux, build **con** el flag, `tmux new-s -d -s x` sale con `1` y su stderr empieza con `ambiguous command: new-s, could be: ` y nombra `new-session` y `new-ssh-window` (`cmd.c:506`: toda abreviatura se resuelve por prefijo único; cualquier nombre que empiece con `new-s` la rompe, y es la única abreviatura existente que cambia); en el build sin flag crea la sesión.

**VC-INV3.2:** `(cd regress && make)`, con y sin el flag, imprime `PASS` para los mismos scripts que la línea de base (171 en el entorno medido, incluido `new-window-command.sh`, que crea cuatro ventanas con `new-window`) y ningún `FAIL` fuera de los que ya fallaban en ella (en el entorno medido, solo `prompt-words-history.sh`). El código de salida de `make` no se usa: es `1` mientras exista un `FAIL` previo.

**VC-INV4.1:** `git diff --name-only $BASE | grep -cE '^(window|server|server-fn|job|format|input|cmd-find|osdep-.*)\.c$'` da `0`, y
`git diff --numstat $BASE -- spawn.c cmd.c tmux.h | awk '$2 > 0' | wc -l` da `0` (ninguna línea eliminada en los tres).

**VC-INV4.3:** `git diff --name-only $BASE -- '*.c' '*.h' 'compat/*' 'regress/*' '.github/*' configure.ac Makefile.am tmux.1` solo lista paths de la tabla "Dentro de alcance" (`cmd-new-ssh-window.c`, `ssh-client.c`, `spawn.c`, `tmux.h`, `cmd.c`, `configure.ac`, `Makefile.am`, `tmux.1`, `regress/new-ssh-window.sh`, `regress/native-ssh-guard.sh`, `regress/native-ssh-build.sh`, `regress/list-commands.base`, `.github/workflows/regress.yml`), y `git diff -U0 $BASE -- spawn.c | grep -c '^@@'` da `1` (un solo bloque).

**VC-INV4.2:** el escenario de VC-71.1 (`new-window -d -P -F '#{pane_id} #{pane_pid}' sleep 100`), en el build **con** el flag, y además `display-message -p -t <pane_id> '#{pane_tty}'` es un `/dev/pts/N` y `kill <pane_pid>` deja `display-message -p -t <pane_id> '#{pane_dead_signal}'` en `15`.

**VC-INV5.1:** con el hijo colgado (harness), `grep -c "^$PANE_PID .*execve" $TRACE` da `0` (ningún `execve` en el pid del pane SSH; los del shell de la ventana inicial y los del arranque del server tienen otro pid y no cuentan) y `pgrep -P $PANE_PID` no devuelve nada (el pane no tiene hijos, así que tampoco hay `execve` de descendientes).

**VC-INV5.2:** con la sesión lista (harness), `grep -c "^$PANE_PID .*execve" $TRACE` sigue dando `0` y `pgrep -P $PANE_PID` sigue sin devolver nada.

**VC-INV6.1:** `ls /proc/<pid del server>/task | wc -l` da el mismo número antes de crear el pane SSH y con el hijo colgado (harness).

**VC-INV6.2:** `ls /proc/<pid del server>/task | wc -l` da el mismo número con la sesión lista (harness).

**VC-INV7.1:** `grep -c 'define SPAWN_SSH 0x2000' tmux.h` da `1`, y
`grep -o 'SPAWN_[A-Z]* 0x[0-9a-f]*' tmux.h | awk '{print $2}' | sort | uniq -d` no imprime nada.

## Matriz de cobertura

| Requisito | VC | Camino de falla incluido |
|---|---|---|
| FR-1 … FR-9 | VC-1.1 … VC-9.1 | VC-3.1 (sin `libssh`), VC-4.1 (versión vieja), VC-5.1 (no-Linux) |
| FR-10, FR-11 | VC-10.1, VC-11.1 | — |
| FR-12, FR-13 | VC-12.1, VC-13.1 | sin destino, dos destinos |
| FR-14 … FR-20, FR-71 | VC-14.1 … VC-20.1, VC-71.1 | — |
| FR-21, FR-22 | VC-21.1, VC-22.1 | flags rechazados |
| FR-23 … FR-27, FR-63, FR-73 | VC-23.1 … VC-27.1, VC-63.1, VC-73.1 | destino mal formado, puerto inválido, ruta de clave no absoluta, clave inexistente, ilegible o directorio |
| FR-28 | VC-28.1 … VC-28.6 | precedencia de errores de uso |
| FR-29 … FR-33 | VC-29.1 … VC-33.1 | — |
| FR-34 … FR-37 | VC-34.1 … VC-37.1 | host desconocido (también sin archivo y sin `\n` final), no modifica `known_hosts` (existente o no), host cambiado, `known_hosts` ilegible |
| FR-38 … FR-44 | VC-38.1 … VC-44.1 | FR-41: clave por defecto ilegible, la siguiente se usa |
| FR-64 … FR-67, FR-70, FR-72, FR-74, FR-75 | VC-64.1 … VC-75.1 | agent inalcanzable o sin clave aceptada, clave por defecto que no es clave, usuario por defecto, passphrase correcta |
| FR-45 … FR-49 | VC-45.1 … VC-49.2 | passphrase incorrecta, sin password, sin clave, clave rechazada, archivo que no es clave |
| FR-50 … FR-57 | VC-50.1 … VC-57.1 | — |
| FR-58, FR-59 | VC-58.1, VC-59.1 | conexión cortada, pane cerrado |
| FR-60 … FR-62, FR-68, FR-69 | VC-60.1 … VC-62.1, VC-68.1, VC-69.1 | host inexistente, puerto cerrado, sesión que no abre, `connect` que no completa, índice de ventana ocupado |
| BR-1 … BR-4 | VC-BR1.1 … VC-BR4.1 | exit `1` de todas las fallas del cliente (BR-3) |
| NFR-1 … NFR-3 | VC-NFR1.1 … VC-NFR3.1 | — |
| INV-1 … INV-7 | VC-INV1.1 … VC-INV7.1 | build no-Linux (INV-2) |

## Plan de iteraciones

Cada iteración termina con la línea de base **sin** el flag igual a la inicial (VC-INV1.1, VC-INV3.2).

| Iteración | Alcance | Cierra |
|---|---|---|
| **1** | Guardas de build: `configure.ac`, `Makefile.am` y `regress/native-ssh-guard.sh`, `regress/native-ssh-build.sh` (con la copia limpia del harness) y `regress/list-commands.base`; las dos fuentes nuevas con `ssh_client_run()` como esqueleto que devuelve `1` | FR-1 … FR-9, INV-1, INV-2 |
| **2** | Comando, validación de argumentos, `SPAWN_SSH`, bloque en `spawn.c`, `ssh_client_run()` que solo conecta; **`regress/new-ssh-window.sh` con el harness: `sshd` de prueba, listener mudo, puerto libre, listener lleno y `strace`** | FR-10 … FR-33, FR-60 … FR-62, FR-63, FR-68, FR-69, FR-71, FR-73, NFR-1, INV-3, INV-4, INV-7 |
| **3** | Host key y autenticación | FR-34 … FR-49, FR-64 … FR-67, FR-70, FR-72, FR-74, FR-75, BR-1 |
| **4** | Sesión interactiva, resize y fin de sesión | FR-50 … FR-59, BR-3, NFR-3, INV-5, INV-6 |
| **5** | `tmux.1` y entrada de CI | NFR-2, BR-2, BR-4 |

La Iteración 1 es el camino más angosto que se puede verificar solo: el esqueleto de
`ssh_client_run()` basta para VC-8.1 y la prueba del límite solo-Linux completo no necesita
lógica de red. FR-5 e INV-2 cierran con VC-5.1 y VC-INV2.1, que se ejecutan en una máquina no Linux (en Linux no son ejecutables, ver Limitaciones). El `sshd` de prueba aparece en la Iteración 2, no al final, para que ningún VC
de red quede sin forma de ejercitarse.

Cada VC se puede correr en la iteración que lo cierra, salvo VC-5.1 y VC-INV2.1, que se ejecutan en una máquina no Linux. Los de la Iteración 2 no dependen de
autenticar: observan la llegada a `sshd` en su log (VC-32.1) o un hijo que sigue vivo contra
el listener mudo (VC-29.1, VC-30.1, VC-NFR1.1). Los de la Iteración 3 observan
`Accepted publickey` en el log de `sshd` o un mensaje del pane, no el prompt remoto, que
necesita la shell de la Iteración 4. VC-INV5.1 y VC-INV6.1 pueden correrse a mano ya en la
Iteración 2, con la conexión colgada; INV-5 e INV-6 cierran en la 4 porque VC-INV5.2 y
VC-INV6.2 piden la sesión abierta. VC-BR3.1, VC-BR3.4, VC-BR3.12 y VC-BR3.13 pueden correrse a mano en la 2; VC-BR3.2, VC-BR3.3, VC-BR3.5 … VC-BR3.10, VC-BR3.14 y VC-BR3.15 en la 3 (todos entran al script al cierre de la iteración de su requerimiento); VC-BR3.11 necesita la sesión SSH abierta, y por eso BR-3 cierra en la 4.

## Decisiones

Cada decisión dice qué se eligió, qué código de `tmux` (`5a820e63`) la funda, qué se
descartó y por qué, y en qué FR queda escrita.

| # | Pregunta | Elegido | Fundamento (código de `tmux`) | Descartado y por qué | FR |
|---|---|---|---|---|---|
| 1 | ¿Comando nuevo, y dónde se registra? | `new-ssh-window`, alias `sshw`, en `cmd-new-ssh-window.c`, registrado en `cmd_table[]` de `cmd.c` bajo `#ifdef` | `struct spawn_context` (`tmux.h:2500-2532`) no tiene campo "tipo de pane" y `new-window` trata todo posicional como comando a ejecutar (`.args` con `0, -1`, `cmd-new-window.c:41`). Todo comando se registra en `cmd_table[]` (`cmd.c:123`), que hoy no tiene ningún `#if` | Un flag `-S` en `new-window`: obliga a tocar un comando existente y su `.args` (INV-3). Un prefijo `ssh:` en el comando de `new-window`: mezcla significados en un argumento que hoy es "ejecutar" | FR-10, FR-11 |
| 2 | ¿Dónde engancha en el spawn? | **En el hijo**, tras `fdforkpty` y `environ_push`, donde hoy están los `exec` | `spawn.c:478` deja `pid`, `fd` y `tty` reales; `window_pane_send_resize` hace `fatal` si el `ioctl(TIOCSWINSZ)` falla (`window.c:612-622`), así que el fd tiene que seguir siendo un pty; `server_child_exited` busca el pane por `wp->pid` (`server.c:491-498`). Ya hay código opcional en ese tramo del hijo (`spawn.c:504`) | Un pane con fd de socket y sin hijo: rompe `window_pane_send_resize` y `server_child_exited` (modelo de pane, INV-4). Engancharse en el server: bloquea (decisión 4) | FR-29, FR-30 |
| 3 | ¿`libssh` u OpenSSH? | **`libssh >= 0.9.0`**, enlazada | (a) La única forma que tiene `tmux` de usar OpenSSH es ejecutar su binario por los tres `exec` de `spawn.c:552`, `:567` y `:574`: lo que hoy hace `new-window ssh host` y lo que INV-5 prohíbe. (b) `configure.ac` detecta cada librería opcional con `PKG_CHECK_MODULES` sobre un `.pc` (`libsystemd` en `:508`, `libutf8proc` en `:463`, `jemalloc` en `:670`). (c) El hijo recibe solo los fds 0-2 (`closefrom(STDERR_FILENO + 1)`, `spawn.c:541`): la librería abre su propio socket y convive con el tty en un bucle propio. (d) No hay código SSH que reusar en el árbol. El mínimo 0.9.0 es el de la API de lectura de `known_hosts` `ssh_session_is_known_server`, que en 0.9.0 reemplaza a la ya deprecada `ssh_is_server_known` (la spec nunca escribe `known_hosts`, FR-35) | OpenSSH por `exec`: viola INV-5. Escribir el protocolo a mano: miles de líneas sin base en el árbol. `libssh` sin mínimo: obligaría a soportar la API deprecada. El mínimo sale de la deprecación de `ssh_is_server_known` en 0.9.0, que los proyectos que la usaban documentaron al migrar (p. ej. libvirt); no es código de `tmux` | FR-2, FR-3, FR-4, FR-30 |
| 4 | ¿Cómo se integra con el event loop? | **No se integra**: el cliente corre en el hijo con su propio bucle | El server es de un solo hilo: la base de libevent se crea en `osdep_event_init` (`osdep-linux.c:92`, llamada en `tmux.c:624`), el server la reusa con `event_reinit` (`server.c:198`) y entra a `proc_loop` (`server.c:258`), que llama a `event_loop(EVLOOP_ONCE)` (`proc.c:227`) | Handshake dentro del bucle del server: congela a todos los clientes. Un hilo en el server: viola INV-6 y `tmux` no usa hilos | FR-30, NFR-1 |
| 5 | ¿Auth por claves o por agent? | **Ambas**, agent primero; sin password | `update-environment` incluye `SSH_AUTH_SOCK` y `SSH_AGENT_PID` por default (`options-table.c:1207-1214`), `environ_for_session` copia el entorno de la sesión de `tmux` (`environ.c:253-261`) y `environ_push` lo instala en el hijo antes del bloque nuevo (`spawn.c:544`): el agent llega sin código nuevo, y por eso va primero. La passphrase se puede pedir porque el hijo tiene el pty de `spawn.c:478` como stdin | Password y keyboard-interactive: `tmux` no propaga ningún mecanismo de password, y dejarlos fuera acota los secretos que pasan por el pane a uno solo. Solo agent: no cubre a quien usa clave en archivo | FR-38 … FR-49, FR-64 … FR-66, FR-70 |
| 6 | ¿Qué guarda deja afuera a no-Linux? | **Opt-in** `--enable-native-ssh`, solo Linux, con `AM_CONDITIONAL` y `#ifdef` | `tmux` no usa `#ifdef __linux__` (cero ocurrencias) y sí usa flags de feature: `ENABLE_SIXEL` (`configure.ac:545-552`, `Makefile.am:253-255`) agrega fuentes propias solo con el flag, y `HAVE_SYSTEMD` (`configure.ac:503-527`, `Makefile.am:243-245`) hace lo mismo con detección por `pkg-config`. `PLATFORM` sale del `case "$host_os"` de `configure.ac:1006-1118` | `#ifdef __linux__`: no hay precedente en el árbol. Condicionar solo por `IS_LINUX` (`configure.ac:1122`): activaría el feature en todo Linux sin que nadie lo pida. Activar el feature solo por detectar `libssh`: un Linux con `libssh-dev` instalado cambiaría de binario sin pedirlo (viola INV-1) | FR-1 … FR-9 |

Decisiones menores:

| Pregunta | Elegido | Fundamento | Descartado y por qué | FR |
|---|---|---|---|---|
| ¿Ventana o split? | **Ventana** | reusa `spawn_window` (`spawn.c:209` llama a `spawn_pane`) y evita el manejo de layout de `cmd-split-window.c` | Split: 424 líneas contra 196 de `new-window` por el layout | FR-29 |
| ¿Qué flags acepta? | `-d -F -n -P -t` como `new-window`, más `-i` y `-p` | `.args` de `cmd-new-window.c:41` y sus usos en `:46`, `:83`, `:140`, `:156-157`, `:172-173`; `-i` y `-p` son los únicos datos propios de SSH | `-c` y `-e`: un directorio o entorno locales no aplican a un destino remoto (`arguments.c:240` los rechaza). `-l usuario`, `-o`: `usuario@host` ya lo cubre | FR-14 … FR-22 |
| ¿Host desconocido? | **Se rechaza** | un pane puede crearse con `-d`, sin nadie mirando. El hijo recibe solo los fds 0-2 (`closefrom(STDERR_FILENO + 1)`, `spawn.c:541`): su único canal con el usuario es el tty del pane, y una pregunta ahí quedaría colgada sin un cliente adjunto | TOFU con pregunta: cuelga el pane. TOFU silencioso: acepta man-in-the-middle sin que nadie lo vea | FR-34 |
| ¿Qué estado de salida tienen las fallas? | **`1`** para todas | `server_child_exited` guarda el status en `wp->status` (`server.c:499`) y `format_cb_pane_dead_status` lo expone (`format.c:2288`): ningún otro componente interpreta el valor | Un código por causa: nadie lo consume en ningún FR ni VC. `255` como `ssh`: el código remoto `255` se confundiría con una falla del cliente | BR-3 |
| ¿Timeout de conexión? | **15 s** | `tmux` no tiene ningún timeout de red que reusar: el único `connect()` del árbol es el del socket Unix del cliente (`client.c:126`). El valor es de usabilidad: más corto que el timeout TCP del sistema (en Linux ~2 minutos con `tcp_syn_retries=6`), más largo que un handshake normal en LAN | Sin timeout: el pane queda vivo sin información. El timeout del sistema: ~2 minutos parecen un cuelgue | FR-62, FR-69 |
| ¿Umbrales de NFR-1 y NFR-3? | **1 s** y **30 s** para 200000 líneas | el bucle del server hace `event_loop(EVLOOP_ONCE)` (`proc.c:227`): una iteración bloqueada congela a todos los clientes, y 1 s es el límite en que un cliente percibe el server congelado; 200000 líneas ≈ 1,3 MB en 30 s exige ≥ 44 KB/s, un orden de magnitud bajo lo que da un canal SSH en loopback. `history-limit` vale 2000 por default (`options-table.c:845-851`): NFR-3 lo sube a 250000 para poder contar las 200000 líneas | 100 ms: ruido de la máquina de CI. Sin umbral: no discrimina un bucle bloqueante | NFR-1, NFR-3 |
| ¿Se restaura el tty al salir? | **No se especifica** | cuando el hijo termina, `server_child_exited` marca el pane como terminado por `wp->pid` (`server.c:491-499`) y nadie más lee ese tty: no hay nada observable que restaurar | Restaurarlo: código sin ningún VC posible | — |
| ¿Alias del comando? | **`sshw`** | `new-window` ya usa `.alias = "neww"` (`cmd-new-window.c:39`): el alias abreviado es el mecanismo de `cmd_entry` para los comandos de ventana | Sin alias: nombre de 14 caracteres que se escribe seguido. `ssh`: se confundiría con el binario que INV-5 prohíbe | FR-11 |
| ¿Rutas de `-i` relativas o con `~`? | **Solo absolutas**; `~` no se expande | `tmux` expande `~/` solo en rutas de archivos que él mismo lee (`file.c:47`) y en el parser de comandos sin comillas (`cmd-parse.y:1594` (`yylex_token_tilde`)); un argumento entrecomillado llega literal y el hijo no tiene un cwd con significado para la persona (`spawn.c:478`) | Resolver rutas relativas: dependería del cwd del server, no del de quien invoca | FR-25 |
| ¿Qué claves por defecto y en qué orden? | **`id_ed25519`, luego `id_rsa`**, bajo `$HOME/.ssh` | `HOME` se resuelve con `find_home` (`file.c:50`) y el hijo hereda el entorno por `environ_push` (`spawn.c:544`); el orden pone primero el tipo de clave más nuevo. No hay código de `tmux` que fije los nombres: es el criterio de la spec | Leer `~/.ssh/config` o enumerar `id_*`: fuera de alcance, haría el resultado dependiente del contenido del directorio | FR-40 |
| ¿Qué usuario se usa si el destino no lo trae? | **El del proceso del server** | `tmux` obtiene el usuario con `getpwuid(getuid())` (`format.c:3562`) | `$USER` del entorno: puede no coincidir con el uid real | FR-67 |
| ¿`-i` reemplaza o suma a las claves por defecto? | **Reemplaza** | `new-window` trata `-n` como un valor explícito que desplaza al automático (`cmd-new-window.c:83`, `spawn.c:216-224`): lo nombrado a propósito no se mezcla con lo implícito | Sumar: una clave rechazada se enmascararía con una por defecto | FR-72 |
| ¿En qué orden se informan varios errores de uso? | **Flag, cantidad, `-t`, destino, puerto, ruta de clave** | `args_parse` rechaza flags desconocidos (`arguments.c:240`) y la cantidad de argumentos (`arguments.c:333`, `arguments.c:340`); luego `cmd_find_target` resuelve `-t` antes de ejecutar (`cmd-find.c:1262`); el resto lo valida el comando en ese orden | Cualquier otro orden: los dos primeros ya los fija el parser | FR-28 |
| ¿Un `-i` que es un directorio? | **Error de uso**, como un `-i` inexistente | `cmd_new_window_exec` valida todo antes de llamar a `spawn_window` (`cmd-new-window.c:161`): un error de uso no deja ventana a medio crear | Falla en el pane: la ventana existiría solo para mostrar un error que ya se conoce al invocar | FR-63 |
| ¿Un agent que no sirve corta la conexión? | **No**: el agent es solo la primera fuente | `environ_for_session` copia `SSH_AUTH_SOCK` sin comprobar que el socket responda (`environ.c:253-261`): el entorno no garantiza un agent vivo | Fallar con el agent roto: `tmux` no controla qué agent hereda el server | FR-64, FR-65 |
| ¿Una clave por defecto que no es clave? | **Se salta**, como la ilegible | el hijo solo tiene el tty del pane como canal (`spawn.c:541`): abortar ahí por un archivo ajeno impediría usar la siguiente clave | Abortar con `cannot load key` (FR-49): reservado para `-i`, que la persona nombró a propósito | FR-41, FR-66 |
| ¿Qué mensaje gana: servidor solo con password o sin claves locales? | **`no supported authentication method`** | el hijo recibe solo los fds 0-2 (`closefrom(STDERR_FILENO + 1)`, `spawn.c:541`): su único canal es el tty del pane, y lo que el servidor ofrece se conoce antes de pedir una passphrase en ese tty (FR-43); pedirla para una clave que `sshd` no aceptaría sería inútil | Priorizar `no authentication key found`: mandaría a crear una clave que `sshd` no aceptaría | FR-46, FR-47 |

## Limitaciones conocidas

- **`respawn-pane` sobre un pane SSH** relanza `argv` como un comando común (`spawn.c`):
  con un solo argumento lo pasa a `$SHELL -c` (`spawn.c:562-567`) y con más lo ejecuta con
  `execvp` (`spawn.c:552`). Como `argv[0]` es el destino, lo trata como comando local:
  normalmente falla con `command not found`, y un destino con metacaracteres de shell se
  interpretaría con los permisos de quien ya controla `tmux`. Es confuso, no una escalada. Soportarlo exige guardar en el pane que es SSH (`struct window_pane`,
  `tmux.h:1306`), fuera del alcance.
- `pane_current_command` / `pane_current_path` no reflejan al host remoto: muestran el
  proceso local (hallazgo 4 de las notas).
- **Licencia:** `libssh` se distribuye bajo LGPL, distinta de la licencia de `tmux`
  (`COPYING`: "Permission to use, copy, modify, and distribute…"). Al ser opcional y
  enlazada solo con el flag, el riesgo queda acotado; el empaquetado es una decisión del
  proyecto `tmux`, no de esta spec.
- **Funciones de `libssh`:** las elige `ssh-client.c`; la spec fija solo la versión mínima
  (`>= 0.9.0`, FR-2, FR-4) y el comportamiento observable. Los textos de los mensajes de esta
  spec son contrato de `ssh-client.c`, no de la librería.
- **VC-5.1 y VC-INV2.1** (build en una plataforma no Linux) solo se ejecutan en una máquina no Linux
  (el job de macOS de `tmux/tmux`, o a mano en cualquier macOS o BSD con `make -C regress`); en
  Linux no son ejecutables.
- **BR-4 / VC-BR4.1:** el script `regress/new-ssh-window.sh` exige `sshd` en la máquina que corre
  la regresión; sin `sshd` falla, no se saltea.
- **CI en el repo del TP:** `regress.yml` solo corre en `tmux/tmux` (`if: github.repository == 'tmux/tmux'`, disparo por `schedule` o `workflow_dispatch`). En el repo del TP, BR-4 se verifica ejecutando en Linux `regress/native-ssh-guard.sh`, `regress/native-ssh-build.sh` y `regress/new-ssh-window.sh` con `make -C regress`; VC-5.1 y VC-INV2.1, a mano en una máquina no Linux.
- **API de `libssh` para el agent:** la spec no depende de qué función de `libssh` obtiene la identidad del agent ni de si la conexión es bloqueante; fija solo el comportamiento observable (FR-38, FR-64, FR-65, FR-62).
