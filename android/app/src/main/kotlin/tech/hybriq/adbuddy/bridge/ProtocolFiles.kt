package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.Serializable

// Attachment and file payloads (protocol spec §6): M3. Mirrors host/internal/bridge/protocol_files.go.

/** Starts or resumes an upload. id: chosen by the app, 32 lower-case hex digits. */
@Serializable
data class AttachBegin(val id: String, val name: String, val mime: String, val size: Long, val sha256: String)

/** Where the upload goes on: the bytes the host already has, or 0. */
@Serializable
data class AttachReady(val id: String, val offset: Long)

/** Every byte of an upload was sent. */
@Serializable
data class AttachEnd(val id: String)

/** A finished upload, kept in inbox/. */
@Serializable
data class AttachDone(val attachment: Attachment)

/** Asks for an attachment's bytes from offset on. */
@Serializable
data class FileGet(val id: String, val offset: Long = 0)

/** Follows the last binary frame of a download. */
@Serializable
data class FileEnd(val id: String, val size: Long)

/** Stops a download in progress. */
@Serializable
data class FileCancel(val id: String)

/** A screenshot the host took for capture.request, kept in inbox/. */
@Serializable
data class CaptureReady(val attachment: Attachment)
