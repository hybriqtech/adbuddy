package tech.hybriq.adbuddy.listener

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The notification listener may only read (M5c plan, security rules): the
 * agent must not be able to dismiss, snooze or act on a notification, nor
 * read one of a protected app. A source check, since nothing on the JVM can
 * run the service: it keeps a later edit from adding a way to act, a way
 * round the report, or a log of what a notification says.
 */
class ListenerIsReadOnlyTest {
    private val root = File("src/main/kotlin")
    private val dir = File(root, "tech/hybriq/adbuddy/listener")

    // Kotlin and, if one ever appears, Java: both are scanned (a Java file could do what a Kotlin one may not).
    private val roots = listOf(root, File("src/main/java")).filter { it.exists() }
    private val listenerDirs = roots.map { File(it, "tech/hybriq/adbuddy/listener") }

    /**
     * The code of a file without comments and strings (a small scanner: a regex overflows the stack on big
     * files). What is inside a template ("${...}") is code, not string: a call hidden in one is seen.
     */
    private fun code(f: File): String = code(f.readText())

    private fun code(t: String): String {
        val out = StringBuilder()
        var i = 0
        while (i < t.length) {
            val c = t[i]
            when {
                t.startsWith("//", i) -> while (i < t.length && t[i] != '\n') i++
                t.startsWith("/*", i) -> {
                    val end = t.indexOf("*/", i + 2)
                    i = if (end < 0) t.length else end + 2
                }
                t.startsWith("\"\"\"", i) -> {
                    i = skipString(t, i + 3, raw = true, out)
                    out.append("\"\"")
                }
                c == '"' -> {
                    i = skipString(t, i + 1, raw = false, out)
                    out.append("\"\"")
                }
                c == '\'' -> { // a char literal, such as '"'
                    i = skipChar(t, i + 1)
                    out.append("''")
                }
                else -> {
                    out.append(c)
                    i++
                }
            }
        }
        return out.toString()
    }

    /** The index just past a string whose content starts at [from]; the code of its templates goes to [out]. */
    private fun skipString(t: String, from: Int, raw: Boolean, out: StringBuilder): Int {
        var i = from
        while (i < t.length) {
            when {
                raw && t.startsWith("\"\"\"", i) -> return i + 3
                !raw && t[i] == '"' -> return i + 1
                !raw && t[i] == '\\' -> i += 2
                t.startsWith("\${", i) -> {
                    val end = templateEnd(t, i + 2)
                    out.append(' ').append(code(t.substring(i + 2, minOf(end, t.length)))).append(' ')
                    i = end + 1
                }
                else -> i++
            }
        }
        return t.length
    }

    private fun skipChar(t: String, from: Int): Int {
        var i = from
        while (i < t.length && t[i] != '\'') i += if (t[i] == '\\') 2 else 1
        return i + 1
    }

    /** The index of the } that closes a template whose code starts at [from]; unbalanced, the rest of the file is code. */
    private fun templateEnd(t: String, from: Int): Int {
        var depth = 1
        var i = from
        while (i < t.length) {
            when {
                t.startsWith("\"\"\"", i) -> i = skipString(t, i + 3, raw = true, StringBuilder())
                t[i] == '"' -> i = skipString(t, i + 1, raw = false, StringBuilder())
                t[i] == '\'' -> i = skipChar(t, i + 1)
                t[i] == '{' -> { depth++; i++ }
                t[i] == '}' -> {
                    if (--depth == 0) return i
                    i++
                }
                else -> i++
            }
        }
        return t.length
    }

    private fun kotlinFiles(d: File) = d.walkTopDown().filter { it.isFile && (it.extension == "kt" || it.extension == "java") }.toList()

    private fun allSources() = roots.flatMap { kotlinFiles(it) }

    @Test fun nothingInTheAppActsOnNotificationsOrReadsThemOutsideTheListener() {
        // The app's own NotificationManager.activeNotifications (its confirmation, to cancel it) is not the listener's.
        val ownNotifications = setOf("ConfirmNotifier.kt")
        for (f in allSources()) {
            val src = code(f)
            for (token in listOf("cancelNotification", "cancelAllNotifications", "snoozeNotification")) {
                assertFalse("${f.name} uses $token", src.contains(token))
            }
            if (f.name != "AgentNotificationListener.kt" && f.name !in ownNotifications) {
                assertFalse("${f.name} reads the active notifications", src.contains("activeNotifications") || src.contains("getActiveNotifications"))
            }
            if (f.parentFile !in listenerDirs) {
                assertFalse("${f.name} reaches the service itself", src.contains("AgentNotificationListener.current"))
            }
        }
    }

