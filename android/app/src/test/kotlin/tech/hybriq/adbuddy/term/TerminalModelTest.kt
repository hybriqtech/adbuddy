package tech.hybriq.adbuddy.term

import java.util.Base64
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.TerminalOutput
import tech.hybriq.adbuddy.bridge.TerminalSize
import tech.hybriq.adbuddy.bridge.TerminalSnapshot

class TerminalModelTest {
    private fun b64(s: String) = Base64.getEncoder().encodeToString(s.toByteArray())
    private fun snap(epoch: String = "e1", offset: Long = 10, text: String = "SCREEN") = TerminalSnapshot(epoch, offset, 80, 24, b64(text))
    private fun out(offset: Long, text: String, epoch: String = "e1") = TerminalOutput(epoch, offset, b64(text))
    private fun text(e: TermEffect) = String((e as TermEffect.Write).bytes)

    @Test fun aSnapshotResetsTheViewAndSetsThePosition() {
        val m = TerminalModel()
        val fx = m.onSnapshot(snap())
        val r = fx.single() as TermEffect.Reset
        assertEquals(80, r.cols); assertEquals(24, r.rows); assertEquals("SCREEN", String(r.bytes))
        assertEquals("e1", m.watch().epoch)
        assertEquals(10L, m.watch().offset)
    }

