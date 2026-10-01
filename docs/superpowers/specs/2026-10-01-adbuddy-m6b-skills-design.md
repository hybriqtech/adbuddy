# ADBuddy M6b · Skills de contexto

Fecha: 2026-10-01 · Estado: borrador para plan · Rama: `m6a` (M6b se apila sobre M6a y va en el mismo PR)

Parte de la spec v1 (§7, "Skills de la v1") y de la de M6a (§2, "Las skills de contexto (M6b), la i18n y el README completo van después").

## 1. Alcance

M6b entrega **las nueve skills de la v1** como recetas que el agente carga bajo demanda, y la maquinaria para instalarlas, refrescarlas y comprobar que no mienten:

| Skill | Qué hace |
|---|---|
| `device-profile` | Resumen del móvil: almacenamiento, batería, apps y ajustes relevantes |
| `debloat` | Detectar bloatware por fabricante y operador y desactivarlo con registro |
| `battery` | Restricciones en segundo plano, animaciones, ajustes adaptativos |
| `storage-cleanup` | Cachés, duplicados, descargas antiguas y ficheros grandes, con confirmación |
| `organize-files` | Ordenar `/sdcard` por categorías con `mv` registrado |
| `home-screen` | Organizar la pantalla de inicio por categorías (`adbuddy screen`, `tap`) |
| `backup-to-pc` | Copiar fotos y documentos al PC de forma incremental |
| `permissions-audit` | Revisar permisos peligrosos concedidos y proponer revocaciones |
| `build-utility-apk` | Crear, compilar e instalar un APK sencillo |

Además:

1. Instalación en `.claude/skills/` y `.agents/skills/` del entorno (§3).
2. Un puntero a las skills en `AGENTS.md` (§3.3).
3. Configuración de Gemini CLI para que lea `AGENTS.md` (§3.4).
4. Pruebas que verifican cada skill: formato, comandos clasificados con el clasificador real y subcomandos de `adbuddy` que existen (§5).
5. Verificación opcional de los comandos 🟢 en el emulador (§5.4).

**Fuera de M6b:** i18n EN/ES de la app y del host, README completo y donaciones (M6c o posterior); `adbuddy update` como comando propio (la v1 refresca con `adbuddy env init --force`, que ya existe); que `adbuddy doctor` compruebe las skills.

### 1.1 Criterios de éxito

- `adbuddy env init` en un entorno vacío crea las nueve skills en las dos carpetas, con el mismo contenido, y un segundo `env init` no cambia nada.
- `adbuddy env init --force` las reescribe, salvo las que el usuario marcó con `<!-- adbuddy:user -->`.
- Cada comando de ejemplo de una skill está anotado con el nivel que el clasificador le da hoy, y la prueba falla si cambia.
- Ninguna skill pide algo que `AGENTS.md` prohíba (otro adb, saltarse el shim, escribir contraseñas en el chat).
- Claude Code, Codex, Gemini CLI y OpenCode encuentran las skills sin configuración del usuario, y Gemini CLI también `AGENTS.md`.

## 2. Principios de las skills

