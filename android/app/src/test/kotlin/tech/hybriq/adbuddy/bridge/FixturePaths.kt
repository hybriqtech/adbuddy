package tech.hybriq.adbuddy.bridge

import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.descriptors.StructureKind
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject

/** The field paths the fixture tests compare: a type's against its fixtures'. */
object FixturePaths {
    /** The JSON paths of a descriptor's fields ("attachment.thumb"): lists are transparent; JsonElement is a leaf. */
    fun descriptorPaths(d: SerialDescriptor, prefix: String, out: MutableSet<String>) {
        if (d.kind != StructureKind.CLASS) return
        for (i in 0 until d.elementsCount) {
            val path = prefix + d.getElementName(i)
            out += path
            var child = d.getElementDescriptor(i)
            while (child.kind == StructureKind.LIST) child = child.getElementDescriptor(0)
            descriptorPaths(child, "$path.", out)
        }
    }

    /** The paths of the keys in a JSON value. */
    fun jsonPaths(e: JsonElement, prefix: String, out: MutableSet<String>) {
        when (e) {
            is JsonObject -> e.forEach { (k, v) -> out += prefix + k; jsonPaths(v, "$prefix$k.", out) }
            is JsonArray -> e.forEach { jsonPaths(it, prefix, out) }
            else -> {}
        }
    }
}
