package tech.hybriq.adbuddy.control

import java.io.IOException
import java.net.InetSocketAddress
import java.net.Socket
import kotlinx.coroutines.delay

/** What is seen after a cut whose settings were written (addendum 2026-10-19, R22). */
enum class CutCheck { DONE, STILL_CONNECTED }

/**
 * The check after a cut (R22): on Android 17 the reading cannot prove adb_enabled=0 applied, so the cut is also
 * judged by what it must stop. A channel that was alive at the tap must fall, and nothing may answer on
 * 127.0.0.1:5555 (adbd in tcpip mode, the remote mode's port), both within [WAIT_MS]. The channel only speaks against
 * the cut here: it never undoes one (R20).
 */
object CutConfirm {
    const val WAIT_MS = 5_000L
    const val STEP_MS = 250L

    /** [wasUp]: the channel was alive at the tap; [channelUp] and [listening] are asked again on each step. Blocking: run off the main thread. */
    suspend fun check(wasUp: Boolean, channelUp: () -> Boolean, listening: () -> Boolean, waitMs: Long = WAIT_MS, stepMs: Long = STEP_MS): CutCheck {
        var waited = 0L
        while (true) {
            val connected = wasUp && channelUp()
            if (!connected && !listening()) return CutCheck.DONE
            if (waited >= waitMs) return CutCheck.STILL_CONNECTED
            delay(stepMs)
            waited += stepMs
        }
    }

    /** Whether something accepts connections on 127.0.0.1:5555, as CutAccessOnEmulatorTest.listening(). Never on the main thread. */
    fun adbdListening(): Boolean = try {
        Socket().use { it.connect(InetSocketAddress("127.0.0.1", 5555), 500) }
        true
    } catch (e: IOException) {
        false
    }
}