1. **Una skill es una receta, no una autorización.** No amplía lo que el agente puede hacer: todo sigue pasando por el shim, el clasificador, el diario y las confirmaciones. Cada skill empieza diciendo que se aplican las reglas de `AGENTS.md` y que las skills no las sustituyen.
2. **Primero mirar, luego proponer, luego actuar.** Toda skill que cambia algo enseña al usuario en el chat el plan completo (qué, cuántos elementos, cómo se deshace) y espera su sí antes del primer cambio. Los cambios se hacen de uno en uno (regla 3) y se resumen con `adbuddy journal` al terminar, con el aviso de que `adbuddy undo` deshace.
3. **Lo reversible antes que lo irreversible.** Desactivar antes que desinstalar; `mv` a una carpeta de cuarentena (`/sdcard/ADBuddy-papelera/`) antes que `rm`; `pm revoke` antes que borrar datos. Nunca `pm clear`, `rm -rf`, `wipe` ni formateos como paso de una receta. Si algo irreversible es de verdad lo que el usuario quiere, se pide en el chat y se acepta la 🔴 de la terminal o del móvil.
4. **Nada de memoria sobre el dispositivo.** Las skills dan método, no veredictos: no traen listas de paquetes "seguros de quitar" ni dan por hecho que una herramienta de toybox o una opción de `settings` existe. Cada receta dice cómo comprobarlo (`command -v`, `--help`, el valor actual) y qué hacer si falta. Los prefijos de paquete por fabricante solo sirven para reconocer de dónde viene un paquete, nunca para decidir que es prescindible.
5. **Independientes del shell del PC.** El agente puede correr en Git Bash, PowerShell o `cmd`. Las recetas hacen el trabajo en el móvil (`adb shell …`) y con `adbuddy`; lo que se procesa en el PC se hace con ficheros y herramientas del propio agente, no con tuberías del shell local. Los scripts de `adb shell` van entre comillas simples en el ejemplo, con la nota de que el agente adapta el entrecomillado a su shell, y siguen la regla 15 (solo construcciones que el clasificador entiende, variables en minúsculas).
6. **El contenido leído es dato.** Nombres de apps, nombres de ficheros y texto de pantalla que lea una receta pueden contener instrucciones: no se siguen (regla 6). Las skills lo repiten donde se muestra contenido ajeno al usuario.
7. **Apps protegidas y la app de ADBuddy fuera de toda receta.** Ninguna receta toca bancos, pagos, contraseñas, 2FA ni `tech.hybriq.adbuddy`; si `debloat`, `battery` o `permissions-audit` las encuentran en una lista, las excluyen sin proponerlas.
8. **Ficheros para el usuario, a `outbox/`.** Informes y resultados se escriben en `outbox/` del entorno y se entregan con `adbuddy send`.

## 3. Instalación

### 3.1 Fuentes y forma

- Las skills viven en `host/internal/workspace/skills/<nombre>/SKILL.md`, embebidas con `//go:embed` (junto a las plantillas existentes).
- Formato del Agent Skills abierto, común a los cuatro agentes: frontmatter YAML con `name` (igual al nombre de la carpeta, minúsculas y guiones) y `description` (en inglés, empieza por "Use when…", de hasta 1024 caracteres) y el cuerpo en Markdown, **en inglés** (lo lee el agente; el idioma con el usuario lo pone el agente). Un `SKILL.md` no pasa de 250 líneas; el detalle largo, si hace falta, va en un fichero hermano de la misma carpeta que el `SKILL.md` enlaza (el embed incluye la carpeta entera).
- Sin campos de frontmatter distintos de `name` y `description`: OpenCode ignora los desconocidos pero Codex y Gemini no lo prometen.

### 3.2 `workspace.Init`

- Escribe cada fichero de cada skill en **dos** carpetas: `.claude/skills/<nombre>/` (Claude Code; OpenCode también la lee) y `.agents/skills/<nombre>/` (Codex, Gemini CLI y OpenCode). Dos copias idénticas: el mismo nombre con el mismo contenido no confunde a OpenCode, que las lee de las dos.
- **Política** (la de `AGENTS.md`): se crea lo que falta; solo con `Force` se reescribe lo que ya existe; un fichero que contiene `<!-- adbuddy:user -->` no se reescribe ni con `Force`. `adbuddy run` llama a `Init` sin `Force`, así que las skills nuevas de una versión posterior aparecen solas y las existentes no cambian hasta `env init --force`.
- Se usa `writeFile` (fichero temporal más `rename`), como para `AGENTS.md`. Si `.claude/skills/<nombre>` o `.agents/skills/<nombre>` existe y es un enlace simbólico o no es un directorio, `Init` falla con un error que lo dice (no escribe a través de él).
- Las skills que una versión posterior quite del binario no se borran del entorno.
- `Result.Created` y `Result.Updated` incluyen cada fichero, con ruta relativa. `adbuddy env init` agrupa en la salida humana las rutas bajo `.claude/skills/` y `.agents/skills/` en una línea por carpeta ("created 9 skills in .claude/skills"), para no imprimir dieciocho líneas; `--json` las lista todas.

