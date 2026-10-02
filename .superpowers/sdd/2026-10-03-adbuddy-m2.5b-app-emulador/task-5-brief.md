### Tarea 5: la app en el job `emulator` de la CI

**Implementador:** `adbuddy-implementer` (Sonnet). **Revisor:** `adbuddy-reviewer`.

Cambio mínimo sobre el `emulator.yml` de `main` (ya con el PR de puffin): rutas, Gradle, compilar la app antes de arrancar el emulador y dos pasos tras arrancarlo, antes del oráculo (el oráculo para y rearranca el sistema Android con `su 0 stop`/`start`, y la app lo necesita en marcha). El job sigue siendo uno: un job a la vez en el runner.

**Ficheros:**
- Modificar: `.github/workflows/emulator.yml`, `.github/workflows/android.yml` (solo el nombre de un paso)

**Interfaces:**
- Consume: la guarda de la Tarea 2 (`ANDROID_SERIAL=emulator-*`), `./internal/emutest` (tareas 3 y 4), las variables que `scripts/emulator.sh start` escribe en `$GITHUB_ENV` (`ADBUDDY_EMULATOR_SERIAL`, `ADBUDDY_EMULATOR_ADB`, `ANDROID_ADB_SERVER_PORT`).

- [ ] **Paso 1: comprueba la base.** `grep -n 'ADBUDDY_EMU_AVD\|Clear a leftover emulator\|Record the oracle' .github/workflows/emulator.yml` debe mostrar las tres. Si el fichero de `main` no coincide con los textos "que hay" de los pasos siguientes, detente y explícalo (puffin pudo cambiarlo otra vez).

- [ ] **Paso 2: comentario de cabecera y rutas.** En `.github/workflows/emulator.yml` sustituye

  ```yaml
  # Tests that need a real Android: the classifier's oracle (what the phone's
  # mksh does with each differential case) and the read-only examples of the
  # skills (they must run on the phone's toybox). Like host.yml: only on pull
  # requests and pushes to main, and only when what it checks changes.
  on:
    pull_request:
      paths: ["host/internal/classify/**", "host/internal/workspace/skills/**", "host/internal/cli/skills_*_test.go", "scripts/emulator.sh", ".github/workflows/emulator.yml"]
    push:
      branches: [main]
      paths: ["host/internal/classify/**", "host/internal/workspace/skills/**", "host/internal/cli/skills_*_test.go", "scripts/emulator.sh", ".github/workflows/emulator.yml"]
  ```

  por

  ```yaml
  # Tests that need a real Android: the classifier's oracle (what the phone's
  # mksh does with each differential case), the read-only examples of the
  # skills (they must run on the phone's toybox) and the app's tests (its
  # instrumented tests and the host against it, M2.5b). Like host.yml: only on
  # pull requests and pushes to main, and only when what it checks changes.
  on:
    pull_request:
      paths: ["host/internal/classify/**", "host/internal/workspace/skills/**", "host/internal/cli/skills_*_test.go", "host/internal/emutest/**", "host/internal/phoneapp/**", "host/internal/adb/**", "host/internal/bridge/**", "android/**", "scripts/emulator.sh", ".github/workflows/emulator.yml"]
    push:
      branches: [main]
      paths: ["host/internal/classify/**", "host/internal/workspace/skills/**", "host/internal/cli/skills_*_test.go", "host/internal/emutest/**", "host/internal/phoneapp/**", "host/internal/adb/**", "host/internal/bridge/**", "android/**", "scripts/emulator.sh", ".github/workflows/emulator.yml"]
  ```

- [ ] **Paso 3: Gradle**, como en `android.yml`. Sustituye

  ```yaml
        - uses: actions/setup-java@cf277c60eb25467037889841efdb72551f06f6c3 # v4
          with:
            distribution: temurin
            java-version: "17"
        - name: Point the emulator at the AVD (Linux runners)
  ```

  por

  ```yaml
        - uses: actions/setup-java@cf277c60eb25467037889841efdb72551f06f6c3 # v4
          with:
            distribution: temurin
            java-version: "17"
        - uses: gradle/actions/setup-gradle@ed408507eac070d1f99cc633dbcf757c94c7933a # v4
          with:
            gradle-version: "9.8.0"
            cache-disabled: ${{ runner.environment != 'github-hosted' }}
        - name: Point the emulator at the AVD (Linux runners)
  ```

