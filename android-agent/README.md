# Android Agent App

客服 Android App 基础测试版。

## Current Features

- Kotlin + Jetpack Compose
- Username/email + password login
- Token persisted with SharedPreferences
- Customer list
- Conversation detail
- Send text replies
- Send image replies
- Mark customer messages internally read
- Emergency duty switch
- Foreground Service
- WebSocket event stream
- Fixed 3-second `/api/v1/agent/sync` fallback
- Normal and emergency notifications
- Emergency call channel with ringtone/vibration

## Build

Use Android Studio, GitHub Actions, or a server with Android SDK and Gradle installed:

```bash
gradle :android-agent:app:assembleDebug
```

The generated APK will be under:

```text
android-agent/app/build/outputs/apk/debug/
```

GitHub Actions also includes a manual `Android Agent APK` workflow that uploads a debug APK artifact.

## Manual Test Checklist

1. Install APK on an Android test device.
2. Allow notifications.
3. Disable battery optimization for the app.
4. Allow background running / auto-start on vendor ROMs.
5. Login with the first agent account created by `./supportctl init-agent`.
6. Toggle emergency duty on.
7. Send a customer message from the Widget and confirm notification.
8. Start an emergency call from the Widget and confirm emergency notification.
9. Lock the phone for several minutes and confirm `/sync` still updates `last_sync_at`.

## Known Gaps

- Emergency notification still needs real-device tuning for each vendor ROM.
- Needs real-device testing across Huawei/Xiaomi/other Android variants.
