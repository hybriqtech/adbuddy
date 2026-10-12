package tech.hybriq.adbuddy.bridge

import java.io.File
import kotlinx.serialization.KSerializer
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The args of each ui.request fixture and the result of each ui.response one
 * (M5 spec §3), as host/internal/bridge/ui_ops_test.go reads them, against
 * protocol/fixtures/ui.json: they decode strictly into the type of their
 * operation, come out the same when encoded again, and between them have
 * every field of their type. The limits and validPackage match ui.json too.
 */
class UiOpsFixturesTest {
    private val dir = File(
        System.getProperty("adbuddy.protocolFixtures") ?: error("adbuddy.protocolFixtures is not set (app/build.gradle.kts)"),
    )

    private val strict = Json(Protocol.json) { ignoreUnknownKeys = false }

    @Serializable
    private data class Packages(val valid: List<String>, val invalid: List<String>)

    @Serializable
    private data class Contract(
        val ops: List<String>,
        val responses: Map<String, String>,
        val errors: List<String>,
        val limits: Map<String, Int>,
        val packages: Packages,
    )

    private val contract = strict.decodeFromString(Contract.serializer(), File(dir, "ui.json").readText())

    /** An args or result, the fixture it comes from and the serializer of its type. */
    private class Part(val name: String, val json: JsonElement, val serializer: KSerializer<*>)

    /** The error codes of ErrorCode, by reflection. */
    private val codes = ErrorCode::class.java.declaredFields
        .filter { java.lang.reflect.Modifier.isStatic(it.modifiers) && it.type == String::class.java }
        .map { it.get(null) as String }
        .toSet()

    private fun parts(): List<Part> = listOf("host", "app").flatMap { d ->
        File(dir, d).listFiles { f -> f.name.endsWith(".json") }!!.mapNotNull { file ->
            val name = file.name.removeSuffix(".json")
            val m = strict.decodeFromString(Message.serializer(), file.readText())
            when (m.type) {
                Protocol.UI_REQUEST -> {
                    val r = strict.decodeFromJsonElement(UiRequest.serializer(), m.payload)
                    Part(name, r.args, (UiOp.ALL[r.op] ?: error("$name: unknown op ${r.op}")).first)
                }
                Protocol.UI_RESPONSE -> {
                    val r = strict.decodeFromJsonElement(UiResponse.serializer(), m.payload)
                    if (name in contract.errors) {
                        assertFalse(name, r.ok)
                        assertNull(name, r.result)
                        assertTrue("$name: code ${r.error?.code}", r.error?.code in codes)
                        null
                    } else {
                        val op = contract.responses[name] ?: error("$name is neither in ui.json's responses nor in its errors")
                        assertTrue(name, r.ok && r.error == null)
                        Part(name, r.result!!, UiOp.ALL.getValue(op).second)
                    }
                }
                else -> null
            }
        }
    }

    @Test fun theOperationsAreUiJsons() {
        assertEquals(contract.ops.toSet(), UiOp.ALL.keys)
        for (name in contract.responses.keys) assertTrue(name, File(dir, "app/$name.json").exists())
    }

    @Suppress("UNCHECKED_CAST")
    @Test fun argsAndResultsDecodeAndRoundTrip() {
        val failures = mutableListOf<String>()
        for (p in parts()) {
            val serializer = p.serializer as KSerializer<Any?>
            val value = try {
                strict.decodeFromJsonElement(serializer, p.json)
            } catch (e: IllegalArgumentException) {
                failures += "${p.name}: ${e.message}"
                continue
            }
            val again = strict.encodeToJsonElement(serializer, value)
            if (again != p.json) failures += "${p.name}: encoded again\n$again\nfixture\n${p.json}"
        }
        assertTrue(failures.joinToString("\n\n"), failures.isEmpty())
    }

    @Test fun argsAndResultsHaveEveryField() {
        val present = mutableMapOf<KSerializer<*>, MutableSet<String>>()
        for (p in parts()) FixturePaths.jsonPaths(p.json, "", present.getOrPut(p.serializer) { mutableSetOf() })
        val failures = UiOp.ALL.flatMap { (op, s) -> listOf(op to s.first, op to s.second) }.mapNotNull { (op, serializer) ->
            val got = present[serializer] ?: return@mapNotNull "$op: no fixture of ${serializer.descriptor.serialName}"
            val want = mutableSetOf<String>().also { FixturePaths.descriptorPaths(serializer.descriptor, "", it) }
            (want - got).takeIf { it.isNotEmpty() }?.let { "${serializer.descriptor.serialName}: no fixture has ${it.sorted()}" }
        }
        assertTrue(failures.joinToString("\n"), failures.isEmpty())
    }

    @Test fun theLimitsAreUiJsons() {
        assertEquals(
            contract.limits,
            mapOf(
                "maxUiResult" to UiLimits.MAX_UI_RESULT,
                "maxNotifications" to UiLimits.MAX_NOTIFICATIONS,
                "maxNotificationText" to UiLimits.MAX_NOTIFICATION_TEXT,
                "maxSetText" to UiLimits.MAX_SET_TEXT,
                "maxProtectedApps" to UiLimits.MAX_PROTECTED_APPS,
                "maxPackage" to UiLimits.MAX_PACKAGE,
                "maxHideNoticeMs" to UiLimits.MAX_HIDE_NOTICE_MS,
            ),
        )
    }

    @Test fun validPackage() {
        for (p in contract.packages.valid) assertTrue(p, UiLimits.validPackage(p))
        for (p in contract.packages.invalid) assertFalse(p, UiLimits.validPackage(p))
    }

    /** protectedApps absent is null ("no change"); present and empty is a list. */
    @Test fun protectedAppsAbsentIsNotEmpty() {
        fun info(name: String) = strict.decodeFromJsonElement(
            DeviceInfo.serializer(),
            strict.decodeFromString(Message.serializer(), File(dir, "app/$name.json").readText()).payload,
        )
        assertNull(info("device.info@empty").protectedApps)
        assertEquals(emptyList<String>(), info("device.info@protectednone").protectedApps)
        // A DeviceInfo without the list does not send one.
        assertFalse("protectedApps" in Protocol.json.encodeToString(DeviceInfo.serializer(), DeviceInfo(tailscaleIp = "192.0.2.7")))
    }

    @Test fun boundsNeedFourNumbers() {
        val bad = """{"clicked":true,"bounds":[1,2,3],"how":"action"}"""
        assertTrue(runCatching { strict.decodeFromString(UiClicked.serializer(), bad) }.isFailure)
    }
}
