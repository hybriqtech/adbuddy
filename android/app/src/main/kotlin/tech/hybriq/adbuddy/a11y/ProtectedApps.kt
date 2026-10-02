package tech.hybriq.adbuddy.a11y

import android.content.Context
import android.content.Intent
import android.content.SharedPreferences
import android.provider.Telephony
import androidx.core.content.edit
import tech.hybriq.adbuddy.bridge.DeviceInfo
import tech.hybriq.adbuddy.bridge.UiLimits

/**
 * The protected apps (M5 spec §4.3): banking, payments, passwords and 2FA.
 * The helpers never read nor touch them, and the host's guard refuses adb
 * input and screenshots while one is in front. ADBuddy itself always is.
 *
 * Until the user confirms a list (the protected apps screen, M5c), it is
 * proposed: the apps in the launcher whose package or label looks like one,
 * and the apps that receive the login codes of all the others (Messages,
 * Gmail, the default SMS app). The proposal protects from the start, before
 * the person has seen the screen. From the confirmation on it never touches
 * the list again.
 */
class ProtectedApps(private val prefs: SharedPreferences) {
    /** The list, without ADBuddy (the host adds it itself). */
    fun list(): Set<String> = prefs.getStringSet(KEY_LIST, null).orEmpty()

    fun isProtected(pkg: String): Boolean = pkg == Packages.ADBUDDY || pkg in list()

    /** Whether the user confirmed the list: then proposals no longer change it. */
    val confirmed: Boolean get() = prefs.getBoolean(KEY_CONFIRMED, false)

    /**
     * Sets the proposal, unless the user confirmed a list: the heuristic's apps plus receivers, the apps
     * that receive login codes (Heuristic.receivers), when they are installed. Returns whether it changed.
     * Under the lock, with confirm(): a confirmation that lands meanwhile is never overwritten.
     */
    fun propose(apps: List<Pair<String, String>>, receivers: Set<String> = emptySet()): Boolean = synchronized(this) {
        if (confirmed) return@synchronized false
        val proposed = Heuristic.proposeWithReceivers(apps, receivers)
        if (proposed == list() && prefs.contains(KEY_LIST)) return@synchronized false
        prefs.edit { putStringSet(KEY_LIST, proposed) }
        true
    }

    /**
     * The user's own list (M5c), even if it is the proposal unchanged or empty:
     * confirming is what stops the proposals. ADBuddy is never in it (always protected).
     */
    fun confirm(list: Set<String>) = synchronized(this) {
        val clean = list.filter { it != Packages.ADBUDDY && UiLimits.validPackage(it) }.take(UiLimits.MAX_PROTECTED_APPS).toSet()
        prefs.edit {
            putStringSet(KEY_LIST, clean)
            putBoolean(KEY_CONFIRMED, true)
        }
    }

    /** The whole list for the host's device.info: present even when empty (absent would mean "no change"). */
    fun deviceInfo(): DeviceInfo = DeviceInfo(protectedApps = list().sorted())

    /** Guesses by name which apps to protect. */
    object Heuristic {
        /**
         * Apps that receive the login codes of every other service, by SMS, mail or sign-in prompt: Google
         * Messages, Samsung Messages, AOSP Messaging, Gmail and Google Play services (sign-in prompts and
         * verification codes). Proposed, never forced: the user edits the list.
         */
        val CODE_RECEIVERS: Set<String> = setOf(
            "com.google.android.apps.messaging",
            "com.samsung.android.messaging",
            "com.android.mms",
            "com.google.android.gm",
            "com.google.android.gms",
        )

        /** Receivers that have no icon in the launcher, so "installed" cannot be told from the launcher's list. */
        private val NO_LAUNCHER_ICON: Set<String> = setOf("com.google.android.gms")

        /** CODE_RECEIVERS and the phone's default SMS app (null: unknown). */
        fun receivers(defaultSms: String?): Set<String> = CODE_RECEIVERS + listOfNotNull(defaultSms?.takeIf(UiLimits::validPackage))

        /** propose(apps) plus the receivers that are installed (in the launcher, or with no icon of their own). */
        fun proposeWithReceivers(apps: List<Pair<String, String>>, receivers: Set<String>): Set<String> {
            val here = apps.map { it.first }.toSet()
            // The receivers go first: if the heuristic fills the list, it is the heuristic that yields at the limit,
            // never the apps that receive the codes.
            val installed = receivers.filter { it in here || it in NO_LAUNCHER_ICON }.filter(UiLimits::validPackage)
            return (installed + propose(apps)).filter { it != Packages.ADBUDDY }.take(UiLimits.MAX_PROTECTED_APPS).toSet()
        }

        /** Found anywhere in a package part or a label word ("kutxabank", "mybank"). */
        private val anywhere = listOf("bank", "banco", "banca", "caja", "kutxa", "wallet", "crypto", "password", "contrase")

        /** Words that start a package part or a label word of a protected app. */
        private val starts = listOf(
            "pay", "auth", "passwort", "bitwarden", "1password", "keepass", "lastpass", "dashlane", "2fa", "otp", "bizum",
            "bbva", "caixa", "santander", "sabadell", "revolut", "n26", "openbank", "binance", "coinbase", "kraken", "paypal",
            "abanca", "ibercaja", "unicaja", "cajamar", "imagin", "bunq", "myinvestor", "traderepublic",
        )

        /** Whole words: too short or too common to look for inside others. */
        private val words = setOf("ing", "wise")

        fun propose(apps: List<Pair<String, String>>): Set<String> = apps
            .filter { (pkg, label) -> pkg != Packages.ADBUDDY && UiLimits.validPackage(pkg) && looksProtected(pkg, label) }
            .map { it.first }
            .take(UiLimits.MAX_PROTECTED_APPS)
            .toSet()

        fun looksProtected(pkg: String, label: String): Boolean {
            val words = (pkg.split('.', '_') + label.split(Regex("[^\\p{L}\\p{N}]+"))).map { it.lowercase() }.filter { it.isNotEmpty() }
            return words.any { w -> w in Heuristic.words || starts.any { w.startsWith(it) } || anywhere.any { it in w } }
        }
    }

    companion object {
        private const val PREFS = "protected_apps"
        private const val KEY_LIST = "list"
        private const val KEY_CONFIRMED = "confirmed"

        fun of(context: Context) = ProtectedApps(context.getSharedPreferences(PREFS, Context.MODE_PRIVATE))

        /** The phone's default SMS app, or null when it cannot be told. */
        fun defaultSms(context: Context): String? = runCatching { Telephony.Sms.getDefaultSmsPackage(context) }.getOrNull()

        /** The apps with an icon in the launcher, as package and label. */
        fun launcherApps(context: Context): List<Pair<String, String>> {
            val pm = context.packageManager
            val intent = Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER)
            return pm.queryIntentActivities(intent, 0).map { it.activityInfo.packageName to it.loadLabel(pm).toString() }.distinctBy { it.first }
        }
    }
}
