# Spike S1 · Modo remoto (`adb tcpip` + Tailscale) y "Cortar acceso"

> Spike desechable. Dispositivo: Pixel 9 Pro XL, Android 17. PC: Windows 11, adb 37.0.1, Tailscale 1.102.4.

## Preguntas

| # | Pregunta | Resultado | Evidencia (sección del registro) |
|---|---|---|---|
| Q1 | ¿`adb tcpip 5555` funciona estando conectado por depuración inalámbrica en Android 17? ¿Se pierde la conexión inalámbrica al hacerlo? | **Sí funciona** (`restarting in TCP mode port: 5555`, salida de consola). La conexión inalámbrica se corta un instante y **vuelve sola**: mismo serial mDNS, `transport_id` 25 → 28, `device` a los 6 s. | "Línea base", "Tras adb tcpip 5555" |
| Q2 | ¿Se puede conectar a `<ip-tailnet>:5555` por Tailscale con el móvil en Wi-Fi? ¿Latencia? | **Sí**: `connected to <ip-tailnet>:5555`, `service.adb.tcp.port=5555`. `shell echo` 165 ms (red local por mDNS: 123 ms); `tailscale ping` 5 ms por ruta directa LAN (`via <LAN>`). Captura PNG de 1,35 MB en 2488 ms. Sin diálogo "¿Permitir depuración?". | "Conexión por Tailscale con Wi-Fi" y línea "captura Wi-Fi+Tailscale" |
| Q3 | ¿Y con solo datos móviles? ¿Latencia y tiempo de una captura? | **Sí** (con mala cobertura móvil): `connected`, `OK 213ms` al primer intento (1 muestra, con mala cobertura), `service.adb.tcp.port=5555`. Tailscale sin ruta directa, por relay DERP(mad): 121-149 ms. Captura PNG de 3,99 MB en 47326 ms (~84 KB/s): usable para comandos, lenta para capturas. | "Solo datos móviles" y línea "captura datos móviles+Tailscale" |
| Q4 | ¿El modo `tcpip` sobrevive al cambio Wi-Fi → datos → Wi-Fi? ¿Vuelve sola la depuración inalámbrica (Wi-Fi 2.0)? | **Sí a las dos, con matices.** (a) El modo `tcpip` (el puerto) sobrevive: `service.adb.tcp.port=5555` con solo datos y `OK 123ms` al volver a Wi-Fi. (b) La sesión abierta sobrevivió al paso datos → Wi-Fi: el `adb connect` al volver dice `already connected`. (c) El paso Wi-Fi → datos **no se probó con la sesión abierta**: en ese paso se hizo `disconnect` + `connect` a propósito. El serial mDNS de Wi-Fi 2.0 vuelve solo (`OK 126ms`) sin tocar nada en el móvil. | "Solo datos móviles", "De vuelta en Wi-Fi" |
| Q5 | ¿`adb_wifi_enabled=0` corta el modo `tcpip`? ¿Y `adb_enabled=0`? | **`adb_wifi_enabled=0`: No.** La sesión abierta sigue (`OK 144ms`) y el puerto 5555 **acepta conexiones nuevas** (`disconnect`, `connect`: `OK 138ms`); solo desaparece el serial mDNS. **`adb_enabled=0`: Sí.** `cannot connect ... actively refused it (10061)`, `probe` FAIL, `adb devices` vacío. | "Cortar acceso, nivel 1", "nivel 1: conexión nueva", "nivel 2" |
| Q6 | ¿Reactivar la depuración a mano recupera el modo `tcpip`? ¿`adb usb` vuelve al modo normal y recupera la depuración inalámbrica? | **Sí, y es un problema.** Reactivar a mano vuelve a abrir el 5555 sin hacer nada más: `connected`, `OK 120ms`, `service.adb.tcp.port=5555`. **`adb usb` sí** vuelve al modo normal (`restarting in USB mode`, `service.adb.tcp.port=0`, `connect` al 5555 rechazado) y la depuración inalámbrica sigue `device` (`adb_enabled=1`, `adb_wifi_enabled=1`). | "Tras reactivar a mano", "Estado final" |

