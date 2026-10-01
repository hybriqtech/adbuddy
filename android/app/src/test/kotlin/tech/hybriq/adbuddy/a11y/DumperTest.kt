package tech.hybriq.adbuddy.a11y

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import tech.hybriq.adbuddy.a11y.ScreenFixtures.SCREEN
import tech.hybriq.adbuddy.a11y.ScreenFixtures.windows
import tech.hybriq.adbuddy.bridge.UiLimits

class DumperTest {
    private fun dump(all: Boolean = false, locked: Boolean = false) =
        Dumper.dump(windows, SCREEN, 0, "com.android.settings", "com.android.settings.Settings", locked, all, { false })

    @Test fun showsTheUsableAndInformativeNodes() {
        val d = dump()
        assertEquals("com.android.settings", d.`package`)
        assertEquals(1344, d.width)
        assertEquals(2992, d.height)
        assertFalse(d.truncated)
        val texts = d.elements.map { "${it.`class`}:${it.text}:${it.desc}" }
        assertEquals(
            listOf(
                "Toolbar::Search settings", "RecyclerView::",
                "LinearLayout::", "TextView:Network & internet:", "LinearLayout::", "TextView:OK:", "LinearLayout::", "TextView:ok:",
                "LinearLayout::", "TextView:Half:", "Switch::Wi-Fi", "EditText::",
            ),
            texts,
        )
    }

    @Test fun neverShowsADBuddyNorInvisibleNodes() {
        val d = dump(all = true)
        assertTrue(d.elements.none { it.text == "Allow" || it.text == "Hidden" })
        // all keeps the empty FrameLayouts (the root and one in the list).
        assertEquals(dump().elements.size + 2, d.elements.size)
    }

    @Test fun clipsToTheScrollableParent() {
        val half = dump().elements.first { it.text == "Half" }
        assertEquals(listOf(42, 320, 980, 360), half.bounds)
    }

    @Test fun hidesPasswords() {
        val pin = dump().elements.first { it.`class` == "EditText" }
        assertTrue(pin.password)
        assertEquals("", pin.text)
        assertEquals("", pin.hint)
        assertTrue(pin.edit && pin.focused)
    }

    @Test fun aPasswordFieldKeepsItsDescription() {
        // The text and hint of a password field go out empty; its desc is the field's name, never its content.
        val field = UiNode(pkg = "p.q", cls = "a.EditText", text = "hunter2", hint = "PIN", desc = "PIN field", bounds = Box(0, 0, 100, 50), editable = true, password = true)
        val w = listOf(UiWindow("app", 1, SCREEN, UiNode(pkg = "p.q", cls = "a.F", bounds = SCREEN, children = listOf(field))))
        val e = Dumper.dump(w, SCREEN, 0, "p.q", "", locked = false, all = false, hidden = { false }).elements.single()
        assertEquals("", e.text)
        assertEquals("", e.hint)
        assertEquals("PIN field", e.desc)
    }

    @Test fun lockedDumpGivesNoNodesNorTheProtectedApp() {
        val d = Dumper.dump(windows, SCREEN, 0, "", "", locked = true, all = true, hidden = { it == "com.android.settings" })
        assertEquals("", d.`package`)
        assertTrue(d.elements.isEmpty())
    }

    @Test fun checkedOnlyWhenCheckable() {
        val d = dump()
        assertEquals(true, d.elements.first { it.desc == "Wi-Fi" }.checked)
        assertNull(d.elements.first { it.text == "OK" }.checked)
    }

    @Test fun lockedShowsNothing() {
        val d = dump(locked = true)
        assertTrue(d.locked)
        assertTrue(d.elements.isEmpty())
    }

    @Test fun cutsWhatIsTooBig() {
        val long = "x".repeat(3000)
        val many = (0 until 1500).map { UiNode(pkg = "p.q", cls = "a.T", text = long, bounds = Box(0, it, 10, it + 1)) }
        val w = listOf(UiWindow("app", 1, SCREEN, UiNode(pkg = "p.q", cls = "a.F", bounds = SCREEN, children = many)))
        val d = Dumper.dump(w, SCREEN, 0, "p.q", "", locked = false, all = false, hidden = { false })
        assertTrue(d.truncated)
        assertTrue(Dumper.size(d) <= UiLimits.MAX_UI_RESULT)
        assertTrue(d.elements.all { it.text.length <= 201 })
    }

    @Test fun capsTheElements() {
        val many = (0 until Dumper.MAX_ELEMENTS + 10).map { UiNode(pkg = "p.q", cls = "a.T", text = "x", bounds = Box(0, 0, 10, 10)) }
        val w = listOf(UiWindow("app", 1, SCREEN, UiNode(pkg = "p.q", cls = "a.F", bounds = SCREEN, children = many)))
        val d = Dumper.dump(w, SCREEN, 0, "p.q", "", locked = false, all = false, hidden = { false })
        assertEquals(Dumper.MAX_ELEMENTS, d.elements.size)
        assertTrue(d.truncated)
    }
}
