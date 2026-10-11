package tech.hybriq.adbuddy.notify

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class NotificationPermissionTest {
    private fun ask(sdk: Int = 34, granted: Boolean = false, asked: Boolean = false, step: Boolean = false, question: Boolean = false) =
        NotificationPermission.shouldAsk(sdk, granted, asked, step, question)

    @Test fun asksWhenNothingGetsInTheWay() = assertTrue(ask())

    @Test fun neverBeforeAndroid13() = assertFalse(ask(sdk = 32))

    @Test fun neverWhenGranted() = assertFalse(ask(granted = true))

    @Test fun onlyOnce() = assertFalse(ask(asked = true))

    @Test fun neverOverAFirstRunStep() = assertFalse(ask(step = true))

    @Test fun neverOverAQuestion() = assertFalse(ask(question = true))
}