## Conclusión e impacto en la spec (§3.3 modo remoto, §4.6 Cortar acceso)

1. **Modo remoto con solo datos móviles: viable tal como lo describe §3.3.** Con `adb tcpip 5555` hecho desde la depuración inalámbrica, adbd escucha también en la interfaz de Tailscale con solo datos móviles (`OK 213ms`). Además, el modo `tcpip` (el puerto) sobrevive a Wi-Fi → datos → Wi-Fi (`service.adb.tcp.port=5555` con datos, `OK` al volver), la sesión abierta sobrevivió al paso datos → Wi-Fi (`already connected`) y la depuración inalámbrica (Wi-Fi 2.0) vuelve sola. No se probó si una sesión abierta sobrevive al paso Wi-Fi → datos: en esa prueba se reconectó a propósito. Solo hay que preparar el modo `tcpip` mientras el móvil está en una Wi-Fi con depuración inalámbrica. Con datos, Tailscale fue por relay DERP (no hubo conexión directa) y, con mala cobertura, las capturas son muy lentas (3,99 MB en 47 s). La spec debería comprimir o reducir las capturas en modo remoto (JPEG/escala) y avisar de que pueden tardar.
2. **"Cortar acceso" (§4.6) tiene que poner `adb_enabled=0`.** `adb_wifi_enabled=0` no basta: el puerto 5555 sigue aceptando conexiones nuevas por Tailscale. Solo `adb_enabled=0` lo cierra (conexión rechazada). **"Reactivar" tiene que resetear el puerto:** al volver a activar la depuración a mano, reaparece `service.adb.tcp.port=5555` y el 5555 vuelve a aceptar conexiones. Tras reactivar, hay que ejecutar `adb usb` (deja `service.adb.tcp.port=0` y conserva la depuración inalámbrica) y activar el modo remoto solo si el usuario lo pide expresamente. Otra opción es que "Cortar acceso" haga primero `adb usb` y después `adb_enabled=0`, pero no se ha probado si así el puerto queda limpio al reactivar.

   Matices:
   - Lo validado es la combinación `adb_wifi_enabled=0` (nivel 1) seguida de `adb_enabled=0` (nivel 2). No se probó `adb_enabled=0` por sí solo.
   - "Reactivar" se probó con los interruptores de Ajustes, no con la app escribiendo los ajustes mediante `WRITE_SECURE_SETTINGS`. Probablemente sea equivalente, pero no está probado.
   - Hacer `adb usb` tras reactivar hace que "Reactivar" dependa del host y deja el 5555 abierto hasta que el host lo ejecute. Eso choca con el principio de §4.6 de no depender del PC, y hay que resolverlo en la spec.
3. **Latencias medidas** (`adb shell echo ok`, extremo a extremo):
   - red local por mDNS (Wi-Fi 2.0): 123-126 ms;
   - Tailscale con Wi-Fi (ruta directa LAN, `tailscale ping` 5 ms): 120-165 ms;
   - Tailscale con datos móviles (DERP mad, `tailscale ping` 121-149 ms, mala cobertura): 213 ms.

   Capturas PNG: 1,35 MB en 2488 ms por Tailscale con Wi-Fi; 3,99 MB en 47326 ms con datos (las pantallas eran distintas, así que la comparación no es directa).

   La latencia de un comando la domina el arranque de `adb shell`, no la red, salvo cuando se transfieren datos.

Nota: durante todo el spike `serial_wifi` nunca apuntó a un dispositivo `offline`, así que no hizo falta el filtro por estado.

## Registro

### Línea base (2026-09-29T17:52:07Z)
```
List of devices attached
adb-<serie>-<id>._adb-tls-connect._tcp device product:komodo model:Pixel_9_Pro_XL device:komodo transport_id:25

adb_enabled=1
adb_wifi_enabled=1
service.adb.tcp.port=
OK adb-<serie>-<id>._adb-tls-connect._tcp 123ms
```

