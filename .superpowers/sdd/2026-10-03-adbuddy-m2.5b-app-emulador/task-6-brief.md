### Tarea 6: documentación — qué queda automatizado y qué manual

**Implementador:** `adbuddy-implementer` (Sonnet). **Revisor:** `adbuddy-reviewer`.

Los textos dependen de los rulings de la Tarea 1 (variante y `tcpipOnEmulator`) y del resultado de las tareas 2-4. Usa el bloque de la variante que se hizo. Si una validación quedó `BLOQUEADO` y el controlador la dejó manual, escribe en su lugar "Sigue manual: <motivo del ruling>".

**Ficheros:**
- Modificar: `docs/superpowers/plans/2026-09-30-adbuddy-m2-app-base.md` (sección "Validaciones manuales pendientes")
- Modificar: `docs/superpowers/plans/pendientes-m3-m5.md` (sección M2.5b y la línea de `LocalesTest` en M6c)
- Modificar: `docs/ci-self-hosted.md` (fila `CI_RUNNER_EMULATOR`)
- Modificar: `.superpowers/sdd/2026-10-03-adbuddy-m2.5b-emulador/progress.md` (cierre)

- [ ] **Paso 1: plan de M2.** En "Validaciones manuales pendientes (necesitan al usuario y un móvil)", añade al final de cada punto, tras su último punto y seguido, el texto indicado (el resto de cada punto no cambia):

  - **1:** ` **Automatizada en M2.5b:** `TokenProviderTest` corre en el emulador (`connectedDebugAndroidTest`) y `host/internal/emutest` comprueba que el shell recibe respuesta; el job `emulator` de la CI los ejecuta.`
  - **2:** ` **Automatizada en M2.5b para Android 17** (`TestTheHandshakeOnStdinReachesTheApp`, en el emulador). **Sigue manual en Android 11-13:** exigiría otra imagen de sistema, y esa instalación la decide el usuario (spec de M2.5, ruling de M2.5b).`
  - **3:** ` **Automatizada en M2.5b** (`TestTheServiceIsInTheForegroundAfterTheHandshake`, Android 17 en el emulador).`
  - **4:** ` **Sigue manual** (M2.5b): necesita un móvil de otro fabricante.`
  - **5:** ` **Sigue manual** (M2.5b): necesita la pantalla de bloqueo y a una persona.`
  - **6, variante A o A' con `tcpipOnEmulator = true`:** ` **Automatizada en M2.5b**, con y sin el modo `tcpip` (`TestCutAccessAndReactivate` lanza `CutAccessOnEmulatorTest`, que corta y reactiva con el código de la app, y comprueba desde el PC que adb se va y vuelve).`
  - **6, variante A o A' con `tcpipOnEmulator = false`:** ` **Automatizada en M2.5b sin el modo `tcpip`** (`TestCutAccessAndReactivate` lanza `CutAccessOnEmulatorTest`, que corta y reactiva con el código de la app, y comprueba desde el PC que adb se va y vuelve). **Con `tcpip` sigue manual:** <motivo del ruling de la Tarea 1, S5>.`
  - **6, variante "A en local" o "A' en local":** el texto de A con el `tcpipOnEmulator` que toque, cambiando `**Automatizada en M2.5b**` por `**Automatizada en M2.5b en local**` y añadiendo al final: ` Solo corre con `ADBUDDY_EMULATOR_CUT=1` en un emulador de desarrollo; la CI no la ejecuta: <motivo del ruling de la Tarea 1, S7>.`
  - **6, variante B:** ` **Automatizada en M2.5b solo la mitad de `adb_wifi_enabled`** (`CutAccessWifiTest`: la app, con el permiso que concede `setup`, escribe y lee el ajuste). **Sigue manual el corte de `adb_enabled` y Reactivar:** <motivo del ruling de la Tarea 1: cuál de (i)-(v) falló, con la medida>.`
  - **7:** ` **Sigue manual** (M2.5b, ruling del plan): automatizarla en el emulador exigiría preparar `config.toml` a mano (sin el emparejamiento por QR o código), simular los toques de consentimiento y Permitir (`uiautomator dump` e `input tap`) y un agente real con credenciales en la CI; cuesta mucho y demuestra poco más que lo ya cubierto. Las piezas sí están cubiertas en el emulador: instalación y verificación del APK, handshake, servicio en primer plano y Cortar acceso.`
  - **8:** ` **Sigue manual** (M2.5b): necesita el bouncer y a una persona.`
  - **9:** ` **Sigue manual** (M2.5b): necesita un reloj o Phone Link.`
  - **10:** ` **Automatizada en M2.5b** (`AdbWifiObserverTest`: un cambio desde el shell y otro con la identidad de permisos del shell llegan al observador de la app; además, la app puede leer el ajuste, que `CutAccess` guarda antes de cortar).`

