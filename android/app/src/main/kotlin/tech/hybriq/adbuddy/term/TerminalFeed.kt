package tech.hybriq.adbuddy.term

import java.util.Base64
import tech.hybriq.adbuddy.bridge.Channel
import tech.hybriq.adbuddy.bridge.Empty
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.bridge.TerminalInput
import tech.hybriq.adbuddy.bridge.TerminalOutput
import tech.hybriq.adbuddy.bridge.TerminalResize
import tech.hybriq.adbuddy.bridge.TerminalSize
import tech.hybriq.adbuddy.bridge.TerminalSnapshot
import tech.hybriq.adbuddy.bridge.TerminalWatch

fun interface TermSink { fun apply(e: TermEffect) }

/**
 * The terminal's stream between the host and the view (protocol spec §7).
 * One per process: the model survives a rotation, so the new view is redrawn
 * from memory. It watches only while a view is attached (the host sends
 * nothing else), and Channel's listeners run on OkHttp's reader thread: the
 * sink must hand over to the main thread by itself.
 */
class TerminalFeed(private val channel: Channel) {
    private val lock = Any()
    private val model = TerminalModel()
    private var sink: TermSink? = null
    private var resyncing = false

    init {
        channel.listen(Protocol.TERMINAL_SNAPSHOT) { m -> Protocol.payload<TerminalSnapshot>(m)?.let { s -> feed { resyncing = false; model.onSnapshot(s) } } }
        channel.listen(Protocol.TERMINAL_OUTPUT) { m -> Protocol.payload<TerminalOutput>(m)?.let { o -> feed { model.onOutput(o) } } }
        channel.listen(Protocol.TERMINAL_SIZE) { m -> Protocol.payload<TerminalSize>(m)?.let { s -> feed { model.onSize(s) } } }
    }

    /**
     * Runs a model step and hands its effects to the view, under the lock: attach() replays under it too, so a
     * Write can never overtake the Reset it belongs after (the sink only posts to the main thread, so it is cheap).
     * A Resync becomes a new watch, once until the snapshot it asks for arrives.
     */
    private fun feed(step: () -> List<TermEffect>) {
        val watch = synchronized(lock) {
            val fx = step()
            val target = sink
            if (target != null) fx.forEach { if (it !is TermEffect.Resync) target.apply(it) }
            if (target != null && !resyncing && fx.any { it is TermEffect.Resync }) {
                resyncing = true
                model.watch()
            } else {
                null
            }
        }
        watch?.let(::sendWatch)
    }

    /** The view appeared: redraw from memory and watch. */
    fun attach(sink: TermSink) {
        val watch = synchronized(lock) {
            this.sink = sink
            resyncing = false
            model.replay().forEach { sink.apply(it) }
            model.watch()
        }
        sendWatch(watch)
    }

    /**
     * The view left: stop the stream. Only the view that is attached can: with a bubble over the activity the new
     * view attaches before the old one detaches, and the old one must not take the new one's stream away.
     */
    fun detach(sink: TermSink) {
        val was = synchronized(lock) { if (this.sink === sink) { this.sink = null; true } else false }
        if (was) channel.send(Protocol.TERMINAL_UNWATCH, Protocol.json.encodeToJsonElement(Empty.serializer(), Empty))
    }

    /** The channel is back (or gone): a view on screen watches again, from what it has. */
    fun onConnectedChanged(connected: Boolean) {
        val watch = synchronized(lock) { if (connected && sink != null) model.watch() else null }
        if (watch != null) sendWatch(watch)
    }

    fun input(bytes: ByteArray) {
        for (c in Keys.chunks(bytes)) {
            channel.send(Protocol.TERMINAL_INPUT, Protocol.json.encodeToJsonElement(TerminalInput.serializer(), TerminalInput(Base64.getEncoder().encodeToString(c))))
        }
    }

    /** The size that fits the view; the host caps it and ignores it while a PC terminal is attached. */
    fun resize(cols: Int, rows: Int) {
        val r = TerminalResize(cols.coerceIn(TermBridge.MIN_COLS, TermBridge.MAX_COLS), rows.coerceIn(TermBridge.MIN_ROWS, TermBridge.MAX_ROWS))
        channel.send(Protocol.TERMINAL_RESIZE, Protocol.json.encodeToJsonElement(TerminalResize.serializer(), r))
    }

    private fun sendWatch(w: TerminalWatch) {
        channel.send(Protocol.TERMINAL_WATCH, Protocol.json.encodeToJsonElement(TerminalWatch.serializer(), w))
    }
}