### Tras adb tcpip 5555 (2026-09-29T17:52:18Z)
```
List of devices attached
adb-<serie>-<id>._adb-tls-connect._tcp device product:komodo model:Pixel_9_Pro_XL device:komodo transport_id:28

serial inalámbrico ahora: adb-<serie>-<id>._adb-tls-connect._tcp
```
- salida de consola de `adb tcpip 5555`: `restarting in TCP mode port: 5555`

### Conexión por Tailscale con Wi-Fi (2026-09-29T17:52:24Z)
```
OK <ip-tailnet>:5555 165ms
service.adb.tcp.port=5555
pong from pixel-9-pro-xl (<ip-tailnet>) via <LAN> in 5ms
```
- salida de consola de `adb connect`: `connected to <ip-tailnet>:5555`
- captura Wi-Fi+Tailscale: 2488 ms, 1345579 bytes

### Solo datos móviles (2026-09-29T17:54:14Z)
```
OK <ip-tailnet>:5555 213ms
adb_wifi_enabled=1
service.adb.tcp.port=5555
pong from pixel-9-pro-xl (<ip-tailnet>) via DERP(mad) in 121ms
pong from pixel-9-pro-xl (<ip-tailnet>) via DERP(mad) in 149ms
direct connection not established
```
- salida de consola de `adb connect`: `connected to <ip-tailnet>:5555`
- Condición de la prueba: el usuario informa de **mala cobertura móvil** durante esta sección. `probe` dio OK al primer intento (sin reintentos). Tailscale va por relay DERP (mad), sin conexión directa.
- captura datos móviles+Tailscale: 47326 ms, 3990926 bytes

### De vuelta en Wi-Fi (2026-09-29T17:56:16Z)
```
OK <ip-tailnet>:5555 123ms
serial inalámbrico: adb-<serie>-<id>._adb-tls-connect._tcp
OK adb-<serie>-<id>._adb-tls-connect._tcp 126ms
```
- salida de consola de `adb connect`: `already connected to <ip-tailnet>:5555`

### Cortar acceso, nivel 1: adb_wifi_enabled=0 (2026-09-29T17:56:36Z)
```
OK <ip-tailnet>:5555 144ms
serial inalámbrico: 
```

### Cortar acceso, nivel 1: conexión nueva (2026-09-29T17:58:00Z)
```
adb connect: connected to <ip-tailnet>:5555
OK <ip-tailnet>:5555 138ms
```
- Nota: en este bloque no se volvió a leer `adb_wifi_enabled`. Pasaron 84 s desde que se puso a 0 (bloque anterior, 17:56:36Z) y no se tocó el móvil entremedias.

### Cortar acceso, nivel 2: adb_enabled=0 (2026-09-29T17:58:12Z)
```
FAIL <ip-tailnet>:5555: adb.exe: device '<ip-tailnet>:5555' not found
```
- `adb connect` en el nivel 2: `cannot connect to <ip-tailnet>:5555: No connection could be made because the target machine actively refused it. (10061)`: el puerto está cerrado (RST), no es que no haya ruta.
- salida de consola de `adb devices -l` tras el nivel 2: `List of devices attached` sin ningún dispositivo.

### Tras reactivar a mano (2026-09-29T17:59:04Z)
```
OK <ip-tailnet>:5555 120ms
serial inalámbrico: adb-<serie>-<id>._adb-tls-connect._tcp
service.adb.tcp.port=5555
```
- salida de consola de `adb connect`: `connected to <ip-tailnet>:5555`

### Estado final (2026-09-29T17:59:19Z)
- salida de consola de `adb -s <ip-tailnet>:5555 usb`: `restarting in USB mode`

```
List of devices attached
adb-<serie>-<id>._adb-tls-connect._tcp device product:komodo model:Pixel_9_Pro_XL device:komodo transport_id:36

service.adb.tcp.port=0
adb_enabled=1
adb_wifi_enabled=1
cannot connect to <ip-tailnet>:5555: No connection could be made because the target machine actively refused it. (10061)
```
