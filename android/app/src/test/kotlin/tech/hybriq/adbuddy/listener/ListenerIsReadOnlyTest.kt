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

    private fun code(t: String): String = CodeScan.code(t)

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


    /**
     * The clearing must be the FIRST statement of the body: not inside an `if`, nor behind a `return`, nor (on a
     * connection) behind `current = this` or `_connected.value = true`, which make the listener visible to the gate.
     * [via] is NotificationGate's function for that moment: onConnected or onUnbound.
     */
    private fun clearsFirst(via: String) = Regex("^\\{\\s*NotificationGate\\.$via\\s*\\{[^}]*NotificationConsent\\.of\\(this\\)\\.revoke\\(\\)[^}]*}")
    private fun clearsTheMarkFirst(body: String, via: String = "onUnbound") = clearsFirst(via).containsMatchIn(body)

    /** Each method of the listener's lifecycle and the gate's function it must clear the mark through, first. */
    private val clearing = mapOf(
        "onListenerConnected" to "onConnected",
        "onListenerDisconnected" to "onUnbound",
        "onDestroy" to "onUnbound",
    )

    @Test fun theListenerClearsTheUsersMarkWhenTheSystemBindsOrUnbindsIt() {
        // Revoking the access unbinds the service: the mark goes at once, even if nobody is looking (the gate and the
        // screen only see the revoke when asked, and the agent could turn the switch back on before that). And the
        // system may bind it again without unbinding it first (M5c-32 b: a restart it had scheduled, with the app dead
        // when the access was revoked): every new binding clears it too, before the listener can be read.
        val src = code(File(dir, "AgentNotificationListener.kt"))
        for ((name, via) in clearing) {
            assertTrue("$name does not clear the mark first (NotificationGate.$via)", clearsTheMarkFirst(body(src, name), via))
        }
    }

    @Test fun theClearingMustBeTheFirstStatementOfTheBody() {
        val fine = "{ // comment\n NotificationGate.onUnbound { NotificationConsent.of(this).revoke() }\n if (x) {} }"
        assertTrue(clearsTheMarkFirst(code(fine)))
        val connected = "{ NotificationGate.onConnected { NotificationConsent.of(this).revoke() }\n current = this\n _connected.value = true }"
        assertTrue(clearsTheMarkFirst(code(connected), "onConnected"))
        val bad = listOf(
            "{ if (current === this) { NotificationGate.onUnbound { NotificationConsent.of(this).revoke() } } }",
            "{ if (x) return\n NotificationGate.onUnbound { NotificationConsent.of(this).revoke() } }",
            "{ return; NotificationGate.onUnbound { NotificationConsent.of(this).revoke() } }",
            "{ current = null\n NotificationGate.onUnbound { NotificationConsent.of(this).revoke() } }",
            "{ NotificationGate.onUnbound { } }",
        )
        for (b in bad) assertFalse(b, clearsTheMarkFirst(code(b)))
        val badConnected = listOf(
            "{ current = this\n NotificationGate.onConnected { NotificationConsent.of(this).revoke() } }",
            "{ _connected.value = true; NotificationGate.onConnected { NotificationConsent.of(this).revoke() } }",
            "{ NotificationGate.onConnected { } }",
            "{ if (x) return\n NotificationGate.onConnected { NotificationConsent.of(this).revoke() } }",
            // The other moment's function does not count.
            "{ NotificationGate.onUnbound { NotificationConsent.of(this).revoke() }\n current = this }",
        )
        for (b in badConnected) assertFalse(b, clearsTheMarkFirst(code(b), "onConnected"))
    }

    // UiHelpers is exempt from the white list above (it does the wiring), so its use is pinned exactly here: the class
    // appears once, as the last argument of NotificationGate.read(...), as a reference `::read` and never called.
    private val gateWiring = Regex("NotificationGate\\s*\\.\\s*read\\s*\\((?:[^()]|\\([^()]*\\))*,\\s*AgentNotificationListener\\s*::\\s*read\\s*,?\\s*\\)")
    private val readRef = Regex("\\bAgentNotificationListener\\s*::\\s*read\\b")

    /** What is wrong with the way [src] (UiHelpers, without comments and strings) reaches the listener; empty if right. */
    private fun wiringProblems(src: String): List<String> {
        val noImport = src.lines().filterNot { classImport.matches(it) }.joinToString("\n")
        val problems = mutableListOf<String>()
        val uses = name.findAll(noImport).count()
        if (uses != 1) problems += "AgentNotificationListener appears $uses times (not once)"
        if (!readRef.containsMatchIn(noImport)) problems += "not used as AgentNotificationListener::read"
        if (!gateWiring.containsMatchIn(noImport)) problems += "not the last argument of NotificationGate.read("
        return problems
    }

    @Test fun uiHelpersReadsOnlyThroughTheGate() {
        val src = code(File(root, "tech/hybriq/adbuddy/a11y/UiHelpers.kt"))
        assertEquals(emptyList<String>(), wiringProblems(src))
    }

    @Test fun theWiringCheckSeesTheGateRemoved() {
        val right = "import tech.hybriq.adbuddy.listener.AgentNotificationListener\n" +
            "val n = { NotificationGate.read(consent::confirmed, { NotificationAccessStatus.listed(app) }, consent::revoke, AgentNotificationListener::read) }\n"
        assertEquals(emptyList<String>(), wiringProblems(code(right)))
        val direct = right.replace("val n = {", "val m = AgentNotificationListener.read()\nval n = {")
        assertTrue(wiringProblems(code(direct)).isNotEmpty())
        val bypass = "import tech.hybriq.adbuddy.listener.AgentNotificationListener\nval n = { AgentNotificationListener.read() }\n"
        assertTrue(wiringProblems(code(bypass)).isNotEmpty())
        val notLast = "val n = { NotificationGate.read(AgentNotificationListener::read, consent::confirmed) }\n"
        assertTrue(wiringProblems(code(notLast)).isNotEmpty())
        val twice = right + "val f = AgentNotificationListener::read\n"
        assertTrue(wiringProblems(code(twice)).isNotEmpty())
    }
}
