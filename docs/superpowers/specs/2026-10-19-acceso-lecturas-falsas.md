# Acceso: lecturas falsas de `adb_enabled` (adenda)

- Fecha: 2026-10-19. Rama: `ux/acceso-a17` (desde `main` 9f3e353a).
- Adenda de `docs/superpowers/specs/2026-10-18-app-visual-y-acceso.md`: mantiene R1-R15 y sustituye su §9.2 (V2). Las reglas nuevas siguen la numeración: R16-R21.
- Plan: `docs/superpowers/plans/2026-10-19-acceso-lecturas-falsas.md`.

## 1. Hecho medido

El 2026-10-19, en el Pixel 9 Pro XL del usuario (Android 17, sdk 37) con la build de `main`, el registro `ADBuddyAccess` dice `adb_enabled=raw:0 adb_wifi_enabled=raw:1`, y `adb shell settings get global` da `1` y `1`. **Android 17 da a las apps un 0 falso en `adb_enabled`; `adb_wifi_enabled` se lee bien.** Esto cierra V1 (UX1-1). No sabemos todavía si `putInt(adb_enabled, …)` con `WRITE_SECURE_SETTINGS` tiene efecto real: lo mide V2 (§4), porque el shell sí lee el valor real.

## 2. Problema

Con la build de `main`:

1. `cut()` guarda `adb_enabled=0` (el valor falso) y `restore()` escribe ese 0. Si la escritura tiene efecto real, al reactivar la depuración USB queda apagada aunque antes estuviera encendida. Y si en Android 17 la inalámbrica dependiera de la USB, el PC se quedaría sin acceso.
2. La relectura de `adb_enabled` tras cortar (un 0) no demuestra nada. La relectura tras restaurar (un 0 cuando se escribió 1) hace que reactivar falle siempre.

## 3. Decisiones (rulings)

- **R16. Un 0 contradicho se guarda como 1.** Ruling: si al guardar antes de cortar `adb_enabled` lee 0 y además `adb_wifi_enabled` lee 1 o hay un canal vivo con el PC, se guarda `adb_enabled=1` — por qué: el 0 de `adb_enabled` es justo el valor que Android 17 falsea. La inalámbrica se lee bien, y en el Pixel, con la inalámbrica encendida, el shell da `adb_enabled=1`. Un canal vivo va por `adb reverse` y prueba que adbd corre. Creerse el 0 cuesta más: en el móvil del usuario (el caso principal), reactivar apagaría la USB de verdad — qué cuesta si me equivoco: en un Android 11-16 con solo la inalámbrica y la USB apagada **de verdad**, reactivar enciende también la depuración USB. Es una puerta más, que solo sirve con cable y con una clave ya autorizada o aceptando el diálogo de huella del móvil, y se ve y se apaga en Opciones de desarrollador. El test `cuttingWithOnlyWirelessSavesBothAndRestoresThemAsTheyWere` (fuera de la lista protegida §8 de la spec) cambia por esto: con 0/1 leído guarda 1/1 y restaura 1/1. Se renombra a `cuttingWithOnlyWirelessReadSavesUsbAsOn_andRestoresBoth`.
- **R17. Se detecta por contradicción, no por versión, y solo se pone en duda el 0.** Ruling: `adb_enabled` no pasa a ser "ilegible" en general ni a partir de un sdk. Un 1 leído se cree siempre, porque las lecturas falsas medidas son 0. Un 0 solo se pone en duda cuando la inalámbrica lee 1 o hay canal (`AccessSwitch.adbZeroIsDoubtful`); sin nada en contra, se cree — por qué: así un Android 17 que lea bien o un Android 16 que mienta se tratan igual de bien, y sigue en pie la comprobación de que el corte se aplicó (`aWriteThatDidNotStickIsNotDone`: escribir 0 y leer 1 es un fallo) — qué cuesta si me equivoco: (a) en Android 17 con solo la USB, sin inalámbrica ni canal, se guarda 0/0 y reactivar es manual (R4, `restore_manual`); (b) si en ese caso sí había canal (registro 1/0), reactivar escribe `adb_enabled=1`, relee un 0 falso sin contradicción y dice "no se pudo" con el botón a Opciones de desarrollador, aunque seguramente funcionó. El registro se queda, y la pastilla dice "Conectado" en cuanto vuelve el PC (R3). (c) En Android 17, encender a mano solo la USB no se ve (lee 0): sigue "cortado" hasta que vuelve el canal (R3) o se enciende la inalámbrica.
- **R18. Cortar se confirma con la inalámbrica a 0 y sin un 1 en `adb_enabled`.** Ruling: `cut()` da `true` si las escrituras no lanzan, `adb_wifi_enabled` relee 0 y `adb_enabled` no relee 1 (y un valor ilegible no vale, R2 sin cambios) — por qué: es lo único que la lectura puede probar en Android 17 — qué se pierde: en Android 17 no se confirma que `adb_enabled=0` se aplicara. Si Android ignorara la escritura sin lanzar, la USB seguiría encendida y adbd vivo: la conexión inalámbrica del PC cae igual (se apaga el transporte inalámbrico), pero un puerto `adb tcpip` abierto seguiría escuchando mientras adbd viva. Si el canal sobreviviera, la pastilla diría "Conectado" y no "cortado" (R3). V2 lo mide desde el shell — qué cuesta si me equivoco: un "cortado" que no corta la USB en Android 17 hasta que V2 lo descubra. La protección real ante el agente (el PC pierde el acceso inalámbrico) sí se ve en la pastilla.
- **R19. Restaurar se confirma con la inalámbrica como estaba y `adb_enabled` escrito.** Ruling: `restore()` da `true` si las escrituras no lanzan, `adb_wifi_enabled` relee lo guardado (lectura fiable) y `adb_enabled` relee lo guardado o, si se escribió 1, relee un 0 en duda (la inalámbrica ya relee 1, o hay canal). No se exige releer un 1 que Android no deja ver. Sin registro, todo sigue igual (R4) — por qué: con la regla vieja, reactivar falla siempre en el Pixel — qué cuesta si me equivoco: si Android 17 ignorara `adb_enabled=1` sin lanzar, "reactivado" dejaría la USB apagada. La inalámbrica sí vuelve (es la que usa ADBuddy) y V2 lo mide.
- **R20. El canal solo sirve para lo que se guarda, nunca para deshacer un corte.** Ruling: "canal vivo" es `BridgeState.connection == CONNECTED` (incluida la pausa) en el instante de tocar Cortar, por el escudo o por la notificación (`CutAccess.channelUp`). `INCOMPATIBLE` no cuenta: la app paró y puede no haber socket. Entra solo en R16 y R19. Nunca en `isCut()` ni en `forgetIfRestored()` — por qué: el túnel tarda un segundo en caer tras cortar y el canal sigue `CONNECTED`; si el canal borrase el registro, se perdería el corte — qué cuesta si me equivoco: si el canal acaba de caer (`CONNECTING`) y los dos ajustes leen 0, se guarda 0/0 y reactivar es manual (R4).
- **R21. El registro de diagnóstico dice cuándo una lectura se contradice.** Ruling: la línea de `ADBuddyAccess` añade `channel=up|down` cuando se sabe (al cortar) y `doubtful=adb_enabled(wifi|channel|wifi+channel)` cuando el 0 leído de `adb_enabled` está en duda. Solo números y palabras fijas, como hasta ahora — por qué: V2 y cualquier informe del usuario tienen que poder decir qué creyó la app — qué cuesta: nada visible.

