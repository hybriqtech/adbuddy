// AGP 9 compiles Kotlin by itself (built-in Kotlin): no org.jetbrains.kotlin.android plugin.
plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

android {
    namespace = "tech.hybriq.adbuddy"
    compileSdk = 37

    defaultConfig {
        applicationId = "tech.hybriq.adbuddy"
        minSdk = 30
        targetSdk = 37
        // Releases pass both from the tag (scripts/release-version.sh, spec M6a §3).
        versionCode = providers.gradleProperty("adbuddy.versionCode").map(String::toInt).getOrElse(1)
        versionName = providers.gradleProperty("adbuddy.versionName").getOrElse("0.2.0")
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        compose = true
    }

    // Compose and Material bring strings in dozens of languages the app does not offer (it has two: spec v1 section 6).
    androidResources {
        localeFilters += listOf("en", "es")
    }

    // The i18n controls of the platform, as a second net behind StringsParityTest and NoHardcodedTextTest.
    // `error +=` raises them to errors and leaves every other check as it was (checkOnly would switch the rest off).
    lint {
        error += listOf("MissingTranslation", "ExtraTranslation", "StringFormatCount", "StringFormatMatches", "StringFormatInvalid")
    }
}

// The protocol's shared fixtures (protocol/fixtures/, protocol spec §12): FixturesTest reads them.
val protocolFixtures = rootProject.file("../protocol/fixtures")
tasks.withType<Test>().configureEach {
    inputs.dir(protocolFixtures).withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("adbuddy.protocolFixtures", protocolFixtures.absolutePath)
    // M6c: the i18n controls (src/test/kotlin/tech/hybriq/adbuddy/i18n) read the app's own resources and sources.
    inputs.dir("src/main/res").withPathSensitivity(PathSensitivity.RELATIVE)
    inputs.dir("src/main/kotlin").withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("adbuddy.resDir", file("src/main/res").absolutePath)
    systemProperty("adbuddy.kotlinDir", file("src/main/kotlin").absolutePath)
}

// The instrumented tests run only on an emulator, never on the user's phone (M2.5 spec §2.4): ANDROID_SERIAL
// must name exactly one emulator (AGP splits it on commas, so "emulator-5554,<phone>" is refused too). Checked
// when a connected task starts, before it installs anything.
tasks.configureEach {
    if (name.startsWith("connected")) {
        doFirst {
            val serial = System.getenv("ANDROID_SERIAL").orEmpty()
            check(Regex("emulator-[0-9]+").matches(serial)) {
                "connected tests run only on one emulator: set ANDROID_SERIAL=emulator-<port> " +
                    "(scripts/emulator.sh start prints it as ADBUDDY_EMULATOR_SERIAL), not \"$serial\""
            }
        }
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.service)
    implementation(libs.androidx.webkit)
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.material3)
    implementation(libs.compose.material.icons.core)
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)

    testImplementation(libs.junit)
    testImplementation(libs.okhttp.mockwebserver)
    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.androidx.test.ext.junit)
}
