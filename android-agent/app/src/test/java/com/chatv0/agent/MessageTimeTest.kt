package com.chatv0.agent

import org.junit.Assert.assertEquals
import org.junit.Test

class MessageTimeTest {
    @Test fun includesDateForHistoricalMessages() {
        assertEquals("2025-10-17 09:14", formatTime("2025-10-17T09:14:12+08:00"))
    }
    @Test fun utcMessagesUseTheCorrectBeijingDateAcrossMidnightAndNewYear() {
        assertEquals("2026-11-04 01:30", formatTime("2026-11-03T17:30:00Z"))
        assertEquals("2027-01-01 00:01", formatTime("2026-12-31T16:01:00Z"))
    }
}
