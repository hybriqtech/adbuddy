package tech.hybriq.adbuddy.screens

import tech.hybriq.adbuddy.ui.ProtectedAppsUi
import tech.hybriq.adbuddy.ui.ProtectedAppsUi.Reason

/** The rows of the samples: a code receiver, a guess and an app nobody proposed (as in the previews). */
val SAMPLE_ROWS = listOf(
    ProtectedAppsUi.Row("com.google.android.gm", "Gmail", true, Reason.CODES),
    ProtectedAppsUi.Row("com.example.mybank", "Mi Banco", true, Reason.GUESS),
    ProtectedAppsUi.Row("com.whatsapp", "WhatsApp", false, Reason.NONE),
)
