package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.Serializable

// Terminal payloads (protocol spec §7): M3. Mirrors host/internal/bridge/protocol_terminal.go.
// epoch names one PTY process and offset counts its output bytes from 0; data is base64.

/** Subscribes to the terminal, from what the app already has (empty and 0: nothing). */
@Serializable
data class TerminalWatch(val epoch: String = "", val offset: Long = 0)

/** The screen at offset: data, written to a clean terminal of cols×rows, draws it. */
@Serializable
data class TerminalSnapshot(val epoch: String, val offset: Long, val cols: Int, val rows: Int, val data: String)

/** Output of the PTY; offset is the position of its first byte. */
@Serializable
data class TerminalOutput(val epoch: String, val offset: Long, val data: String)

/** The size changes from byte offset on. */
@Serializable
data class TerminalSize(val epoch: String, val offset: Long, val cols: Int, val rows: Int)

/** Bytes for the PTY, as they are (quick keys included), at most 64 KiB. */
@Serializable
data class TerminalInput(val data: String)

/** The size that fits the phone's view. */
@Serializable
data class TerminalResize(val cols: Int, val rows: Int)
