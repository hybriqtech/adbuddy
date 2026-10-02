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

    private fun kotlinFiles(d: File) = d.walkTopDown().filter { it.isFile && it.extension == "kt" }.toList()

    @Test fun nothingInTheAppActsOnNotificationsOrReadsThemOutsideTheListener() {
        // The app's own NotificationManager.activeNotifications (its confirmation, to cancel it) is not the listener's.
        val ownNotifications = setOf("ConfirmNotifier.kt")
        for (f in kotlinFiles(root)) {
            val src = code(f)
            for (token in listOf("cancelNotification", "cancelAllNotifications", "snoozeNotification")) {
                assertFalse("${f.name} uses $token", src.contains(token))
            }
            if (f.name != "AgentNotificationListener.kt" && f.name !in ownNotifications) {
                assertFalse("${f.name} reads the active notifications", src.contains("activeNotifications") || src.contains("getActiveNotifications"))
            }
            if (f.parentFile != dir) {
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
            "android.util.Log", "Log.", "System.out", "System.err", "println", "Timber", "printStackTrace", "sendBroadcast",
        )
        // The consent keeps a flag in preferences; nothing else here stores anything.
        val storage = listOf("SharedPreferences", "openFileOutput", "FileWriter", "File(")
        for (f in kotlinFiles(dir)) {
            val src = code(f)
            for (token in forbidden) assertFalse("${f.name} uses $token", src.contains(token))
            assertFalse("${f.name} uses the notification actions", Regex("\\bactions\\b").containsMatchIn(src))
            if (f.name != "NotificationConsent.kt") for (token in storage) assertFalse("${f.name} uses $token", src.contains(token))
        }
    }

    @Test fun onlyTheLifecycleIsOverridden() {
        // The listener overrides only its lifecycle: no onNotificationPosted/Removed that could pass a notification on.
        val overrides = Regex("override fun (\\w+)").findAll(code(File(dir, "AgentNotificationListener.kt"))).map { it.groupValues[1] }.toSet()
        assertEquals(setOf("onListenerConnected", "onListenerDisconnected", "onDestroy"), overrides)
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

    // read() is the one way out of the listener: the call, with or without the companion, `.` or `::`.
    private val readCall = Regex("AgentNotificationListener\\s*(\\.\\s*Companion\\s*)?(\\.|::)\\s*read\\b")
    private val alias = Regex("import\\s+\\S*AgentNotificationListener\\s+as\\b")

    // A static import of read (or of everything) from the class or its companion: `read()` then needs no prefix.
    private val readImport = Regex("import\\s+[\\w.]*AgentNotificationListener\\s*(\\.\\s*Companion\\s*)?\\.\\s*(read\\b|\\*)")

    @Test fun onlyTheWiringOfTheHelpersReadsThroughTheListener() {
        // Only UiHelpers (through NotificationGate, so with the user's confirmation) may call read(), and nothing
        // outside the package may rename the class or import read() to hide the call.
        val outside = kotlinFiles(root).filter { it.parentFile != dir }
        val callers = outside.filter { readCall.containsMatchIn(code(it)) }.map { it.name }.toSet()
        assertTrue(callers.toString(), setOf("UiHelpers.kt").containsAll(callers))
        assertTrue("an alias of the listener hides its read()", outside.none { alias.containsMatchIn(code(it)) })
        val importers = outside.filter { it.name != "UiHelpers.kt" && readImport.containsMatchIn(code(it)) }.map { it.name }
        assertTrue("a static import of read() hides the call: $importers", importers.isEmpty())
        assertTrue(readCall.containsMatchIn("AgentNotificationListener::read") && readCall.containsMatchIn("AgentNotificationListener . read"))
    }

    @Test fun theScannerOfTheReadCallSeesTheCompanionAndTheStaticImport() {
        // The call through the companion, the callable reference and the static imports are all seen.
        val viaCompanion = code("val n = AgentNotificationListener.Companion.read()\n")
        assertTrue(viaCompanion, readCall.containsMatchIn(viaCompanion))
        val spaced = code("val n = AgentNotificationListener . Companion :: read\n")
        assertTrue(spaced, readCall.containsMatchIn(spaced))
        val imported = code("import tech.hybriq.adbuddy.listener.AgentNotificationListener.Companion.read\n")
        assertTrue(imported, readImport.containsMatchIn(imported))
        val importedPlain = code("import tech.hybriq.adbuddy.listener.AgentNotificationListener.read\n")
        assertTrue(importedPlain, readImport.containsMatchIn(importedPlain))
        val star = code("import tech.hybriq.adbuddy.listener.AgentNotificationListener.Companion.*\n")
        assertTrue(star, readImport.containsMatchIn(star))
        // What is not a way to read does not trip it: the class import, or `connected`.
        assertFalse(readImport.containsMatchIn("import tech.hybriq.adbuddy.listener.AgentNotificationListener\n"))
        assertFalse(readImport.containsMatchIn("import tech.hybriq.adbuddy.listener.AgentNotificationListener.Companion.connected\n"))
        assertFalse(readCall.containsMatchIn("AgentNotificationListener.connected"))
    }

    @Test fun theTextOfARawNotificationIsOnlyReadByTheReport() {
        // RawNotification travels only through these files; the report turns it into what leaves the phone.
        val users = kotlinFiles(root).filter { it.readText().contains("RawNotification") }.map { it.name }.toSet()
        val allowed = setOf("NotificationReport.kt", "AgentNotificationListener.kt", "NotificationRead.kt", "NotificationGate.kt", "UiExecutor.kt")
        assertTrue(users.toString(), allowed.containsAll(users))
        assertTrue(users.toString(), "NotificationReport.kt" in users)
    }
}
