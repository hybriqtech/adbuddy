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
        versionCode = 1
        versionName = "0.2.0"
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
}

// The protocol's shared fixtures (protocol/fixtures/, protocol spec §12): FixturesTest reads them.
val protocolFixtures = rootProject.file("../protocol/fixtures")
tasks.withType<Test>().configureEach {
    inputs.dir(protocolFixtures).withPathSensitivity(PathSensitivity.RELATIVE)
    systemProperty("adbuddy.protocolFixtures", protocolFixtures.absolutePath)
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.service)
    implementation(platform(libs.compose.bom))
    implementation(libs.compose.material3)
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)

    testImplementation(libs.junit)
    testImplementation(libs.okhttp.mockwebserver)
    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.androidx.test.ext.junit)
}