### 3.3 Puntero en `AGENTS.md`

Una sección `## Skills` corta, después de `## The screen`: las recetas están en `.agents/skills/` (o `.claude/skills/`), se usan cuando lo que pide el usuario coincide con su descripción, **se leen enteras antes de empezar**, y no sustituyen a las reglas de este fichero. No lista las nueve (el agente las descubre por la descripción) para que `AGENTS.md` no crezca ni se desfase.

### 3.4 Gemini CLI y `AGENTS.md`

Gemini CLI lee `GEMINI.md` por defecto; Codex y OpenCode leen `AGENTS.md` y Claude Code lee `CLAUDE.md` (que importa `AGENTS.md`). `Init` crea `.gemini/settings.json` con la clave que Gemini CLI documente para `AGENTS.md` como nombre del fichero de contexto, y la mezcla (como `mergeClaudeSettings`) sin tocar lo demás del usuario. **El plan verifica la clave exacta (`context.fileName` o `contextFileName` según versión) en la documentación de Gemini CLI antes de escribir código**; si no se puede confirmar, se crea `GEMINI.md` con `@AGENTS.md`, como `CLAUDE.md`, y no se toca `settings.json`.

## 4. Las skills

Cada skill sigue la plantilla: *Cuándo usarla* · *Antes de empezar* (comprobaciones) · *Pasos* (con los comandos anotados, §5.2) · *Qué no hacer* · *Al terminar* (resumen con `adbuddy journal`, cómo deshacer, `outbox/` y `adbuddy send` si hay informe).

### 4.1 `device-profile`

Solo lectura. Reúne modelo, fabricante, versión de Android y de parche, almacenamiento (`df`, `dumpsys diskstats`), batería (`dumpsys battery`), cantidad de apps de usuario y de sistema, y los ajustes relevantes (`settings get` de las escalas de animación y del brillo adaptativo). Escribe `outbox/device-profile.md` y lo entrega con `adbuddy send`. Con permiso del usuario, propone una línea para `NOTES.md` (el agente nunca la escribe sin que lo pida). No cambia nada en el móvil.

### 4.2 `debloat`

1. Identifica fabricante y operador (`getprop ro.product.manufacturer`, `ro.carrier`/`gsm.operator.alpha`) y las apps de sistema (`pm list packages -s`) y las ya desactivadas (`pm list packages -d`).
2. Clasifica cada candidata con evidencia, no con una lista: ¿tiene icono de lanzador? ¿corre servicios en segundo plano (`dumpsys activity services`)? ¿la pide otra app (`dumpsys package`)? Los prefijos de fabricante (`com.samsung.`, `com.sec.`, `com.miui.`, `com.xiaomi.`, `com.huawei.`, `com.oppo.`/`com.coloros.`, `com.google.` para Pixel) solo dicen de quién es.
3. **Lista de exclusión fija en la skill** (lo que nunca se propone): el sistema, la interfaz, el teléfono, los SMS, el teclado activo, el lanzador activo, el WebView, los servicios de Google Play, el instalador de paquetes, los ajustes, Bluetooth, NFC, las apps protegidas y la de ADBuddy.
4. Propone un lote pequeño (hasta 10) en una tabla (paquete, origen, para qué parece servir, por qué es seguro desactivarla, cómo se deshace) y espera el sí por lote.
5. Desactiva con `pm disable-user --user 0 <pkg>`, de una en una; después comprueba que el móvil sigue respondiendo (`adbuddy screen`, `dumpsys window` en primer plano) y que la app deja de aparecer en `pm list packages -e`. Si algo falla, `adbuddy undo`.
6. `pm uninstall --user 0` solo si el usuario lo pide con nombre propio; nunca `pm uninstall` sin `--user 0`.

### 4.3 `battery`

