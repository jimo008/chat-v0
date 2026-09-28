package http

import (
	"fmt"
	"html"
	"net/http"
	"strings"
)

func (s *Server) widgetJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `(function(){
  var script = document.currentScript;
  var site = script && script.getAttribute("data-site");
  if (!site) return;
  if (document.getElementById("chat-v0-widget")) return;
  var iframe = document.createElement("iframe");
  iframe.id = "chat-v0-widget";
  iframe.src = "%s/widget/frame?site=" + encodeURIComponent(site) + "&entry_url=" + encodeURIComponent(window.location.href);
  iframe.title = "在线客服";
  iframe.style.position = "fixed";
  iframe.style.right = "18px";
  iframe.style.bottom = "18px";
  iframe.style.width = "360px";
  iframe.style.height = "560px";
  iframe.style.maxWidth = "calc(100vw - 24px)";
  iframe.style.maxHeight = "calc(100vh - 24px)";
  iframe.style.border = "0";
  iframe.style.zIndex = "2147483647";
  iframe.style.background = "transparent";
  iframe.allow = "clipboard-write";
  document.body.appendChild(iframe);
})();`, s.cfg.AppBaseURL)
}

func (s *Server) widgetFrame(w http.ResponseWriter, r *http.Request) {
	site := html.EscapeString(r.URL.Query().Get("site"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	page := `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>在线客服</title>
  <style>
    :root { color-scheme: light; font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    * { box-sizing: border-box; }
    body { margin: 0; background: transparent; }
    .panel { width: 100vw; height: 100vh; border: 1px solid #d7dde8; border-radius: 8px; overflow: hidden; background: #fff; box-shadow: 0 16px 50px rgba(15,23,42,.18); display: flex; flex-direction: column; }
    .header { height: 52px; padding: 0 16px; display: flex; align-items: center; justify-content: space-between; background: #0f172a; color: #fff; font-size: 15px; font-weight: 650; }
    .body { flex: 1; padding: 18px; display: flex; flex-direction: column; gap: 14px; color: #1f2937; }
    .hint { font-size: 13px; line-height: 1.55; color: #64748b; }
    .site { font-size: 12px; color: #94a3b8; }
    .input { margin-top: auto; border-top: 1px solid #e5e7eb; padding-top: 12px; display: flex; gap: 8px; }
    input { flex: 1; height: 38px; border: 1px solid #cbd5e1; border-radius: 6px; padding: 0 10px; font-size: 14px; }
    button { height: 38px; border: 0; border-radius: 6px; padding: 0 14px; background: #2563eb; color: #fff; font-weight: 650; }
    .emergency { background: #dc2626; width: 100%; margin-top: 10px; }
  </style>
</head>
<body>
  <div class="panel">
    <div class="header"><span>在线客服</span><span class="site">__SITE__</span></div>
    <div class="body">
      <div class="hint">客服组件基础壳已加载。下一阶段会接入 XBoard 登录、邮箱验证、消息同步和紧急呼叫界面。</div>
      <div class="input"><input placeholder="输入消息..." disabled><button disabled>发送</button></div>
      <button class="emergency" disabled>紧急呼叫客服</button>
    </div>
  </div>
</body>
</html>`
	_, _ = w.Write([]byte(strings.ReplaceAll(page, "__SITE__", site)))
}