    @Test fun outputInOrderIsWrittenAndAdvancesTheOffset() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        assertEquals("abc", text(m.onOutput(out(10, "abc")).single()))
        assertEquals(13L, m.watch().offset)
        assertEquals("de", text(m.onOutput(out(13, "de")).single()))
    }

    @Test fun overlappingOutputWritesOnlyWhatIsNew() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        m.onOutput(out(10, "abc"))
        assertEquals("only what is new: d", "d", text(m.onOutput(out(12, "cd")).single()))
        assertTrue("fully seen", m.onOutput(out(10, "abc")).isEmpty())
        assertEquals(14L, m.watch().offset)
    }

    @Test fun aGapOrAnotherEpochAsksForTheScreenAgain() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        assertEquals(listOf(TermEffect.Resync), m.onOutput(out(99, "x")))
        assertEquals(listOf(TermEffect.Resync), m.onOutput(out(10, "x", epoch = "e2")))
        assertEquals("nothing was written", 10L, m.watch().offset)
        // Without a screen yet, any output asks for it too.
        assertEquals(listOf(TermEffect.Resync), TerminalModel().onOutput(out(0, "x")))
    }

    @Test fun aSizeChangeAppliesAtItsByte() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        assertTrue(m.onSize(TerminalSize("e1", 13, 100, 30)).isEmpty())
        val fx = m.onOutput(out(10, "abcdef"))
        assertEquals(listOf("abc", "def"), fx.filterIsInstance<TermEffect.Write>().map { String(it.bytes) })
        val r = fx.filterIsInstance<TermEffect.Resize>().single()
        assertEquals(100, r.cols); assertEquals(30, r.rows)
        assertTrue("the resize sits between the two writes", fx[1] is TermEffect.Resize)
    }

    @Test fun twoSizesInsideOneChunkApplyAtTheirBytesAndTheLoopEnds() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        m.onSize(TerminalSize("e1", 12, 100, 30)); m.onSize(TerminalSize("e1", 14, 120, 40))
        val fx = m.onOutput(out(10, "abcdef"))
        assertEquals(listOf("Write", "Resize", "Write", "Resize", "Write"), fx.map { it::class.simpleName })
        assertEquals(listOf("ab", "cd", "ef"), fx.filterIsInstance<TermEffect.Write>().map { String(it.bytes) })
        assertEquals(16L, m.watch().offset)
    }

    @Test fun anAbsurdOffsetAsksForTheScreenAgain() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        assertEquals(listOf(TermEffect.Resync), m.onOutput(out(-5, "x")))
    }

    @Test fun aSizeAlreadyDueAppliesAtOnce() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        val r = m.onSize(TerminalSize("e1", 10, 120, 40)).single() as TermEffect.Resize
        assertEquals(120, r.cols)
    }

    @Test fun replayRebuildsTheScreenForANewView() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10, text = "S"))
        m.onOutput(out(10, "ab")); m.onOutput(out(12, "cd"))
        val fx = m.replay()
        assertEquals("S", String((fx[0] as TermEffect.Reset).bytes))
        assertTrue("replayed output is marked", (fx[0] as TermEffect.Reset).replay && fx.drop(1).all { (it as TermEffect.Write).replay })
        assertEquals("abcd", fx.drop(1).joinToString("") { text(it) })
        assertTrue("nothing to replay before the first snapshot", TerminalModel().replay().isEmpty())
    }

    @Test fun tooMuchToReplayStartsOverFromAFreshSnapshot() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 0, text = "S"))
        var off = 0L
        val chunk = "x".repeat(64 * 1024)
        repeat(20) { m.onOutput(out(off, chunk)); off += chunk.length } // 1.25 MiB > the cap
        assertTrue(m.replay().isEmpty())
        assertEquals("a new watch from nothing: the host sends a snapshot", "", m.watch().epoch)
    }

    @Test fun forgetEmptiesIt() {
        val m = TerminalModel(); m.onSnapshot(snap())
        m.forget()
        assertEquals("", m.watch().epoch); assertEquals(0L, m.watch().offset)
    }

    @Test fun aSnapshotKeepsTheQueuedSizesThatAreStillAhead() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        m.onSize(TerminalSize("e1", 20, 100, 30)); m.onSize(TerminalSize("e1", 12, 90, 20))
        m.onSnapshot(snap(offset = 15))
        val fx = m.onOutput(out(15, "abcdefghij"))
        assertEquals("the one at 12 is behind the snapshot and dropped; the one at 20 stays", listOf("Write", "Resize", "Write"), fx.map { it::class.simpleName })
        assertEquals(100, (fx[1] as TermEffect.Resize).cols)
        assertEquals("abcde", text(fx[0]))
    }

    @Test fun aSizeOfAnotherEpochIsDroppedBySnapshot() {
        val m = TerminalModel(); m.onSnapshot(snap(epoch = "e1", offset = 10))
        m.onSize(TerminalSize("e1", 20, 100, 30))
        m.onSnapshot(snap(epoch = "e2", offset = 0))
        val fx = m.onOutput(out(0, "abcdefghijklmnopqrstuvwxyz", epoch = "e2"))
        assertEquals(listOf("Write"), fx.map { it::class.simpleName })
    }

    @Test fun resizesCountInTheTailAndStartOverWhenItIsFull() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        repeat(TerminalModel.MAX_TAIL / 16 + 1) { m.onSize(TerminalSize("e1", 10, 100, 30)) }
        assertTrue(m.replay().isEmpty())
        assertEquals("", m.watch().epoch)
    }

    @Test fun tooManyQueuedSizesAsksForTheScreenAgain() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10))
        repeat(256) { assertTrue(m.onSize(TerminalSize("e1", 1000L + it, 100, 30)).isEmpty()) }
        assertEquals(listOf(TermEffect.Resync), m.onSize(TerminalSize("e1", 5000, 100, 30)))
        assertEquals("", m.watch().epoch)
    }

    @Test fun absurdSizesAreIgnored() {
        val m = TerminalModel()
        for ((c, r) in listOf(0 to 24, 80 to 0, -1 to 24, 1001 to 24, 80 to 1001)) {
            assertEquals(listOf(TermEffect.Resync), m.onSnapshot(TerminalSnapshot("e1", 0, c, r, b64("S"))))
        }
        m.onSnapshot(snap(offset = 10))
        for ((c, r) in listOf(0 to 24, 80 to 0, 1001 to 24, 80 to 1001)) {
            assertTrue(m.onSize(TerminalSize("e1", 10, c, r)).isEmpty())
        }
        assertTrue(m.replay().drop(1).isEmpty())
        val big = m.onSnapshot(TerminalSnapshot("e1", 0, 300, 150, b64("S"))).single() as TermEffect.Reset
        assertEquals("a PC terminal may be bigger than 200x100", 300, big.cols)
        assertEquals(1000, (m.onSize(TerminalSize("e1", 0, 1000, 1000)).single() as TermEffect.Resize).rows)
    }

    @Test fun whileNotReplayableOutputAsksForTheScreenAndIsNotWritten() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 0, text = "S"))
        var off = 0L
        val chunk = "x".repeat(64 * 1024)
        repeat(17) { m.onOutput(out(off, chunk)); off += chunk.length }
        assertTrue(m.replay().isEmpty())
        assertEquals(listOf(TermEffect.Resync), m.onOutput(out(off, "more")))
        m.onSnapshot(snap(offset = off + 1))
        assertEquals("abc", text(m.onOutput(out(off + 1, "abc")).single()))
    }

    @Test fun aSnapshotWithBadBase64AsksAgain() {
        val m = TerminalModel()
        assertEquals(listOf(TermEffect.Resync), m.onSnapshot(TerminalSnapshot("e1", 0, 80, 24, "***")))
        assertEquals("", m.watch().epoch)
    }

    @Test fun replayKeepsTheResizesOfTheTail() {
        val m = TerminalModel(); m.onSnapshot(snap(offset = 10, text = "S"))
        m.onOutput(out(10, "ab")); m.onSize(TerminalSize("e1", 12, 100, 30)); m.onOutput(out(12, "cd"))
        assertEquals(listOf("Reset", "Write", "Resize", "Write"), m.replay().map { it::class.simpleName })
    }
}
