package tech.hybriq.adbuddy.a11y

import kotlinx.serialization.json.JsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.bridge.Message
import tech.hybriq.adbuddy.bridge.Protocol

class UiHelpersTest {
    private val welcome = Message(v = Protocol.VERSION, type = Protocol.WELCOME, ts = "2026-10-01T10:00:00Z", payload = JsonObject(emptyMap()))

    @Test fun aHandlerThatThrowsDoesNotCutTheListener() {
        val errors = mutableListOf<Throwable>()
        var calls = 0
        val handler = UiHelpers.welcomeHandler(work = { it.run() }, sendList = { calls++; throw IllegalStateException("boom") }, log = { errors += it })
        handler(welcome)
        handler(welcome)
        assertEquals(2, calls)
        assertEquals(2, errors.size)
        assertTrue(errors.all { it is IllegalStateException })
    }

    @Test fun theWelcomeSendsTheList() {
        var calls = 0
        UiHelpers.welcomeHandler(work = { it.run() }, sendList = { calls++ }, log = {})(welcome)
        assertEquals(1, calls)
    }
}