- [ ] **Paso 4: compilar antes de arrancar el emulador** (así no comparten la RAM). Sustituye

  ```yaml
        - name: Clear a leftover emulator of this job
          run: scripts/emulator.sh stop
  ```

  por

  ```yaml
        # compileSdk = 37: its SDK package is "platforms;android-37.0", as in android.yml.
        # Kotlin in-process: no Kotlin daemon is left running on a self-hosted runner.
        - name: Build the app and its instrumented tests
          working-directory: android
          run: |
            sm="$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager"
            [ -e "$sm" ] || sm="$sm.bat"
            yes | "$sm" "platforms;android-37.0" > /dev/null
            gradle --no-daemon -Pkotlin.compiler.execution.strategy=in-process :app:assembleDebug :app:assembleDebugAndroidTest
        - name: Clear a leftover emulator of this job
          run: scripts/emulator.sh stop
  ```

- [ ] **Paso 5: los tests de la app, antes del oráculo.** Sustituye

  ```yaml
        - name: Record the oracle
  ```

  por

  ```yaml
        # Before the oracle, which stops and restarts Android's framework.
        # ANDROID_SERIAL keeps Gradle on the emulator (android/app/build.gradle.kts
        # refuses anything else); without the variables the tests would skip
        # and the steps would pass.
        - name: The app's instrumented tests on the emulator
          working-directory: android
          run: test -n "$ADBUDDY_EMULATOR_SERIAL" && ANDROID_SERIAL="$ADBUDDY_EMULATOR_SERIAL" gradle --no-daemon -Pkotlin.compiler.execution.strategy=in-process :app:connectedDebugAndroidTest
        - name: The host against the app on the emulator
          working-directory: host
          run: test -n "$ADBUDDY_EMULATOR_SERIAL" && test -n "$ADBUDDY_EMULATOR_ADB" && go test -timeout 20m -count=1 -v ./internal/emutest
        - name: Record the oracle
  ```

- [ ] **Paso 6: `android.yml`.** Sustituye

  ```yaml
        - name: "Unit tests, debug build, lint, and the instrumented tests compile (they run on an emulator by hand: see android/README.md)"
  ```

  por

  ```yaml
        - name: "Unit tests, debug build, lint, and the instrumented tests compile (emulator.yml runs them)"
  ```

- [ ] **Paso 7: valida.** Desde `host/` (para la caché de módulos), con rutas absolutas:

  ```bash
  cd host && go run github.com/rhysd/actionlint/cmd/actionlint@latest "$(cd .. && pwd)/.github/workflows/emulator.yml" "$(cd .. && pwd)/.github/workflows/android.yml"; echo "exit $?"
  ```

  Esperado: sin salida y `exit 0`. Además, `git diff -- .github/workflows/emulator.yml` solo muestra los cuatro bloques de los pasos 2-5.

- [ ] **Paso 8: ensayo local de los pasos nuevos** (lo que hará el runner, con el emulador de desarrollo; CI-LIBRE y SIN-EMULADOR antes de arrancar, CI-LIBRE antes de cada orden larga): arranca el emulador y ejecuta los cuerpos de los tres pasos nuevos tal cual, con `bash -e`:

  ```bash
  cd android && gradle --no-daemon -Pkotlin.compiler.execution.strategy=in-process :app:assembleDebug :app:assembleDebugAndroidTest && cd ..
  EMU; cd android && test -n "$ADBUDDY_EMULATOR_SERIAL" && ANDROID_SERIAL="$ADBUDDY_EMULATOR_SERIAL" gradle --no-daemon -Pkotlin.compiler.execution.strategy=in-process :app:connectedDebugAndroidTest; echo "exit $?"
  EMU; cd host && test -n "$ADBUDDY_EMULATOR_SERIAL" && test -n "$ADBUDDY_EMULATOR_ADB" && go test -timeout 20m -count=1 -v ./internal/emutest; echo "exit $?"
  ```

  Esperado: `exit 0` en los dos. Anota la duración de cada uno en el informe (el job ronda hoy los 15 min). Para el emulador.

