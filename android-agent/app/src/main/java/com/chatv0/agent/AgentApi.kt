package com.chatv0.agent

import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns
import okhttp3.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONArray
import org.json.JSONObject
import java.time.OffsetDateTime
import java.util.concurrent.TimeUnit

class AgentApi(private val baseUrl: String, private val token: String = "") {
    private val client = OkHttpClient.Builder().callTimeout(60, TimeUnit.SECONDS).build()

    fun login(login: String, password: String, deviceId: String): String {
        val json = JSONObject(mapOf("login" to login, "password" to password, "device_id" to deviceId))
        val req = Request.Builder().url("$baseUrl/api/v1/agent/login").post(jsonBody(json)).build()
        client.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) error("登录失败: ${resp.code}")
            return JSONObject(resp.body?.string().orEmpty()).getString("token")
        }
    }

    fun customers(): List<CustomerItem> {
        return parseCustomers(customersJson())
    }

    fun customersJson(): JSONObject = getJson("/api/v1/agent/customers")

    fun me(): JSONObject = getJson("/api/v1/agent/me")

    fun devicesJson(): JSONObject = getJson("/api/v1/agent/devices")

    fun isUnauthorized(error: Throwable): Boolean = error.message?.contains("401") == true

    fun parseCustomers(root: JSONObject): List<CustomerItem> {
        val arr = root.optJSONArray("customers") ?: JSONArray()
        return (0 until arr.length()).map { i ->
            val o = arr.getJSONObject(i)
            CustomerItem(
                o.getLong("id"),
                o.optString("site_name"),
                o.optString("email"),
                o.getLong("conversation_id"),
                o.optBoolean("blocked"),
                o.optInt("unread_count"),
                o.optInt("ringing_count"),
                o.optString("last_message"),
                o.optString("last_message_at")
            )
        }.sortedWith(compareByDescending<CustomerItem> { if (it.ringingCount > 0) 1 else 0 }.thenByDescending { parseTimeMillis(it.lastMessageAt) })
    }

    fun conversation(id: Long): ConversationDetail {
        return parseConversation(conversationJson(id))
    }

    fun conversationJson(id: Long, beforeSeq: Long? = null, afterSeq: Long? = null): JSONObject {
        val params = mutableListOf<String>()
        if (beforeSeq != null && beforeSeq > 0L) params.add("before_seq=$beforeSeq")
        if (afterSeq != null && afterSeq > 0L) params.add("after_seq=$afterSeq")
        val suffix = if (params.isEmpty()) "" else "?" + params.joinToString("&")
        return getJson("/api/v1/agent/conversations/$id$suffix")
    }

    fun parseConversation(root: JSONObject): ConversationDetail {
        val conv = root.getJSONObject("conversation")
        val customer = conv.getJSONObject("customer")
        val site = conv.getJSONObject("site")
        val readState = root.optJSONObject("read_state")
        val arr = root.optJSONArray("messages") ?: JSONArray()
        val messages = (0 until arr.length()).map { i -> parseMessage(arr.getJSONObject(i)) }
        return ConversationDetail(
            conv.getLong("id"),
            customer.optString("email"),
            site.optString("name"),
            customer.optBoolean("blocked"),
            messages,
            readState?.optLong("customer_last_seen_seq") ?: 0L,
            readState?.optString("customer_last_seen_at")?.takeIf { it.isNotBlank() && it != "null" },
            readState?.optLong("agent_last_seen_seq") ?: 0L,
            readState?.optString("agent_last_seen_at")?.takeIf { it.isNotBlank() && it != "null" }
        )
    }

    fun parseMessage(o: JSONObject): ChatMessage =
        ChatMessage(
            o.getLong("id"),
            o.optString("sender_type"),
            o.optLong("seq"),
            o.optString("type"),
            o.optString("content"),
            if (o.isNull("customer_read_at")) null else o.optString("customer_read_at").ifBlank { null },
            if (o.isNull("attachment_id")) null else o.optLong("attachment_id"),
            o.optString("created_at"),
            if (o.isNull("client_msg_id")) null else o.optString("client_msg_id").ifBlank { null }
        )

    fun sendMessage(conversationId: Long, content: String, clientMsgId: String): ChatMessage =
        parseMessage(postJson("/api/v1/agent/conversations/$conversationId/messages", JSONObject(mapOf("type" to "text", "content" to content, "client_msg_id" to clientMsgId))).getJSONObject("message"))

    fun uploadImage(context: Context, conversationId: Long, uri: Uri) {
        val bytes = context.contentResolver.openInputStream(uri)?.use { it.readBytes() } ?: error("无法读取图片")
        val mime = context.contentResolver.getType(uri)?.takeIf { it.isNotBlank() } ?: "application/octet-stream"
        val filename = displayName(context, uri)
        val body = MultipartBody.Builder().setType(MultipartBody.FORM)
            .addFormDataPart("file", filename, bytes.toRequestBody(mime.toMediaType()))
            .build()
        val req = Request.Builder()
            .url("$baseUrl/api/v1/agent/conversations/$conversationId/images")
            .headers(authHeaders())
            .post(body)
            .build()
        client.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) {
                val code = runCatching { JSONObject(resp.body?.string().orEmpty()).optString("error") }.getOrDefault("")
                error(imageError(code, resp.code))
            }
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
    private fun imageError(code: String, status: Int): String = when (code) {
        "file_too_large" -> "图片太大，请选择 30MB 以内的图片"
        "unsupported_image_type" -> "图片格式不支持，请选择 JPG、PNG 或 WebP 图片"
        "file_required", "invalid_upload" -> "请选择要发送的图片"
        "storage_prepare_failed", "storage_write_failed" -> "图片存储失败，请检查服务器上传目录权限"
        else -> "图片上传失败: $status"
    }
}

private fun parseTimeMillis(value: String): Long =
    runCatching { OffsetDateTime.parse(value).toInstant().toEpochMilli() }.getOrDefault(0L)

private fun displayName(context: Context, uri: Uri): String {
    context.contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { cursor ->
        if (cursor.moveToFirst()) {
            val index = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
            if (index >= 0) {
                val name = cursor.getString(index)
                if (!name.isNullOrBlank()) return name
            }
        }
    }
    return "image"
}
