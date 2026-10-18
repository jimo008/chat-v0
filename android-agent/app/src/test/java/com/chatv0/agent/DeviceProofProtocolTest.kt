package com.chatv0.agent

import okhttp3.Request
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody
import org.junit.Assert.assertEquals
import org.junit.Test
import java.io.File

class DeviceProofProtocolTest {
    @Test fun matchesServerProtocolVector() {
        val bytes = "{\"content\":\"hello\"}".toByteArray(Charsets.UTF_8)
        val request = Request.Builder()
            .url("https://support.example/api/v1/agent/conversations/1/messages?after_seq=42")
            .post(bytes.toRequestBody("application/json".toMediaType()))
            .header("Authorization", "Bearer test-token")
            .header("X-Chat-Device-ID", "android-test-device")
            .header("X-Chat-Device-Time", "1792288800")
            .header("X-Chat-Device-Nonce", "00112233445566778899aabbccddeeff")
            .build()
        val vector = File("tests/device_proof_message.txt").takeIf { it.isFile }
            ?: File("../../tests/device_proof_message.txt")
        assertEquals(vector.readText().trimEnd('\n'), deviceProofMessage(request, bytes).toString(Charsets.UTF_8))
    }
}
