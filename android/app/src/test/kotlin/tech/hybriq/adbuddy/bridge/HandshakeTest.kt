package tech.hybriq.adbuddy.bridge

import java.io.ByteArrayInputStream
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class HandshakeTest {
    private val token = "0123456789abcdef".repeat(4)
    private val nonce = "fedcba9876543210".repeat(2)

    @Test fun acceptsWhatTheHostWrites() {
        val h = HandshakeParser.parse("""{"host":"DESKTOP-1","nonce":"$nonce","port":47123,"token":"$token"}""".toByteArray())!!
        assertEquals(47123, h.port)
        assertEquals(nonce, h.nonce)
        assertEquals("DESKTOP-1", h.host)
    }

    @Test fun refusesAnythingOff() {
        for (bad in listOf(
            """{"host":"x","nonce":"$nonce","port":47123,"token":"short"}""",
            """{"host":"x","nonce":"$nonce","port":80,"token":"$token"}""",
            """{"host":"x","nonce":"$nonce","port":47123}""",
            """{"host":"x","port":47123,"token":"$token"}""",
            """{"host":"x","nonce":"XYZ","port":47123,"token":"$token"}""",
            "not json",
            "{" + " ".repeat(5000) + "}",
        )) assertNull(bad, HandshakeParser.parse(bad.toByteArray()))
    }

    @Test fun oddHostNamesBecomePC() {
        val h = HandshakeParser.parse("""{"host":"<b>evil</b>","nonce":"$nonce","port":47123,"token":"$token"}""".toByteArray())!!
        assertEquals("PC", h.host)
    }

    @Test fun readLimitedStopsEarly() {
        val big = ByteArray(1 shl 20)
        assertTrue(HandshakeParser.readLimited(ByteArrayInputStream(big)).size <= 4096 + 1024)
    }

    @Test fun onlyTheShellUser() {
        assertTrue(Caller.isShell(2000))
        assertFalse(Caller.isShell(0))
        assertFalse(Caller.isShell(10123))
    }
}
