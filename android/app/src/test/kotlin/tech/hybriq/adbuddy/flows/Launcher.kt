package tech.hybriq.adbuddy.flows

import android.content.Intent
import android.content.pm.ActivityInfo
import android.content.pm.ApplicationInfo
import android.content.pm.ResolveInfo
import org.robolectric.RuntimeEnvironment
import org.robolectric.Shadows.shadowOf

/** Apps with an icon in the launcher, as ProtectedApps.launcherApps reads them (package to label). */
fun installLauncherApps(vararg apps: Pair<String, String>) {
    val app = RuntimeEnvironment.getApplication()
    val launcher = Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER)
    for ((pkg, label) in apps) {
        shadowOf(app.packageManager).addResolveInfoForIntent(
            launcher,
            ResolveInfo().apply {
                nonLocalizedLabel = label
                activityInfo = ActivityInfo().apply {
                    packageName = pkg
                    name = "$pkg.Main"
                    applicationInfo = ApplicationInfo().apply { packageName = pkg }
                }
            },
        )
    }
}
