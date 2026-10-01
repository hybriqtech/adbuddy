package tech.hybriq.adbuddy.files

object FileNames {
    /** The host cuts names to 100 bytes; the app does it first so the draft shows the name that will be kept. */
    const val MAX_BYTES = 100

    /** The base name, without control characters or dots at the ends; "file" when nothing is left. */
    fun clean(raw: String): String {
        val base = raw.substringAfterLast('/').substringAfterLast('\\')
        val name = base.filterNot { it.isISOControl() }.trim().trim('.').trim()
        return if (name.isEmpty()) "file" else cut(name)
    }

    private fun cut(name: String): String {
        if (name.toByteArray().size <= MAX_BYTES) return name
        val dot = name.lastIndexOf('.')
        val ext = if (dot > 0 && name.length - dot <= 11) name.substring(dot) else ""
        val budget = MAX_BYTES - ext.toByteArray().size
        val sb = StringBuilder()
        var used = 0
        for (cp in name.removeSuffix(ext).codePoints().toArray()) {
            val s = String(Character.toChars(cp))
            val n = s.toByteArray().size
            if (used + n > budget) break
            sb.append(s)
            used += n
        }
        return sb.toString() + ext
    }
}
