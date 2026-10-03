# M5c: la confirmación no sobrevive a un reenlace del listener (cierre de M5c-32 b)

> Plan corto (una tarea de producción de consentimiento). Ejecuta con `superpowers:subagent-driven-development`; revisión opus del plan antes de ejecutarlo y de la revisión del diff después.

**Objetivo:** una revocación del acceso a notificaciones debe PERSISTIR: tras revocar y volver a permitir (por el usuario o por el agente con `adbuddy tap` en Ajustes, que no está protegido), el agente no puede leer hasta que el usuario toque otra vez «Dejar que el agente lea».

**Spec:** `docs/superpowers/specs/*m5*` §3.5 y el ruling 15 de `docs/superpowers/plans/2026-10-02-adbuddy-m5c-notificaciones.md` (línea ~52). Hallazgo y medición: `docs/superpowers/plans/2026-10-03-validaciones-manuales.md` («Hallazgos de la T6», M5c-32 b) y `pendientes-m3-m5.md` («Post-v1: consentimiento del listener (M5c-32 b)»).

## Hecho medido (API 34)
Con la app MUERTA al revocar, el sistema retira el permiso pero un reinicio ya programado conecta el servicio sin llamar a `onListenerDisconnected`: la marca de confirmación (`NotificationConsent`) sigue en `true`, y al volver a permitir la puerta lee sin otro toque (`mark=true`, `Ok`). Hoy la marca solo se borra en `onListenerDisconnected`, `onDestroy`, en la puerta cuando ve `Listed.NO` y en la pantalla mientras está abierta.

## Cambio (producción)
Archivo: `android/app/src/main/kotlin/tech/hybriq/adbuddy/listener/AgentNotificationListener.kt` y `NotificationGate.kt`.
1. `NotificationGate.onConnected(revoke: () -> Unit) = revoke()` (puro, como `onUnbound`), con su KDoc: el sistema conectó el listener; cualquier confirmación anterior pertenece a un enlace que ya no existe: se borra ANTES de que el listener sea visible (`current`/`connected`) para que la puerta no pueda leer con una marca vieja.
2. `AgentNotificationListener.onListenerConnected()`: primero `NotificationGate.onConnected { NotificationConsent.of(this).revoke() }` y solo después `current = this; _connected.value = true`. Se mantienen los `revoke()` de `onListenerDisconnected` y `onDestroy` (no quitar ninguno: quitar solo uno no hace fallar el test, defecto del plan de T6).
3. El botón «Dejar que el agente lea» exige `GRANTED`, es decir, listener ya conectado: el borrado de `onListenerConnected` nunca pisa una confirmación recién hecha. Un reenlace posterior (reinicio, actualización, kill del sistema) exige un toque nuevo: coste aceptado por el supervisor.
4. `revoke()` es síncrono (`commit()`); en `onListenerConnected` corre en el hilo principal, igual que ya hace `onListenerDisconnected`: sin cambio de riesgo.
5. Sin aviso de baja prioridad en esta tarea (post-v1, si se quiere): no añadir superficie.

## Tests (RED→GREEN)
- JVM: `NotificationGateTest`: `onConnected` llama a `revoke`; test de fuente en `ListenerIsReadOnlyTest` que exige `revoke()` (vía `onUnbound`/`onConnected`) en los TRES métodos del ciclo de vida y que en `onListenerConnected` va ANTES de `current = this` (mutación: quitarlo de `onListenerConnected` → rojo; moverlo después → rojo). `onlyTheLifecycleIsOverridden` no cambia.
- Instrumentados/emutest de T6 (`host/internal/emutest/listener_test.go`, `NotificationListenerOnEmulatorTest.kt`): OJO, cada `am instrument` reinicia el proceso de la app y el sistema reconecta el listener: con el cambio, la marca puesta en un `am instrument` NO sobrevive al siguiente. Adapta: confirmar y leer la puerta dentro del MISMO proceso/`am instrument` (o confirmar por el botón real con uiautomator una vez estabilizado el enlace y leer por el canal real del arnés sin reiniciar el proceso). Cambios esperados: `b_dead` (32 b) pasa a EXIGIR lo seguro (`mark=false`, `NotConfirmed`) sin condición de API; `TestTheListenerStaysBound` (M5c-29: tras `kill -9`/`force-stop`/reinicio el listener sigue enlazado y la marca YA NO sobrevive: `mark=false`; se exige un toque nuevo); el paso «confirmed» de `TestTheListenerGateStates` se rehace como arriba. RED: con el código de hoy esos tests (versión nueva) fallan; GREEN con el cambio.
- Docs: `pendientes-m3-m5.md` (la sección post-v1 de M5c-32 b pasa a CERRADA, con el coste: un toque tras cada reenlace en proceso nuevo), el inventario (`2026-10-03-validaciones-manuales.md`, M5c-29/32), spec §3.5 si dice lo contrario, `docs/safety.md` si lo menciona.

## Riesgos y preguntas para el revisor del plan
- ¿Hay algún camino en que `onListenerConnected` se llame justo después de que el usuario confirme (reenlace inmediato) y borre una confirmación legítima de forma repetida (bucle de reconexión)? Coste: molestia, no fuga.
- ¿Algún camino en que `onListenerConnected` NO se llame tras volver a permitir con la app muerta (Android arranca el proceso y conecta: sí lo llama)? Si el sistema dijera «already bound» sin llamar, el hueco seguiría: medirlo en API 34 con el test nuevo (`b_dead` exigiendo `mark=false`).
- ¿Conviene además que la puerta (`NotificationGate.read`) compare un contador/hora de enlace guardados con la marca? (descartado: sin API pública fiable).