## 4. Garantías que se mantienen

| Garantía | Cómo queda |
|---|---|
| R1: cortado = corte propio que nada desmiente | Igual. Un 1 legible sigue desmintiendo; el canal no entra en `isCut` (R20). |
| R2: ilegible no crea ni deshace un corte | Igual, también en la relectura (R18, R19). |
| R3: el canal vivo gana a "cortado" | Igual (`LinkStatus.of`). |
| R4: cortar guarda siempre; reactivar no adivina; 0/0 ⇒ manual | Igual. 0/0 solo se guarda cuando nada contradice el 0 (R16-R17). |
| R5: cortar siempre posible y nunca restaura; reactivar con diálogo | Igual. `cut()` solo escribe 0; la UI no cambia. |
| §8.8 de la spec (`cuttingTurnsBothOff`, `aWriteThatDidNotStickIsNotDone`, `withoutThePermissionNothingIsCut`, `restoringWithNothingSavedTurnsOnlyAdbOn`) | Sin tocar y en verde. |

## 5. Validaciones manuales (sustituyen a V2-V3 de la spec del 2026-10-18; siempre con permiso explícito del usuario)

- **V2 (rehecha). Cortar y reactivar en el Pixel, mirando los valores reales desde el shell.** Instalar la build de este plan es un cambio en el móvil: pedir permiso. Pasos:
  1. Antes: `adb shell settings get global adb_enabled` y `… adb_wifi_enabled` (esperado `1` y `1`), y `adb logcat -c`.
  2. Cortar desde el escudo de la app (con el chat conectado). Esperado: el PC pierde la conexión (`adb devices` ya no lo lista o sale `offline`; `adbuddy serve` dice que se ha desconectado) y la app dice "Acceso cortado".
  3. Si hay un cable USB conectado al PC: `adb -d shell settings get global adb_enabled`. Si responde, la USB **no** se cortó (R18 se queda corto en Android 17: apuntarlo y abrir un plan). Si no aparece ningún dispositivo, el corte se aplicó.
  4. Reactivar desde la app (escudo tachado, diálogo y confirmar). Esperado: "El PC vuelve a tener acceso" y el PC reconecta (el puerto inalámbrico puede cambiar).
  5. Después: `adb shell settings get global adb_enabled` y `… adb_wifi_enabled` → `1` y `1`. Y `adb logcat -d -s ADBuddyAccess`: `cut.before … channel=up doubtful=adb_enabled(wifi+channel)`, `cut.after ok=true … saved.adb_enabled=1 saved.adb_wifi_enabled=1`, `restore.after ok=true`.
  6. Si reactivar falla: el usuario enciende la depuración inalámbrica (y la USB, si la tenía) en Opciones de desarrollador. Apuntar la línea `restore.after` del registro.
- **V3. Icono monocromo** con los iconos temáticos del lanzador del Pixel (Android 13+). Sin cambios respecto a la spec del 2026-10-18.
