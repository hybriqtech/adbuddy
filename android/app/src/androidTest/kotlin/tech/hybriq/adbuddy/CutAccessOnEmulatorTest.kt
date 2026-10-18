package tech.hybriq.adbuddy

import android.os.Build
import android.util.Log
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.IOException
import java.net.InetSocketAddress
import java.net.Socket
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith
import tech.hybriq.adbuddy.control.CutAccess

/**
 * Validation 6 of M2, the app's half: Cut access and, a while later, Reactivar, with the app's own code
 * (CutAccess, as its buttons do). Cutting closes adb, and with it whoever started this test, so it only runs
 * when host/internal/emutest starts it without waiting (`am instrument` without -w, with -e adbuddyCut 1) and
 * watches the emulator from outside; it reports in logcat (tag ADBuddyEmuCut). Anywhere else (the whole suite,
 * which TestInstrumentedSuite runs) it is skipped.
 */
@RunWith(AndroidJUnit4::class)
class CutAccessOnEmulatorTest {
    @Test fun cutThenReactivate() {
        val args = InstrumentationRegistry.getArguments()
        assumeTrue("only host/internal/emutest runs it", args.getString("adbuddyCut") == "1")
        // Never on a phone, whoever starts it: the emulator's hardware is ranchu (goldfish before it).
        assertTrue("not an emulator (Build.HARDWARE=${Build.HARDWARE}): Cut access is only tested on one",
            Build.HARDWARE == "ranchu" || Build.HARDWARE == "goldfish")
        val after = args.getString("reactivateAfterMs")?.toLongOrNull() ?: 15_000L
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val resolver = context.contentResolver
        log("listening before the cut: ${listening()}")
        val cut = CutAccess.cut(context)
        log("cut=$cut isCut=${CutAccess.isCut(context)}")
        var restored = false
        try {
            // adbd stops a moment after the setting changes.
            var open = listening()
            for (i in 0 until 10) {
                if (!open) break
                Thread.sleep(500)
                open = listening()
            }
            log("listening after the cut: $open")
            Thread.sleep(after)
        } finally {
            // Reactivar whatever happened since the cut: nobody else can, adb is gone.
            restored = CutAccess.restore(context)
            log("restored=$restored isCut=${CutAccess.isCut(context)}")
        }
        assertTrue("Cut access failed", cut)
        assertTrue("Reactivar failed", restored)
    }

    /** Whether something (adbd) accepts connections on 127.0.0.1:5555. */
    private fun listening(): Boolean = try {
        Socket().use { it.connect(InetSocketAddress("127.0.0.1", 5555), 1000) }
        true
    } catch (e: IOException) {
        false
    }

    private fun log(line: String) {
        Log.i(TAG, line)
    }

    private companion object {
        const val TAG = "ADBuddyEmuCut"
    }
}
