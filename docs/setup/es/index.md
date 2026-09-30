---
title: Prepara tu móvil para ADBuddy
lang: es
layout: default
---

# Prepara tu móvil para ADBuddy

[Read in English](../)

ADBuddy habla con tu móvil por **depuración inalámbrica**, una opción de desarrollador que trae Android 11 y posteriores. Se activa una sola vez; después el PC encuentra el móvil solo en tu Wi-Fi.

Deja `adbuddy setup` en marcha en el PC mientras sigues estos pasos.

## 1. Activa las opciones de desarrollador

1. Abre **Ajustes → Información del teléfono**.
2. Toca **Número de compilación** siete veces. Puede que el móvil te pida el PIN.
3. Verás "¡Ya eres desarrollador!".

![Captura pendiente: Información del teléfono, Número de compilación](../img/pending.svg){: width="240"}

En algunos móviles, **Número de compilación** está en **Ajustes → Información del teléfono → Información de software**.

## 2. Activa la depuración inalámbrica

1. Abre las **Opciones para desarrolladores**: en un Pixel están en **Ajustes → Sistema → Opciones para desarrolladores**; en otros móviles, normalmente al final de **Ajustes**.
2. Activa **Depuración inalámbrica**.
3. Cuando el móvil pregunte "¿Permitir la depuración inalámbrica en esta red?", marca **Permitir siempre en esta red** y toca **Permitir**. Así esa red queda como red de confianza y el móvil se vuelve a conectar solo en ella.

![Captura pendiente: interruptor de Depuración inalámbrica](../img/pending.svg){: width="240"}

El móvil y el PC tienen que estar en la **misma red Wi-Fi**.

## 3. Empareja con el código QR

`adbuddy setup` muestra dos códigos QR. El primero abre esta guía y se escanea con la cámara. El de emparejar es el **segundo**, el que aparece bajo "On the phone: Wireless debugging → Pair device with QR code, and scan:".

1. Toca las palabras **Depuración inalámbrica** (no el interruptor) para abrir su pantalla.
2. Toca **Emparejar dispositivo con código QR**.
3. Escanea el **segundo** código QR con el lector que se abre en el móvil, no con la app de la cámara.

![Captura pendiente: Emparejar dispositivo con código QR](../img/pending.svg){: width="240"}

El móvil muestra el PC en **Dispositivos vinculados** y el terminal dice "Paired".

Tienes unos 3 minutos desde que aparece el segundo código QR.

### Si el código QR no funciona

Ejecuta `adbuddy setup --code`:

1. En el móvil, toca **Emparejar dispositivo con código de sincronización**. Deja ese cuadro abierto hasta que el terminal diga "Paired": si lo cierras, el código deja de valer.
2. Escribe en el terminal la **dirección IP y puerto** y el **código de vinculación Wi-Fi** de 6 cifras que aparecen dentro de ese cuadro, no la dirección IP y el puerto de la pantalla principal de Depuración inalámbrica.

![Captura pendiente: Emparejar dispositivo con código de sincronización](../img/pending.svg){: width="240"}

### Si el móvil está en otra red (Tailscale)

El código QR solo funciona en la misma red. Por Tailscale, o en una Wi-Fi que bloquea mDNS:

1. Ejecuta `adbuddy setup --code` y empareja como arriba: con Tailscale activo, el cuadro de emparejamiento muestra la IP de Tailscale del móvil.
2. Cuando setup pida la **Connection address** (dirección de conexión), escribe la dirección IP y el puerto de la pantalla principal de Depuración inalámbrica (o pásala de entrada: `adbuddy setup --code --connect <ip:puerto>`).
3. El puerto cambia cada vez que se vuelve a activar la depuración inalámbrica. Cuando `adbuddy serve` no llegue al móvil, ejecuta `adbuddy connect <nueva ip:puerto>` con la dirección que muestre el móvil en ese momento.

## Si algo falla

- **"I can't see any phone with wireless debugging on this network"** (no veo ningún móvil con la depuración inalámbrica en esta red): mientras espera el código QR, el PC no ve ningún móvil con la depuración inalámbrica. Si el móvil está en otra red (por ejemplo, Tailscale) o la Wi-Fi bloquea mDNS, pulsa Ctrl+C y ejecuta `adbuddy setup --code` (mira el apartado anterior).
- **El móvil no aparece:** comprueba que los dos están en la misma Wi-Fi, que la depuración inalámbrica sigue activa (Android la apaga al cambiar de red, salvo en redes de confianza) y que la red no aísla a sus clientes (las redes de invitados suelen hacerlo).
- **Se acabó el tiempo (`PAIRING_TIMEOUT`):** el móvil tiene unos 3 minutos para escanear el código QR desde que aparece. Vuelve a ejecutar `adbuddy setup`; se salta lo que ya está hecho.
- **"Wrong password" (contraseña incorrecta):** el código cambia cada vez que se abre el cuadro; usa el que se ve en ese momento.
- **`protocol fault` al emparejar con código:** el cuadro se cerró antes de terminar el emparejamiento. También se cierra si cambias a otra app para copiar el código. Mantenlo abierto y escribe el código desde el PC (o desde otro dispositivo), o ábrelo de nuevo para obtener un código nuevo.
- **Windows pregunta por el cortafuegos:** permite adb (la herramienta de depuración de Android, que ADBuddy descarga) en redes privadas; solo habla con dispositivos de tu Wi-Fi.

## Qué hace ADBuddy con este acceso

Cada comando que envía el agente pasa por ADBuddy: se clasifica por riesgo, se apunta en un diario con su forma de deshacerlo, y lo irreversible espera a que lo confirmes. Nunca se pide ni se guarda el PIN del móvil.
