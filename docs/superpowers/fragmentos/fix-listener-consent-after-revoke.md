# Fragmentos para `pendientes-m3-m5.md` y el inventario de validaciones (fix/listener-consent-after-revoke)

El líder pidió no editar esos dos ficheros en esta rama. Aquí va el texto exacto a sustituir al integrar. Cada sección dice el fichero y qué sustituir.

## 1. `docs/superpowers/plans/pendientes-m3-m5.md`

Sustituir la sección entera «### Post-v1: consentimiento del listener (M5c-32 b)» (de su título hasta la línea anterior a «### Post-v1: Permitir tras desbloquear con PIN (M2-8)») por:

```
### CERRADA: consentimiento del listener (M5c-32 b)

CERRADA en la rama `fix/listener-consent-after-revoke` (plan `2026-10-03-m5c-reconfirmar-al-enlazar.md`). Era un RIESGO CONOCIDO (ruling 15 de M5c): severidad importante, no crítica (no ejecutaba un 🔴 sin confirmar).

- **Hueco medido** (API 34, 2026-10-03): con la app muerta, al revocar la marca sobrevivía; al volver a permitir, la puerta leía sin otro toque (`mark=true`, `Ok:5`). El sistema retira el permiso, pero un reinicio ya programado conecta el servicio sin llamar a `onListenerDisconnected` ni a `onDestroy`.
- **Arreglo:** `AgentNotificationListener.onListenerConnected` borra la marca (`NotificationGate.onConnected { NotificationConsent.of(this).revoke() }`) como PRIMERA sentencia, antes de `current = this` y de `_connected.value = true`, de modo que la puerta nunca ve el listener con una marca de un enlace anterior. Se mantienen los `revoke()` de `onListenerDisconnected` y `onDestroy`.
- **Coste aceptado:** un toque en «Dejar que el agente lea» tras cada reenlace en un proceso nuevo (reinicio, actualización, muerte de la app por el sistema). Un toque durante un reenlace (entre la desconexión de un enlace y la conexión del siguiente) también se pierde: molestia visible en pantalla, no fuga. Si molesta, un aviso de prioridad baja «vuelve a confirmar el acceso» (post-v1; no se añadió superficie).
- **Tests:** JVM: `NotificationGateTest.aNewBindingOfTheListenerClearsTheMark`, `ListenerIsReadOnlyTest` (regex por método: `onListenerConnected` exige `onConnected` primero; `onListenerDisconnected` y `onDestroy`, `onUnbound`), `BubbleFlowTest.aNewBindingOfTheListenerAsksForANewTap`. Emulador: `TestRevokingClearsTheMark/b_dead` exige `mark=false` y `NotConfirmed` (sin salto por API; la marca se lee del fichero con `run-as` y por el canal real, nunca con un `gateRead`, que reinicia el proceso), `TestTheListenerStaysBound` exige que la marca no sobreviva a cada reenlace y que un toque nuevo baste, y el paso «confirmed» de `TestTheListenerGateStates` confirma dentro del `am instrument` (`-e confirm 1`).
- **Alternativas descartadas:** una época del sistema no existe en la API pública; `JobScheduler` con `TriggerContentUri` es frágil con Doze; comparar un contador de enlace con la marca (sin API pública fiable).
```

## 2. `docs/superpowers/plans/2026-10-03-validaciones-manuales.md`

### 2a. Línea 158 (viñeta de M5c-32 (b) en «Hallazgos de la T6»)

Sustituir la línea 158 por:

```
- **M5c-32 (b): con la app muerta, revocar no borraba la marca. CERRADO** (rama `fix/listener-consent-after-revoke`, plan `2026-10-03-m5c-reconfirmar-al-enlazar.md`; ver `pendientes-m3-m5.md`, «CERRADA: consentimiento del listener (M5c-32 b)»). Medido en la API 34 (2026-10-03, `TestRevokingClearsTheMark/b_dead`) antes del arreglo; hoy el test exige `mark=false` y `NotConfirmed`.
```

Las líneas 164-165 («Escenario realista…» y «Atenuantes…») se ponen en pasado: «Escenario que había (revisor opus): …; la puerta leía sin otro toque» y «Atenuantes que había: …», o se borran, porque el hueco está cerrado.

Y en las viñetas de detalle que siguen (líneas 163 y 166), sustituir «El arreglo barato funcionaría aquí: … El test de la (b) fija hoy el comportamiento como riesgo conocido.» por «El arreglo (borrar la marca también en `onListenerConnected`) se aplicó: cuesta un toque tras cada reenlace (reinicio, actualización, `kill`, y también cada `am instrument` de los tests) y cambió lo que esperan `TestTheListenerStaysBound` y M5c-29; `b_dead` exige ahora `mark=false`.» y la línea 166 por «Arreglo y tests: en «CERRADA: consentimiento del listener (M5c-32 b)» de `pendientes-m3-m5.md`.»

### 2b. Fila M5c-29 (línea 277)

En la columna de resultado, sustituir la última frase de la parte de la marca «Al final la app lee `GRANTED`, la marca sigue (`ON`) y la puerta lee.» por:

```
Tras cada reenlace la marca YA NO sobrevive (cada `onListenerConnected` la borra, cierre de M5c-32 b): el arnés la lee del fichero de la app (`run-as`) y exige `false`; el agente recibe `unavailable` hasta que la persona toca otra vez, y con un toque basta (la lectura por el canal da `Ok` y ningún enlace tardío lo borra).
```

### 2c. Fila M5c-32 (línea 280)

Sustituir la columna 3 «Sí, hecha en la API 34 (T6); **hallazgo** en (b)» por «Sí, hecha en la API 34 (T6); (b) CERRADA por `fix/listener-consent-after-revoke`», y en la columna de resultado sustituir desde «**Resultado: la marca sobrevive**» hasta «(ver «Hallazgos de la T6»).» por:

```
**Resultado antes del arreglo: la marca sobrevivía** y, permitido de nuevo, la puerta leía sin otro toque (`mark=true`, `Ok`). **Cerrado:** `onListenerConnected` borra la marca en cada enlace; `b_dead` exige ahora `mark=false` / `NotConfirmed` (fichero por `run-as` y canal real, sin `gateRead`, que reinicia el proceso), sin salto por API.
```

Columna «qué falta»: sustituir «El arreglo de (b) lo decide el usuario tras la v1.» por «—», y la última columna «Hecha (T6); riesgo conocido, decisión tras la v1» por «Hecha (T6); (b) cerrada».
