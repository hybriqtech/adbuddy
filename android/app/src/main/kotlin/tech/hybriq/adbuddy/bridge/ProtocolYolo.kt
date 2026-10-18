package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.Serializable

// YOLO mode payloads (protocol spec §8.3). Mirrors host/internal/bridge/protocol_yolo.go.

/** Turns YOLO on for [minutes] (15 or 60) or until the session ends ([session]), or off ([off]): exactly one is set. */
@Serializable
data class YoloSet(val minutes: Int = 0, val session: Boolean = false, val off: Boolean = false)

/** YOLO as the host has it. [until] is epoch milliseconds of the host's clock (0 when off or until the session ends). */
@Serializable
data class YoloState(val active: Boolean = false, val until: Long = 0, val session: Boolean = false)
