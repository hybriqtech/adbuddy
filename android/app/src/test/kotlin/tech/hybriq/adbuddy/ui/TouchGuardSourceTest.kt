package tech.hybriq.adbuddy.ui

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.listener.CodeScan

/**
 * Tapjacking (O1): every surface that grants something, and every dialog, filters touches while another app's
 * window covers it. A dialog is its own window, so the screen's flag does not reach it: each AlertDialog carries
 * the guard itself. A source check, since nothing on the JVM can draw an overlay.
 */
class TouchGuardSourceTest {
    private val uiDir = File("src/main/kotlin/tech/hybriq/adbuddy/ui")
    private val guard = "ObscuredTouchGuard()"

    private fun uiFiles() = uiDir.listFiles { f -> f.extension == "kt" }!!.sortedBy { it.name }

    /** The text from each `name(` to its matching `)`, in code without comments or strings. */
    private fun calls(src: String, name: String): List<String> {
        val out = ArrayList<String>()
        var from = 0
        while (true) {
            val at = Regex("\\b${Regex.escape(name)}\\(").find(src, from) ?: break
            var depth = 0
            var i = at.range.last
            while (i < src.length) {
                if (src[i] == '(') depth++
                if (src[i] == ')' && --depth == 0) break
                i++
            }
            if (depth != 0) { out += "UNBALANCED"; break }
            out += src.substring(at.range.first, i + 1)
            from = at.range.last + 1
        }
        return out
    }

    /** The body of `fun name(...) { ... }`, or null when there is no such function. */
    private fun functionBody(src: String, name: String): String? {
        val at = Regex("\\bfun\\s+${Regex.escape(name)}\\(").find(src) ?: return null
        var depth = 0
        var i = at.range.last
        while (i < src.length) {   // the parameter list
            if (src[i] == '(') depth++
            if (src[i] == ')' && --depth == 0) break
            i++
        }
        val open = src.indexOf('{', i)
        if (open < 0) return null
        depth = 0
        var j = open
        while (j < src.length) {
            if (src[j] == '{') depth++
            if (src[j] == '}' && --depth == 0) return src.substring(open, j + 1)
            j++
        }
        return null
    }

    private fun dialogsWithoutGuard(src: String) = calls(CodeScan.code(src), "AlertDialog").filter { guard !in it }

    @Test fun everyAlertDialogHasTheGuard() {
        var seen = 0
        for (f in uiFiles()) {
            val src = f.readText()
            seen += calls(CodeScan.code(src), "AlertDialog").size
            assertEquals("${f.name}: an AlertDialog without $guard", emptyList<String>(), dialogsWithoutGuard(src))
        }
        assertTrue("no AlertDialog found: the scanner is looking in the wrong place", seen >= 5)
    }

    @Test fun theGrantingScreensAndCardsHaveTheGuard() {
        val required = mapOf(
            "Screens.kt" to "ConsentScreen",
            "PermissionCard.kt" to "PermissionCard",
            "NotificationAccessScreen.kt" to "NotificationAccessScreen",
            "ProtectedAppsScreen.kt" to "ProtectedAppsScreen",
        )
        for ((file, fn) in required) {
            val src = CodeScan.code(File(uiDir, file).readText())
            val body = functionBody(src, fn)
            assertTrue("$file has no function $fn any more: update this list", body != null)
            assertTrue("$fn does not call $guard", guard in body!!)
        }
    }

    @Test fun onlyTouchGuardAssignsTheFlag() {
        for (f in File("src/main").walkTopDown().filter { it.isFile && (it.extension == "kt" || it.extension == "java") }) {
            if (f.name == "TouchGuard.kt") continue
            assertFalse("${f.name} sets filterTouchesWhenObscured itself", "filterTouchesWhenObscured" in CodeScan.code(f.readText()))
        }
    }

    @Test fun theCheckerRejectsBadExamples() {
        assertEquals(1, dialogsWithoutGuard("AlertDialog(onDismissRequest = {}, text = { Text(\"x\") })").size)
        assertEquals(1, dialogsWithoutGuard("AlertDialog(text = { Text(f(a)) }, title = { ObscuredTouchGuard } )").size)
        assertEquals(0, dialogsWithoutGuard("AlertDialog(text = { ObscuredTouchGuard(); Text(f(a)) })").size)
        // The guard in a comment or a string does not count.
        assertEquals(1, dialogsWithoutGuard("AlertDialog(text = { Text(\"ObscuredTouchGuard()\") /* ObscuredTouchGuard() */ })").size)
        // The guard outside the dialog's parentheses does not count.
        assertEquals(1, dialogsWithoutGuard("ObscuredTouchGuard()\nAlertDialog(text = { Text(\"x\") })").size)
        assertEquals(null, functionBody("fun Other() { ObscuredTouchGuard() }", "ConsentScreen"))
        assertFalse(guard in functionBody("fun A() { x() }\nfun B() { ObscuredTouchGuard() }", "A")!!)
    }
}
