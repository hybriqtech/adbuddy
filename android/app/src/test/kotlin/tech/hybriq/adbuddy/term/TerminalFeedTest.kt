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

    private fun lastWatch() = Protocol.json.decodeFromJsonElement(tech.hybriq.adbuddy.bridge.TerminalWatch.serializer(), ch.sent.last().payload)
    private fun snapshot(offset: Long = 5) = ch.emit(Protocol.TERMINAL_SNAPSHOT, """{"epoch":"e1","offset":$offset,"cols":80,"rows":24,"data":"${b64("S")}"}""")
    private fun gap() = ch.emit(Protocol.TERMINAL_OUTPUT, """{"epoch":"e1","offset":99,"data":"${b64("x")}"}""")

    @Test fun theSecondWatchCarriesWhatTheModelHas() {
        feed.attach { }
        gap()
        assertEquals("", lastWatch().epoch); assertEquals(0L, lastWatch().offset)
        snapshot(5)
        gap()
        assertEquals("e1", lastWatch().epoch); assertEquals(5L, lastWatch().offset)
    }

    @Test fun aResyncAnsweredWithOutputInOrderCanAskAgain() {
        feed.attach { got += it }
        snapshot(5)
        gap() // resync #1
        ch.emit(Protocol.TERMINAL_OUTPUT, """{"epoch":"e1","offset":5,"data":"${b64("ab")}"}""") // the host answered with output, not a snapshot
        gap()
        assertEquals(3, types().count { it == Protocol.TERMINAL_WATCH })
    }

    @Test fun aReconnectWithAResyncPendingCanAskAgain() {
        feed.attach { }
        snapshot(5)
        gap()
        feed.onConnectedChanged(true)
        gap()
        assertEquals(4, types().count { it == Protocol.TERMINAL_WATCH })
    }

    @Test fun aReconnectWithoutAViewSendsNothing() {
        feed.onConnectedChanged(true)
        assertTrue(ch.sent.isEmpty())
    }

    @Test fun aSizeFromTheHostReachesTheSink() {
        feed.attach { got += it }
        snapshot(5)
        ch.emit(Protocol.TERMINAL_SIZE, """{"epoch":"e1","offset":5,"cols":100,"rows":30}""")
        assertEquals(listOf("Reset", "Resize"), got.map { it::class.simpleName })
    }

    /**
     * V43: a bubble over a visible activity is a second view on top. When it goes, the view under it draws again
     * (from memory, without asking the host) and the stream goes on: no unwatch, so the terminal does not freeze.
     */
    @Test fun whenTheBubbleGoesTheViewUnderItDrawsAgain() {
        val under = mutableListOf<TermEffect>()
        val activity = TermSink { under += it }
        val bubble = TermSink { got += it }
        feed.attach(activity)
        snapshot(5)
        feed.attach(bubble)
        ch.emit(Protocol.TERMINAL_OUTPUT, """{"epoch":"e1","offset":5,"data":"${b64("ab")}"}""")
        assertEquals("only the view on top draws", listOf("Reset"), under.map { it::class.simpleName })
        under.clear()
        feed.detach(bubble)
        assertEquals("the stream goes on", 0, types().count { it == Protocol.TERMINAL_UNWATCH })
        assertEquals("the view under it is redrawn from memory", listOf("Reset", "Write"), under.map { it::class.simpleName })
        under.clear()
        ch.emit(Protocol.TERMINAL_OUTPUT, """{"epoch":"e1","offset":7,"data":"${b64("c")}"}""")
        assertEquals(listOf("Write"), under.map { it::class.simpleName })
        assertEquals("the bubble no longer draws", 2, got.size)
    }

    /** V43: the stream stops only when the last view goes, in whatever order they go. */
    @Test fun theLastViewToGoUnwatches() {
        val a = TermSink { }
        val b = TermSink { }
        feed.attach(a)
        feed.attach(b)
        feed.detach(b)
        feed.detach(a)
        assertEquals(1, types().count { it == Protocol.TERMINAL_UNWATCH })
        feed.attach(a)
        feed.attach(a) // the same view twice is one view
        feed.detach(a)
        assertEquals(2, types().count { it == Protocol.TERMINAL_UNWATCH })
        feed.detach(a) // a view that is not attached cannot unwatch again
        assertEquals(2, types().count { it == Protocol.TERMINAL_UNWATCH })
    }

    /** When the model cannot redraw (too much output under the bubble), the view under it asks the host for a screen. */
    @Test fun aViewUnderTheBubbleAsksForTheScreenWhenTheModelCannotRedraw() {
        val under = mutableListOf<TermEffect>()
        val activity = TermSink { under += it }
        val bubble = TermSink { got += it }
        feed.attach(activity)
        snapshot(5)
        feed.attach(bubble)
        val big = "x".repeat(TerminalModel.MAX_TAIL + 1)
        ch.emit(Protocol.TERMINAL_OUTPUT, """{"epoch":"e1","offset":5,"data":"${b64(big)}"}""")
        val watches = types().count { it == Protocol.TERMINAL_WATCH }
        feed.detach(bubble)
        assertEquals("a new watch asks for the snapshot", watches + 1, types().count { it == Protocol.TERMINAL_WATCH })
        assertEquals("the stream goes on", 0, types().count { it == Protocol.TERMINAL_UNWATCH })
        under.clear()
        snapshot(5 + big.length.toLong())
        assertEquals("the view under it is not left mute", "Reset", under.first()::class.simpleName)
    }
}
