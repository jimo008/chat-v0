package com.chatv0.agent

import org.junit.Assert.*
import org.junit.Test

class ImageZoomTest {
    @Test fun pinchMaintainsTheFingerAnchor() {
        val result = ImageZoom().transform(2f, 0f, 0f, 150f, 200f, 400f, 400f, 400f, 400f)
        assertEquals(2f, result.scale, .001f)
        assertEquals(50f, result.x, .001f)
        assertEquals(0f, result.y, .001f)
    }
    @Test fun panningIsClampedToTheActualImageNotItsLetterbox() {
        val wide = ImageZoom().transform(2f, 10000f, 10000f, 200f, 200f, 400f, 400f, 800f, 200f)
        assertEquals(200f, wide.x, .001f)
        assertEquals(0f, wide.y, .001f)
        val tall = ImageZoom().transform(2f, -10000f, -10000f, 200f, 200f, 400f, 400f, 200f, 800f)
        assertEquals(0f, tall.x, .001f)
        assertEquals(-200f, tall.y, .001f)
    }
    @Test fun zoomLimitsAndRestoringFitAreStable() {
        val enlarged = ImageZoom().transform(100f, 10000f, 10000f, 200f, 200f, 400f, 400f, 400f, 400f)
        assertEquals(8f, enlarged.scale, .001f)
        assertEquals(1400f, enlarged.x, .001f)
        val fit = enlarged.transform(.001f, 50f, 50f, 200f, 200f, 400f, 400f, 400f, 400f)
        assertEquals(ImageZoom(), fit)
    }
}
