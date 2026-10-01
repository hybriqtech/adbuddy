package tech.hybriq.adbuddy.term

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Test

class QuickKeysTest {
    @Test fun theBytesAreTheTerminalsOwn() {
        assertArrayEquals(byteArrayOf(0x1b), QuickKey.ESC.bytes)
        assertArrayEquals(byteArrayOf(0x03), QuickKey.CTRL_C.bytes)
        assertArrayEquals("\u001b[A".toByteArray(), QuickKey.UP.bytes)
        assertArrayEquals("\u001b[B".toByteArray(), QuickKey.DOWN.bytes)
        assertArrayEquals("\t".toByteArray(), QuickKey.TAB.bytes)
        assertArrayEquals("\r".toByteArray(), QuickKey.ENTER.bytes)
        assertEquals(listOf("Esc", "Ctrl+C", "↑", "↓", "Tab", "Enter", "1", "2", "3"), QuickKey.entries.map { it.label })
    }

    @Test fun inputIsSplitIn64KiBChunksAndNeverInsideACharacter() {
        val big = ByteArray(64 * 1024 + 10) { 'a'.code.toByte() }
        assertEquals(listOf(65536, 10), Keys.chunks(big).map { it.size })
        // "é" is 2 bytes: a cut must not fall between them.
        val s = ("a".repeat(65535) + "é" + "z").toByteArray()
        val parts = Keys.chunks(s)
        assertEquals(s.toList(), parts.flatMap { it.toList() })
        assertEquals("é" + "z", String(parts.last()))
    }
}
