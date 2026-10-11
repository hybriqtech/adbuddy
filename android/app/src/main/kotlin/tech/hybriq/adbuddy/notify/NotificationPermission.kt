package tech.hybriq.adbuddy.notify

/** When the app asks for the notification permission: once, from Android 13, and never over something else. */
object NotificationPermission {
    const val FIRST_SDK_WITH_PERMISSION = 33

    fun shouldAsk(sdk: Int, granted: Boolean, alreadyAsked: Boolean, stepShown: Boolean, questionPending: Boolean): Boolean =
        sdk >= FIRST_SDK_WITH_PERMISSION && !granted && !alreadyAsked && !stepShown && !questionPending
}
