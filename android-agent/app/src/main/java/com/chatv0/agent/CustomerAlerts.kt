package com.chatv0.agent

internal data class CustomerAlert(val conversationId: Long, val messageId: Long, val messageSeq: Long, val text: String, val count: Int = 1)

internal class CustomerAlerts(saved: List<CustomerAlert> = emptyList()) {
    private val pending = linkedMapOf<Long, CustomerAlert>().apply { saved.forEach { put(it.conversationId, it) } }
    fun offer(conversationId: Long, messageId: Long, messageSeq: Long, text: String) {
        if (conversationId <= 0 || messageId <= 0 || messageSeq <= 0) return
        val previous = pending[conversationId]
        if (previous != null && messageSeq <= previous.messageSeq) return
        pending[conversationId] = CustomerAlert(conversationId, messageId, messageSeq, text.take(512), (previous?.count ?: 0) + 1)
    }
    fun read(conversationId: Long, upToSeq: Long) {
        if (upToSeq <= 0) return
        val item = pending[conversationId] ?: return
        if (item.messageSeq <= upToSeq) pending.remove(conversationId)
        else pending[conversationId] = item.copy(count = 1)
    }
    fun delivered(conversationId: Long) { pending.remove(conversationId) }
    fun items(): List<CustomerAlert> = pending.values.toList()
}