    @Test fun theListenerPackageNeverActsReflectsOrLeaks() {
        val forbidden = listOf(
            "cancelNotification", "cancelAllNotifications", "snoozeNotification", "setNotificationsShown",
            "updateNotificationChannel", "adjustNotification", "contentIntent", "deleteIntent", "fullScreenIntent",
            "PendingIntent", "RemoteInput", "requestInterruptionFilter", "setOnNotificationPostedTrigger",
            "getMethod", "getDeclaredMethod", ".invoke(", "Class.forName", ".send(", "bubbleMetadata", "publicVersion",
            "requestListenerHints", "clearRequestedListenerHints", "migrateNotificationFilter", "requestUnbind", "requestRebind",
            "deleteConversationNotificationChannel", "createConversationNotificationChannelForPackage", "getSnoozedNotifications",
            "snoozedNotifications",
            "android.util.Log", "Log.", "System.out", "System.err", "println", "Timber", "printStackTrace", "sendBroadcast",
        )
        // The consent keeps a flag in preferences; nothing else here stores anything.
        val storage = listOf("SharedPreferences", "openFileOutput", "FileWriter", "File(")
        for (f in listenerDirs.flatMap { kotlinFiles(it) }) {
            val src = code(f)
            for (token in forbidden) assertFalse("${f.name} uses $token", src.contains(token))
            assertFalse("${f.name} uses the notification actions", Regex("\\bactions\\b").containsMatchIn(src))
            if (f.name != "NotificationConsent.kt") for (token in storage) assertFalse("${f.name} uses $token", src.contains(token))
        }
    }

    private val overrideDecl = Regex("\\boverride\\s+(?:\\w+\\s+)*(?:fun|val|var)\\s+(?:<[^>]*>\\s*)?(\\w+)")

    /** The members a source overrides; each `override` keyword must give one, so a layout that dodges the regex fails. */
    private fun overrides(src: String): List<String> {
        val found = overrideDecl.findAll(src).map { it.groupValues[1] }.toList()
        assertEquals("an override the scanner does not understand: $src", Regex("\\boverride\\b").findAll(src).count(), found.size)
        return found
    }

    @Test fun onlyTheLifecycleIsOverridden() {
        // The listener overrides only its lifecycle: no onNotificationPosted/Removed that could pass a notification on.
        val seen = overrides(code(File(dir, "AgentNotificationListener.kt")))
        assertEquals(listOf("onListenerConnected", "onListenerDisconnected", "onDestroy"), seen)
    }

    @Test fun theOverrideScannerSeesEveryFormOfDeclaration() {
        assertEquals(listOf("dump"), overrides("override protected fun dump(a: Int) {}"))
        assertEquals(listOf("x"), overrides("override public fun x() {}"))
        assertEquals(listOf("v"), overrides("override val v = 1"))
        assertEquals(listOf("w"), overrides("override var w = 1"))
        assertEquals(listOf("a", "b"), overrides("override\nfun a() {}\noverride\n  internal\n  fun b() {}"))
        assertEquals(listOf("g"), overrides("override fun <T> g(t: T) {}"))
        // One it cannot read makes the count differ, so the check fails instead of letting it through.
        val failed = try { overrides("override /* x */ fun hidden() {}"); false } catch (e: AssertionError) { true }
        assertTrue("an override that the scanner does not capture must fail", failed)
    }

    @Test fun theScannerSeesCodeNotCommentsOrStrings() {
        val f = File.createTempFile("scan", ".kt")
        try {
            f.writeText("val a = \"cancelNotification\" // snoozeNotification\n/* getMethod */ val b = '\"'; val c = cancelAll\n")
            val seen = code(f)
            assertFalse(seen, seen.contains("cancelNotification") || seen.contains("snoozeNotification") || seen.contains("getMethod"))
            assertTrue(seen, seen.contains("val c = cancelAll"))
        } finally {
            f.delete()
        }
    }

    @Test fun theScannerSeesTheCodeInsideATemplateButNotTheTextAroundIt() {
        // "${cancelNotification()}" is a call, not a string: the scanner must not hide it.
        val hidden = code("val a = \"x \${cancelNotification()} y\"\n")
        assertTrue(hidden, hidden.contains("cancelNotification"))
        val nested = code("val b = \"\${listOf(1).map { \"in\" + it }} snoozeNotification \${'}'}\" + getMethod\n")
        assertFalse(nested, nested.contains("snoozeNotification") || nested.contains("\"in\""))
        assertTrue(nested, nested.contains("listOf(1).map") && nested.contains("getMethod"))
        val raw = code("val c = \"\"\"a \${cancelAllNotifications()} b\"\"\"\n")
        assertTrue(raw, raw.contains("cancelAllNotifications"))
        assertTrue(code("val d = \"\${ never closed cancelNotification").contains("cancelNotification"))
        assertFalse(code("val e = \"\$cancelNotification and \\\${snoozeNotification}\"").contains("snoozeNotification"))
    }

