package tech.hybriq.adbuddy.a11y

/** A notification just posted may be on screen as a heads-up over any app, with a 2FA code in it: for a few seconds the helpers treat the screen as protected (M5b plan, ruling 7). */
class NotificationWindow(private val clock: () -> Long, private val windowMs: Long = 8_000) {
    @Volatile private var until = 0L

    fun onNotification(pkg: String) {
        if (pkg.isEmpty() || pkg == ADBUDDY) return
        until = clock() + windowMs
    }

    fun active(): Boolean = clock() < until

    /**
     * The logic of the service's TYPE_NOTIFICATION_STATE_CHANGED handling: toasts
     * fire the same event, and so does anything without a Notification in its
     * parcelableData; neither opens the window. True when the event counted.
     */
    fun accept(className: CharSequence?, pkg: String, isNotification: Boolean): Boolean {
        if (!isNotification || className?.contains("Toast") == true) return false
        onNotification(pkg)
        return pkg.isNotEmpty() && pkg != ADBUDDY
    }

    private companion object { const val ADBUDDY = "tech.hybriq.adbuddy" }
}