Mide antes (`dumpsys battery`, `dumpsys batterystats --checkin` solo si cabe en la salida, apps con más uso por la pantalla de ajustes con `adbuddy screen`), propone y aplica de uno en uno: escalas de animación (`settings put global window_animation_scale …`), bucket de espera de apps concretas (`am set-standby-bucket`), restricción de segundo plano (`cmd appops set <pkg> RUN_ANY_IN_BACKGROUND ignore`) y ajustes adaptativos. **Nunca** restringe mensajería, alarmas, teléfono ni la app de ADBuddy; pregunta por las apps que el usuario no quiere tocar. Cada cambio guarda el valor anterior (el diario lo hace; la skill además lo muestra en la tabla) y se mide de nuevo al final con la misma lectura. No promete un porcentaje de mejora.

### 4.4 `storage-cleanup`

Mide (`df`, `du -s /sdcard/*`, `find … -size +100M`), lista candidatas con tamaño y fecha (descargas de más de 90 días, ficheros grandes, duplicados por `md5sum` si existe en el móvil, comprobado con `command -v`) y las **mueve a `/sdcard/ADBuddy-papelera/`** con `mv`, de una en una o en lotes con el mismo destino que el clasificador acepte como 🟡. Muestra lo que se recupera. Solo si el usuario, ya vista la papelera, pide vaciarla, borra con `rm` (🔴: la confirma él). Nunca toca `/sdcard/Android/`, ni `DCIM`, ni `Pictures`, ni `Documents` sin que el usuario lo pida con nombre; nunca `pm clear`. Para cachés, `cmd package trim-caches` si el móvil lo admite (se comprueba), no ficheros de `Android/data`.

### 4.5 `organize-files`

Lista `/sdcard` (primer nivel y descargas), propone un esquema (documentos, imágenes, vídeo, audio, archivos comprimidos, instaladores) **como una tabla de origen → destino** y espera el sí. Crea las carpetas con `mkdir -p` y mueve con `mv` de una en una (sin sobrescribir: `mv -n`, comprobado) para que cada movimiento tenga su inverso en el diario. No toca carpetas de apps (`Android/`, `WhatsApp/`, etc.), ni ficheros de menos de un minuto de antigüedad (una descarga en curso). Si la lista de movimientos pasa de 50, propone empezar por una categoría.

### 4.6 `home-screen`

Flujo con `adbuddy foreground` (confirma que el lanzador está delante), `adbuddy screen` y `adbuddy tap`. Para arrastrar, `input swipe x1 y1 x2 y2 <ms>` (una por llamada, regla de la sección "The screen" de `AGENTS.md`). Agrupa por categoría las apps de usuario en carpetas y propone el reparto antes. **Nunca suelta un icono en una zona de "Desinstalar", "Quitar" o "Eliminar"**: si `adbuddy screen` muestra una zona así mientras se arrastra, se suelta en un hueco o carpeta. Cada lanzador es distinto: la skill describe el método (leer, probar con un icono, verificar con `adbuddy screen`) y reconoce que puede fallar; si tras dos intentos no funciona, para y se lo cuenta al usuario en vez de insistir con coordenadas.

### 4.7 `backup-to-pc`

`adb pull` solo escribe sin preguntar dentro del entorno; fuera de él pide confirmación. La skill copia a `backup/<carpeta>/` dentro del entorno (el usuario puede moverla luego) y, si prefiere otro destino, explica que cada `pull` fuera del entorno exige su sí y propone una sola orden por carpeta. **Incremental:** guarda un manifiesto (`backup/<carpeta>/.manifest`, ruta, tamaño y fecha) generado con `find … -type f` y `stat` (comprobados); en cada pasada lista lo remoto, compara con el manifiesto **en el agente** (no con tuberías del shell) y copia solo lo nuevo o cambiado. Verifica cada copia por tamaño (y por `md5sum` si está en el móvil). **Nunca** borra del móvil: liberar espacio es `storage-cleanup`, con otra confirmación. Avisa del espacio libre en el PC antes de empezar y de la duración estimada (por la velocidad de la primera carpeta pequeña).

