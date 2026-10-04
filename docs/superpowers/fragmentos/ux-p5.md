# Fragmento: paquete P5 de UX (confirmaciones y consentimiento)

Rama `ux/p5` (sobre `ux/p6`, que lleva P2-P4 y P6a de #79). Plan: `docs/superpowers/plans/2026-10-04-ux-p5-confirmaciones-y-consentimiento.md`. Base de seguridad: #77 (armado del diálogo y tapjacking del consentimiento y de `RemoteCard`, hallazgo O1 de la revisión).

## La regla fija y cómo se cumple

Ningún cambio da al agente una forma de aprobar una 🔴, y ninguno cambia qué se concede ni cómo se responde.

- El aviso del chat solo reabre: su único botón («Ver») quita el id de `closed` y no hay botón de permitir. Al reabrir, el diálogo se recompone desde cero: vuelven el armado de 600 ms, la puerta de desplazamiento (`unread`) y la guarda anti-tapjacking. Lo prueba `ConfirmReopenTest` (tras «Ver», Permitir sigue desactivado y un toque no responde).
- No se tocaron `ConfirmHost` (el `now` de `status`, `canSend` y armado), `ObscuredTouchGuard`, `ARMING_MS`, `armed`, `unread` ni `enabled = !unread && armed`, ni los `onAnswer`. Los tests de armado, de desplazamiento y de tapjacking existentes (`ConfirmHostArmingTest`, `ConfirmDialogArmingTest`, `ConfirmDialogScrollTest`, `TouchGuardSourceTest`) pasan sin una línea modificada.
- Revisión de cada tarea por `adbuddy-reviewer-opus`; revisión final de la rama también.

## Qué cambió por tarea

1. **Pregunta cerrada sin querer (C2).** `ConfirmUi.closedWaiting`, `PendingQuestionsBanner` (superficie de error, plural `confirm_waiting`, botón `confirm_waiting_show`, región viva cortés, sin botón que responda) y cableado en `ChatRoot`/`ChatScreen`/`MainActivity`/`ChatBubbleActivity`. `ConfirmDialog` lleva `DialogProperties(dismissOnClickOutside = false)`: tocar fuera ya no cierra la pregunta; Atrás sí, así que Pausar/Cortar siguen al alcance.
2. **Cuenta atrás (C7).** Una línea fija «Caduca en m:ss» sobre los botones, siempre visible (fuera de la parte que desplaza), en color de error y negrita por debajo de 10 s, sin región viva (no se anuncia cada segundo). `ConfirmUi.secondsLeft` redondea hacia arriba. `ConfirmHost` pasa un `displayNow` propio que solo alimenta esa línea. Con esto el cuerpo del diálogo queda como `Column { Column(scroll){…}; cuenta atrás; aviso de desplazamiento }`.
3. **Botones (C8, C9).** Rechazar es el botón relleno y más llamativo, Permitir va con contorno, «Permitir este mismo comando durante la sesión» queda aparte, discreto y con su alcance en la etiqueta. Hay 8 dp entre botones, medidos sobre las áreas táctiles en los cuatro modos (letra 200 % incluida).
4. **Consentimiento (O9).** `consent_a11y` dice de verdad lo que concede (leer la pantalla, tocar y escribir; marco y botón Pausa mientras actúa; nunca en ADBuddy ni en apps protegidas; los demás servicios siguen activados): verificado contra `UiExecutor` y `EnableAccessibility`. El título es cabecera, la nota va en `bodyMedium` y el pie usa `ActionFooter`: a 200 % los botones se apilan.
5. **Estado de acceso (O21).** `access_state_confirm` pasa a «Android lo permite; falta tu confirmación» / «Android allows it; your confirmation is missing». Solo texto.

## Imágenes

- Nuevas: `pending_questions_banner_*` y `consent_*`, en los cuatro modos.
- Regrabadas: `access_needs_confirmation_*` (incluidas las dos `below_font_200*`) y `settings_proposed_*`, las que enseñan el título del estado. `onboarding_2_notification_access_*` no cambió.
- El diálogo 🔴 se pinta en su propia ventana y no se captura: se comprueba con medidas (`ConfirmDialogLayoutTest`, `ConfirmDialogButtonsTest`, el test de fuente `ConfirmDialogButtonsSourceTest` que fija la clase de cada botón).

## Cambios fuera de la app

- `host/internal/emutest/confirm_test.go`: `txtAllowSession` pasa al texto nuevo en inglés (antes solo se usaba en una comprobación negativa, así que con el literal viejo habría pasado sin vigilar nada).
- `host/internal/emutest/listener_test.go`: `accessNeedsTap` pasa al título nuevo; ese sí lo espera una comprobación positiva (`waitAccessState`) y se habría roto en el emulador del CI. Ninguno de los dos se ha podido ejecutar aquí (el emulador es del CI): `TestAccessState` (sin emulador) pasa.
- Fuera de P5, un arreglo hecho en `ux/p6` (c8f1a81b): `NotificationsRationaleDialog` (P4) no llevaba `ObscuredTouchGuard()` y `TouchGuardSourceTest` fallaba.

## Aplazado con motivo

- C10 (ver el comando en el bloqueo) pasa a `seguridad`: la spec fija los botones del bloqueo, no que se vea el comando, y una versión pública podría llevar acciones.
- `CATEGORY_CALL` de la notificación de confirmación y O12: de `seguridad` y `latencia`.
- C3/O4: se hicieron en P4.
- O1 (tapjacking del consentimiento y de `RemoteCard`): ya lo hizo #77.

## PARA EL CONTROLADOR DE INTEGRACIÓN Y `seguridad` (no se tocan aquí)

- **Spec §4.5 (~línea 210)** dice «Permitir esto durante la sesión» y «solo cubre la misma regla con los mismos argumentos»; la etiqueta ahora es «Permitir este mismo comando durante la sesión».
- **`docs/usage.md:92` y `docs/safety.md:82`** citan «Allow during the session» (y, ya anotado en `ux-revision.md`, `docs/usage.md:~107` y `docs/safety.md:~84` sobre `--confirm deny`).
- **Asimetría de la accesibilidad:** la lectura de pantalla (`dump`) llama a `acting(null)` sin comprobar que el marco llegó a mostrarse, mientras que tocar y escribir sí se niegan sin marco. Es un hecho que `seguridad` decide si cambia; el texto del consentimiento dice «mientras actúa» y no «con el marco visible», y es exacto porque el host bloquea toda operación de pantalla con la pausa activa.
- **Matiz de `EnableAccessibility`:** al poner `accessibility_enabled 1`, un servicio que ya estaba en la lista con la accesibilidad global apagada también se encendería. Raro; nota para la spec.

## Notas honestas

- Tocar fuera del diálogo ya no lo cierra; Atrás sí. Es un cambio de comportamiento deliberado.
- El aviso del chat solo se ve en el chat: en Ajustes o en Actividad, con una pregunta cerrada, solo queda la notificación.
- El reloj propio de `ChatRoot` (que hace desaparecer el aviso al caducar la pregunta cerrada) no tiene test propio; «Ver» vuelve a mirar el reloj al tocar y reabrir una pregunta caducada solo enseña su estado.
- Rechazar y los dos permisos van en una sola columna dentro de `confirmButton` (sin `dismissButton`): en la fila del `AlertDialog`, Rechazar se solapaba con la etiqueta ancha de «durante la sesión» (lo vio el test en los cuatro modos). El orden de lectura es el de antes.
- Con letra de 200 % el consentimiento apila Aceptar arriba a todo el ancho; el consentimiento no tiene armado (ya era así) y `ObscuredTouchGuard` sigue filtrando.
- Las pruebas del armado no pueden afinar a 599/601 ms: el reloj de Compose avanza en fotogramas de 16 ms; usan márgenes de varios fotogramas, como `ConfirmHostArmingTest`.
- El color de error de la cuenta atrás se prueba a 9 s (error y negrita) y a 10 s (ni uno ni otro).

## Validaciones manuales pendientes (Pixel)

Ninguna se pide al usuario ahora:
- Cerrar una 🔴 con Atrás y reabrirla desde el aviso del chat (la tarjeta vuelve con «Permitir» apagado 600 ms).
- El aspecto de los botones de la 🔴 con letra grande: Rechazar relleno, Permitir con contorno y «durante la sesión» aparte.
- El consentimiento con TalkBack leyendo la frase de la accesibilidad, y a 200 %.
