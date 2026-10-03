# Runners: pasos exactos para las tres decisiones del usuario

Origen: `docs/superpowers/plans/2026-10-03-optimizar-runners-local.md` (P3 y P5) y `pendientes-m3-m5.md` (A5-1, A5-3). Aquí solo van los pasos; decidir es del usuario y cada bloque es independiente. Reglas del entorno que se asumen: los scripts de arranque son `start-runner-windows.cmd/.ps1` y `start-runner-linux.cmd/.ps1` (carpeta de runners del workspace, fuera del repositorio); el runner Windows vive hoy en `%USERPROFILE%\actions-runner`, con el nombre `<PC>-adbuddy-local` y la etiqueta `adbuddy-local`.

| Decisión | Qué cambia | Cuánto cuesta | Cómo se deshace |
| --- | --- | --- | --- |
| 1. Segundo runner Windows (P3) | Los jobs `host` dejan de esperar al emulador | ~30 min, unos GB de disco | Parar, `config.cmd remove`, borrar carpeta; volver la variable |
| 2. Arrancar runners al iniciar sesión (P5) | Tras un reinicio, los runners vuelven solos al entrar en Windows | 2 accesos directos | Borrar los accesos directos |
| 3. Cuenta de Windows dedicada (A5-1, A5-3) | Los jobs dejan de poder leer tu adbkey, tus claves SSH, el token de `gh` y tu SDK | ~1 hora | Volver a registrar el runner en tu cuenta |

Orden recomendado: 3 antes que 1 (un segundo runner en tu cuenta duplica la exposición de A5-1). El 2 con la cuenta dedicada necesita pasos propios (ver al final del bloque 3).

Los comandos `gh` de abajo se ejecutan con la cuenta hybriqtech (`GH_CONFIG_DIR="$HOME/.config/gh-hybriqtech"`; en `gh variable` añade `-R hybriqtech/adbuddy`, `gh api` ya lleva el repositorio en la ruta). Nunca guardes el token de registro en un fichero, un log ni un commit.

## 1. Segundo runner Windows solo para `host` (P3)

Primero hay que separar los runners: con las etiquetas de hoy, un job de emulador podría caer en el segundo y habría dos emuladores con los mismos puertos fijos (5590/5049 en el oráculo, 5592/5051 en la app) y sin RAM para ambos.

1. Etiqueta extra en el runner actual (no hace falta pararlo). Mira su id y anótalo:

   ```sh
   gh api repos/hybriqtech/adbuddy/actions/runners --jq '.runners[] | [.id,.name,(.labels|map(.name)|join(","))] | @tsv'
   gh api -X POST repos/hybriqtech/adbuddy/actions/runners/ID/labels -f 'labels[]=adbuddy-emulator'
   ```

2. Apunta el emulador solo a esa etiqueta (la variable la leen los dos workflows de emulador):

   ```sh
   gh variable list -R hybriqtech/adbuddy
   gh variable set CI_RUNNER_EMULATOR --body '["self-hosted","windows","adbuddy-emulator"]' -R hybriqtech/adbuddy
   ```

3. Carpeta del segundo runner, con la misma versión que el primero (PowerShell):

   ```powershell
   (Get-Item "$env:USERPROFILE\actions-runner\bin\Runner.Listener.exe").VersionInfo.ProductVersion   # p. ej. 2.xxx.y
   mkdir "$env:USERPROFILE\actions-runner-2"; cd "$env:USERPROFILE\actions-runner-2"
   Invoke-WebRequest "https://github.com/actions/runner/releases/download/vX/actions-runner-win-x64-X.zip" -OutFile r.zip   # X = esa versión
   (Get-FileHash r.zip -Algorithm SHA256).Hash   # compáralo con el SHA-256 de las notas de esa release
   Expand-Archive r.zip . ; del r.zip
   ```

4. Registro (token en memoria, nombre distinto, sin la etiqueta `adbuddy-emulator`):

   ```sh
   TOKEN=$(gh api -X POST repos/hybriqtech/adbuddy/actions/runners/registration-token --jq .token)
   ```

   ```bat
   cd %USERPROFILE%\actions-runner-2
   config.cmd --unattended --url https://github.com/hybriqtech/adbuddy --token TOKEN --name <PC>-adbuddy-host2 --labels self-hosted,windows,adbuddy-local
   ```