    // Outside the listener's package, the name of the class may only appear as the class import or as its
    // `connected` (the state of the setting). Anything else (a call to read(), with, run, let, an assignment,
    // a typealias, ::class, a static import of read) is a way to reach the service or read(): a white list,
    // not a list of the ways round that someone has thought of.
    private val allowedUse = Regex("\\bAgentNotificationListener\\s*(\\.\\s*Companion\\s*)?\\.\\s*connected\\b")
    private val classImport = Regex("^\\s*import\\s+[\\w.]*\\bAgentNotificationListener(\\s*\\.\\s*Companion)?(\\s*\\.\\s*connected)?\\s*$")
    private val name = Regex("\\bAgentNotificationListener\\b")

    /** The lines of [src] (already without comments and strings) that use the class in a way that is not allowed. */
    private fun badUses(src: String): List<String> = src.lines().filter { line ->
        name.containsMatchIn(line) && !classImport.matches(line) && name.containsMatchIn(allowedUse.replace(line, ""))
    }.map { it.trim() }

    @Test fun onlyTheWiringOfTheHelpersReadsThroughTheListener() {
        // Only UiHelpers (through NotificationGate, so with the user's confirmation) may call read(); outside the
        // package nothing else may mention the class but for its import and `connected`.
        val outside = allSources().filter { it.parentFile !in listenerDirs && it.name != "UiHelpers.kt" }
        for (f in outside) {
            val bad = badUses(code(f))
            assertTrue("${f.name} reaches the listener beyond connected: $bad", bad.isEmpty())
        }
    }

    @Test fun theWhiteListSeesEveryWayRoundTheReadCall() {
        val evasions = listOf(
            "val n = AgentNotificationListener.read()",
            "val n = AgentNotificationListener.Companion.read()",
            "val n = AgentNotificationListener . Companion :: read",
            "val f = AgentNotificationListener::read",
            "with(AgentNotificationListener) { read() }",
            "AgentNotificationListener.run { read() }",
            "AgentNotificationListener.let { it.read() }",
            "val l = AgentNotificationListener; l.read()",
            "typealias L = AgentNotificationListener",
            "val k = AgentNotificationListener::class",
            "import tech.hybriq.adbuddy.listener.AgentNotificationListener as L",
            "import tech.hybriq.adbuddy.listener.AgentNotificationListener.Companion.read",
            "import tech.hybriq.adbuddy.listener.AgentNotificationListener.read",
            "import tech.hybriq.adbuddy.listener.AgentNotificationListener.Companion.*",
        )
        for (e in evasions) assertEquals(e, listOf(e), badUses(code(e + "\n")))
        // What is allowed: the class import, `connected`, and a mention in a comment or a string.
        val fine = listOf(
            "import tech.hybriq.adbuddy.listener.AgentNotificationListener",
            "import tech.hybriq.adbuddy.listener.AgentNotificationListener.Companion.connected",
            "val c = AgentNotificationListener.connected",
            "val c = AgentNotificationListener.Companion.connected.value",
            "val c = AgentNotificationListener . connected",
            "val s = \"AgentNotificationListener.read()\" // AgentNotificationListener.read()",
        )
        for (e in fine) assertEquals(e, emptyList<String>(), badUses(code(e + "\n")))
    }

    @Test fun theTextOfARawNotificationIsOnlyReadByTheReport() {
        // RawNotification travels only through these files; the report turns it into what leaves the phone.
        val users = allSources().filter { it.readText().contains("RawNotification") }.map { it.name }.toSet()
        val allowed = setOf("NotificationReport.kt", "AgentNotificationListener.kt", "NotificationRead.kt", "NotificationGate.kt", "UiExecutor.kt")
        assertTrue(users.toString(), allowed.containsAll(users))
        assertTrue(users.toString(), "NotificationReport.kt" in users)
    }

    /** The body, braces included, of `override fun name(...) {...}` in [src] (already without comments and strings). */
    private fun body(src: String, name: String): String {
        val at = src.indexOf("override fun $name(")
        assertTrue("no override of $name", at >= 0)
        val open = src.indexOf('{', at)
        var depth = 0
        for (i in open until src.length) {
            if (src[i] == '{') depth++
            if (src[i] == '}' && --depth == 0) return src.substring(open, i + 1)
        }
        error("$name is not closed")
    }

    @Test fun theListenerClearsTheUsersMarkWhenTheSystemUnbindsIt() {
        // Revoking the access unbinds the service: the mark goes at once, even if nobody is looking (the gate and the
        // screen only see the revoke when asked, and the agent could turn the switch back on before that).
        val src = code(File(dir, "AgentNotificationListener.kt"))
        val call = Regex("NotificationGate\\.onUnbound\\s*\\{[^}]*NotificationConsent\\.of\\(this\\)\\.revoke\\(\\)[^}]*}")
        for (name in listOf("onListenerDisconnected", "onDestroy")) {
            assertTrue("$name does not clear the mark", call.containsMatchIn(body(src, name)))
        }
    }
}
