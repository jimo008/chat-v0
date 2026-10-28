package com.chatv0.agent

internal class EventCursor(initial: Long) {
    var value: Long = initial
        private set

    fun accept(sequence: Long): Boolean {
        if (sequence <= value || sequence <= 0L) return false
        value = sequence
        return true
    }
}
