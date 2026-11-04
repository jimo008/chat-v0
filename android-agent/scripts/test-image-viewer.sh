#!/usr/bin/env bash
set -u
adb logcat -c
gradle :android-agent:app:connectedDebugAndroidTest
result=$?
mkdir -p image-viewer-ui-tests
adb logcat -d > image-viewer-ui-tests/logcat.txt
adb pull /sdcard/Android/data/com.chatv0.agent/files/ image-viewer-ui-tests/ || true
exit "$result"
