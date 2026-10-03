package tech.hybriq.adbuddy.ui

import android.view.View
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.ui.platform.LocalView
import java.util.WeakHashMap

/**
 * Reference count of who needs a key (a window's root view) to filter obscured touches. The first user records the
 * original value and forces the flag on; the last one to leave puts the original back. Main thread only.
 */
internal class ObscuredCounts<K : Any>(private val read: (K) -> Boolean, private val write: (K, Boolean) -> Unit) {
    private class Entry(var count: Int, val original: Boolean)

    private val entries = WeakHashMap<K, Entry>()

    fun enter(key: K) {
        val e = entries[key]
        if (e != null) {
            e.count++
        } else {
            entries[key] = Entry(1, read(key))
            write(key, true)
        }
    }

    /** An exit with no matching enter does nothing. */
    fun exit(key: K) {
        val e = entries[key] ?: return
        if (--e.count > 0) return
        entries.remove(key)
        write(key, e.original)
    }
}

private val viewCounts = ObscuredCounts<View>(read = { it.filterTouchesWhenObscured }, write = { v, on -> v.filterTouchesWhenObscured = on })

/**
 * Refuses the touches that reach this window while another app's window covers it, so an overlay cannot get the
 * user's tap on a button that grants something. Put it in every surface that grants and in every dialog (a dialog is
 * its own window: the flag of the screen below does not reach it). It restores the original value on leaving, and
 * only when the last user leaves, so a harmless overlay (a blue-light filter) cannot keep blocking the chat after
 * a card has been seen.
 */
@Composable
fun ObscuredTouchGuard() {
    val view = LocalView.current
    DisposableEffect(view) {
        val root = view.rootView
        viewCounts.enter(root)
        onDispose { viewCounts.exit(root) }
    }
}