- [ ] **Paso 2: pendientes, sección M2.5b.** En `docs/superpowers/plans/pendientes-m3-m5.md`, sustituye desde la línea que empieza por `Spec: \`docs/superpowers/specs/2026-09-30-adbuddy-m2.5-clasificador-lista-blanca-design.md\` (§7). M2.5a` hasta la línea `- \`android/local.properties\` no se versiona: …` (ambas incluidas; la subsección "Ampliaciones de la lista blanca" se queda) por:

  ```markdown
  **HECHO en M2.5b** (plan `docs/superpowers/plans/2026-10-03-adbuddy-m2.5b-app-emulador.md`): los instrumentados corren en el emulador (`connectedDebugAndroidTest`, solo con `ANDROID_SERIAL=emulator-<puerto>`) y `host/internal/emutest` prueba el host contra la app; el job `emulator` de la CI ejecuta los dos antes del oráculo. Validaciones de M2 automatizadas: 1, 2 (Android 17), 3, 10 y la 6 según el ruling del spike (ver el plan de M2). Siguen manuales: 4, 5, 7, 8, 9, la 2 en Android 11-13 y lo que el spike dejó de la 6.

  - **El emulador de desarrollo** usa el AVD `adbuddy-dev`, consola 5600 y servidor adb 5060 (`ADBUDDY_EMU_*` de `scripts/emulator.sh`); la CI, `adbuddy-ci-api37`, 5590 y 5049. Nunca dos a la vez en el PC del runner (~5 GB de RAM cada uno). Los dos puertos están fuera del rango que escanea adb (5555-5585): tras perder `adbd`, el emulador vuelve a su servidor solo por los reintentos de adb (~60 s), y por eso Cortar acceso espera 15 s antes de Reactivar.
  - **Cada PR que toca `android/**` lanza también el oráculo** (unos 15 min más los tests de la app), porque es un solo job. Si pesa, separar los tests de la app en otro job del mismo workflow con sus rutas.
  - `scripts/emulator.sh` escribe el log del emulador en `${TMPDIR:-/tmp}/adbuddy-emulator.log` sea cual sea el AVD: dos emuladores a la vez se pisan el log (de puffin).
  - `android/local.properties` no se versiona: crearlo con `sdk.dir=C\:/Users/<usuario>/AppData/Local/Android/Sdk`.
  ```

  Si la Tarea 1 encontró que el fallo de `surfaceflinger` ya no ocurre, añade al final de ese bloque: `- El fallo de \`surfaceflinger\` de la imagen \`android-37.0\` que anotó M2.5a no aparece con el emulador <versión> (spike de M2.5b, S1).` (Si ocurre, el plan se detuvo en la Tarea 1 y este paso no llega.)

- [ ] **Paso 3: pendientes, M6c.** Sustituye

  ```text
  - **`LocalesTest` en el emulador:** `emulator.yml` solo ejecuta el oráculo del clasificador, no los `androidTest`; `android.yml` solo los compila. `scripts/emulator.sh start` y `gradle :app:connectedDebugAndroidTest` con el emulador arrancado.
  ```

  por

  ```text
  - **HECHO en M2.5b:** `LocalesTest` corre en el emulador con el resto de los instrumentados (`emulator.yml`).
  ```

- [ ] **Paso 4: `docs/ci-self-hosted.md`.** En la fila de `CI_RUNNER_EMULATOR`, sustituye

  ```text
  | Android SDK, the AVD `adbuddy-ci-api37`, hardware acceleration; a Linux runner needs passwordless `sudo` or an accessible `/dev/kvm`. |
  ```

  por

  ```text
  | Android SDK, the AVD `adbuddy-ci-api37`, hardware acceleration, and JDK 17 for the app's tests (Gradle 9.8 is downloaded by `setup-gradle`); a Linux runner needs passwordless `sudo` or an accessible `/dev/kvm`. |
  ```

  Si el texto "que hay" no coincide (puffin pudo cambiarlo), detente y explícalo.

- [ ] **Paso 5: comprueba.** `git diff --stat` solo muestra los tres ficheros de docs; `grep -n 'Automatizada en M2.5b\|Sigue manual' docs/superpowers/plans/2026-09-30-adbuddy-m2-app-base.md | wc -l` → `10` (un texto, en la misma línea, por punto).

- [ ] **Paso 6: cierre del ledger.** Añade al ledger una línea por tarea con su commit y el veredicto del revisor, y la lista de "Validaciones manuales pendientes" de este plan tal como quedaron.

- [ ] **Paso 7: commit.**

  ```bash
  git add docs/superpowers/plans/2026-09-30-adbuddy-m2-app-base.md docs/superpowers/plans/pendientes-m3-m5.md docs/ci-self-hosted.md
  git add -f .superpowers/sdd/2026-10-03-adbuddy-m2.5b-emulador/progress.md
  git commit -m "docs(m2.5b): validaciones de M2 automatizadas y las que siguen manuales

  Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
  ```

---


---
# Notas del controlador (MANDAN sobre el texto de arriba; es de antes del spike)
- Hechos reales (ledger `progress.md`, léelo): variante **"A en local"** para la validación 6 con tcpipOnEmulator=true (prueba débil de tcpip: adbd del emulador escucha siempre en 5555; el motivo S7 es que tras un corte que nadie deshace el AVD arranca sin adb y hay que borrar sus datos); **Android 14 (API 34, imagen android-34)**, NO Android 17: donde el texto dice "Android 17 en el emulador", di "Android 14 en el emulador; en Android 17 sigue manual en el Pixel". Los instrumentados corren con `am instrument` (`TestInstrumentedSuite`), NO con `connectedDebugAndroidTest` (la guarda de Gradle lo niega: UTP habla con el servidor adb 5037): quita esas menciones del texto del plan.
- El job `emulator` de la CI corre oráculo en android-37 y luego emutest en android-34 (commit e29c5d4); `TestCutAccessAndReactivate` solo en local (`ADBUDDY_EMULATOR_CUT=1`).
- **Añade** en `pendientes-m3-m5.md`, sección M2.5b, la lista «Validaciones que solo se pueden hacer en el Pixel» con un motivo cada una: corte de la depuración inalámbrica real (Wireless debugging/`adb_wifi_enabled` y el puerto tcpip), que la app lea `adb_wifi_enabled` en Android 17 con targetSdk 37, ajustes restringidos, servicio en primer plano en Android 17, `content write` por stdin en Android 17, edge-to-edge/permiso de red local, y lo que f3 anotó como manual (validaciones 4, 5, 7, 8, 9 del plan de M2).
- Spec/ruling de f3: automatizar al máximo; también lista las validaciones ampliadas aún NO hechas (22, 23, 26, 28, 29, 32, 35, 30, 24, 25/33, 31) como "pendientes de automatizar (tareas T3b/T4b, a decidir)" sin inventar su estado: si no sabes cuáles son, busca "Validaciones manuales pendientes" en los planes y no inventes números.
- Ficheros: el ledger de la ruta del texto ya no existe con ese nombre: es `.superpowers/sdd/2026-10-03-adbuddy-m2.5b-app-emulador/progress.md` (anota ahí el cierre de T6). Textlint/docs guard `host/internal/cli/docs_test.go`: ejecuta `go test ./internal/cli -run Docs` en host/.
- Informe en tu respuesta final; commits con trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; sin push; sin subagentes; sin emulador. BASE = HEAD actual.