### 4.8 `permissions-audit`

Solo lectura hasta que el usuario decide. Para cada app de usuario (`pm list packages -3`) lee los permisos peligrosos **concedidos** (`dumpsys package <pkg>`) y los de operación (`cmd appops get <pkg>`); agrupa por permiso (ubicación en segundo plano, micrófono, cámara, SMS, registro de llamadas, contactos, acceso a todos los ficheros, superposición, accesibilidad). Propone las revocaciones en una tabla con el motivo ("una linterna con acceso al micrófono"), **sin juzgar por memoria**: marca "poco habitual" y pregunta. Revoca con `pm revoke <pkg> <permiso>` de uno en uno tras el sí; el inverso es `pm grant`. Los servicios de accesibilidad y los administradores de dispositivo no se revocan: se enseñan al usuario con la ruta de Ajustes para que decida él. Excluye las apps protegidas y la de ADBuddy.

### 4.9 `build-utility-apk`

La más delicada, porque instala software **en el PC**, fuera del alcance del shim. Reglas:

1. **Antes de descargar nada**, dice en el chat qué instalará (JDK 17, `cmdline-tools` de Android, `build-tools`, una plataforma), de dónde (solo orígenes oficiales: el sitio de Android, el de Gradle, el gestor de paquetes del sistema), cuánto pesa (varios cientos de MB) y dónde irá, y **espera el sí**. Sin instalaciones como administrador sin pedirlo aparte.
2. **Instalar `cmdline-tools`, `build-tools` y `platforms` pero nunca `platform-tools`**: traería otro `adb` y el agente no debe usar otro (regla 12). Comprueba con `command -v adb` (o `where adb`) que el `adb` que se resuelve es el del entorno.
3. **Nunca `gradlew installDebug`, `adb` del SDK, `flutter run` ni nada que hable con el móvil por su cuenta.** Se compila con Gradle y se instala solo con `adb install` del entorno (🔴: la confirma el usuario, y el móvil puede pedirle permiso de instalación).
4. Las licencias del SDK las acepta el usuario: la skill le enseña dónde leerlas y no ejecuta `sdkmanager --licenses` con `yes |` por él.
5. Proyecto en `apps/<nombre>/` del entorno, con una sola actividad y sin permisos salvo los que pida el usuario y el agente le haya explicado; firmado con la clave de depuración. Dice que el APK no está firmado con una clave del usuario (no se actualiza luego desde otro equipo) y que no se publica.
6. Su `applicationId` nunca empieza por `tech.hybriq.adbuddy` (el clasificador trata eso como la app de ADBuddy).
7. Entrega con `adbuddy send` el APK compilado si el usuario lo quiere en su móvil por el chat; no lo instala por otra vía.

## 5. Verificación

### 5.1 Lint de formato (`workspace`)

Para cada skill embebida: `SKILL.md` existe; frontmatter YAML válido con exactamente `name` y `description`; `name` es igual a la carpeta, ≤ 64 caracteres, minúsculas y guiones; `description` empieza por "Use when" y mide ≤ 1024; ≤ 250 líneas; sin direcciones IPv4, números de serie ni rutas de usuario de ningún PC (`C:\Users`, `/home/`, `/Users/`); sin la cadena `adb -s` ni `-d`/`-e` como opción global; nadie nombra otro `adb` (`platform-tools/adb`); las nueve skills de §1 están y no hay otras sin entrada en la tabla.

### 5.2 Comandos anotados (`cli`, que ve `classify` y `workspace`)

Los comandos de ejemplo van en bloques de código con lenguaje `adb` (uno por línea, sin el prefijo `adb `) y una anotación final `# green`, `# yellow` o `# red`:

```adb
shell pm list packages -3   # green
shell pm disable-user --user 0 com.example.app   # yellow
```

