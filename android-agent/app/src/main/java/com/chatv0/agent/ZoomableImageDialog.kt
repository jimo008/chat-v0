package com.chatv0.agent

import android.graphics.Bitmap
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.gestures.detectTransformGestures
import androidx.compose.foundation.layout.*
import androidx.compose.material3.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import kotlin.math.roundToInt

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ZoomableImageDialog(bitmap: Bitmap, onClose: () -> Unit) {
    var zoom by remember(bitmap) { mutableStateOf(ImageZoom()) }
    var viewport by remember { mutableStateOf(IntSize.Zero) }
    LaunchedEffect(viewport) { zoom = ImageZoom() }
    Dialog(onDismissRequest = onClose, properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false)) {
        Column(Modifier.fillMaxSize().background(Color.Black).windowInsetsPadding(WindowInsets.safeDrawing)) {
            Row(Modifier.fillMaxWidth().height(56.dp).padding(horizontal = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Text("图片", color = Color.White, modifier = Modifier.weight(1f))
                Text("${(zoom.scale * 100).roundToInt()}%", color = Color.White, fontSize = 14.sp, maxLines = 1, modifier = Modifier.width(72.dp))
                TooltipBox(positionProvider = TooltipDefaults.rememberPlainTooltipPositionProvider(), tooltip = { PlainTooltip { Text("重置缩放") } }, state = rememberTooltipState()) {
                    IconButton(onClick = { zoom = ImageZoom() }) {
                        Icon(Icons.Filled.Refresh, contentDescription = "重置缩放", tint = Color.White)
                    }
                }
                IconButton(onClick = onClose) {
                    Icon(Icons.Filled.Close, contentDescription = "关闭图片", tint = Color.White)
                }
            }
            Box(Modifier.fillMaxWidth().weight(1f).testTag("image-viewer-viewport").clipToBounds().onSizeChanged { viewport = it }
                .pointerInput(bitmap, viewport) {
                    detectTransformGestures { focus, pan, factor, _ ->
                        zoom = zoom.transform(factor, pan.x, pan.y, focus.x, focus.y,
                            viewport.width.toFloat(), viewport.height.toFloat(), bitmap.width.toFloat(), bitmap.height.toFloat())
                    }
                }
                .pointerInput(bitmap, viewport) {
                    detectTapGestures(onDoubleTap = { focus ->
                        zoom = if (zoom.scale > 1.5f) ImageZoom() else zoom.transform(3f / zoom.scale, 0f, 0f, focus.x, focus.y,
                            viewport.width.toFloat(), viewport.height.toFloat(), bitmap.width.toFloat(), bitmap.height.toFloat())
                    })
                }) {
                Image(bitmap.asImageBitmap(), contentDescription = "图片原图", contentScale = ContentScale.Fit,
                    modifier = Modifier.fillMaxSize().graphicsLayer { scaleX = zoom.scale; scaleY = zoom.scale; translationX = zoom.x; translationY = zoom.y })
            }
        }
    }
}
