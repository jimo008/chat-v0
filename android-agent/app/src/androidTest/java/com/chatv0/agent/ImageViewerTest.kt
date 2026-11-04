package com.chatv0.agent

import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.*
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.test.*
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Rule
import org.junit.Test
import java.io.File

class ImageViewerTest {
    @get:Rule val compose = createComposeRule()

    @Test fun openingPinchingPanningResettingAndClosingImageDoesNotCrash() {
        val bitmap = Bitmap.createBitmap(1200, 1800, Bitmap.Config.ARGB_8888)
        val canvas = Canvas(bitmap)
        canvas.drawColor(Color.WHITE)
        val paint = Paint().apply { color = Color.BLUE; strokeWidth = 8f }
        for (x in 0..1200 step 100) canvas.drawLine(x.toFloat(), 0f, x.toFloat(), 1800f, paint)
        for (y in 0..1800 step 100) canvas.drawLine(0f, y.toFloat(), 1200f, y.toFloat(), paint)
        paint.color = Color.BLACK
        paint.textSize = 72f
        canvas.drawText("Image zoom test", 160f, 900f, paint)
        compose.setContent {
            MaterialTheme {
                var opened by remember { mutableStateOf(false) }
                Button(onClick = { opened = true }) { Text("Open test image") }
                if (opened) ZoomableImageDialog(bitmap) { opened = false }
            }
        }
        compose.onNodeWithText("Open test image").performClick()
        compose.onNodeWithContentDescription("图片原图").assertIsDisplayed()
        compose.onNodeWithText("100%").assertIsDisplayed()
        screenshot("image-viewer-open.png")
        compose.onNodeWithTag("image-viewer-viewport").performTouchInput {
            val left = center - Offset(40f, 0f)
            val right = center + Offset(40f, 0f)
            down(0, left)
            down(1, right)
            moveTo(0, left - Offset(120f, 0f), delayMillis = 150)
            moveTo(1, right + Offset(120f, 0f), delayMillis = 150)
            up(0)
            up(1)
        }
        compose.onNodeWithText("100%").assertDoesNotExist()
        screenshot("image-viewer-zoomed.png")
        compose.onNodeWithTag("image-viewer-viewport").performTouchInput { swipeLeft() }
        compose.onNodeWithContentDescription("重置缩放").performClick()
        compose.onNodeWithText("100%").assertIsDisplayed()
        compose.onNodeWithTag("image-viewer-viewport").performTouchInput { doubleClick() }
        compose.onNodeWithText("100%").assertDoesNotExist()
        compose.onNodeWithContentDescription("关闭图片").performClick()
        compose.onNodeWithText("Open test image").assertIsDisplayed()
        compose.onNodeWithContentDescription("图片原图").assertDoesNotExist()
        // Reopening also exercises teardown/recreation of the full-screen window.
        compose.onNodeWithText("Open test image").performClick()
        compose.onNodeWithContentDescription("图片原图").assertIsDisplayed()
        compose.onNodeWithContentDescription("关闭图片").performClick()
    }

    private fun screenshot(name: String) {
        compose.waitForIdle()
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val file = File(instrumentation.targetContext.getExternalFilesDir(null), name)
        file.outputStream().use { instrumentation.uiAutomation.takeScreenshot().compress(Bitmap.CompressFormat.PNG, 100, it) }
    }
}
