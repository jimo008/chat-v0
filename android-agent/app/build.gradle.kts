plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

android {
    namespace = "com.chatv0.agent"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.chatv0.agent"
        minSdk = 26
        targetSdk = 35
        versionCode = 20261028
        versionName = "0.2.2"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures { compose = true }

    signingConfigs {
        create("release") {
            System.getenv("ANDROID_RELEASE_KEYSTORE")?.let { storeFile = file(it) }
            storePassword = System.getenv("ANDROID_RELEASE_STORE_PASSWORD")?.trim()
            keyPassword = System.getenv("ANDROID_RELEASE_STORE_PASSWORD")?.trim()
            keyAlias = "chat-v0-agent"
            storeType = "PKCS12"
        }
    }
    buildTypes {
        getByName("release") { signingConfig = signingConfigs.getByName("release") }
    }
}

dependencies {
    implementation("androidx.activity:activity-compose:1.9.3")
    implementation("androidx.core:core-ktx:1.15.0")
    implementation("androidx.compose.material3:material3:1.3.1")
    implementation("androidx.compose.ui:ui:1.7.6")
    implementation("androidx.compose.ui:ui-tooling-preview:1.7.6")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.7")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.7")
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")
    debugImplementation("androidx.compose.ui:ui-tooling:1.7.6")
    testImplementation("junit:junit:4.13.2")
}
