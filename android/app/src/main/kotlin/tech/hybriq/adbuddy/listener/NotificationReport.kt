package tech.hybriq.adbuddy.listener

import java.time.Instant
import tech.hybriq.adbuddy.a11y.Packages
import tech.hybriq.adbuddy.bridge.Protocol
import tech.hybriq.adbuddy.bridge.UiLimits
import tech.hybriq.adbuddy.bridge.UiNotification
import tech.hybriq.adbuddy.bridge.UiNotifications

/**
 * A notification as the listener read it, before anything is hidden. Only
 * NotificationReport turns it into what leaves the phone: nothing else may
 * send a RawNotification's title or text anywhere.
 */
data class RawNotification(
    val key: String,
    val pkg: String,
    val title: String,
    val text: String,
    /** Epoch milliseconds, the phone's clock. */
    val postedAtMs: Long,
    val ongoing: Boolean,
    /** The app marked it VISIBILITY_SECRET: not even the lock screen shows it. */
    val secret: Boolean,
) {
    /** A data class would print key, title and text: nothing may log them. */
    override fun toString() =
        "RawNotification(pkg=$pkg, postedAtMs=$postedAtMs, ongoing=$ongoing, secret=$secret, key/title/text=<redacted>)"
}

/**
 * What `adbuddy notifications` gets (M5 spec §3.5). Pure, so it is unit-tested.
 * The content (key, title and text) of ADBuddy, of the protected apps, of a
 * notification an app marked secret and of anything whose package is not a
 * package name is left out: a 2FA code of a protected app never leaves the
 * phone through here. The key goes too, since it carries the app's own tag.
 */
object NotificationReport {
    /** guarded: ADBuddy or a protected app, asked now (the list can change between calls). */
    fun build(raw: List<RawNotification>, guarded: (String) -> Boolean): UiNotifications {
        val newest = raw.sortedWith(compareByDescending<RawNotification> { it.postedAtMs }.thenBy { it.key })
        val items = newest.take(UiLimits.MAX_NOTIFICATIONS).mapIndexed { i, n -> item(n, i, guarded) }
        // The whole answer must fit what the host accepts: drop the oldest until it does.
        // Each item is measured once (+1 for its comma); the envelope is measured with truncated = true.
        val envelope = encodedSize(UiNotifications(emptyList(), truncated = true))
        var total = envelope + items.sumOf { encodedSize(it) + 1 }
        var keep = items.size
        while (keep > 0 && total > UiLimits.MAX_UI_RESULT) {
            keep--
            total -= encodedSize(items[keep]) + 1
        }
        return UiNotifications(items.take(keep), truncated = newest.size > UiLimits.MAX_NOTIFICATIONS || keep < items.size)
    }

    private fun item(n: RawNotification, index: Int, guarded: (String) -> Boolean): UiNotification {
        val validPkg = UiLimits.validPackage(n.pkg)
        val hidden = !validPkg || n.pkg == Packages.ADBUDDY || n.secret || guarded(n.pkg)
        return UiNotification(
            key = if (hidden) "hidden-$index" else clip(n.key),
            `package` = if (validPkg) n.pkg else "",
            title = if (hidden) "" else clip(n.title),
            text = if (hidden) "" else clip(n.text),
            postedAt = Instant.ofEpochSecond(Math.floorDiv(n.postedAtMs, 1000L)).toString(),
            ongoing = n.ongoing,
        )
    }

    /** At most MAX_NOTIFICATION_TEXT code points, never cutting a surrogate pair. */
    fun clip(s: String): String {
        if (s.codePointCount(0, s.length) <= UiLimits.MAX_NOTIFICATION_TEXT) return s
        return s.substring(0, s.offsetByCodePoints(0, UiLimits.MAX_NOTIFICATION_TEXT))
    }

    private fun encodedSize(r: UiNotifications): Int =
        Protocol.json.encodeToString(UiNotifications.serializer(), r).toByteArray(Charsets.UTF_8).size

    private fun encodedSize(n: UiNotification): Int =
        Protocol.json.encodeToString(UiNotification.serializer(), n).toByteArray(Charsets.UTF_8).size
}
