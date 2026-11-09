package com.chatv0.agent

internal data class CustomerAlert(
    val conversationId: Long,
    val messageId: Long,
    val messageSeq: Long,
    val text: String,
    val count: Int = 1,
    val notified: Boolean = false
)

internal class CustomerAlerts(saved: List<CustomerAlert> = emptyList()) {
    private val unread = linkedMapOf<Long, CustomerAlert>().apply { saved.forEach { put(it.conversationId, it) } }
    fun offer(conversationId: Long, messageId: Long, messageSeq: Long, text: String) {
        if (conversationId <= 0 || messageId <= 0 || messageSeq <= 0) return
        val previous = unread[conversationId]
        if (previous != null && messageSeq <= previous.messageSeq) return
        unread[conversationId] = CustomerAlert(conversationId, messageId, messageSeq, text.take(512), (previous?.count ?: 0) + 1)
    }
    fun read(conversationId: Long, upToSeq: Long) {
        if (upToSeq <= 0) return
        val item = unread[conversationId] ?: return
        if (item.messageSeq <= upToSeq) unread.remove(conversationId)
        else unread[conversationId] = item.copy(count = 1)
    }
    fun delivered(conversationId: Long) {
        val item = unread[conversationId] ?: return
        unread[conversationId] = item.copy(notified = true)
    }
    fun pending(): List<CustomerAlert> = unread.values.filterNot { it.notified }
    fun items(): List<CustomerAlert> = unread.values.toList()
}

internal class UnreadReminderClock {
    private var nextAt: Long? = null

    fun due(now: Long, active: Boolean): Boolean {
        if (!active) {
            nextAt = null
            return false
        }
        val deadline = nextAt
        if (deadline != null && now < deadline) return false
        nextAt = now + 10_000L
        return deadline != null
    }
}
