package tech.hybriq.adbuddy.term

/** The quick keys under the terminal (spec M3 §7.2): they go as terminal.input, as the terminal would send them. */
enum class QuickKey(val label: String, val bytes: ByteArray) {
    ESC("Esc", byteArrayOf(0x1b)),
    CTRL_C("Ctrl+C", byteArrayOf(0x03)),
    UP("↑", "\u001b[A".toByteArray()),
    DOWN("↓", "\u001b[B".toByteArray()),
    TAB("Tab", "\t".toByteArray()),
    ENTER("Enter", "\r".toByteArray()),
    ONE("1", "1".toByteArray()),
    TWO("2", "2".toByteArray()),
    THREE("3", "3".toByteArray()),
}

object Keys {
    /** terminal.input carries at most 64 KiB (protocol spec §9): cut into chunks, never inside a UTF-8 character. */
    const val MAX_INPUT = 64 * 1024

    fun chunks(bytes: ByteArray): List<ByteArray> {
        val out = ArrayList<ByteArray>()
        var from = 0
        while (from < bytes.size) {
            var to = minOf(from + MAX_INPUT, bytes.size)
            // A continuation byte is 10xxxxxx: step back to the start of the character.
            while (to < bytes.size && to > from + 1 && (bytes[to].toInt() and 0xC0) == 0x80) to--
            out += bytes.copyOfRange(from, to)
            from = to
        }
        return out
    }
}
