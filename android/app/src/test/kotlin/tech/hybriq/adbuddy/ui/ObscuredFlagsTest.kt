package tech.hybriq.adbuddy.ui

import android.view.MotionEvent.FLAG_WINDOW_IS_OBSCURED
import android.view.MotionEvent.FLAG_WINDOW_IS_PARTIALLY_OBSCURED
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** The decision behind refuseObscuredTouches: either covering flag refuses the touch, nothing else does. */
class ObscuredFlagsTest {
    @Test fun eitherCoveringFlagIsObscured() {
        assertTrue("wholly covered", isObscured(FLAG_WINDOW_IS_OBSCURED))
        assertTrue("partly covered", isObscured(FLAG_WINDOW_IS_PARTIALLY_OBSCURED))
        assertTrue("both", isObscured(FLAG_WINDOW_IS_OBSCURED or FLAG_WINDOW_IS_PARTIALLY_OBSCURED))
    }

    @Test fun theFlagsAmongOtherBitsAreStillSeen() {
        assertTrue(isObscured(FLAG_WINDOW_IS_OBSCURED or 0x100 or Int.MIN_VALUE))
        assertTrue(isObscured(FLAG_WINDOW_IS_PARTIALLY_OBSCURED or 0x40))
        assertTrue(isObscured(-1))
    }

    @Test fun noCoveringFlagIsNotObscured() {
        assertFalse(isObscured(0))
        // Every other bit set, the two covering flags clear.
        assertFalse(isObscured((FLAG_WINDOW_IS_OBSCURED or FLAG_WINDOW_IS_PARTIALLY_OBSCURED).inv()))
        assertFalse(isObscured(0x4))
        assertFalse(isObscured(0x10000))
    }
}
