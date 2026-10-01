package tech.hybriq.adbuddy.term

import java.util.Base64
import tech.hybriq.adbuddy.bridge.TerminalOutput
import tech.hybriq.adbuddy.bridge.TerminalSize
import tech.hybriq.adbuddy.bridge.TerminalSnapshot
import tech.hybriq.adbuddy.bridge.TerminalWatch

/** What the terminal view must do, in order. */
sealed interface TermEffect {
    /** Clean the terminal, size it cols×rows and write bytes: the screen at a snapshot. */
    class Reset(val cols: Int, val rows: Int, val bytes: ByteArray, val replay: Boolean = false) : TermEffect

    /** replay: old output drawn again in a new view; the page must not forward what xterm.js answers to its queries (DSR, DA) as keystrokes. */
    class Write(val bytes: ByteArray, val replay: Boolean = false) : TermEffect
    class Resize(val cols: Int, val rows: Int) : TermEffect

    /** The model lost the thread: watch again from what it has (the host answers with a snapshot). */
    object Resync : TermEffect
}

/**
 * What the app knows of the terminal (protocol spec §7): the position (epoch
 * and offset) and enough to redraw it in a new view without asking the host.
 * Pure and not thread-safe: TerminalFeed calls it under its own lock.
 */
class TerminalModel {
    private var epoch = ""
    private var offset = 0L
    private var haveScreen = false

    /** The last snapshot, and the effects since it, in order: replay(). */
    private var base: TermEffect.Reset? = null
    private val tail = ArrayList<TermEffect>()
    private var tailBytes = 0
    private var replayable = false

    /** Sizes that apply from a byte not written yet, oldest first. */
    private val sizes = ArrayDeque<TerminalSize>()

    /** What to send in terminal.watch: nothing (empty and 0) when a replay is not possible, so the host sends a snapshot. */
    fun watch(): TerminalWatch = if (haveScreen && replayable) TerminalWatch(epoch, offset) else TerminalWatch()

    fun forget() {
        epoch = ""; offset = 0; haveScreen = false; replayable = false
        base = null; tail.clear(); tailBytes = 0; sizes.clear()
    }

    fun onSnapshot(s: TerminalSnapshot): List<TermEffect> {
        if (!validSize(s.cols, s.rows)) return listOf(TermEffect.Resync)
        val bytes = decode(s.data) ?: return listOf(TermEffect.Resync)
        epoch = s.epoch; offset = s.offset
        haveScreen = true; replayable = true
        val reset = TermEffect.Reset(s.cols, s.rows, bytes)
        base = reset; tail.clear(); tailBytes = 0
        // Sizes of this epoch still ahead of the snapshot stay; the rest belong to a stream that is gone.
        sizes.removeAll { it.epoch != s.epoch || it.offset <= s.offset }
        return listOf(reset)
    }

    fun onOutput(o: TerminalOutput): List<TermEffect> {
        if (!haveScreen || !replayable || o.epoch != epoch || o.offset < 0 || o.offset > offset) return listOf(TermEffect.Resync)
        val all = decode(o.data) ?: return listOf(TermEffect.Resync)
        val skip = offset - o.offset
        if (skip >= all.size) return emptyList() // everything was seen already
        return write(all.copyOfRange(skip.toInt(), all.size))
    }

    fun onSize(s: TerminalSize): List<TermEffect> {
        if (!haveScreen || s.epoch != epoch || !validSize(s.cols, s.rows)) return emptyList()
        if (s.offset > offset) {
            if (sizes.size >= MAX_SIZES) { forget(); return listOf(TermEffect.Resync) }
            sizes.addLast(s)
            return emptyList()
        }
        return listOf(resize(s))
    }

    /** The screen again, for a view that was just made. Empty when there is nothing, or too much, to replay. */
    fun replay(): List<TermEffect> {
        val b = base
        if (!haveScreen || !replayable || b == null) return emptyList()
        val first = TermEffect.Reset(b.cols, b.rows, b.bytes, replay = true)
        return listOf<TermEffect>(first) + tail.map { if (it is TermEffect.Write) TermEffect.Write(it.bytes, replay = true) else it }
    }

    private fun resize(s: TerminalSize): TermEffect {
        val e = TermEffect.Resize(s.cols, s.rows)
        keep(e, RESIZE_COST)
        return e
    }

    /** Writes data (which starts at offset), applying the sizes that fall inside it at their byte. */
    private fun write(data: ByteArray): List<TermEffect> {
        val fx = ArrayList<TermEffect>()
        var from = 0
        while (from < data.size) {
            val next = sizes.firstOrNull()
            // next.offset is a position in the stream; data[from] is at `offset`: the cut is relative to from, never below it.
            val cut = if (next == null) data.size else (from + (next.offset - offset).coerceIn(0L, (data.size - from).toLong())).toInt()
            if (cut > from) {
                val w = TermEffect.Write(data.copyOfRange(from, cut))
                fx += w
                keep(w, cut - from)
                offset += cut - from
                from = cut
            }
            if (next != null && next.offset <= offset) {
                sizes.removeFirst()
                fx += resize(next)
            } else if (from >= data.size) {
                break
            }
        }
        return fx
    }

    private fun keep(e: TermEffect, bytes: Int) {
        if (!replayable) return
        tail += e
        tailBytes += bytes
        // Too much to redraw in a hurry: the next watch asks the host for a fresh snapshot instead.
        if (tailBytes > MAX_TAIL) { replayable = false; tail.clear(); tailBytes = 0 }
    }

    private fun validSize(cols: Int, rows: Int) = cols in 1..MAX_DIM && rows in 1..MAX_DIM

    private fun decode(b64: String): ByteArray? = try { Base64.getDecoder().decode(b64) } catch (e: IllegalArgumentException) { null }

    companion object {
        /** Output kept after the snapshot to redraw a new view (spec M3 §7.3). */
        const val MAX_TAIL = 1024 * 1024

        /** A Resize in the tail counts as this many bytes, and no more than MAX_SIZES wait for their byte. */
        const val RESIZE_COST = 16
        const val MAX_SIZES = 256
        const val MAX_DIM = 1000
    }
}
