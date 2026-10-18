package com.chatv0.agent

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import okhttp3.Interceptor
import okhttp3.Response
import okhttp3.Request
import okio.Buffer
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.MessageDigest
import java.security.SecureRandom
import java.security.Signature
import java.security.spec.ECGenParameterSpec

object AgentDeviceIdentity {
    @Volatile var deviceId: String = ""
        private set
    private val random = SecureRandom()
    private val store by lazy { KeyStore.getInstance("AndroidKeyStore").apply { load(null) } }

    fun initialize(id: String) { deviceId = id }
    private fun alias(): String = "chat-v0-device-" + deviceId

    @Synchronized
    fun publicKey(create: Boolean = true): String {
        check(deviceId.isNotBlank()) { "设备身份尚未初始化" }
        if (!store.containsAlias(alias())) {
            if (!create) return ""
            KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, "AndroidKeyStore").apply {
                initialize(KeyGenParameterSpec.Builder(alias(), KeyProperties.PURPOSE_SIGN or KeyProperties.PURPOSE_VERIFY)
                    .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                    .setDigests(KeyProperties.DIGEST_SHA256)
                    .setUserAuthenticationRequired(false)
                    .build())
            }.generateKeyPair()
        }
        return Base64.encodeToString(store.getCertificate(alias()).publicKey.encoded, Base64.NO_WRAP)
    }

    fun nonce(): String = ByteArray(16).also(random::nextBytes).toHex()
    fun digest(bytes: ByteArray): String = MessageDigest.getInstance("SHA-256").digest(bytes).toHex()
    fun sign(message: ByteArray): String {
        publicKey()
        val signature = Signature.getInstance("SHA256withECDSA")
        signature.initSign(store.getKey(alias(), null) as java.security.PrivateKey)
        signature.update(message)
        return Base64.encodeToString(signature.sign(), Base64.NO_WRAP)
    }
    private fun ByteArray.toHex(): String = joinToString("") { "%02x".format(it.toInt() and 255) }
}

class AgentDeviceProofInterceptor : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val original = chain.request()
        if (!original.url.encodedPath.startsWith("/api/v1/agent/") && original.url.encodedPath != "/ws/agent") {
            return chain.proceed(original)
        }
        val buffer = Buffer()
        original.body?.writeTo(buffer)
        val request = original.newBuilder()
            .header("X-Chat-Device-ID", AgentDeviceIdentity.deviceId)
            .header("X-Chat-Device-Time", (System.currentTimeMillis() / 1000L).toString())
            .header("X-Chat-Device-Nonce", AgentDeviceIdentity.nonce())
            .build()
        val message = deviceProofMessage(request, buffer.readByteArray())
        return chain.proceed(request.newBuilder().header("X-Chat-Device-Signature", AgentDeviceIdentity.sign(message)).build())
    }
}

internal fun deviceProofMessage(request: Request, body: ByteArray): ByteArray {
    val target = request.url.encodedPath + (request.url.encodedQuery?.let { "?$it" } ?: "")
    return listOf("chat-v0-device-v1", request.header("X-Chat-Device-ID").orEmpty(),
        request.header("X-Chat-Device-Time").orEmpty(), request.header("X-Chat-Device-Nonce").orEmpty(),
        request.method, target, request.header("Authorization").orEmpty(), AgentDeviceIdentity.digest(body))
        .joinToString("\n").toByteArray(Charsets.UTF_8)
}
