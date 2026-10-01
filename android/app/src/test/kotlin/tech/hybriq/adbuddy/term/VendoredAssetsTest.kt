package tech.hybriq.adbuddy.term

import java.io.File
import java.security.MessageDigest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class VendoredAssetsTest {
    private val dir = File("src/main/assets/term")
    private fun sha(f: File) = MessageDigest.getInstance("SHA-256").digest(f.readBytes()).joinToString("") { "%02x".format(it) }

    @Test fun vendoredFilesMatchTheirRecordedHash() {
        // sha256sum prints "<hash> *<name>" (binary mode) or "<hash>  <name>": both are accepted.
        val recorded = File(dir, "VENDOR.md").readLines().mapNotNull { Regex("^([0-9a-f]{64})\\s+\\*?(\\S+)$").find(it.trim())?.destructured }
            .associate { (h, n) -> n to h }
        for (n in listOf("xterm.js", "xterm.css", "addon-fit.js", "addon-web-links.js")) {
            assertEquals("hash of $n", recorded[n], sha(File(dir, n)))
        }
    }

    @Test fun thePageLoadsOnlyItsOwnFilesUnderAStrictPolicy() {
        val html = File(dir, "index.html").readText()
        assertTrue(html.contains("Content-Security-Policy"))
        // The whole policy, not a fragment: an added 'unsafe-inline' script or a connect-src would not pass.
        assertTrue(html.contains("content=\"default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'none'; base-uri 'none'; form-action 'none'\""))
        assertFalse("no external URL", Regex("(src|href)=\"(https?:)?//").containsMatchIn(html))
        val js = File(dir, "term.js").readText()
        assertFalse("no network from our own script", Regex("fetch\\(|XMLHttpRequest|WebSocket|importScripts|eval\\(").containsMatchIn(js))
    }
}