5. `.env` y `.path`. Un runner solo de `host` no necesita SDK ni JDK: no pongas `ANDROID_HOME`, `ANDROID_SDK_ROOT` ni `JAVA_HOME`. Con cachés propias dentro de su carpeta:

   ```text
   GOMODCACHE=C:\Users\<usuario>\actions-runner-2\_gomod
   GOCACHE=C:\Users\<usuario>\actions-runner-2\_gocache
   ```

   ```powershell
   Copy-Item "$env:USERPROFILE\actions-runner\.path" "$env:USERPROFILE\actions-runner-2\.path"
   ```

6. Script de arranque: copia los del primero cambiando la carpeta.

   ```powershell
   cd <carpeta de los start-runner-*>
   (Get-Content start-runner-windows.ps1) -replace "'actions-runner'","'actions-runner-2'" | Set-Content start-runner-windows-2.ps1
   Set-Content start-runner-windows-2.cmd '@echo off','powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0start-runner-windows-2.ps1"','pause'
   ```

   Lánzalo con `start-runner-windows-2.cmd` y deja la consola abierta.
7. Comprobación antes de fiarte: con un job de emulador en marcha, lanza un job `host` (un push a una rama con PR) y mira que `go test ./...` pasa en el segundo (`gh run list`, columna del runner en el job). La CPU compartida podría volver flaky algún test con tiempos.

Deshacer: para el runner, `config.cmd remove --token TOKEN` (otro token nuevo), borra `actions-runner-2`, y la variable puede quedarse con la etiqueta nueva.

## 2. Arrancar los runners al iniciar sesión (P5)

Solo arrancan cuando entras en Windows (sin sesión no corre nada). Mientras el runner Windows siga en tu cuenta, esto lo deja encendido siempre que estés conectado: `ci-self-hosted.md` §6 pide no dejar variables `CI_*` apuntando a él sin vigilar. Es tu decisión.

```powershell
$runners = '<carpeta de los start-runner-*>'
$startup = [Environment]::GetFolderPath('Startup')
$sh = New-Object -ComObject WScript.Shell
foreach ($n in 'start-runner-windows','start-runner-linux') {     # añade 'start-runner-windows-2' si hiciste el bloque 1
  $l = $sh.CreateShortcut("$startup\$n.lnk")
  $l.TargetPath = "$runners\$n.cmd"; $l.WorkingDirectory = $runners; $l.WindowStyle = 7   # 7 = minimizada
  $l.Save()
}
```

Para probarlo sin reiniciar: `explorer shell:startup` y doble clic en un acceso. Deshacer: borrar los `.lnk` de esa carpeta. El runner Linux mantiene viva la VM de WSL con su proceso `wsl.exe`, y no sobrevive a un cierre de sesión.

## 3. Cuenta de Windows dedicada (A5-1) y SDK propio (A5-3)

Objetivo: el runner corre con una cuenta estándar que no tiene tu adbkey (`C:\Users\<tú>\.android`), ni tus claves SSH, ni el token de `gh`, ni tu SDK. Es la única barrera real de la adbkey (`ci-self-hosted.md` §6).

PowerShell como administrador:

1. Cuenta estándar (no administradora; `New-LocalUser` no la mete en ningún grupo, hay que añadir `Users`):

   ```powershell
   New-LocalUser -Name adbuddy-ci -Password (Read-Host -AsSecureString "Contraseña") -PasswordNeverExpires -UserMayNotChangePassword -Description "Runners de CI de ADBuddy"
   Add-LocalGroupMember -Group Users -Member adbuddy-ci
   icacls "C:\Users\<usuario>"   # no debe aparecer Users, Everyone ni Authenticated Users; lo normal en Windows es SYSTEM, Administrators y tú
   ```

2. Carpeta `C:\ci` propiedad de los administradores. La cuenta lee y ejecuta; solo escribe donde un job necesita escribir:

   ```powershell
   mkdir C:\ci
   icacls C:\ci /inheritance:r /grant "Administrators:(OI)(CI)F" "SYSTEM:(OI)(CI)F" "adbuddy-ci:(OI)(CI)RX"
   foreach ($d in 'Sdk','actions-runner\_work','actions-runner\_diag','actions-runner\_gradle','actions-runner\_gomod','actions-runner\_gocache') {
     mkdir "C:\ci\$d" -Force | Out-Null
     icacls "C:\ci\$d" /grant "adbuddy-ci:(OI)(CI)M"
   }
   ```

   `Sdk` ha de ser escribible porque los workflows ejecutan `sdkmanager` ahí (así un job nunca toca tu SDK; tampoco uses `C:\ci\Sdk` con ADBuddy a diario). `bin`, `.env`, `.path` y `config.cmd` quedan de solo lectura para la cuenta.
