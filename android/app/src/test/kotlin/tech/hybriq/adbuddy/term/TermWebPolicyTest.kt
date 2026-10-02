package tech.hybriq.adbuddy.term

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class TermWebPolicyTest {
    private val base = TermWebPolicy.PREFIX

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

    /** Every file the page loads (index.html and what it links), and the source maps a debugger may ask for. */
    @Test fun theFilesOfTheFolderLoadByTheirPlainName() {
        for (name in listOf("index.html", "xterm.js", "xterm.css", "term.js", "term.css", "addon-fit.js", "addon-web-links.js", "xterm.js.map")) {
            assertTrue(name, TermWebPolicy.allowed(base + name))
        }
    }

    /** V21: one file of the folder by its plain name, nothing else: no query, fragment, sub-folder, escape or odd character. */
    @Test fun edgeCasesAreBlocked() {
        for (rest in listOf(
            "", "?x=1", "index.html?x=1", "#frag", "index.html#frag", "a?/../b", "%2F", "index%2Ehtml", "..", "...", ".",
            ".hidden", "/index.html", "sub/index.html", "index.html/", "a\\b", "a\u0008b", "a b", "índex.html", "index.html\u0000",
        )) assertFalse("rest \"$rest\"", TermWebPolicy.allowed(base + rest))
        for (u in listOf(
            "HTTPS://APPASSETS.ANDROIDPLATFORM.NET/assets/term/index.html",
            "https://appassets.androidplatform.net/ASSETS/TERM/index.html",
            "https://appassets.androidplatform.net//assets/term/index.html",
            "https://appassets.androidplatform.net:443/assets/term/index.html",
            "https://user@appassets.androidplatform.net/assets/term/index.html",
            "https://appassets.androidplatform.net@example.com/assets/term/index.html",
        )) assertFalse(u, TermWebPolicy.allowed(u))
    }
}
