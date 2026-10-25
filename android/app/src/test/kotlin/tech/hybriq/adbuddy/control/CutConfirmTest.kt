package tech.hybriq.adbuddy.control

import java.net.InetAddress
import java.net.ServerSocket
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assume.assumeTrue
import org.junit.Test

/** Addendum 2026-10-19 R22: after a cut, the channel that was alive must fall and nothing may answer on 5555. */
class CutConfirmTest {
    private fun check(wasUp: Boolean, channelUp: () -> Boolean, listening: () -> Boolean) =
        runBlocking { CutConfirm.check(wasUp, channelUp, listening, waitMs = 100, stepMs = 10) }

    private fun answers(vararg values: Boolean): () -> Boolean {
        var i = 0
        return { values[minOf(i++, values.size - 1)] }
    }

    @Test fun aChannelThatFallsIsDone() {
        assertEquals(CutCheck.DONE, check(true, answers(true, true, false), { false }))
    }

    @Test fun aChannelThatStaysIsStillConnected() {
        assertEquals(CutCheck.STILL_CONNECTED, check(true, { true }, { false }))
    }

    @Test fun aPortThatStillAnswersIsStillConnected() {
        assertEquals(CutCheck.STILL_CONNECTED, check(false, { false }, { true }))
    }

    @Test fun aPortThatClosesIsDone() {
        assertEquals(CutCheck.DONE, check(false, { false }, answers(true, true, false)))
    }

    @Test fun aChannelThatWasNotUpIsNotWaitedFor() {
        // R20: the channel only speaks against a cut it was alive for.
        assertEquals(CutCheck.DONE, check(false, { true }, { false }))
    }

    @Test fun thePortProbeSeesAListener() {
        val server = try {
            ServerSocket(5555, 1, InetAddress.getByName("127.0.0.1"))
        } catch (e: java.io.IOException) {
            null // something already listens there (an emulator on this PC): that is a listener too
        }
        try {
            assertEquals(true, CutConfirm.adbdListening())
        } finally {
            server?.close()
        }
        assumeTrue("5555 still taken by someone else", server != null)
        assertFalse(CutConfirm.adbdListening())
    }
}