- [ ] **Paso 9: commit.**

  ```bash
  git add .github/workflows/emulator.yml .github/workflows/android.yml
  git commit -m "ci(emulator): los instrumentados de la app y emutest antes del oráculo; android/** en sus rutas

  Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
  ```

  La ejecución real en el runner llega con el PR (la abre el coordinador). Si el runner falla en un paso nuevo y no en el ensayo local, es un hallazgo de la tarea: se arregla en otra ronda.

---


---
# Notas del controlador (MANDAN sobre el texto de arriba; el texto es de antes del spike)
- El `emulator.yml` de main ya cambió (puffin: hardening A5, mDNS off, puertos propios). Parte del fichero actual de la rama (ya integrada con main), no de los textos "que hay" del plan.
- **Imagen:** el oráculo se queda en android-37 (job actual, AVD adbuddy-ci-api37, puerto 5590). Los tests de la app/emutest van con **android-34** (la 37 entra en bucle de surfaceflinger con los tres GPU probados; ledger). Dentro del MISMO job y secuencialmente (un emulador a la vez, la RAM no da para dos): oráculo + skills en la 37 → `scripts/emulator.sh stop` → instalar `system-images;android-34;google_apis;x86_64` → arrancar con `ADBUDDY_EMU_IMAGE='system-images;android-34;google_apis;x86_64' ADBUDDY_EMU_AVD=adbuddy-ci-api34 ADBUDDY_EMU_PORT=5590 ADBUDDY_EMU_ADB_PORT=5049 ADBUDDY_EMU_STABLE_SECS=120` (mismo puerto que usa el job, otro AVD) → `go test ./internal/emutest` → `scripts/emulator.sh stop`. Comprueba en `scripts/emulator.sh` cómo toma `ADBUDDY_EMU_IMAGE`/`STABLE_SECS` (leer antes de escribir el YAML).
- **Gradle:** NUNCA tareas `connected*` (la guarda de Gradle las niega siempre; UTP habla con el servidor 5037). Compilar los APK con `gradle :app:assembleDebug :app:assembleDebugAndroidTest` (sin dispositivo) ANTES de arrancar el emulador; `TestInstrumentedSuite` los instala por el adb propio. Sin `ANDROID_SERIAL` en el entorno de Gradle. `TestCutAccessAndReactivate` NO se ejecuta en CI: no pongas `ADBUDDY_EMULATOR_CUT` (ruling "A en local"); `TestInstrumentedSuite` ya espera ver ese salto.
- Rutas del `on.pull_request.paths`: añade `android/**`, `host/internal/emutest/**`, `host/internal/phoneapp/**`, `.github/workflows/android.yml`? (solo lo que de verdad cambie lo que el job comprueba). Acciones fijadas por SHA como las existentes; sin acciones nuevas si se puede (setup-gradle no: el runner ya tiene Gradle/Android SDK según `docs/ci-self-hosted.md`; compruébalo y, si no, deja un ruling).
- No ejecutes el workflow ni el emulador (no hay forma local de correr Actions). Valida el YAML con `python -c "import yaml; yaml.safe_load(open(...))"` o `actionlint` si existe, y relee los pasos buscando variables mal pasadas (`scripts/emulator.sh start | sed ... >> $GITHUB_ENV` pisa ADBUDDY_* del paso anterior: razona qué variables quedan al segundo arranque y limpia lo necesario).
- Informe en tu respuesta final; commits con trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`; sin push; sin subagentes. Rama feat/m2.5b-app-emulador, BASE = HEAD actual.
