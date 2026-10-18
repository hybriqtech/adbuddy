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

    // Robolectric reads the merged resources and manifest (the screen tests).
    testOptions {
        unitTests.isIncludeAndroidResources = true
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

// Screen tests (plan tests-ui-jvm). Robolectric's Android jar (SDK 34: src/test/resources/robolectric.properties) comes
// through Gradle like any other dependency, so --offline works once it is in the cache: the tests never download it.
val robolectricAndroid: Configuration by configurations.creating { isTransitive = false }
val robolectricJars = layout.buildDirectory.dir("robolectric-jars")
val syncRobolectricJars = tasks.register<Sync>("syncRobolectricJars") {
    from(robolectricAndroid)
    into(robolectricJars)
}
// Golden images (src/test/screenshots): -Padbuddy.screens=record records them; verify (the default on Windows, where they
// are recorded) fails on a change beyond Roborazzi's default tolerance (0.7 % colour per pixel); compare (the default elsewhere) writes the differences to build/outputs/roborazzi
// and never fails, until the same images are shown to come out on that system too.
val screens = providers.gradleProperty("adbuddy.screens")
    .getOrElse(if (System.getProperty("os.name").startsWith("Windows")) "verify" else "compare")
require(screens in setOf("record", "verify", "compare")) { "adbuddy.screens is record, verify or compare, not $screens" }
tasks.withType<Test>().configureEach {
    dependsOn(syncRobolectricJars)
    systemProperty("robolectric.offline", "true")
    systemProperty("robolectric.dependency.dir", robolectricJars.get().asFile.absolutePath)
    inputs.files(fileTree("src/test/screenshots")).withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("roborazzi.test.record", (screens == "record").toString())
    systemProperty("roborazzi.test.verify", (screens == "verify").toString())
    systemProperty("roborazzi.test.compare", (screens == "compare").toString())
}

// The instrumented tests never run through Gradle's connected* tasks (plan M2.5b, rulings de la Tarea 2): AGP 9.4.1's test engine (UTP)
// runs `adb` in a Gradle worker with an empty environment on Windows, so it always talks to the default adb server
// (5037), which on a developer PC is the one that holds the user's phone, and it fails silently (BUILD SUCCESSFUL, 0
// tests) on an emulator behind another server. They run with `am instrument` against one emulator and its own adb
// server (host/internal/emutest, scripts/emulator.sh). Checked when the task graph is built, before any task runs and
// so before anything is installed (the install tasks that connected tasks depend on would install on every device).
gradle.taskGraph.whenReady {
    val connected = allTasks.filter { it.name.startsWith("connected") }.map { it.path }
    check(connected.isEmpty()) {
        "connected tests are not run through Gradle (they would reach the adb server of the user's phone): " +
            "build the APKs (:app:assembleDebug :app:assembleDebugAndroidTest) and run them with " +
            "`am instrument` on the emulator (android/README.md); refused: $connected"
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.service)
    implementation(libs.androidx.webkit)
    implementation(libs.androidx.biometric)
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.material3)
    implementation(libs.compose.material.icons.core)
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)

    testImplementation(libs.junit)
    testImplementation(libs.okhttp.mockwebserver)
    testImplementation(platform(libs.compose.bom))
    testImplementation(libs.compose.ui.test.junit4)
    testImplementation(libs.robolectric)
    testImplementation(libs.roborazzi)
    robolectricAndroid(libs.robolectric.android.all)
    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.androidx.test.ext.junit)
}
