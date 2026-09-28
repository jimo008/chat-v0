package com.chatv0.agent

import android.content.Context
import android.net.Uri
import okhttp3.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONArray
import org.json.JSONObject
import java.util.concurrent.TimeUnit

class AgentApi(private val baseUrl: String, private val token: String = "") {
    private val client = OkHttpClient.Builder().callTimeout(20, TimeUnit.SECONDS).build()

    fun login(login: String, password: String, deviceId: String): String {
        val json = JSONObject(mapOf("login" to login, "password" to password, "device_id" to deviceId))
        val req = Request.Builder().url("$baseUrl/api/v1/agent/login").post(jsonBody(json)).build()
        client.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) error("登录失败: ${resp.code}")
            return JSONObject(resp.body?.string().orEmpty()).getString("token")
        }
    }

    fun customers(): List<CustomerItem> {
        val arr = getJson("/api/v1/agent/customers").optJSONArray("customers") ?: JSONArray()
        return (0 until arr.length()).map { i ->
            val o = arr.getJSONObject(i)
            CustomerItem(o.getLong("id"), o.optString("site_name"), o.optString("email"), o.getLong("conversation_id"), o.optBoolean("blocked"))
        }
    }

    fun conversation(id: Long): ConversationDetail {
        return parseConversation(conversationJson(id))
    }

    fun conversationJson(id: Long): JSONObject = getJson("/api/v1/agent/conversations/$id")

    fun parseConversation(root: JSONObject): ConversationDetail {
        val conv = root.getJSONObject("conversation")
        val customer = conv.getJSONObject("customer")
        val site = conv.getJSONObject("site")
        val arr = root.optJSONArray("messages") ?: JSONArray()
        val messages = (0 until arr.length()).map { i ->
            val o = arr.getJSONObject(i)
            ChatMessage(
                o.getLong("id"),
                o.optString("sender_type"),
                o.optLong("seq"),
                o.optString("type"),
                o.optString("content"),
                o.optString("customer_read_at").ifBlank { null },
                if (o.isNull("attachment_id")) null else o.optLong("attachment_id"),
                o.optString("created_at")
            )
        }
        return ConversationDetail(conv.getLong("id"), customer.optString("email"), site.optString("name"), customer.optBoolean("blocked"), messages)
    }

    fun sendMessage(conversationId: Long, content: String) {
        postJson("/api/v1/agent/conversations/$conversationId/messages", JSONObject(mapOf("type" to "text", "content" to content)))
    }

    fun uploadImage(context: Context, conversationId: Long, uri: Uri) {
        val bytes = context.contentResolver.openInputStream(uri)?.use { it.readBytes() } ?: error("无法读取图片")
        val body = MultipartBody.Builder().setType(MultipartBody.FORM)
            .addFormDataPart("file", "image", bytes.toRequestBody("application/octet-stream".toMediaType()))
            .build()
        val req = Request.Builder()
            .url("$baseUrl/api/v1/agent/conversations/$conversationId/images")
            .headers(authHeaders())
            .post(body)
            .build()
        client.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) error("上传失败: ${resp.code}")
        }
    }

    fun attachmentBytes(id: Long): ByteArray {
        val req = Request.Builder().url("$baseUrl/api/v1/agent/attachments/$id").headers(authHeaders()).build()
        client.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) error("图片加载失败: ${resp.code}")
            return resp.body?.bytes() ?: error("图片为空")
        }
    }

    fun markRead(conversationId: Long) { postJson("/api/v1/agent/conversations/$conversationId/read", JSONObject()) }
    fun setForeground(foreground: Boolean) { postJson("/api/v1/agent/devices/foreground", JSONObject(mapOf("foreground" to foreground))) }
    fun setEmergencyDuty(accept: Boolean) { postJson("/api/v1/agent/devices/emergency-duty", JSONObject(mapOf("accept_emergency" to accept))) }
    fun sync(afterSeq: Long): JSONObject = getJson("/api/v1/agent/sync?after_seq=$afterSeq")

    private fun getJson(path: String): JSONObject {
        val req = Request.Builder().url(baseUrl + path).headers(authHeaders()).build()
        client.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) error("请求失败: ${resp.code}")
            return JSONObject(resp.body?.string().orEmpty())
        }
    }

    private fun postJson(path: String, json: JSONObject): JSONObject {
        val req = Request.Builder().url(baseUrl + path).headers(authHeaders()).post(jsonBody(json)).build()
        client.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) error("请求失败: ${resp.code}")
            val body = resp.body?.string().orEmpty()
            return if (body.isBlank()) JSONObject() else JSONObject(body)
        }
    }

    private fun authHeaders(): Headers = Headers.Builder().add("Authorization", "Bearer $token").build()
    private fun jsonBody(json: JSONObject): RequestBody = json.toString().toRequestBody("application/json".toMediaType())
}
