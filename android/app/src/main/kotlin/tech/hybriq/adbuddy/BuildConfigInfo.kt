package tech.hybriq.adbuddy

import android.content.Context

/** The app's version, read from the package (AGP 9 does not generate BuildConfig by default). */
object BuildConfigInfo {
    fun versionName(context: Context): String =
        context.packageManager.getPackageInfo(context.packageName, 0).versionName ?: ""
}
