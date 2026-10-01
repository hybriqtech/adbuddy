package tech.hybriq.adbuddy.term

import tech.hybriq.adbuddy.chat.Links

/**
 * What xterm.js may tell the app (spec M3 §7.3): keyboard data, a new size
 * and a link to open. Pure; TerminalView puts the three @JavascriptInterface
 * methods in front of it. Everything it gets comes from a page that shows the
 * agent's output, so each value is checked.
 */
class TermBridge(
    private val onInput: (ByteArray) -> Unit,
    private val onSize: (cols: Int, rows: Int) -> Unit,
    private val onLink: (url: String, domain: String) -> Unit,
) {
    fun data(s: String) {
        if (s.isNotEmpty()) onInput(s.toByteArray(Charsets.UTF_8))
    }

    /** The host caps terminal.resize at 200×100; the app does not ask for less than 20×5. */
    fun size(cols: Int, rows: Int) = onSize(cols.coerceIn(MIN_COLS, MAX_COLS), rows.coerceIn(MIN_ROWS, MAX_ROWS))

    /** Only web links, with the domain the dialog shows. OSC 8 and detected URLs come through here. */
    fun link(url: String) {
        val domain = Links.domain(url) ?: return
        onLink(url, domain)
    }

    companion object {
        const val MIN_COLS = 20
        const val MAX_COLS = 200
        const val MIN_ROWS = 5
        const val MAX_ROWS = 100
    }
}
