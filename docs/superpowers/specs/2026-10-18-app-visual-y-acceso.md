# App Android: aspecto visual y estado del acceso (diseño)

- Fecha: 2026-10-18. Rama: `ux/visual`.
- Origen: petición del usuario ("mejora visualmente la app de android, revísala entera y haz que sea atractiva, además corrige bugs, además sale adb cortado siempre y no es intuitivo") y la auditoría del 2026-10-18 (`auditoria-app.md` del scratchpad del líder; sus hallazgos se copian aquí donde hacen falta).
- Plan: `docs/superpowers/plans/2026-10-18-app-visual-y-acceso.md`.
- Spec v1 a la que se subordina: `docs/superpowers/specs/2026-09-29-adbuddy-v1-design.md` (§4.6 cortar acceso, §6 app).

## 1. Objetivos

1. **"Acceso cortado" solo cuando es verdad.** Hoy sale siempre con la configuración que pide la guía (solo depuración inalámbrica: `adb_enabled=0`, `adb_wifi_enabled=1`), esconde "Cortar acceso" del menú y "Reactivar" apaga la inalámbrica. Se arregla la regla, el guardado y la restauración (bugs 1-4 de la auditoría).
2. **Que la pantalla principal se entienda de un vistazo:** a qué PC estoy conectado, si el agente puede tocar el móvil y qué tengo que hacer, con un único sitio para cada cosa.
3. **Que la app tenga identidad:** paleta propia completa en claro y oscuro, icono propio, monoespaciada para lo que se va a ejecutar, color propio del agente.
4. Corregir los bugs de la auditoría que no son de acceso (5-18), salvo los aparcados con ruling.

## 2. Alcance

Dentro: `android/` (código, recursos, tests JVM y capturas Roborazzi). Fuera: `host/`, protocolo, CLI, docs de instalación (salvo una línea en la guía, ver 7.3), FLAG_SECURE (decisión aparte, ver R11).

## 3. Decisiones (rulings)

**Dato del 2026-10-18 (después de la auditoría):** en el Pixel del usuario (Android 17, app 0.2.0) `adb shell` da `adb_enabled=1` y `adb_wifi_enabled=1` y la app decía "cortado". La hipótesis de la auditoría (USB apagado) no explica su caso: la app lee algo distinto de lo que dice `adb shell` (posible restricción de Android 16/17 a apps normales) o hay otra causa. El diagnóstico queda **abierto** (registro `ADBuddyAccess`, tarea 1 del plan, y V1 en §9). R1-R2 evitan el falso "cortado" aunque la lectura no sea fiable, pero con lecturas falsas cortar y reactivar **no se pueden confirmar**: ese caso no queda resuelto aquí y lo decide V1 (si se confirma, plan aparte que confirme con el canal en vez de con la lectura).

