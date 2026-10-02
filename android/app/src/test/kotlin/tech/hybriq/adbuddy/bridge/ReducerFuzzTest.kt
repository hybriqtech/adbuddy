package tech.hybriq.adbuddy.bridge

import kotlin.random.Random
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.fail
import org.junit.Test

/**
 * V42: no reducer throws, whatever the payload (protocol spec §10). A reducer that throws on OkHttp's reader
 * thread cuts the socket, and the snapshot that comes again with the next connection would throw again: a
 * reconnection loop. Deterministic: fixed seeds, so a failure repeats; the message says which one.
 */
class ReducerFuzzTest {
    private val seeds = listOf(20261002L, 1L, 42L)

    /** A valid payload of each type the reducers read, which the generator then breaks a little or a lot. */
    private val templates: Map<String, String> = mapOf(
        Protocol.CHAT_MESSAGE to """{"id":"m1","role":"assistant","text":"hi","attachments":[{"id":"00112233445566778899aabbccddeeff","name":"a.png","mime":"image/png","size":3,"sha256":"x"}],"conversation":"c1"}""",
        Protocol.TOOL_ACTIVITY to """{"id":"t1","messageId":"a_m1","summary":"Run ls","status":"running"}""",
        Protocol.PERMISSION_REQUEST to """{"id":"p1","tool":"Bash","summary":"ls","detail":"ls -la","expiresAt":"2026-10-02T00:00:00Z","allowAlways":true}""",
        Protocol.PERMISSION_RESOLVED to """{"id":"p1","decision":"allow","by":"phone"}""",
        Protocol.REMOTE_STATE to """{"enabled":true,"address":"100.64.0.5:5555","expiresAt":"2026-10-02T12:00:00Z"}""",
        Protocol.SESSION_STATE to """{"agent":"claude","state":"working","activity":"x","paused":false,"conversation":"c1","view":"chat","pcAttached":false}""",
        Protocol.CONFIRM_REQUEST to """{"id":"j_1","level":"red","title":"Restart","rawCommand":"adb shell reboot","expiresAt":"2026-10-02T00:00:00Z","special":true}""",
        Protocol.JOURNAL_ENTRY to """{"id":"j_1","status":"ok","level":"red","argv":["shell","reboot"],"exitCode":0,"inverse":{"argv":["x"]},"backups":[]}""",
        Protocol.WELCOME to """{"session":"s_1","fromSeq":1,"protocol":1}""",
    )

    private val keys = listOf(
        "id", "role", "text", "attachments", "conversation", "messageId", "summary", "status", "tool", "detail", "expiresAt",
        "allowAlways", "decision", "by", "enabled", "address", "state", "agent", "paused", "pending", "journal", "chat",
        "permissions", "remote", "title", "rawCommand", "level", "argv", "exitCode", "inverse", "session", "fromSeq", "protocol",
    )

    private class Gen(seed: Long, val names: List<String>) {
        val rnd = Random(seed)

        fun primitive(): JsonElement = when (rnd.nextInt(12)) {
            0 -> JsonNull
            1 -> JsonPrimitive(rnd.nextBoolean())
            2 -> JsonPrimitive(rnd.nextInt())
            3 -> JsonPrimitive(rnd.nextLong())
            4 -> JsonPrimitive(rnd.nextDouble() * 1e300)
            5 -> JsonPrimitive(listOf(Double.NaN, Double.POSITIVE_INFINITY, -0.0, 3.5).random(rnd))
            6 -> JsonPrimitive(java.math.BigDecimal("1e400"))
            7 -> JsonPrimitive(listOf(Long.MAX_VALUE, Long.MIN_VALUE, 0L, -1L).random(rnd))
            8 -> JsonPrimitive("a".repeat(rnd.nextInt(0, 3000)))
            else -> JsonPrimitive(listOf("", "m1", "m2", "t1", "p1", "j_1", "running", "done", "error", "true", "1e400", "-1", "\u0000", "\uD800").random(rnd))
        }

        fun element(depth: Int): JsonElement = if (depth <= 0) primitive() else when (rnd.nextInt(4)) {
            0 -> primitive()
            1 -> JsonArray(List(rnd.nextInt(4)) { element(depth - 1) })
            else -> JsonObject((0 until rnd.nextInt(6)).associate { names.random(rnd) to element(depth - 1) })
        }

