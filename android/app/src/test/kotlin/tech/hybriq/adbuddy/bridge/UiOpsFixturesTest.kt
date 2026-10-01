package tech.hybriq.adbuddy.bridge

import java.io.File
import kotlinx.serialization.KSerializer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The args of each ui.request fixture and the result of each ui.response one
 * (M5 spec §3), as host/internal/bridge/ui_ops_test.go reads them: they decode
 * strictly, come out the same when encoded again, and between them have every
 * field of their type.
 */
class UiOpsFixturesTest {
    private val dir = File(
        System.getProperty("adbuddy.protocolFixtures") ?: error("adbuddy.protocolFixtures is not set (app/build.gradle.kts)"),
    )

    private val strict = Json(Protocol.json) { ignoreUnknownKeys = false }

    /** Each fixture and the serializer of its args or result; null for the error variants. */
    private val parts: Map<String, KSerializer<*>?> = mapOf(
        "host/ui.request" to UiClickArgs.serializer(),
        "host/ui.request@dump" to UiDumpArgs.serializer(),
        "host/ui.request@dumpall" to UiDumpArgs.serializer(),
        "host/ui.request@clickid" to UiClickArgs.serializer(),
        "host/ui.request@clickxy" to UiClickArgs.serializer(),
        "host/ui.request@settext" to UiSetTextArgs.serializer(),
        "host/ui.request@foreground" to Empty.serializer(),
        "host/ui.request@notifications" to Empty.serializer(),
        "app/ui.response" to UiClicked.serializer(),
        "app/ui.response@dump" to UiDump.serializer(),
        "app/ui.response@dumplocked" to UiDump.serializer(),
        "app/ui.response@settext" to UiTextSet.serializer(),
        "app/ui.response@foreground" to UiForeground.serializer(),
        "app/ui.response@notifications" to UiNotifications.serializer(),
        "app/ui.response@error" to null,
        "app/ui.response@ambiguous" to null,
    )

    /** The args of a ui.request fixture or the result of a ui.response one (JsonNull if none). */
    private fun part(key: String): JsonElement {
        val m = strict.decodeFromString(Message.serializer(), File(dir, "$key.json").readText())
        return if (m.type == Protocol.UI_REQUEST) {
            strict.decodeFromJsonElement(UiRequest.serializer(), m.payload).args
        } else {
            strict.decodeFromJsonElement(UiResponse.serializer(), m.payload).result ?: JsonNull
        }
    }

    @Test fun everyUiFixtureIsListed() {
        val names = listOf("host", "app").flatMap { d ->
            File(dir, d).listFiles { f -> f.name.endsWith(".json") }!!.map { "$d/${it.name.removeSuffix(".json")}" }
        }.filter { it.substringAfter("/").substringBefore("@") in setOf(Protocol.UI_REQUEST, Protocol.UI_RESPONSE) }
        assertEquals(parts.keys, names.toSet())
    }

    @Suppress("UNCHECKED_CAST")
    @Test fun argsAndResultsDecodeAndRoundTrip() {
        val failures = mutableListOf<String>()
        for ((key, serializer) in parts) {
            val p = part(key)
            if (serializer == null) {
                if (p != JsonNull) failures += "$key: an error variant with a result"
                continue
            }
            val value = try {
                strict.decodeFromJsonElement(serializer as KSerializer<Any?>, p)
            } catch (e: IllegalArgumentException) {
                failures += "$key: ${e.message}"
                continue
            }
            val again = strict.encodeToJsonElement(serializer, value)
            if (again != p) failures += "$key: encoded again\n$again\nfixture\n$p"
        }
        assertTrue(failures.joinToString("\n\n"), failures.isEmpty())
    }

    @Test fun argsAndResultsHaveEveryField() {
        val fixtures = FixturesTest()
        val present = mutableMapOf<KSerializer<*>, MutableSet<String>>()
        for ((key, serializer) in parts) {
            if (serializer == null) continue
            fixtures.jsonPaths(part(key), "", present.getOrPut(serializer) { mutableSetOf() })
        }
        val failures = present.mapNotNull { (serializer, got) ->
            val want = mutableSetOf<String>().also { fixtures.descriptorPaths(serializer.descriptor, "", it) }
            (want - got).takeIf { it.isNotEmpty() }?.let { "${serializer.descriptor.serialName}: no fixture has ${it.sorted()}" }
        }
        assertTrue(failures.joinToString("\n"), failures.isEmpty())
    }
}
