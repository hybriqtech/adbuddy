package tech.hybriq.adbuddy.files

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import tech.hybriq.adbuddy.bridge.Attachment

class FileCacheTest {
    @get:Rule val tmp = TemporaryFolder()

    private fun att(id: String, name: String = "a.png", size: Long = 3) = Attachment(id.padEnd(32, '0'), name, "image/png", size, "x")

    @Test fun namesAreSanitisedAndStayInsideTheFolder() {
        val c = FileCache(tmp.root)
        val f = c.finalFile(att("ab", "../../evil.png"))
        assertEquals(tmp.root, f.parentFile)
        assertEquals("ab".padEnd(32, '0') + "-evil.png", f.name)
    }

    @Test fun hasNeedsTheRightSize() {
        val c = FileCache(tmp.root)
        val a = att("aa", size = 3)
        assertFalse(c.has(a))
        c.finalFile(a).writeBytes(byteArrayOf(1, 2))
        assertFalse(c.has(a))
        c.finalFile(a).writeBytes(byteArrayOf(1, 2, 3))
        assertTrue(c.has(a))
    }

    @Test fun trimRemovesTheOldestFirstUntilItFits() {
        val c = FileCache(tmp.root, maxBytes = 10)
        fun put(name: String, size: Int, age: Long): File = File(tmp.root, name).apply { writeBytes(ByteArray(size)); setLastModified(1_000_000L + age) }
        val old = put("old", 6, 1)
        val mid = put("mid", 6, 2)
        val new = put("new", 6, 3)
        c.trim()
        assertFalse(old.exists()); assertFalse(mid.exists()); assertTrue(new.exists())
    }

    @Test fun trimNeverTouchesPartialDownloads() {
        val c = FileCache(tmp.root, maxBytes = 1)
        val part = File(tmp.root, "x.part").apply { writeBytes(ByteArray(10)) }
        c.trim()
        assertTrue(part.exists())
    }

    @Test fun trimLeavesAnUnderLimitCacheAlone() {
        val c = FileCache(tmp.root, maxBytes = 100)
        val f = File(tmp.root, "x").apply { writeBytes(ByteArray(10)) }
        c.trim()
        assertTrue(f.exists())
    }
}