        /** The template with fields replaced, dropped or added, or nested values broken further down. */
        fun mutate(el: JsonElement, depth: Int = 3): JsonElement = when {
            rnd.nextInt(10) == 0 -> element(2)
            el is JsonObject -> JsonObject(buildMap {
                for ((k, v) in el) {
                    when (rnd.nextInt(10)) {
                        0 -> Unit // dropped
                        1, 2 -> put(k, element(2))
                        else -> put(k, if (depth > 0) mutate(v, depth - 1) else v)
                    }
                }
                if (rnd.nextInt(5) == 0) put(names.random(rnd), element(2)) // not `keys`: inside buildMap that is the map's own
            })
            el is JsonArray -> JsonArray(el.map { if (depth > 0) mutate(it, depth - 1) else it } + List(rnd.nextInt(2)) { element(2) })
            else -> el
        }
    }

    private fun template(type: String): JsonElement = Protocol.json.parseToJsonElement(templates.getValue(type))

    /** A snapshot: the session state, pending (mostly valid, so each reducer's own part is reached), and those parts. */
    private fun Gen.snapshot(): JsonElement {
        val parts = mutableMapOf(
            "state" to mutate(template(Protocol.SESSION_STATE)),
            "pending" to if (rnd.nextInt(4) == 0) element(2) else JsonArray(List(rnd.nextInt(3)) { template(Protocol.CONFIRM_REQUEST) }),
            "journal" to JsonArray(List(rnd.nextInt(4)) { mutate(template(Protocol.JOURNAL_ENTRY)) }),
            "chat" to JsonArray(List(rnd.nextInt(4)) { mutate(template(Protocol.CHAT_MESSAGE)) }),
            "permissions" to JsonArray(List(rnd.nextInt(4)) { mutate(template(Protocol.PERMISSION_REQUEST)) }),
            "remote" to mutate(template(Protocol.REMOTE_STATE)),
        )
        if (rnd.nextInt(5) == 0) parts[names.random(rnd)] = element(3)
        return if (rnd.nextInt(10) == 0) element(3) else JsonObject(parts)
    }

    private fun Gen.message(): Message {
        val types = BridgeState.REDUCERS.flatMap { it.types } + listOf(Protocol.SNAPSHOT, Protocol.WELCOME, Protocol.ERROR, "x.unknown")
        val type = types.random(rnd)
        val payload = when {
            type == Protocol.SNAPSHOT -> snapshot()
            type in templates -> mutate(template(type))
            else -> element(3)
        }
        val seq = listOf(0L, 1L, 2L, rnd.nextLong(0, 50), Long.MAX_VALUE, -1L).random(rnd)
        return Message(v = 1, seq = seq, rid = if (rnd.nextBoolean()) null else "r1", type = type, ts = "x", payload = payload)
    }

    private fun check(seed: Long, i: Int, what: String, m: Message?, f: () -> Unit) {
        try {
            f()
        } catch (e: Throwable) {
            fail("seed $seed, step $i, $what threw $e with ${m?.type} ${m?.payload.toString().take(500)}")
        }
    }

    @Test fun noReducerThrowsWithAnyPayload() {
        val welcome = Message(v = 1, type = Protocol.WELCOME, ts = "x", payload = template(Protocol.WELCOME))
        for (seed in seeds) {
            val g = Gen(seed, keys)
            var s = BridgeState().apply(welcome)
            for (i in 0 until 5_000) {
                val m = g.message()
                // Through the state, as the channel does.
                check(seed, i, "BridgeState.apply", m) { s = s.apply(m) }
                // And each reducer directly: an event of its own types, any readable snapshot, every reset.
                for (r in BridgeState.REDUCERS) {
                    val type = r.types.random(g.rnd)
                    val own = m.copy(type = type, payload = if (type in templates) g.mutate(template(type)) else g.element(3))
                    check(seed, i, "${r::class.simpleName}.event", own) { r.event(s, own) }
                    val snap = Protocol.payload<Snapshot>(Message(v = 1, type = Protocol.SNAPSHOT, payload = g.snapshot()))
                    if (snap != null) check(seed, i, "${r::class.simpleName}.snapshot", m) { r.snapshot(s, snap) }
                    for (why in Reset.entries) check(seed, i, "${r::class.simpleName}.reset($why)", null) { r.reset(s, why) }
                }
                check(seed, i, "disconnected", null) { s = s.disconnected() }
                if (i % 50 == 0) check(seed, i, "welcome", welcome) { s = s.apply(welcome) } // out of INCOMPATIBLE now and then
            }
        }
    }
}
