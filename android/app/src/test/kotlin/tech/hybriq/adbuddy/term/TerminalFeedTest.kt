package tech.hybriq.adbuddy.term

import java.util.Base64
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.FakeChannel
import tech.hybriq.adbuddy.bridge.Protocol

class TerminalFeedTest {
    private val ch = FakeChannel()
    private val got = mutableListOf<TermEffect>()
    private val feed = TerminalFeed(ch)
    private fun b64(s: String) = Base64.getEncoder().encodeToString(s.toByteArray())
    private fun types() = ch.sent.map { it.type }

    @Test fun attachWatchesAndStreamsToTheSink() {
        feed.attach { got += it }
        assertEquals(listOf(Protocol.TERMINAL_WATCH), types())
        ch.emit(Protocol.TERMINAL_SNAPSHOT, """{"epoch":"e1","offset":5,"cols":80,"rows":24,"data":"${b64("S")}"}""")
        ch.emit(Protocol.TERMINAL_OUTPUT, """{"epoch":"e1","offset":5,"data":"${b64("ab")}"}""")
        assertEquals(listOf("Reset", "Write"), got.map { it::class.simpleName })
    }

    @Test fun detachUnwatchesAndStopsTheSink() {
        val sink = TermSink { got += it }
        feed.attach(sink)
        feed.detach(sink)
        assertEquals(listOf(Protocol.TERMINAL_WATCH, Protocol.TERMINAL_UNWATCH), types())
        ch.emit(Protocol.TERMINAL_OUTPUT, """{"epoch":"e1","offset":0,"data":"${b64("x")}"}""")
        assertTrue(got.isEmpty())
    }

    @Test fun theSecondWatchResumesFromWhatTheModelHas() {
        val first = TermSink { }
        feed.attach(first)
        ch.emit(Protocol.TERMINAL_SNAPSHOT, """{"epoch":"e1","offset":5,"cols":80,"rows":24,"data":"${b64("S")}"}""")
        feed.detach(first)
        feed.attach { got += it }
        val w = Protocol.json.decodeFromJsonElement(tech.hybriq.adbuddy.bridge.TerminalWatch.serializer(), ch.sent.last().payload)
        assertEquals("e1", w.epoch); assertEquals(5L, w.offset)
        assertEquals("the new view gets the screen without asking the host", "Reset", got.first()::class.simpleName)
    }

    @Test fun aResyncAsksAgainFromWhatItHas() {
        feed.attach { got += it }
        ch.emit(Protocol.TERMINAL_OUTPUT, """{"epoch":"e1","offset":5,"data":"${b64("x")}"}""") // no screen yet
        assertEquals(listOf(Protocol.TERMINAL_WATCH, Protocol.TERMINAL_WATCH), types())
    }

    @Test fun anOldViewCannotDetachTheNewOne() {
        val old = TermSink { }
        feed.attach(old)
        feed.attach { got += it } // a bubble opened over the activity
        feed.detach(old)
        assertEquals("no unwatch: the new view still watches", 0, types().count { it == Protocol.TERMINAL_UNWATCH })
        ch.emit(Protocol.TERMINAL_SNAPSHOT, """{"epoch":"e1","offset":5,"cols":80,"rows":24,"data":"${b64("S")}"}""")
        assertEquals(1, got.size)
    }

    @Test fun manyResyncsAskOnceUntilTheSnapshotComes() {
        feed.attach { got += it }
        repeat(3) { ch.emit(Protocol.TERMINAL_OUTPUT, """{"epoch":"e1","offset":5,"data":"${b64("x")}"}""") }
        assertEquals(2, types().count { it == Protocol.TERMINAL_WATCH }) // the attach and one resync
    }

    @Test fun aReconnectWatchesAgain() {
        feed.attach { }
        feed.onConnectedChanged(true)
        assertEquals(2, types().count { it == Protocol.TERMINAL_WATCH })
    }

    @Test fun inputAndResizeGoToTheHost() {
        feed.input("ls\r".toByteArray())
        feed.resize(300, 5)
        assertEquals(listOf(Protocol.TERMINAL_INPUT, Protocol.TERMINAL_RESIZE), types())
        val r = Protocol.json.decodeFromJsonElement(tech.hybriq.adbuddy.bridge.TerminalResize.serializer(), ch.sent[1].payload)
        assertEquals("the host caps at 200x100 and the app asks for what fits", 200, r.cols)
        assertEquals(5, r.rows)
    }
}