- **R1. Cortado = lo cortó ADBuddy y nada lo desmiente.** Ruling: `isCut` es verdadero solo si hay registro de un corte hecho por ADBuddy (los valores guardados, que solo quedan si el corte se confirmó leyendo) **y** ninguna lectura legible dice que `adb_enabled` o `adb_wifi_enabled` está a 1 (si lo dice, se olvida el registro: alguien devolvió el acceso a mano). Sin registro nunca se dice "cortado", lea lo que lea; con los dos ajustes legibles la regla equivale a "cortado solo si los dos están a 0" — por qué: en Android 11+ adbd corre con cualquiera de los dos encendido (S1, AOSP `AdbService`), la guía solo enciende la inalámbrica, y en el Pixel la lectura de la app no coincide con `adb shell` — qué cuesta si me equivoco: un corte hecho fuera de ADBuddy (en Ajustes del sistema) se ve como "Buscando tu PC…" en vez de "Acceso cortado"; la tarjeta dice que encienda la depuración inalámbrica, y el escudo ofrece "Cortar", que es inocuo.
- **R2. Ilegible no crea ni deshace un corte.** Ruling: si leer un ajuste lanza `SecurityException`, la lectura no cuenta: sin registro, no cortado; con registro, sigue cortado y el registro no se borra. La relectura tras escribir no da por bueno un valor ilegible, y un corte que no se confirma borra lo que guardó (no deja un registro falso) — por qué: un falso "cortado" esconde el botón de corte, que es el peligro de verdad — qué cuesta: en un móvil donde el ajuste oculto fuera ilegible, cortar diría "no se pudo" aunque hubiera escrito; el usuario tiene el botón a Opciones de desarrollador.
- **R3. Un canal vivo gana a "cortado".** Ruling: en `LinkStatus.of`, `INCOMPATIBLE`, `CONNECTED` y `PAUSED` se evalúan antes que `cut` — por qué: el canal va por `adb reverse`; si está vivo, adb no está cortado. Así nunca se pinta "conectado" y "cortado" a la vez — qué cuesta: durante el segundo que tarda en caer el túnel tras un corte, la pastilla sigue diciendo "Conectado"; lo cubre el aviso `cut_done` (snackbar) y la notificación cambia en cuanto cae.
- **R4. Cortar guarda siempre lo que había; reactivar lo pone tal cual y nunca adivina.** Ruling: `cut()` guarda `adb_enabled` y `adb_wifi_enabled` siempre que no haya registro de un corte anterior, y si el corte no se confirma borra lo que guardó; `restore()` escribe lo guardado tal cual, también `adb_enabled=0`. Sin registro: si una lectura legible ya dice que hay acceso, no escribe nada (nunca apaga la inalámbrica); si no, escribe solo `adb_enabled=1` sin tocar la inalámbrica. Con un registro de "los dos a 0" no escribe nada, conserva el registro y la pantalla dice que la enciendan en Opciones de desarrollador (`restore_manual`) — por qué: hoy restaurar enciende el USB que nunca estuvo y apaga la inalámbrica que sí estaba (bug 2), un corte con `adb=0` no guardaba `wifi=1` (bug 3), y un registro 0/0 solo puede venir de lecturas falsas o de un corte con todo apagado, donde cualquier valor sería adivinar — qué cuesta: en ese caso raro, el usuario enciende la depuración inalámbrica a mano.
- **R5. Cortar: escudo siempre en la cabecera y siempre en ⋮, sin confirmación; volver a dar acceso: con diálogo.** Ruling: el escudo (icono propio `ic_shield`) corta al momento; cortado, pasa a escudo tachado (`ic_shield_off`) y abre un diálogo que dice qué se enciende; el diálogo solo existe mientras el estado es CUT. El menú ⋮ lleva siempre "Cortar acceso del PC", que solo corta (idempotente). Cortar y reactivar son dos acciones, nunca un conmutador — por qué: cortar siempre es seguro y no debe cambiar de sentido; con un conmutador, un registro de corte y el canal aún vivo, tocar "Cortar" restauraba — qué cuesta: un toque más al reactivar.
- **R6. Material You desactivado por defecto.** Ruling: `AdbuddyTheme(dynamic = false)` por defecto y un interruptor en Ajustes ("Usar los colores del fondo de pantalla", apagado) — por qué: con color dinámico los colores semánticos (rojo = delicado, ámbar = cuidado, verde = conectado) y la marca dependen del fondo de pantalla del usuario y el rediseño no se ve en su Pixel — qué cuesta: quien quiera la app a juego con su fondo tiene que activarlo; los colores semánticos propios (§4.2) no cambian con el interruptor.
- **R7. Monoespaciada del sistema, sin empaquetar fuente.** Ruling: `FontFamily.Monospace` (Droid Sans Mono / Roboto Mono según el móvil) para comandos, IP:puerto, teclas rápidas y detalle de preguntas; el texto sigue con la fuente del sistema — por qué: sin binarios ni licencias nuevas en el repo y sin descargar nada (sin red en los implementadores ni GMS) — qué cuesta: menos personalidad que JetBrains Mono; añadirla después es un `res/font` y una línea en `AdbuddyType`.
- **R8. Una sola tarjeta de aviso.** Ruling: `LinkBanner`, `PendingQuestionsBanner` y `NotificationsBanner` se sustituyen por un `ActionCard` elegido por una función pura (`HomeNotice.of`) con prioridad fija: preguntas 🔴 cerradas > corte > actualizar app/host > sin configurar > notificaciones bloqueadas > en pausa > esperando al PC (esta con botón "Cómo conectar", que abre la guía https://hybriqtech.github.io/adbuddy/setup/). Los títulos de tarjeta no repiten el texto de la pastilla — por qué: las franjas apiladas parecían errores del sistema y se contradecían — qué cuesta: si hay dos problemas a la vez solo se ve el primero; las preguntas 🔴 van primero porque caducan, y el corte después porque explica por qué nada funciona.
- **R9. Modo remoto solo en Ajustes.** Ruling: `RemoteCard` sale del chat; en el chat solo queda la etiqueta "Remoto" en la pastilla de estado cuando el modo remoto está activado **o es desconocido** (regla de "falso apagado" de `RemoteUi.treatAsRemote`) — por qué: la tarjeta duplicaba "sin conexión" en rojo y ocupaba el chat; su riesgo se ve igual en la pastilla — qué cuesta: un paso más para activarlo (Ajustes).
- **R10. El nombre del agente en la tarjeta de pregunta, no en la notificación de pantalla bloqueada.** Ruling: `perm_title` pasa a "%1$s pregunta" con `sessionState.agent` (o "El agente" si viene vacío); `notif_asking` pasa a genérico ("Tu agente necesita una respuesta") — por qué: Codex o Gemini no son "Claude Code"; la versión pública de la notificación no lleva argumentos (`theLockScreenLineNeverHasAgentTextOrPermissionData`) — qué cuesta: nada que se vea en pantalla bloqueada.
- **R11. FLAG_SECURE fuera.** Ruling: no se añade — por qué: no existe hoy y ADBuddy ya es app protegida; impediría las capturas que el usuario hace para pedir ayuda — qué cuesta: lo que se ve en ADBuddy puede salir en capturas del propio usuario, como hoy.
- **R12. Selector de idioma: la fila de Ajustes abre la página del sistema** (`LanguageSettings`, Android 13+; por debajo, la fila no sale) — por qué: ya está hecho y probado, solo faltaba cablearlo — qué cuesta: en Android 11-12 se sigue el idioma del sistema.
- **R13. Consentimiento con `Toast`: se queda.** Ruling: el bug 17 se aparca — por qué: lo blinda `ConsentFeedbackFlowTest`, el texto es corto y cambiarlo exige otro flujo — qué cuesta: el mensaje desaparece a los 3,5 s.
- **R14. Las capturas se regraban en Windows** (`-Padbuddy.screens=record`), se revisan a ojo por el revisor (claro, oscuro, 200 %, 200 % es) y se verifican con la ejecución normal (verify en Windows). En la CI Linux siguen en modo compare (no fallan).
- **R15. Terminal plegada por defecto y al 35 %.** Ruling: la app abre con la terminal plegada (como ya hace la burbuja) y, abierta, ocupa el 35 % del alto (antes 45 %) — por qué: es lo menos importante para quien vigila desde el móvil y se comía medio chat — qué cuesta: un toque en la flecha para verla.

## 4. Dirección visual: "consola de control tranquila"

ADBuddy es un mando a distancia con freno de mano: alguien desde el PC toca tu móvil y tú vigilas. Superficies limpias, un verde azulado profundo como marca, el agente en azul tinta, el semáforo (verde/ámbar/rojo) solo para estados, monoespaciada para lo que se va a ejecutar. Nada lila, nada de degradados.

### 4.1 Paleta Material 3 (todos los roles; `ui/theme/Palette.kt`)

| Rol | Claro | Oscuro |
|---|---|---|
| primary / onPrimary | `#00695C` / `#FFFFFF` | `#7FD8C6` / `#00382F` |
| primaryContainer / on | `#B8F0E4` / `#00201B` | `#004F45` / `#B8F0E4` |
| inversePrimary | `#7FD8C6` | `#00695C` |
| secondary / onSecondary | `#4A635E` / `#FFFFFF` | `#B1CCC6` / `#1C3531` |
| secondaryContainer / on | `#CDE8E1` / `#06201C` | `#334B47` / `#CDE8E1` |
| tertiary / onTertiary (agente) | `#3F5F90` / `#FFFFFF` | `#A8C8FF` / `#06305F` |
| tertiaryContainer / on | `#D6E3FF` / `#001B3E` | `#26477A` / `#D6E3FF` |
| background, surface / on | `#F6FAF8` / `#171D1B` | `#0F1513` / `#DEE4E1` |
| surfaceVariant / on | `#DBE5E1` / `#3F4946` | `#3F4946` / `#BFC9C5` |
| surfaceTint | = primary | = primary |
| surfaceDim / surfaceBright | `#D6DBD9` / `#F6FAF8` | `#0F1513` / `#353B39` |
| surfaceContainerLowest / Low / (base) / High / Highest | `#FFFFFF` / `#F0F5F2` / `#EAEFEC` / `#E4E9E7` / `#DEE4E1` | `#0A0F0E` / `#171D1B` / `#1B211F` / `#252B2A` / `#303634` |
| inverseSurface / inverseOnSurface | `#2B3230` / `#EDF2EF` | `#DEE4E1` / `#2B3230` |
| outline / outlineVariant | `#6F7976` / `#BFC9C5` | `#899390` / `#3F4946` |
| error / onError | `#BA1A1A` / `#FFFFFF` | `#FFB4AB` / `#690005` |
| errorContainer / on | `#FFDAD6` / `#410002` | `#93000A` / `#FFDAD6` |
| scrim | `#000000` | `#000000` |

Colores semánticos propios (`AdbuddyColors`, `CompositionLocal`; no cambian con Material You):

| Rol | Claro | Oscuro |
|---|---|---|
| success / onSuccess / successContainer / onSuccessContainer | `#2E6B30` / `#FFFFFF` / `#C8E6C9` / `#0A2E0C` | `#8BD88F` / `#00390A` / `#1F5323` / `#C8E6C9` |
| warning / onWarning / warningContainer / onWarningContainer | `#7A4A00` / `#FFFFFF` / `#FFDDB3` / `#291800` | `#FFB951` / `#452B00` / `#633F00` / `#FFDDB3` |

`PaletteContrastTest` exige ≥ 4,5:1 en todos los pares texto/fondo de las dos tablas y en `onSurface`/`onSurfaceVariant` sobre cada `surfaceContainer*`, y comprueba que no falta ningún rol.

### 4.2 Tipografía, forma y espaciado

- Texto: fuente del sistema. `titleLarge` 22 sp SemiBold para el nombre del agente; `labelLarge` 14 sp Medium para la pastilla; burbujas en `bodyLarge` 16 sp.
- `AdbuddyType.code` = `bodyMedium` con `FontFamily.Monospace` y `textDirection = Ltr`; `AdbuddyType.codeSmall` = `bodySmall` monoespaciada.
- Formas: `small` 8 dp, `medium` 16 dp (tarjetas), `large` 24 dp (diálogos), `extraLarge` 28 dp. Pastilla: totalmente redonda. Burbujas: 18/18/18/4 (la esquina de la "cola" hacia quien habla).
- Espaciado en múltiplos de 4: márgenes laterales 16 dp, separación entre burbujas 8 dp, interior de tarjetas 16 dp.

### 4.3 Icono

- Lanzador adaptativo vectorial (`mipmap-anydpi/ic_launcher.xml` y `ic_launcher_round.xml`): fondo `#00695C`, primer plano una burbuja de chat blanca con el indicador de terminal `>_` en verde azulado, dentro de la zona segura (66 dp); capa `monochrome` (Android 13) con la misma burbuja y el `>_` hueco.
- Cabecera: `drawable/ic_shield.xml` (escudo relleno, Cortar) y `drawable/ic_shield_off.xml` (escudo en contorno tachado, Volver a dar acceso), vectores propios (sin `material-icons-extended`).
- Notificación: `drawable/ic_stat_adbuddy.xml` (la burbuja con `>_` hueco, blanca sobre transparente) para la conversación, "Listo" y preguntas; `drawable/ic_stat_shield.xml` (escudo con "!" hueco) para las 🔴. La burbuja del sistema usa el icono de la app.

## 5. Pantalla principal

```
┌──────────────────────────────────────────┐
│ Claude Code                     [⛨]  ⌄ ⋮ │  ⛨ = Cortar acceso (siempre; si está cortado, "Volver a dar acceso")
│ (● Conectado a mi-pc · Remoto)           │  pastilla de estado (color + icono + texto)
├──────────────────────────────────────────┤
│ ┌ tarjeta de aviso (solo si hay algo) ──┐ │  icono · título · una línea · [un botón]
│ └───────────────────────────────────────┘ │
│ terminal (como hoy) · teclas rápidas      │
│ conversación / estado vacío con sugerencias│
│ tarjeta de pregunta del agente (azul)     │
│ (+) [ Escribe al agente… ]       (➤ / ■)  │
└──────────────────────────────────────────┘
```

- **Pastilla de estado** (`StatusPill`): una por `LinkStatus` — Conectado a %s (success), Conectando… / Buscando tu PC… (neutra), En pausa (warning), Acceso cortado (error), Actualiza (error), Sin configurar (neutra). Si el modo remoto está activo o es desconocido y hay canal, añade " · Remoto". El estado del agente (Trabajando / Esperándote / Listo) va como segunda parte del título solo cuando el enlace está CONNECTED y no en pausa: "Claude Code · Trabajando". Así la cabecera nunca dice "Listo" con una pausa o un corte (bug 9).
- **Cabecera propia y pastilla**: la cabecera es propia (`ChatHeaderBar`, altura mínima 64 dp, título hasta 3 líneas) y la pastilla es una `RoundedCornerShape(16.dp)` de hasta 3 líneas, no una cápsula de una línea: al 200 % el texto se cortaba.
- **Botón de escudo** en la cabecera, siempre visible y sin filtro de toques tapados (un corte forzado es inofensivo). Sin cortar: `ic_shield`, descripción "Cortar acceso del PC", corta al momento. Cortado: `ic_shield_off`, descripción "Volver a dar acceso", abre el diálogo de R5. El menú ⋮ lleva siempre "Cortar acceso del PC" (solo corta) y ya no lleva "Reactivar".
- **Tarjeta de aviso** (`ActionCard`, R8): `CUT` → "El PC no puede tocar tu móvil" · "Le cortaste el acceso. Devuélveselo cuando quieras que el agente vuelva a trabajar." · [Volver a dar acceso]; `QUESTIONS` → "N acciones delicadas esperan" · [Ver]; `UPDATE_APP` → "Actualiza la app" · texto actual con el comando · sin botón; `UPDATE_HOST` → "Actualiza ADBuddy en el PC" · sin botón; `NOT_SET_UP` → "Falta emparejar con tu PC" · "En el PC: adbuddy setup"; `NOTIFICATIONS` → "Las notificaciones están apagadas" · [Abrir ajustes]; `PAUSED` → "El agente está en pausa" · texto de `status_paused` · [Reanudar]; `WAITING` → "Tu PC no responde" · texto de `status_waiting` · [Cómo conectar]. La conservación de la última tarjeta durante `CONNECTING` (`lastBanner`) se mantiene.
- **Terminal**: plegada al abrir la app; abierta, 35 % del alto (R15).
- **Estado vacío**: también en pausa (con su texto), con scroll propio para el texto al 200 %. Sugerencias tocables que rellenan el campo (no envían): "¿Cuánta batería queda?", "Baja el volumen", "Haz una captura de pantalla".
- **Burbujas**: propias en `primaryContainer`, del agente en `surfaceContainerHigh`, forma 18/18/18/4.

## 6. Resto de pantallas

- **Tarjeta de pregunta del agente** (`PermissionCard`): `tertiaryContainer` (azul tinta, ya no malva), título "%1$s pregunta" (R10), detalle con `AdbuddyType.codeSmall`. Nada más cambia.
- **Diálogo 🔴** (`ConfirmDialog`): solo lo visual. El comando va en un bloque `surfaceContainerHighest` con `AdbuddyType.code`, **dentro del mismo scroll vigilado** (sin scroll propio). El armado, el orden y peso de los botones y el resto quedan igual (§8).
- **Ajustes**: dos secciones con cabecera — "Seguridad" (apps protegidas, acceso a notificaciones, modo remoto) y "App" (idioma, colores del fondo de pantalla). La tarjeta del modo remoto desconectada dice "Disponible cuando el PC esté conectado" en neutro, no en rojo.
- **Actividad**: barra lateral de 4 dp del color del nivel (success/warning/error) y `contentDescription` con el nombre del nivel (bug 16); el comando en `AdbuddyType.codeSmall`.
- **Chips de borrador**: el nombre siempre, el error como segunda línea (bug 12).
- **Superposición del agente** (`Overlay`): colores fijos de marca, sin `system_accent1_*` (API 31, bug 5): claro primary `#00695C` con texto blanco; oscuro `#7FD8C6` con texto `#00382F` (bug 6). Texto "⏸ Pausar" en la pastilla.
- **Onboarding y consentimiento**: no cambian de estructura; heredan paleta, formas y tipografía.

## 7. Microcopys (en y es; las dos locales que existen)

### 7.1 Cambian

| Clave | en | es |
|---|---|---|
| `status_cut` | Access cut: the PC cannot touch your phone | Acceso cortado: el PC no puede tocar tu móvil |
| `status_waiting` | Looking for your PC… On the PC, run adbuddy serve with wireless debugging on. | Buscando tu PC… En el PC, ejecuta adbuddy serve con la depuración inalámbrica encendida. |
| `status_short_waiting` | Looking for your PC… | Buscando tu PC… |
| `status_not_set_up` | Not paired with your PC yet. On the PC: adbuddy setup | Falta emparejar con tu PC. En el PC: adbuddy setup |
| `action_cut` | Cut PC access | Cortar acceso del PC |
| `action_restore` | Give access back | Volver a dar acceso |
| `cut_done` | Done. The PC can no longer touch your phone. | Hecho. El PC ya no puede tocar tu móvil. |
| `cut_restored` | The PC has access again. If you used remote mode, its port stays open until the PC connects or the phone restarts. | El PC vuelve a tener acceso. Si usabas el modo remoto, su puerto sigue abierto hasta que el PC se conecte o reinicies. |
| `perm_title` | %1$s asks | %1$s pregunta |
| `notif_asking` | Your agent needs an answer | Tu agente necesita una respuesta |
| `chat_empty_ready` | Ask your phone for something. If the agent is about to do something risky, it will ask you first. | Pídele algo a tu móvil. Si el agente va a hacer algo delicado, te preguntará antes. |
| `remote_disconnected` | Available when the PC is connected | Disponible cuando el PC esté conectado |
| `overlay_pill` | Agent using the screen · ⏸ Pause | Agente usando la pantalla · ⏸ Pausar |

### 7.2 Nuevas

`restore_confirm_title` (Give the PC access again? / ¿Volver a dar acceso al PC?), `restore_confirm_body` (ADBuddy turns debugging back on as it was before the cut, and the agent on the PC can use your phone again. If it cannot tell how it was, it tells you how to do it. / ADBuddy vuelve a encender la depuración como estaba antes del corte y el agente del PC podrá volver a usar tu móvil. Si no sabe cómo estaba, te dice cómo hacerlo.), `status_pill_remote` (%1$s · Remote / %1$s · Remoto), `title_agent_state` (%1$s · %2$s), `agent_fallback` (The agent / El agente), `card_cut_title` (The PC cannot touch your phone / El PC no puede tocar tu móvil), `card_cut_body` (You cut its access. Give it back when you want the agent to work again. / Le cortaste el acceso. Devuélveselo cuando quieras que el agente vuelva a trabajar.), `card_waiting_title` (Your PC is not answering / Tu PC no responde), `card_waiting_action` (How to connect / Cómo conectar), `card_paused_title` (The agent is paused / El agente está en pausa), `restore_manual`, `restore_confirm_confirm`, `setup_guide_url` (sin traducir), `card_update_app_title` (Update the app / Actualiza la app), `card_update_host_title` (Update ADBuddy on the PC / Actualiza ADBuddy en el PC), `card_not_set_up_title` (Not paired yet / Falta emparejar), `card_notifications_title` (Notifications are off / Las notificaciones están apagadas), `chat_empty_paused` (The agent is paused: it cannot use the phone until you resume. / El agente está en pausa: no puede usar el móvil hasta que lo reanudes.), `suggestion_battery`, `suggestion_volume`, `suggestion_screenshot`, `settings_section_security` (Security / Seguridad), `settings_section_app` (App / App), `settings_dynamic_color` (Use wallpaper colors / Usar los colores del fondo de pantalla), `settings_dynamic_color_hint`, `level_bar_red/yellow/green` (si no bastan `level_red/yellow/green`, se reutilizan esas). El texto exacto de cada una está en la tarea del plan que la introduce.

### 7.3 Docs

La guía `docs/setup/{es,en}/index.md` no cambia de pasos; el plan solo añade, en la tarea 1, una nota en `android/README.md` sobre la regla R1.

## 8. Protecciones de seguridad que no se pueden romper (con sus tests)

Cada tarea de UI las lista en su criterio de aceptación; ningún test de esta lista se borra ni se debilita. Si una tarea necesita cambiar uno, se para y lo adjudica el controlador.

1. **Armado anti-tapjacking**: Permitir y Permitir en la sesión esperan `ConfirmUi.ARMING_MS = 600` ms desde que aparecen o vuelve la app. Tests: `ConfirmDialogArmingTest`, `ConfirmHostArmingTest`, `ConfirmArmingTest`.
2. **Leer hasta el final**: los botones de permitir siguen apagados mientras `scroll.canScrollForward` (diálogo 🔴 y `PermissionCard`); todo lo que hay que leer (incluido el bloque del comando) sigue dentro del scroll vigilado. Tests: `ConfirmDialogScrollTest`, `PermissionCardScrollTest`.
3. **Orden y peso de los botones**: Rechazar primero, relleno y activo al momento; Permitir con contorno; Permitir en la sesión aparte. Tests: `ConfirmDialogButtonsSourceTest`, `ConfirmDialogLayoutTest`.
4. `DialogProperties(dismissOnClickOutside = false)`; Atrás cierra sin responder; las preguntas cerradas se reabren sin responder (`ConfirmReopenTest`, `ChatPendingQuestionsTest`, `PendingQuestionsBannerTest` adaptado a `ActionCard` sin perder sus aserciones: "Ver" no responde nada).
5. **Toques tapados**: `ObscuredTouchGuard()` solo dentro de ventanas propias (diálogos, menús: el diálogo 🔴, el de reactivar, el de nueva conversación, el menú ⋮); en pantalla, `Modifier.refuseObscuredTouches()` solo en el botón que concede algo (el de `ActionCard`, las teclas rápidas, las tarjetas de pregunta como hoy). Nunca la guarda a nivel de ventana en un composable de pantalla (chat vacío, Ajustes). El escudo de Cortar va sin filtro: un corte forzado es inofensivo. Tests: `TouchGuardSourceTest` (ajustado solo para aceptar el filtro en el botón de `ActionCard`), `TouchGuardBehaviorTest`, `ObscuredFlagsTest`, `ObscuredCountsTest`, `QuickKeysObscuredTest`.
6. El ⚠ especial va primero; host y comando en LTR (`ConfirmTextDirectionTest`); comandos > 20 000 caracteres solo se pueden rechazar; la notificación no deja permitir comandos recortados ni especiales (`ConfirmNotificationTest`, `ConfirmReceiverHandleTest`).
7. **Pantalla bloqueada**: versión pública sin comando, sin actividad ni nombre del PC ni argumentos (`ConversationViewTest.theLockScreen*`). Cortar acceso disponible siempre en la notificación y sin desbloqueo (`askingOffersOnlyCut_otherwiseTheM3Actions`).
8. **Cortar** sigue escribiendo los dos ajustes a 0 y comprobándolos de vuelta (`AccessSwitchTest.cuttingTurnsBothOff`, `aWriteThatDidNotStickIsNotDone`, `withoutThePermissionNothingIsCut`); restaurar nunca enciende la inalámbrica a ciegas (`restoringWithNothingSavedTurnsOnlyAdbOn`).
9. **Modo remoto**: nunca "apagado" si es desconocido (`RemoteUiTest`, `RemoteCardSemanticsTest`); la etiqueta "Remoto" de la pastilla sigue `RemoteUi.treatAsRemote`.
10. **i18n**: `NoHardcodedTextTest`, `StringsParityTest`, `UpdateAppTextTest` y el lint de traducciones; todo texto nuevo en `values` y `values-es`.

## 9. Validaciones manuales pendientes (exigen el móvil real)

Solo con permiso explícito del usuario (no se toca su móvil sin él):

1. **Diagnóstico abierto**: con la build que registra `ADBuddyAccess` instalada en el Pixel (instalar exige permiso), `adb logcat -d -s ADBuddyAccess` frente a `adb shell settings get global adb_enabled` / `adb_wifi_enabled` (hoy `1` y `1`). Dice si la app recibe un valor distinto (Android 17) o si el "cortado" venía de otra parte (p. ej. un registro de corte antiguo). Y que la app ya no dice "cortado" con el chat funcionando.
2. **Cortar y reactivar en el Pixel con solo la inalámbrica** (modifica el móvil: pedir permiso). **Prerrequisito: V1**; si V1 muestra lecturas falsas, no se hace con esta build. Cortar desde la cabecera → el PC pierde la conexión y la app dice "Acceso cortado"; "Volver a dar acceso" → vuelven los ajustes que había (y no la USB si no estaba), el PC reconecta (el puerto puede cambiar). **Sustituida (2026-10-19)** por la V2 de `2026-10-19-acceso-lecturas-falsas.md` §5: V1 mostró lecturas falsas (`adb_enabled=raw:0` frente a `1` del shell).
3. **Icono monocromo** con los iconos temáticos de Android 13+ del Pixel (el emulador lo muestra, pero el lanzador de Pixel es el que usa el usuario).

Todo lo demás (Overlay en API 30, capturas, contraste) se valida con tests JVM o en emulador.
