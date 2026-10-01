package tech.hybriq.adbuddy.chat

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class LinksTest {
    private fun urls(t: String) = Links.find(t).map { it.url }

    @Test fun findsWebLinksAndTrimsSentencePunctuation() {
        assertEquals(listOf("https://a.com/x"), urls("see https://a.com/x."))
        assertEquals(listOf("https://a.com"), urls("(https://a.com)"))
        assertEquals(listOf("https://en.wikipedia.org/wiki/A_(b)"), urls("https://en.wikipedia.org/wiki/A_(b)"))
        assertEquals(listOf("http://a.com/1", "https://b.org/2?q=1"), urls("http://a.com/1, then https://b.org/2?q=1!"))
    }

    @Test fun ignoresWhatIsNotAWebLink() {
        assertEquals(emptyList<String>(), urls("ftp://x.com file:///etc/passwd javascript:alert(1) http:// https://"))
    }

    @Test fun spansPointAtTheLinkInTheText() {
        val t = "go to https://a.com/x now"
        val s = Links.find(t).single()
        assertEquals("https://a.com/x", t.substring(s.start, s.end))
    }

    @Test fun aNonAsciiHostIsNeverShownAsAnAsciiOne() {
        // java.net.URI reads no host out of a non-ASCII name: the link is dropped, so a look-alike never gets a dialog.
        assertNull(Links.domain("https://ex\u0430mple.com/"))
    }

    @Test fun domainIsLowerCaseAndNullWithoutAHost() {
        assertEquals("example.com", Links.domain("https://Example.com/a"))
        assertNull(Links.domain("https://"))
        assertNull(Links.domain("mailto:a@b.c"))
    }

    private val fsi = String(Character.toChars(0x2068))
    private val pdi = String(Character.toChars(0x2069))

    @Test fun aLinkTextNeverPassesForAnotherDomain() {
        assertEquals("https://mibanco.com $fsi(evil.example)$pdi", Links.label("https://mibanco.com", "https://evil.example"))
        assertEquals("mibanco.com $fsi(evil.example)$pdi", Links.label("mibanco.com", "https://evil.example/x"))
        assertEquals("la web $fsi(example.com)$pdi", Links.label("la web", "https://example.com/a"))
        assertEquals("evil.example", Links.label("evil.example", "https://EVIL.example/x"))
        assertEquals("https://a.com/x", Links.label("https://a.com/x", "https://a.com/x"))
    }

    @Test fun userinfoMakesItNoLink() {
        assertNull(Links.domain("https://mibanco.com@evil.example/p"))
        assertEquals(emptyList<String>(), urls("see https://mibanco.com@evil.example/p now"))
        val b = Markdown.parse("[https://mibanco.com@evil.example/p](https://mibanco.com@evil.example/p) y [](https://u:p@evil.example)")
        val inl = b.filterIsInstance<Block.Paragraph>().flatMap { it.inlines }
        assertEquals(emptyList<Inline.Link>(), inl.filterIsInstance<Inline.Link>())
    }

    @Test fun invisibleAndBidiCharactersAreStrippedFromTheText() {
        val rlo = String(Character.toChars(0x202E))
        val zw = String(Character.toChars(0x200B))
        assertEquals("moc.knab $fsi(evil.example)$pdi", Links.label("${rlo}moc.knab$zw", "https://evil.example"))
    }
}
