package tech.hybriq.adbuddy.bridge

import java.nio.ByteBuffer

/** One binary frame: data at offset in the attachment id. */
class Chunk(val id: String, val offset: Long, val data: ByteArray)

/**
 * Binary frames (protocol spec §6.4): the 16-byte id of an attachment, the
 * 8-byte big-endian offset of the data in the file, and 1 B to 256 KiB of
 * data. Mirrors host/internal/bridge/frame.go.
 */
object Frame {
    const val ID_LEN = 16
    const val OFFSET_LEN = 8
    const val HEADER_LEN = ID_LEN + OFFSET_LEN
    const val MAX_DATA = 256 * 1024

    /** The id written as 32 lower-case hex digits, or null when it is not. */
    fun parseId(s: String): ByteArray? {
        if (s.length != 2 * ID_LEN || s.any { it !in '0'..'9' && it !in 'a'..'f' }) return null
        return ByteArray(ID_LEN) { i -> s.substring(2 * i, 2 * i + 2).toInt(16).toByte() }
    }

    fun idString(id: ByteArray): String = id.toHexString()

    /** The frame of data at offset in id, or null for a bad id, a negative offset or a size outside 1 B to 256 KiB. */
    fun encode(id: String, offset: Long, data: ByteArray): ByteArray? {
        val raw = parseId(id) ?: return null
        if (offset < 0 || data.isEmpty() || data.size > MAX_DATA) return null
        return ByteBuffer.allocate(HEADER_LEN + data.size).put(raw).putLong(offset).put(data).array()
    }

    /** The chunk a frame carries, or null when its size or its offset is wrong. */
    fun decode(frame: ByteArray): Chunk? {
        if (frame.size <= HEADER_LEN || frame.size > HEADER_LEN + MAX_DATA) return null
        val offset = ByteBuffer.wrap(frame, ID_LEN, OFFSET_LEN).long
        if (offset < 0) return null
        return Chunk(idString(frame.copyOfRange(0, ID_LEN)), offset, frame.copyOfRange(HEADER_LEN, frame.size))
    }
}
