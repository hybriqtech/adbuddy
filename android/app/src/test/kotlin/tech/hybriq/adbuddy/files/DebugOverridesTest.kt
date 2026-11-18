package tech.hybriq.adbuddy.files

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class DebugOverridesTest {
    @get:Rule val tmp = TemporaryFolder()

    private fun capFile(text: String?): File =
        File(tmp.root, DebugOverrides.CACHE_CAP_FILE).also { if (text != null) it.writeText(text) }

    @Test fun aDebuggableAppHonoursTheFile() {
        assertEquals(4194304L, DebugOverrides.cacheCap(true, capFile("4194304\n")))
    }

    @Test fun anAppThatIsNotDebuggableIgnoresIt() {
        assertNull(DebugOverrides.cacheCap(false, capFile("4194304")))
    }

    @Test fun aMissingOrBadFileMeansNoOverride() {
        assertNull(DebugOverrides.cacheCap(true, capFile(null)))
        for (bad in listOf("", "abc", "0", "-5", "1.5")) assertNull(bad, DebugOverrides.cacheCap(true, capFile(bad)))
    }
}
