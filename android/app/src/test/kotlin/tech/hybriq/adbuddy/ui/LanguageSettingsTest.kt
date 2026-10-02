package tech.hybriq.adbuddy.ui

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class LanguageSettingsTest {
    @Test fun theAppsOwnLanguageExistsFromAndroid13() {
        assertFalse(LanguageSettings.available(30))
        assertFalse(LanguageSettings.available(32))
        assertTrue(LanguageSettings.available(33))
        assertTrue(LanguageSettings.available(37))
    }
}
