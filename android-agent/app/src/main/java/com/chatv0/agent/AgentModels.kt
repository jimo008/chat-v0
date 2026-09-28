package com.chatv0.agent

data class CustomerItem(
    val id: Long,
    val siteName: String,
    val email: String,
    val conversationId: Long,
    val blocked: Boolean
)

data class ChatMessage(
    val id: Long,
    val senderType: String,
    val seq: Long,
    val type: String,
    val content: String,
    val customerReadAt: String?
)

data class ConversationDetail(
    val conversationId: Long,
    val customerEmail: String,
    val siteName: String,
    val blocked: Boolean,
    val messages: List<ChatMessage>
)
