package tech.hybriq.adbuddy.a11y

import android.content.Context
import android.content.Intent
import android.content.SharedPreferences
import tech.hybriq.adbuddy.bridge.UiLimits

/**
 * The protected apps (M5 spec §4.3): banking, payments, passwords and 2FA.
 * The helpers never read nor touch them, and the host's guard refuses adb
 * input and screenshots while one is in front. ADBuddy itself always is.
 *
 * Until the user confirms a list (the screens of M5c), it is proposed by
 * name: the apps in the launcher whose package or label looks like one.
 */
class ProtectedApps(private val prefs: SharedPreferences) {
    /** The list, without ADBuddy (the host adds it itself). */
    fun list(): Set<String> = prefs.getStringSet(KEY_LIST, null).orEmpty()

    fun isProtected(pkg: String): Boolean = pkg == Packages.ADBUDDY || pkg in list()

    /** Whether the user confirmed the list: then proposals no longer change it. */
    val confirmed: Boolean get() = prefs.getBoolean(KEY_CONFIRMED, false)

    /** Sets the proposal, unless the user confirmed a list. Returns whether it changed. */
    fun propose(apps: List<Pair<String, String>>): Boolean {
        if (confirmed) return false
        val proposed = Heuristic.propose(apps)
        if (proposed == list() && prefs.contains(KEY_LIST)) return false
        prefs.edit().putStringSet(KEY_LIST, proposed).apply()
        return true
    }

    /** The user's own list (M5c). */
    fun confirm(list: Set<String>) {
        prefs.edit().putStringSet(KEY_LIST, list.filter(UiLimits::validPackage).take(UiLimits.MAX_PROTECTED_APPS).toSet())
            .putBoolean(KEY_CONFIRMED, true).apply()
    }

    /** Guesses by name which apps to protect. */
    object Heuristic {
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

        /** The apps with an icon in the launcher, as package and label. */
        fun launcherApps(context: Context): List<Pair<String, String>> {
            val pm = context.packageManager
            val intent = Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER)
            return pm.queryIntentActivities(intent, 0).map { it.activityInfo.packageName to it.loadLabel(pm).toString() }.distinctBy { it.first }
        }
    }
}
