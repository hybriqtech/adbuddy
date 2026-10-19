# Fragmento: modo YOLO (rama feat/modo-yolo)

Cierre del plan `2026-10-18-modo-yolo.md` (spec `2026-10-18-adbuddy-modo-yolo-design.md`; ledger `.superpowers/sdd/2026-10-18-modo-yolo/progress.md`). Las filas de aquí las integra el controlador en `pendientes-m3-m5.md` (sección «Modo YOLO») y en `2026-10-03-validaciones-manuales.md`.

## Pendientes

- **Fuera de la v1 de YOLO (decidido):**
  - Activar YOLO desde el PC: solo se activa desde la app (con biometría fuerte o el bloqueo de pantalla), para que el agente no pueda hacerlo.
  - YOLO persistente: vive en la memoria de `adbuddy serve` y no sobrevive a reiniciarlo; las duraciones son 15 min, 1 h o `session`.
  - YOLO parcial (levantar solo algunas puertas): es todo salvo la pausa.
- **Límites conocidos y aceptados por la persona usuaria** (escritos en `docs/safety.md#yolo-mode`):
  - Un proceso o servicio de accesibilidad que el agente deje vivo bajo YOLO (`nohup &`, `install`, `settings put secure enabled_accessibility_services`, un script opaco o texto armado con fragmentos) puede pulsar los botones de confirmación o reescribir el enlace de la app cuando YOLO acaba. Consejo: revisar procesos y servicios de accesibilidad, o reiniciar el móvil. `TestYoloRunsOpaqueScriptsKnownLimit` lo fija.
  - Con YOLO activo y la app conectada, una app protegida del usuario no se protege (la app deja de marcarla y el host deja de aplicar su lista); ADBuddy sí sigue protegida, igual que las ventanas que el móvil no deja leer. Cambiarlo es decidir qué marca la app (`YoloProtection`) y qué levanta `d.lift` en `appProtected`.
  - `am force-stop`, `pm clear`, `pm disable-user`, `appops` y `reboot` de ADBuddy se ejecutan; el instalador y la ficha de ADBuddy en Ajustes no están protegidos. El canal de la app (`content`, `run-as`, `am` dinámico) sí está cerrado.
  - PIN observable por un script del agente sin biometría fuerte; sin bloqueo de pantalla, un toque inyectado puede terminar la activación; el botón «Confirmar» del sistema con cara fuerte es del sistema.
  - Con el aviso de la app imposible de pintar, `dump`, `tap` y `type` van sin la línea `⚠ YOLO`: la app no tiene cómo avisar al host sin un campo de protocolo nuevo.
- **Aplazados con motivo (revisiones):**

| Id | Qué | Nota |
|---|---|---|
| Y-a | `noticeInImage` se marca aunque con YOLO el aviso nunca llegó a pintarse | Cosmético. |
| Y-b | `androidx.biometric:1.1.0` es de 2021 | Mirar si hay una estable más nueva cuando haya red. |
| Y-c | Tras un `offFailed` en caliente el texto de fallo de la notificación se queda hasta el siguiente cambio de modo | El consejo (`adbuddy yolo off`) sigue siendo cierto. |
| Y-d | En un proceso en frío el `Bridge` no está conectado: el Apagar de una notificación que sobrevivió a su proceso dice «usa `adbuddy yolo off`» | Arrancar el servicio desde el receptor sería la mejora. |
| Y-e | `setSocketForTest` interno en `Bridge` en lugar de inyectar el socket por reflexión en un test de A3 | Opcional. |

## Inventario (ficheros nuevos y clave)

- Host: `daemon/yolo.go` (estado, caducidad, observadores), `cli/phone_yolo.go` (`yolo.set`/`yolo.state`, `adbuddy yolo off|status`), `classify` (`AppChannel`), `daemon.go`/`guard.go`/`ui.go`/`undo.go` (`d.paused()`, `d.lift`, `liftQuestion`), `cli/phone_perm.go` (permisos de Claude Code `allow` por YOLO).
- App: `yolo/` (`YoloUi`, `YoloAuth`, `YoloBiometric`, `YoloNotifier`, `YoloReceiver`), `a11y/YoloProtection`, banda y aviso de activación en `ui/`.
- Dependencias nuevas: `androidx.biometric:1.1.0`, `androidx.fragment:1.8.9` (explícita: biometric arrastra un fragment 1.2.5 incompatible con `activity-compose` 1.13 en una `FragmentActivity`).
- Protocolo (aditivo): `yolo.set`, `yolo.state`, `snapshot.yolo`, `permission.resolved.summary` y `by: "yolo"`, `journal.entry.yolo`.
- Pruebas de emulador: `host/internal/emutest/yolo_test.go` (`TestYolo*`).

## Validaciones manuales (exigen el móvil real; con permiso explícito del usuario)

1. **Apps protegidas con el servicio de accesibilidad real:** con YOLO, `adbuddy screen` y `adbuddy tap` en una app protegida funcionan; con ADBuddy delante siguen bloqueados; al apagar YOLO vuelven a bloquear.
2. **Notificación de YOLO en la pantalla de bloqueo del Pixel:** visible, con «Apagar», y el botón apaga sin pedir el PIN.
3. **Pausa → apagado desde el móvil real:** pausar con YOLO activo apaga la banda y la notificación a la vez.
4. **Claude Code real:** una pregunta de permiso con YOLO se aprueba sin tarjeta; `AskUserQuestion`/`ExitPlanMode` no quedan en un estado raro al responderse `allow`.
5. **Caducidad con el PC dormido:** activar 15 min, suspender el PC más de 15 min, despertar: el modo está apagado y la app recibe el `yolo.state` apagado.
6. **Cuenta atrás de la banda con un reloj del móvil desajustado:** usa el reloj del host (`HostClock`).
7. **Aviso de activación con letra grande y TalkBack:** la lista se lee entera y el botón espera al armado; la banda no lee los segundos.
8. **Que el agente real no activa YOLO tocando la app:** con ADBuddy delante, `adb shell input tap` sobre el interruptor está bloqueado.
9. **Autenticación de la activación:** con huella o cara fuerte sale el `BiometricPrompt` (solo biometría) y cancelarlo no manda nada; sin biometría fuerte sale la pantalla del PIN; sin bloqueo de pantalla sale el aviso específico y, tras el armado, se activa. La rama biométrica no se probó en el emulador (no tiene biometría).
10. **Un proceso en segundo plano tras la caducidad** (límite aceptado): comprobar que, con ADBuddy delante, la guarda lo bloquea mientras YOLO esté apagado y que activar exige la credencial.
11. **Recrear la Activity con el aviso del PIN abierto** pierde el resultado (falla seguro): comprobar que no se manda nada y que se puede volver a activar.
12. **El Apagar de una notificación que sobrevivió a su proceso** (Y-d): comprobar el texto de fallo.
13. **A ojo:** el apretón vertical entre la banda y el título «ADBuddy · Ready» de la cabecera (los bounds no se solapan: banda `[110,165][873,218]`, menú `[975,311][1038,374]`, barra de estado hasta 128 px en el emulador API 34).
