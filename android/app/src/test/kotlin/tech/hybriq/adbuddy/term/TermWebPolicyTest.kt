package tech.hybriq.adbuddy.term

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class TermWebPolicyTest {
    @Test fun onlyTheBundledTerminalAssetsLoad() {
        assertTrue(TermWebPolicy.allowed("https://appassets.androidplatform.net/assets/term/index.html"))
        assertTrue(TermWebPolicy.allowed("https://appassets.androidplatform.net/assets/term/xterm.js"))
    }

    @Test fun everythingElseIsBlocked() {
        for (u in listOf(
            "https://example.com/assets/term/index.html",
            "https://appassets.androidplatform.net/assets/other.html",
            "https://appassets.androidplatform.net/assets/term/../secret",
            "https://appassets.androidplatform.net/assets/term/%2e%2e/secret",
            "https://appassets.androidplatform.net.evil.com/assets/term/index.html",
            "http://appassets.androidplatform.net/assets/term/index.html",
            "file:///android_asset/term/index.html", "content://x/y", "data:text/html,hi", "javascript:alert(1)", "",
        )) assertFalse(u, TermWebPolicy.allowed(u))
    }
}
