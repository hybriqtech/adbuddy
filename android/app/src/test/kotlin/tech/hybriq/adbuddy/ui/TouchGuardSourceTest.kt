package tech.hybriq.adbuddy.ui

import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.listener.CodeScan

/**
 * Tapjacking (O1): every surface that grants something, and every dialog, menu and popup, filters touches while another app's
 * window covers it. A dialog is its own window, so the screen's flag does not reach it: each of them carries
 * the guard itself. A source check, since nothing on the JVM can draw an overlay.
 */
class TouchGuardSourceTest {
    private val uiDir = File("src/main/kotlin/tech/hybriq/adbuddy/ui")
    private val guard = "ObscuredTouchGuard()"

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
            // A trailing lambda (DropdownMenu { ... }, Dialog { ... }) is part of the call too.
            var end = i + 1
            var k = end
            while (k < src.length && src[k].isWhitespace()) k++
            if (k < src.length && src[k] == '{') {
                var braces = 0
                var m = k
                while (m < src.length) {
                    if (src[m] == '{') braces++
                    if (src[m] == '}' && --braces == 0) break
                    m++
                }
                if (braces == 0) end = m + 1
            }
            out += src.substring(at.range.first, end)
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

    /** Every container that opens its own window (a dialog, a menu, a popup, a sheet): the screen's flag does not reach it. */
    private val windows = listOf("AlertDialog", "BasicAlertDialog", "Dialog", "ModalBottomSheet", "DropdownMenu", "Popup")

    private fun windowsOf(src: String) = windows.flatMap { calls(src, it) }

    private fun windowsWithoutGuard(src: String) = windowsOf(CodeScan.code(src)).filter { guard !in it }

    /** Imports that rename one of those symbols would hide its calls from the scan. */
    private val aliasOfWindow = Regex("""\bimport\s+[\w.]*\.(${windows.joinToString("|")})\s+as\b""")

    private fun aliasesOfWindows(src: String) = aliasOfWindow.findAll(src).map { it.value }.toList()

    private fun mainSources() = File("src/main/kotlin").walkTopDown().filter { it.isFile && it.extension == "kt" }.sortedBy { it.path }.toList()

    @Test fun everyWindowContainerHasTheGuard() {
        var seen = 0
        for (f in mainSources()) {
            val src = f.readText()
            seen += windowsOf(CodeScan.code(src)).size
            assertEquals("${f.path}: a dialog, menu or popup without $guard", emptyList<String>(), windowsWithoutGuard(src))
        }
        assertTrue("no window container found: the scanner is looking in the wrong place", seen >= 7)
    }

    @Test fun noWindowContainerIsImportedUnderAnAlias() {
        for (f in mainSources()) assertEquals("${f.path} renames a window container", emptyList<String>(), aliasesOfWindows(f.readText()))
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
        assertEquals(1, windowsWithoutGuard("AlertDialog(onDismissRequest = {}, text = { Text(\"x\") })").size)
        assertEquals(1, windowsWithoutGuard("AlertDialog(text = { Text(f(a)) }, title = { ObscuredTouchGuard } )").size)
        assertEquals(0, windowsWithoutGuard("AlertDialog(text = { ObscuredTouchGuard(); Text(f(a)) })").size)
        assertEquals(1, windowsWithoutGuard("Dialog(onDismissRequest = {}) { Text(\"x\") }").size)
        assertEquals(1, windowsWithoutGuard("Popup(alignment = A) { Text(\"x\") }").size)
        assertEquals(1, windowsWithoutGuard("DropdownMenu(expanded = e, onDismissRequest = {}) { DropdownMenuItem(onClick = {}) }").size)
        assertEquals(1, windowsWithoutGuard("BasicAlertDialog(onDismissRequest = {}) { Text(\"x\") }").size)
        assertEquals(1, windowsWithoutGuard("ModalBottomSheet(onDismissRequest = {}) { Text(\"x\") }").size)
        assertEquals(0, windowsWithoutGuard("DropdownMenu(expanded = e, onDismissRequest = {}) { ObscuredTouchGuard(); Item() }").size)
        assertEquals(0, windowsWithoutGuard("Dialog(onDismissRequest = {}) { ObscuredTouchGuard(); Text(\"x\") }").size)
        assertEquals(1, windowsWithoutGuard("Popup(alignment = A) { Text(\"x\") } ObscuredTouchGuard()").size)
        // The guard in a comment or a string does not count.
        assertEquals(1, windowsWithoutGuard("AlertDialog(text = { Text(\"ObscuredTouchGuard()\") /* ObscuredTouchGuard() */ })").size)
        // The guard outside the container's parentheses does not count.
        assertEquals(1, windowsWithoutGuard("ObscuredTouchGuard() AlertDialog(text = { Text(\"x\") })").size)
        // Not a container: the menu item and a name that only ends the same way.
        assertEquals(0, windowsWithoutGuard("DropdownMenuItem(onClick = {}); MyDialog(x)").size)
        assertEquals(1, aliasesOfWindows("import androidx.compose.ui.window.Dialog as D").size)
        assertEquals(1, aliasesOfWindows("import androidx.compose.material3.DropdownMenu as Menu").size)
        assertEquals(0, aliasesOfWindows("import android.graphics.Color as AColor; import androidx.compose.ui.window.Dialog").size)
        assertEquals(null, functionBody("fun Other() { ObscuredTouchGuard() }", "ConsentScreen"))
        assertFalse(guard in functionBody("fun A() { x() } fun B() { ObscuredTouchGuard() }", "A")!!)
    }
}
