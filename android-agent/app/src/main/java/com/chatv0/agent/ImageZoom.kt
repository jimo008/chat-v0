package com.chatv0.agent

import kotlin.math.max
import kotlin.math.min

internal data class ImageZoom(val scale: Float = 1f, val x: Float = 0f, val y: Float = 0f) {
    fun transform(factor: Float, panX: Float, panY: Float, focusX: Float, focusY: Float,
                  viewWidth: Float, viewHeight: Float, imageWidth: Float, imageHeight: Float): ImageZoom {
        if (!factor.isFinite() || factor <= 0 || min(viewWidth, viewHeight) <= 0 || min(imageWidth, imageHeight) <= 0) return this
        val nextScale = (scale * factor).coerceIn(1f, 8f)
        if (nextScale == 1f) return ImageZoom()
        val ratio = nextScale / scale
        val fit = min(viewWidth / imageWidth, viewHeight / imageHeight)
        val maxX = max(0f, (imageWidth * fit * nextScale - viewWidth) / 2f)
        val maxY = max(0f, (imageHeight * fit * nextScale - viewHeight) / 2f)
        val nextX = x * ratio + (focusX - viewWidth / 2f) * (1f - ratio) + panX
        val nextY = y * ratio + (focusY - viewHeight / 2f) * (1f - ratio) + panY
        return ImageZoom(nextScale, nextX.coerceIn(-maxX, maxX), nextY.coerceIn(-maxY, maxY))
    }
}
