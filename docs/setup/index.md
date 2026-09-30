---
title: Set up your phone for ADBuddy
lang: en
---

# Set up your phone for ADBuddy

[Leer en español](es/)

ADBuddy talks to your phone over **wireless debugging**, a developer option built into Android 11 and newer. You turn it on once; after that the PC finds the phone by itself on your Wi-Fi.

Keep `adbuddy setup` running on the PC while you follow these steps.

## 1. Turn on Developer options

1. Open **Settings → About phone**.
2. Tap **Build number** seven times. The phone may ask for your PIN.
3. You will see "You are now a developer!".

![Pending screenshot: About phone, Build number](img/pending.svg)

On some phones **Build number** is under **Settings → About phone → Software information**.

## 2. Turn on Wireless debugging

1. Open **Developer options**: on a Pixel it is in **Settings → System → Developer options**; on other phones it is usually at the end of **Settings**.
2. Turn on **Wireless debugging**.
3. When the phone asks "Allow wireless debugging on this network?", tick **Always allow on this network** and tap **Allow**. This network becomes a trusted network: the phone reconnects on it by itself.

![Pending screenshot: Wireless debugging switch](img/pending.svg)

The phone and the PC must be on the **same Wi-Fi network**.

## 3. Pair with the QR code

1. Tap the words **Wireless debugging** (not the switch) to open its screen.
2. Tap **Pair device with QR code**.
3. Scan the QR code that `adbuddy setup` shows on the PC.

![Pending screenshot: Pair device with QR code](img/pending.svg)

The phone shows the PC in **Paired devices** and the terminal says "Paired".

### If the QR code does not work

Run `adbuddy setup --code` instead:

1. On the phone, tap **Pair device with pairing code**.
2. Type the **IP address & Port** and the 6-digit **Wi-Fi pairing code** that the phone shows into the terminal.

![Pending screenshot: Pair device with pairing code](img/pending.svg)

## Troubleshooting

- **The phone never shows up**: check that both are on the same Wi-Fi, that wireless debugging is still on (Android turns it off when you change network unless the network is trusted) and that the network does not isolate its clients (guest networks often do).
- **"Wrong password"**: the pairing code changes every time the dialog opens; use the current one.
- **Windows asks about the firewall**: allow adb on private networks; it only listens on your PC.

## What ADBuddy does with this access

Every command the agent sends goes through ADBuddy: it is classified by risk, written to a journal with its undo, and anything irreversible waits for your confirmation. Your PIN is never asked for or stored.
