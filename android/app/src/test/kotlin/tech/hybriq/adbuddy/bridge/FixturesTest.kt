package tech.hybriq.adbuddy.bridge

import java.io.File
import java.util.Base64
import kotlinx.serialization.KSerializer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.long
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The protocol's shared fixtures (protocol/fixtures/, protocol spec §12), as
 * host/internal/bridge/fixtures_test.go reads them: each one decodes strictly,
 * and the base file of each type comes out the same when encoded again.
 */
class FixturesTest {
    private val dir = File(
        System.getProperty("adbuddy.protocolFixtures") ?: error("adbuddy.protocolFixtures is not set (app/build.gradle.kts)"),
    )

    /** Protocol.json, except that a field the payload does not have fails. */
    private val strict = Json(Protocol.json) { ignoreUnknownKeys = false }

    private val directions = mapOf("host" to Registry.HOST, "app" to Registry.APP)

    /** The fixture files of a direction by name: "<type>" or "<type>@<variant>". */
    private fun files(direction: String): Map<String, File> {
        val all = File(dir, direction).listFiles { f -> f.name.endsWith(".json") }
        assertTrue("no fixtures in ${File(dir, direction)}", !all.isNullOrEmpty())
        return all!!.associateBy { it.name.removeSuffix(".json") }
    }

    @Suppress("UNCHECKED_CAST")
    private fun check(direction: String, name: String, file: File, payloads: Map<String, KSerializer<*>>): String? {
        val m = try {
            strict.decodeFromString(Message.serializer(), file.readText())
        } catch (e: IllegalArgumentException) {
            return "$direction/$name: envelope: ${e.message}"
        }
        val type = name.substringBefore("@")
        if (m.type != type || m.v != Protocol.VERSION) return "$direction/$name: type ${m.type} v${m.v}"
        if (m.rid != null && !Protocol.validRid(m.rid)) return "$direction/$name: rid ${m.rid}"
        val serializer = payloads[type] as KSerializer<Any?>? ?: return "$direction/$name: $type is not a type the $direction sends"
        val value = try {
            strict.decodeFromJsonElement(serializer, m.payload)
        } catch (e: IllegalArgumentException) {
            return "$direction/$name: payload: ${e.message}"
        }
        if (name.contains("@")) return null
        val again: JsonElement = strict.encodeToJsonElement(serializer, value)
        return if (again == m.payload) null else "$direction/$name: encoded again\n$again\nfixture\n${m.payload}"
    }

    @Test fun everyFixtureDecodesAndRoundTrips() {
        val failures = directions.flatMap { (direction, payloads) ->
            files(direction).mapNotNull { (name, file) -> check(direction, name, file, payloads) }
        }
        assertTrue(failures.joinToString("\n\n"), failures.isEmpty())
    }

    /**
     * Every field of every payload appears in at least one fixture of its type:
     * a field added to one side only would otherwise go unseen, since an absent
     * optional field is encoded again as absent.
     */
    @Test fun fixturesHaveEveryField() {
        val failures = mutableListOf<String>()
        for ((direction, payloads) in directions) {
            val present = mutableMapOf<String, MutableSet<String>>()
            for ((name, file) in files(direction)) {
                val m = Protocol.decode(file.readText()) ?: continue
                FixturePaths.jsonPaths(m.payload, "", present.getOrPut(name.substringBefore("@")) { mutableSetOf() })
            }
            for ((type, serializer) in payloads) {
                val want = mutableSetOf<String>().also { FixturePaths.descriptorPaths(serializer.descriptor, "", it) }
                val missing = want - present[type].orEmpty()
                if (missing.isNotEmpty()) failures += "$direction/$type: no fixture has ${missing.sorted()}"
            }
        }
        assertTrue(failures.joinToString("\n"), failures.isEmpty())
    }

    @Test fun everyTypeHasAFixtureAndEveryFixtureAType() {
        for ((direction, payloads) in directions) {
            val files = files(direction)
            assertEquals("$direction types without a fixture", emptySet<String>(), payloads.keys - files.keys)
            assertEquals("$direction fixtures without a type", emptySet<String>(), files.keys.map { it.substringBefore("@") }.toSet() - payloads.keys)
        }
    }

    @Test fun theDirectionsDoNotShareTypes() {
        assertEquals(emptySet<String>(), Registry.HOST.keys intersect Registry.APP.keys)
    }

    /** The registry lists every type constant: one added to Protocol but not to Registry fails here. */
    @Test fun theRegistryListsEveryTypeConstant() {
        val constants = Protocol::class.java.declaredFields
            .filter { java.lang.reflect.Modifier.isStatic(it.modifiers) && it.type == String::class.java }
            .map { it.get(null) as String }
            .filter { it.isNotEmpty() && it.first().isLowerCase() }
            .toSet()
        assertEquals("constants found", Registry.HOST.size + Registry.APP.size, constants.size)
        assertEquals(emptySet<String>(), constants - Registry.HOST.keys - Registry.APP.keys)
    }

    private fun hex(s: String): ByteArray = ByteArray(s.length / 2) { i -> s.substring(2 * i, 2 * i + 2).toInt(16).toByte() }

    @Test fun frameVectors() {
        val v = Json.parseToJsonElement(File(dir, "frames.json").readText()).jsonObject
        for (c in v.getValue("valid").jsonArray.map { it.jsonObject }) {
            val id = c.getValue("id").jsonPrimitive.content
            val offset = c.getValue("offset").jsonPrimitive.long
            val data = Base64.getDecoder().decode(c.getValue("data").jsonPrimitive.content)
            val frame = hex(c.getValue("frame").jsonPrimitive.content)
            assertArrayEquals(frame, Frame.encode(id, offset, data))
            val chunk = Frame.decode(frame)!!
            assertEquals(id, chunk.id)
            assertEquals(offset, chunk.offset)
            assertArrayEquals(data, chunk.data)
        }
        for (c in v.getValue("invalid").jsonArray.map { it.jsonObject }) {
            assertNull(c.getValue("why").jsonPrimitive.content, Frame.decode(hex(c.getValue("frame").jsonPrimitive.content)))
        }
        for (id in v.getValue("badIds").jsonArray.map { it.jsonPrimitive.content }) {
            assertNull(id, Frame.parseId(id))
        }
    }

    @Test fun frameLimits() {
        val id = "00".repeat(Frame.ID_LEN)
        assertNull(Frame.encode(id, 0, ByteArray(0)))
        assertNull(Frame.encode(id, -1, ByteArray(1)))
        assertNull(Frame.encode(id, 0, ByteArray(Frame.MAX_DATA + 1)))
        val frame = Frame.encode(id, 0, ByteArray(Frame.MAX_DATA))
        assertNotNull(frame)
        assertNotNull(Frame.decode(frame!!))
        assertNull(Frame.decode(frame + 0))
    }

    /** An M2 host sends none of the new fields: the app reads its messages with the defaults. */
    @Test fun anM2HostIsRead() {
        val m = Protocol.decode(File(dir, "host/snapshot@m2.json").readText())!!
        val s = Protocol.payload<Snapshot>(m)!!
        assertEquals(emptyList<JsonElement>(), s.chat)
        assertNull(s.remote)
        assertNull(s.yolo)
        assertEquals(JsonObject(emptyMap()), Protocol.decode(File(dir, "app/pause.json").readText())!!.payload)
    }
}
