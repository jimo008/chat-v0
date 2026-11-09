package com.chatv0.agent

import org.junit.Assert.*
import org.junit.Test

class CustomerAlertsTest {
    @Test fun fastConsecutiveMessagesAreMergedNotDiscarded() {
        val alerts = CustomerAlerts()
        alerts.offer(1, 11, 101, "first")
        alerts.offer(1, 12, 102, "second")
        assertEquals(2, alerts.items().single().count)
        assertEquals("second", alerts.items().single().text)
        alerts.delivered(1)
        assertTrue(alerts.pending().isEmpty())
        alerts.offer(1, 13, 103, "third immediately after posting")
        assertEquals("third immediately after posting", alerts.pending().single().text)
    }
    @Test fun customersDoNotOverwriteEachOthersNotifications() {
        val alerts = CustomerAlerts()
        alerts.offer(1, 11, 101, "A")
        alerts.offer(2, 12, 102, "B")
        assertEquals(2, alerts.items().size)
        alerts.delivered(1)
        assertEquals(2L, alerts.pending().single().conversationId)
    }
    @Test fun failedDeliveryCanBeRetriedAfterRestartWithoutLosingTheMessage() {
        val alerts = CustomerAlerts()
        alerts.offer(1, 11, 101, "pending")
        val restored = CustomerAlerts(alerts.items())
        assertEquals(alerts.items(), restored.items())
        restored.read(1, 101)
        assertTrue(restored.items().isEmpty())
    }
    @Test fun oldReadReceiptsCannotDiscardNewerMessages() {
        val alerts = CustomerAlerts()
        alerts.offer(1, 11, 101, "older")
        alerts.offer(1, 12, 102, "newer")
        alerts.read(1, 101)
        assertEquals(102L, alerts.items().single().messageSeq)
        assertEquals(1, alerts.items().single().count)
        alerts.offer(1, 11, 101, "duplicate older event")
        assertEquals("newer", alerts.items().single().text)
    }

    @Test fun postingOnceKeepsTheUnreadMessageThroughRestartUntilRead() {
        val alerts = CustomerAlerts()
        alerts.offer(1, 11, 101, "unread")
        alerts.delivered(1)
        val restored = CustomerAlerts(alerts.items())
        assertTrue(restored.pending().isEmpty())
        restored.read(1, 100)
        assertEquals(101L, restored.items().single().messageSeq)
        restored.read(1, 101)
        assertTrue(restored.items().isEmpty())
    }

    @Test fun unreadRemindersRepeatAtTenSecondsAndResetWhenInactive() {
        val clock = UnreadReminderClock()
        assertFalse(clock.due(1000, active = true))
        assertFalse(clock.due(10_999, active = true))
        assertTrue(clock.due(11_000, active = true))
        assertFalse(clock.due(20_999, active = true))
        assertTrue(clock.due(21_000, active = true))
        assertFalse(clock.due(31_000, active = false))
        assertFalse(clock.due(61_000, active = false))
        assertFalse(clock.due(61_001, active = true))
        assertTrue(clock.due(71_001, active = true))
    }
}
