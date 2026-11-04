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
        alerts.offer(1, 13, 103, "third immediately after posting")
        assertEquals("third immediately after posting", alerts.items().single().text)
    }
    @Test fun customersDoNotOverwriteEachOthersNotifications() {
        val alerts = CustomerAlerts()
        alerts.offer(1, 11, 101, "A")
        alerts.offer(2, 12, 102, "B")
        assertEquals(2, alerts.items().size)
        alerts.delivered(1)
        assertEquals(2L, alerts.items().single().conversationId)
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
}
