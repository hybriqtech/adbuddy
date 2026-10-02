package tech.hybriq.adbuddy.ui

import tech.hybriq.adbuddy.a11y.Packages
import tech.hybriq.adbuddy.a11y.ProtectedApps
import tech.hybriq.adbuddy.bridge.UiLimits

/**
 * What the protected apps screen shows and does (M5c). Pure, so it is
 * unit-tested; the screen only draws it. The list is the user's: this proposes
 * (the heuristic of M5a, and the apps that receive login codes), never decides.
 */
object ProtectedAppsUi {
    /** Why an app is proposed; it does not depend on whether it is checked now, so rows keep their place. */
    enum class Reason { CODES, GUESS, NONE }

    /** knownName: a readable name for an app that has no launcher label (label is its package name); the screen may show it beside it. */
    data class Row(val pkg: String, val label: String, val checked: Boolean, val reason: Reason, val knownName: String? = null)

    private val KNOWN_NAMES = mapOf("com.google.android.gms" to "Google Play services")

    fun reason(pkg: String, label: String, receivers: Set<String>): Reason = when {
        pkg in receivers -> Reason.CODES
        ProtectedApps.Heuristic.looksProtected(pkg, label) -> Reason.GUESS
        else -> Reason.NONE
    }

    /**
     * What is checked when the screen opens. A confirmed list is the user's, as it is. Otherwise it is
     * the current proposal plus what would be proposed now (ProtectedApps.propose: the heuristic and the
     * code receivers that are installed), which is what is already protected. ADBuddy never appears: it is
     * always protected and cannot be unchecked.
     */
    fun initialSelection(installed: List<Pair<String, String>>, current: Set<String>, confirmed: Boolean, receivers: Set<String>): Set<String> {
        val base = current - Packages.ADBUDDY
        if (confirmed) return base
        return base + ProtectedApps.Heuristic.proposeWithReceivers(installed, receivers)
    }

    /**
     * The rows: every app of the launcher (minus ADBuddy) and the apps that were saved but have no
     * launcher icon (shown by package name, so they can be unchecked), filtered by query, the proposed
     * ones first and then by name. initial is what was checked when the screen opened.
     */
    fun rows(installed: List<Pair<String, String>>, initial: Set<String>, selection: Set<String>, receivers: Set<String>, query: String): List<Row> {
        val known = installed.filter { it.first != Packages.ADBUDDY && UiLimits.validPackage(it.first) }.distinctBy { it.first }
        // The saved apps without a launcher icon, and Play services even when unchecked and saved: otherwise
        // there would be no row left to protect it again.
        val extra = (initial + (receivers intersect ProtectedApps.Heuristic.NO_LAUNCHER_ICON) - Packages.ADBUDDY - known.map { it.first }.toSet())
            .sorted().map { it to it }
        val q = query.trim().lowercase()
        return (known + extra)
            .map { (pkg, label) -> Row(pkg, label, pkg in selection, reason(pkg, label, receivers), KNOWN_NAMES[pkg].takeIf { pkg == label }) }
            .filter { q.isEmpty() || it.label.lowercase().contains(q) || it.pkg.lowercase().contains(q) }
            .sortedWith(compareBy<Row>({ it.reason == Reason.NONE }, { it.label.lowercase() }, { it.pkg }))
    }

    /** Checks or unchecks one app. ADBuddy and invalid names are ignored; the list stops at the protocol's limit. */
    fun toggle(selection: Set<String>, pkg: String, on: Boolean): Set<String> = when {
        pkg == Packages.ADBUDDY || !UiLimits.validPackage(pkg) -> selection
        !on -> selection - pkg
        selection.size >= UiLimits.MAX_PROTECTED_APPS -> selection
        else -> selection + pkg
    }

    /** What is saved: valid packages, without ADBuddy, at most the protocol's limit. */
    fun result(selection: Set<String>): Set<String> =
        selection.filter { it != Packages.ADBUDDY && UiLimits.validPackage(it) }.take(UiLimits.MAX_PROTECTED_APPS).toSet()

    /** What the screen reads before it can be used. */
    class Loaded(val installed: List<Pair<String, String>>, val saved: Set<String>, val confirmed: Boolean, val receivers: Set<String>)

    /**
     * Loading, failed or ready. A failure is never an empty list: confirming over an incomplete view of
     * what is installed would drop protected apps without the user knowing.
     */
    sealed interface LoadState {
        data object Loading : LoadState
        data object Error : LoadState
        class Ready(val data: Loaded) : LoadState
    }

    /** Runs the (blocking) read; a RuntimeException from the system becomes Error, cancellation and real Errors pass through. */
    fun load(read: () -> Loaded): LoadState = try {
        LoadState.Ready(read())
    } catch (e: kotlin.coroutines.cancellation.CancellationException) {
        throw e
    } catch (e: RuntimeException) {
        LoadState.Error
    }

    /** Whether the save button is on: the apps are loaded, and there is something to confirm or save. */
    fun canSave(loaded: Boolean, onboarding: Boolean, dirty: Boolean, confirmed: Boolean): Boolean =
        loaded && (onboarding || dirty || !confirmed)

    /** Whether the selection differs from the saved list. */
    fun dirty(selection: Set<String>, saved: Set<String>): Boolean = result(selection) != result(saved)
}
