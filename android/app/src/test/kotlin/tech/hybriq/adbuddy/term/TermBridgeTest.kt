package tech.hybriq.adbuddy.term

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TermBridgeTest {
    private val inputs = mutableListOf<String>()
    private val sizes = mutableListOf<Pair<Int, Int>>()
    private val links = mutableListOf<Pair<String, String>>()
    private val b = TermBridge({ inputs += String(it) }, { c, r -> sizes += c to r }, { u, d -> links += u to d })

    @Test fun keyboardDataGoesOutAsUtf8() {
        b.data("ñ\r")
        assertEquals(listOf("ñ\r"), inputs)
        b.data("")
        assertEquals(1, inputs.size)
    }

    @Test fun sizesAreClamped() {
        b.size(500, 1)
        b.size(0, 0)
        assertEquals(listOf(200 to 5, 20 to 5), sizes)
        b.size(80, 24)
        assertEquals(80 to 24, sizes.last())
    }

    @Test fun onlyWebLinksOpenAndTheyShowTheirDomain() {
        b.link("https://Example.com/a?b=c")
        b.link("http://localhost:8080/x")
        b.link("intent://evil#Intent;end")
        b.link("file:///sdcard/x")
        b.link("javascript:alert(1)")
        b.link("https://user:pw@example.com@evil.test/")
        // java.net.URI cannot read a host out of that last one: it is dropped, never shown as example.com.
        assertEquals(listOf("https://Example.com/a?b=c" to "example.com", "http://localhost:8080/x" to "localhost"), links)
    }

    @Test fun veryLongUrlsAreDropped() {
        b.link("https://example.com/" + "a".repeat(5000))
        assertTrue(links.isEmpty())
    }
}
