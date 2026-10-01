package tech.hybriq.adbuddy.a11y

/** A notification just posted may be on screen as a heads-up over any app, with a 2FA code in it: for a few seconds the helpers treat the screen as protected (M5b plan, ruling 7). */
class NotificationWindow(private val clock: () -> Long, private val windowMs: Long = 8_000) {
    @Volatile private var until = 0L

    /** True when the notification opened (or restarted) the window. */
    fun onNotification(pkg: String): Boolean {
        if (pkg.isEmpty() || pkg == ADBUDDY) return false
        until = clock() + windowMs
        return true
    }

    fun active(): Boolean = clock() < until

    /**
     * The logic of the service's TYPE_NOTIFICATION_STATE_CHANGED handling: a toast
     * fires the same event and does not open the window. Nothing else is
     * required of the event (a real notification may come without its
     * parcelableData, e.g. redacted). True when the event counted.
     */
    fun accept(className: CharSequence?, pkg: String): Boolean {
        if (className?.contains("Toast") == true) return false
        return onNotification(pkg)
    }

    private companion object { const val ADBUDDY = "tech.hybriq.adbuddy" }
}
