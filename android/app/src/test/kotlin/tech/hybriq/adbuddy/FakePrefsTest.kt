package tech.hybriq.adbuddy

import android.content.SharedPreferences
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** FakePrefs must behave like SharedPreferences where the tests that use it depend on it. */
class FakePrefsTest {
    @Test fun editsOnlyTakeEffectOnApplyOrCommit() {
        val p = FakePrefs()
        p.edit().putBoolean("a", true)
        assertFalse(p.contains("a"))
        p.edit().putBoolean("a", true).apply()
        assertTrue(p.getBoolean("a", false))
    }

    @Test fun removeDeletesTheStoredValueWhenApplied() {
        val p = FakePrefs()
        p.edit().putBoolean("a", true).putString("b", "x").commit()
        val e = p.edit().remove("a")
        assertTrue("nothing changes before apply", p.contains("a"))
        e.commit()
        assertFalse(p.contains("a"))
        assertEquals("x", p.getString("b", null))
    }

    @Test fun clearEmptiesTheStoreWhenAppliedAndTheSameEditsPutsSurvive() {
        val p = FakePrefs()
        p.edit().putBoolean("a", true).putBoolean("b", true).commit()
        val e = p.edit().clear()
        assertTrue("nothing changes before apply", p.contains("a"))
        e.apply()
        assertTrue(p.all.isEmpty())
        // As the real one: clear() is applied first, whatever the order of the calls.
        p.edit().putBoolean("c", true).clear().putBoolean("d", true).commit()
        assertEquals(setOf("c", "d"), p.all.keys)
    }

    @Test fun aLaterPutWinsOverAnEarlierRemoveOfTheSameKey() {
        val p = FakePrefs()
        p.edit().putBoolean("a", false).commit()
        p.edit().remove("a").putBoolean("a", true).commit()
        assertTrue(p.getBoolean("a", false))
    }

    @Test fun listenersHearEveryChangedKeyAndStopWhenUnregistered() {
        val p = FakePrefs()
        val heard = mutableListOf<String?>()
        val l = SharedPreferences.OnSharedPreferenceChangeListener { _, key -> heard.add(key) }
        p.registerOnSharedPreferenceChangeListener(l)
        p.edit().putBoolean("a", true).putBoolean("b", true).commit()
        assertEquals(listOf<String?>("a", "b"), heard)
        p.unregisterOnSharedPreferenceChangeListener(l)
        p.edit().putBoolean("c", true).commit()
        assertEquals(2, heard.size)
        assertEquals(0, p.listenerCount)
    }
}