La prueba divide cada línea en argumentos (comillas simples y dobles, como el shell), la pasa por `classify.SplitGlobal` y `Classifier.Classify` y compara el nivel con la anotación. **Una línea sin anotación, con anotación desconocida o con nivel distinto falla.** Los marcadores de posición son valores realistas (`com.example.app`, `/sdcard/Download/ejemplo.pdf`), no `<pkg>`, porque `<` es una redirección para el shell. Una receta cuyo ejemplo sale 🔴 lo dice en la anotación y en el texto ("esperará tu confirmación"); el texto de la skill y la anotación no pueden contradecirse.

### 5.3 Subcomandos de `adbuddy`

Los bloques con lenguaje `adbuddy` (`screen --json`, `tap "Ajustes"`, `send outbox/informe.md`) se comprueban contra el árbol de comandos de cobra: el subcomando existe y los *flags* usados existen. Un cambio de CLI que rompa una skill rompe el test.

### 5.4 Emulador (opcional)

`TestSkillsReadOnlyOnEmulator`, protegido por la misma variable que `TestRecordOracle` (ver `host/README.md`), ejecuta contra el emulador los ejemplos anotados `# green` (no los 🟡 ni los 🔴) y comprueba que terminan sin `not found` ni `Unknown option`. Detecta herramientas que el toybox del emulador no trae; no sustituye la prueba en un móvil real (§6).

### 5.5 Instalación (`workspace`)

Pruebas de `Init`: crea en las dos carpetas con el mismo contenido; es idempotente; sin `Force` no reescribe un fichero editado; con `Force` lo reescribe; con `<!-- adbuddy:user -->` no lo reescribe ni con `Force`; falla (sin escribir) si `.claude/skills/<nombre>` es un enlace simbólico; deja intacta una skill del usuario que no es nuestra (`.claude/skills/mi-skill/`); mezcla `.gemini/settings.json` sin tocar el resto (o crea `GEMINI.md`, según §3.4).

## 6. Validaciones manuales pendientes

Nada de esto se prueba sin móvil; se apunta en el plan y no se ejecuta sin permiso explícito del usuario (regla del proyecto: nunca se modifica su móvil sin pedirlo):

- Que `claude`, `codex`, `gemini` y `opencode` listan las nueve skills desde un entorno recién creado (`/skills` o equivalente; solo lectura).
- Una pasada de solo lectura de `device-profile`, `permissions-audit` (hasta la propuesta), `storage-cleanup` (hasta la tabla) y `debloat` (hasta el lote propuesto) en el Pixel del usuario.
- Un cambio real y deshecho con `battery` (animaciones) y `organize-files` (una carpeta de prueba), con el usuario delante.
- `home-screen` en el lanzador de su Pixel: es la más frágil; se mide cuántos intentos necesita.
- `build-utility-apk` de principio a fin en un equipo sin SDK, con el usuario eligiendo entre instalar o no.

## 7. Riesgos y decisiones

- **Una skill es texto que el agente sigue: si dice algo imprudente, lo hace.** Se acota con los principios de §2, con la revisión de seguridad de las skills que cambian algo (`debloat`, `battery`, `home-screen`, `build-utility-apk`) por un revisor opus, y con que **todo cambio sigue pasando por el shim**: lo peor que puede hacer una skill mal escrita es lo que el clasificador ya deja ocurrir con confirmación. La excepción es `build-utility-apk`, que actúa en el PC; por eso es la que más reglas lleva y la que revisa opus con el detalle.
- **Dos copias de cada skill** duplican ~30 ficheros pero evitan enlaces simbólicos (frágiles en Windows) y dependen solo de rutas documentadas por los cuatro agentes (§3.2).
- **Las anotaciones de nivel acoplan las skills al clasificador.** Es lo buscado: si el clasificador cambia, la skill se actualiza o falla en CI. No se fijan por mksh ni por el móvil: el nivel es una función pura de la línea.
- **Veredictos por memoria (debloat, permisos):** descartados (§2.4). Cuesta que las recetas sean menos "mágicas" y que el usuario decida más; es lo que quiere el proyecto.
- **Idioma:** las skills están en inglés porque las lee el agente, como `AGENTS.md`. La i18n del producto es otro hito.
