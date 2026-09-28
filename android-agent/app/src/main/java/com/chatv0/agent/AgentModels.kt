package com.chatv0.agent

data class CustomerItem(
    val id: Long,
    val siteName: String,
    val email: String,
    val conversationId: Long,
    val blocked: Boolean,
    val unreadCount: Int = 0,
    val ringingCount: Int = 0,
    val lastMessage: String = ""
)

data class ChatMessage(
    val id: Long,
    val senderType: String,
    val seq: Long,
    val type: String,
    val content: String,
    val customerReadAt: String?,
    val attachmentId: Long?,
    val createdAt: String,
    val localStatus: String = "sent",
    val localKey: String = id.toString()
)

data class ConversationDetail(
    val conversationId: Long,
    val customerEmail: String,
    val siteName: String,
    val blocked: Boolean,
    val messages: List<ChatMessage>
)