3. SDK propio mínimo: descarga `commandlinetools-win` de developer.android.com, descomprímelo en `C:\ci\Sdk\cmdline-tools\latest` y, desde tu cuenta (tu JDK 17 sirve), instala lo imprescindible. Los workflows instalan el resto (emulador, imágenes, plataforma 37):

   ```bat
   C:\ci\Sdk\cmdline-tools\latest\bin\sdkmanager --sdk_root=C:\ci\Sdk "platform-tools" "cmdline-tools;latest"
   ```

4. El runner en `C:\ci\actions-runner` (mismos pasos 3 del bloque 1 con esa carpeta, ejecutados como administrador y con `Expand-Archive` ahí) y su registro, con las mismas etiquetas que el actual más `adbuddy-emulator`, y nombre `<PC>-adbuddy-ci`:

   ```bat
   cd C:\ci\actions-runner
   config.cmd --unattended --url https://github.com/hybriqtech/adbuddy --token TOKEN --name <PC>-adbuddy-ci --labels self-hosted,windows,adbuddy-local,adbuddy-emulator
   ```

   Para no pelear con dos runners con la misma etiqueta de emulador, para el runner antiguo (o quita `adbuddy-emulator` de él con `DELETE .../runners/ID/labels/adbuddy-emulator`) antes de arrancar el nuevo.
5. `.env` (barras `/` en `ANDROID_HOME`) y `.path` (según §3 pasos 4 y 5 de `ci-self-hosted.md`, con `C:\ci`):

   ```text
   ANDROID_HOME=C:/ci/Sdk
   ANDROID_SDK_ROOT=C:/ci/Sdk
   GRADLE_USER_HOME=C:\ci\actions-runner\_gradle
   GOMODCACHE=C:\ci\actions-runner\_gomod
   GOCACHE=C:\ci\actions-runner\_gocache
   ```

   Sin `JAVA_HOME`: `setup-java` instala el JDK 17 dentro del job. Git y Go ya están en `C:\Program Files` (legibles por todos).
6. Script de arranque: copia `start-runner-windows.ps1` a `C:\ci\start-runner-windows.ps1` cambiando la línea `WorkingDirectory` a `'C:\ci\actions-runner'` y su `.cmd`; ponlos en `C:\ci` (propiedad de administradores, la cuenta no puede cambiarlos).
7. Lanzarlo desde tu sesión, sin cerrar la tuya (la primera vez crea el perfil de la cuenta; saldrá una consola con esa identidad):

   ```bat
   runas /user:adbuddy-ci /profile "cmd /k C:\ci\start-runner-windows.cmd"
   ```

8. Comprobaciones, por este orden:
   - En esa consola, `dir C:\Users\<usuario>` debe dar acceso denegado.
   - Lanza un job `host` y uno de emulador (un push a una rama con PR; el emulador se crea con su propio `.android` en el perfil de la cuenta, así que la primera vez tarda más). Que el job de emulador arranque bajo esta cuenta es lo que no se ha probado (aceleración WHPX para una cuenta estándar, ventana del emulador bajo `runas`): si falla, vuelve a la cuenta antigua y apunta el fallo en `pendientes-m3-m5.md`.
9. Cuando todo pase: para el runner antiguo, `config.cmd remove` (token nuevo) y borra `%USERPROFILE%\actions-runner` (guarda credenciales del runner). A5-1 y A5-3 quedan cerradas y hay que actualizar `ci-self-hosted.md` §6 ("Requirement" cumplido) y `pendientes-m3-m5.md`.

Arrancar este runner al iniciar sesión (bloque 2) no sirve tal cual: el acceso directo tendría que pedir la contraseña de `adbuddy-ci` con `runas`. Las salidas son (a) un acceso que ejecute el `runas` y tú escribes la contraseña, o (b) una tarea programada `schtasks /Create /SC ONSTART /RU adbuddy-ci /RP * /TN adbuddy-runner /TR C:\ci\start-runner-windows.cmd` (corre sin sesión; no probado con el emulador, que podría necesitar un escritorio).

Sin probar (no se ha creado ninguna cuenta ni runner): el emulador bajo la cuenta estándar, `runas /profile` con el script, la tarea programada, y que el runner pueda actualizarse solo con `bin` de solo lectura (si exige una versión nueva, reextrae el zip de la release como administrador sobre `bin` y `externals`).
