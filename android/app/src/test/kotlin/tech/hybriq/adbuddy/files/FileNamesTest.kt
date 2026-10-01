package tech.hybriq.adbuddy.files

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class FileNamesTest {
    @Test fun onlyTheBaseNameWithoutControlCharacters() {
        assertEquals("passwd", FileNames.clean("../../etc/passwd"))
        assertEquals("x.txt", FileNames.clean("C:\\Users\\me\\x.txt"))
        assertEquals("ab.txt", FileNames.clean("a\u0000b\n.txt"))
        assertEquals("file", FileNames.clean(""))
        assertEquals("file", FileNames.clean(".."))
        assertEquals("bashrc", FileNames.clean(".bashrc"))
    }

    @Test fun longNamesKeepTheirExtensionAndNeverSplitACharacter() {
        val n = FileNames.clean("é".repeat(80) + ".png")
        assertTrue(n.toByteArray().size <= FileNames.MAX_BYTES)
        assertTrue(n.endsWith(".png"))
        assertTrue("no broken character", n.removeSuffix(".png").all { it == 'é' })
        assertEquals("short.txt", FileNames.clean("short.txt"))
    }

    @Test fun aCutNeverLeavesDotsOrSpacesBeforeTheExtension() {
        assertEquals("a".repeat(95) + ".png", FileNames.clean("a".repeat(95) + ". " + "b".repeat(10) + ".png"))
        assertEquals("a".repeat(95) + ".png", FileNames.clean("a".repeat(95) + "  " + "b".repeat(10) + ".png"))
        assertEquals("a".repeat(99), FileNames.clean("a".repeat(99) + ". " + "b".repeat(10)))
    }
}
