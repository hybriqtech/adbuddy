package tech.hybriq.adbuddy.a11y

/** A screen like Settings, with a dialog of ADBuddy's on top, for the helpers' tests. */
object ScreenFixtures {
    val SCREEN = Box(0, 0, 1344, 2992)

    fun row(text: String, top: Int) = UiNode(
        pkg = "com.android.settings", cls = "android.widget.LinearLayout", bounds = Box(0, top, 1344, top + 90), clickable = true,
        children = listOf(UiNode(pkg = "com.android.settings", cls = "android.widget.TextView", id = "android:id/title", text = text, bounds = Box(42, top + 10, 980, top + 80))),
    )

    val settings = UiNode(
        pkg = "com.android.settings", cls = "android.widget.FrameLayout", bounds = SCREEN,
        children = listOf(
            UiNode(
                pkg = "com.android.settings", cls = "android.widget.Toolbar", id = "com.android.settings:id/search_action_bar",
                desc = "Search settings", bounds = Box(42, 180, 1302, 300), clickable = true,
            ),
            UiNode(
                pkg = "com.android.settings", cls = "androidx.recyclerview.widget.RecyclerView", bounds = Box(0, 320, 1344, 2900), scrollable = true,
                children = listOf(
                    row("Network & internet", 600),
                    row("OK", 700),
                    row("ok", 800),
                    // Half under the toolbar: clipped by the list.
                    row("Half", 280),
                    UiNode(
                        pkg = "com.android.settings", cls = "android.widget.Switch", desc = "Wi-Fi", bounds = Box(980, 1000, 1302, 1080),
                        clickable = true, checkable = true, checked = true,
                    ),
                    UiNode(pkg = "com.android.settings", cls = "android.widget.EditText", id = "com.android.settings:id/pin", text = "hunter2", hint = "PIN", bounds = Box(42, 1100, 1302, 1200), editable = true, focused = true, password = true, clickable = true),
                    UiNode(pkg = "com.android.settings", cls = "android.widget.TextView", text = "Hidden", bounds = Box(0, 1300, 100, 1400), visible = false),
                    UiNode(pkg = "com.android.settings", cls = "android.widget.FrameLayout", bounds = Box(0, 1500, 1344, 1600)),
                ),
            ),
        ),
    )

    val adbuddy = UiNode(
        pkg = Packages.ADBUDDY, cls = "android.widget.FrameLayout", bounds = Box(0, 0, 1344, 120),
        children = listOf(UiNode(pkg = Packages.ADBUDDY, cls = "android.widget.Button", text = "Allow", bounds = Box(900, 20, 1300, 100), clickable = true)),
    )

    val notification = UiNode(
        pkg = Packages.SYSTEM_UI, cls = "android.widget.FrameLayout", bounds = Box(0, 0, 1344, 400),
        children = listOf(UiNode(pkg = Packages.SYSTEM_UI, cls = "android.widget.Button", text = "Allow it", bounds = Box(900, 300, 1300, 380), clickable = true)),
    )

    val windows = listOf(
        UiWindow("app", 1, SCREEN, settings),
        UiWindow("system", 5, Box(0, 0, 1344, 120), adbuddy),
    )
}
