package http

import (
	"database/sql"
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
  iframe.addEventListener("load", function(){
    if (window.SupportChatIdentity) {
      iframe.contentWindow.postMessage({type:"chat-v0:xboard-identity", identity: window.SupportChatIdentity}, "%s");
    }
  });
  document.body.appendChild(iframe);
})();`, s.cfg.AppBaseURL, s.cfg.AppBaseURL)
}

func (s *Server) widgetFrame(w http.ResponseWriter, r *http.Request) {
	siteKey := strings.TrimSpace(r.URL.Query().Get("site"))
	siteLabel := "在线客服"
	if site, err := s.findSiteByKey(r.Context(), siteKey); err == nil {
		siteLabel = site.Name
	} else if err != sql.ErrNoRows {
		s.logger.Error("widget site lookup", "error", err)
	}
	site := html.EscapeString(siteKey)
	siteName := html.EscapeString(siteLabel)
	entryURL := html.EscapeString(r.URL.Query().Get("entry_url"))
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
    .header { height: 52px; padding: 0 14px; display: flex; align-items: center; justify-content: space-between; background: #0f172a; color: #fff; font-size: 15px; font-weight: 650; }
    .site { font-size: 12px; color: #cbd5e1; max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .messages { flex: 1; padding: 14px; overflow-y: auto; background: #f8fafc; display: flex; flex-direction: column; gap: 10px; }
    .msg { max-width: 82%; padding: 9px 10px; border-radius: 8px; font-size: 13px; line-height: 1.45; white-space: pre-wrap; word-break: break-word; }
    .customer { align-self: flex-end; background: #2563eb; color: #fff; }
    .agent { align-self: flex-start; background: #fff; border: 1px solid #e2e8f0; color: #111827; }
    .msg.imageMsg { padding: 4px; background: transparent; border: 0; }
    .msgImage { display: block; max-width: 180px; max-height: 180px; border-radius: 8px; object-fit: cover; cursor: zoom-in; border: 1px solid #d7dde8; background: #fff; }
    .customer .msgImage { border-color: rgba(255,255,255,.38); }
    .system { align-self: center; color: #64748b; font-size: 12px; text-align: center; }
    .system.success { color: #15803d; background: #dcfce7; border: 1px solid #86efac; border-radius: 8px; padding: 9px 10px; font-size: 13px; font-weight: 650; max-width: 86%; }
    .system.error { color: #b91c1c; background: #fee2e2; border: 1px solid #fecaca; border-radius: 8px; padding: 9px 10px; font-size: 13px; font-weight: 650; max-width: 86%; }
    .auth { padding: 14px; border-top: 1px solid #e5e7eb; display: grid; gap: 8px; }
    .composer { padding: 10px; border-top: 1px solid #e5e7eb; display: grid; gap: 8px; background: #fff; }
    .row { display: flex; gap: 8px; }
    input { flex: 1; min-width: 0; height: 38px; border: 1px solid #cbd5e1; border-radius: 6px; padding: 0 10px; font-size: 14px; }
    button { height: 38px; border: 0; border-radius: 6px; padding: 0 12px; background: #2563eb; color: #fff; font-weight: 650; cursor: pointer; transition: transform .12s ease, opacity .12s ease, filter .12s ease; }
    button:hover:not(:disabled) { filter: brightness(.97); }
    button:active:not(:disabled) { transform: translateY(1px) scale(.98); }
    button:disabled { opacity: .55; cursor: not-allowed; }
    button.loading { position: relative; color: transparent; }
    button.loading::after { content: ""; position: absolute; width: 14px; height: 14px; left: 50%; top: 50%; margin-left: -7px; margin-top: -7px; border: 2px solid rgba(255,255,255,.55); border-top-color: #fff; border-radius: 999px; animation: spin .8s linear infinite; }
    @keyframes spin { to { transform: rotate(360deg); } }
    .secondary { background: #475569; }
    .imageBtn { width: 44px; padding: 0; background: #475569; flex: 0 0 44px; }
    .emergency { background: #dc2626; width: 100%; }
    .muted { color: #64748b; font-size: 12px; }
    .hidden { display: none; }
  </style>
</head>
<body>
  <div class="panel">
    <div class="header"><span>在线客服</span><span class="site">__SITE_NAME__</span></div>
    <div id="messages" class="messages"><div class="system">正在初始化...</div></div>
    <div id="auth" class="auth hidden">
      <div class="muted">请输入邮箱验证后开始聊天。</div>
      <div class="row"><input id="email" placeholder="邮箱"><button id="sendCode">发送验证码</button></div>
      <div class="row"><input id="code" placeholder="6 位验证码"><button id="verifyCode">进入客服</button></div>
    </div>
    <div id="composer" class="composer hidden">
      <input id="imageInput" type="file" accept="image/png,image/jpeg,image/webp" class="hidden">
      <div class="row"><input id="text" placeholder="输入消息..."><button id="imageBtn" class="imageBtn" title="发送图片">图片</button><button id="send">发送</button></div>
      <button id="emergency" class="emergency">紧急呼叫客服</button>
    </div>
  </div>
  <div id="lightbox" class="hidden" style="position:fixed; inset:0; z-index:10; background:rgba(15,23,42,.82); display:none; align-items:center; justify-content:center; padding:18px;">
    <img id="lightboxImg" alt="原图" style="max-width:100%; max-height:100%; border-radius:8px; background:#fff;">
  </div>
<script>
(function(){
  var site = "__SITE__";
  var entryURL = "__ENTRY_URL__";
  var api = "";
  var tokenKey = "chat_v0_token_" + site;
  var token = localStorage.getItem(tokenKey) || "";
  var messagesEl = document.getElementById("messages");
  var lightboxEl = document.getElementById("lightbox");
  var lightboxImgEl = document.getElementById("lightboxImg");
  var authEl = document.getElementById("auth");
  var composerEl = document.getElementById("composer");
  var emailEl = document.getElementById("email");
  var codeEl = document.getElementById("code");
  var textEl = document.getElementById("text");
  var imageInputEl = document.getElementById("imageInput");
  var sendCodeBtn = document.getElementById("sendCode");
  var verifyCodeBtn = document.getElementById("verifyCode");
  var sendBtn = document.getElementById("send");
  var imageBtn = document.getElementById("imageBtn");
  var emergencyBtn = document.getElementById("emergency");
  var lastReadSeq = 0;
  var codeTimer = null;
  var codeCountdown = 0;

  function addSystem(text, kind){
    var cls = "system" + (kind ? " " + kind : "");
    messagesEl.innerHTML = '<div class="' + cls + '">' + escapeHTML(text) + '</div>';
  }
  function appendMessage(msg){
    var div = document.createElement("div");
    div.className = "msg " + (msg.sender_type === "customer" ? "customer" : "agent");
    if (msg.type === "image" && msg.attachment_id) {
      div.className += " imageMsg";
      var img = document.createElement("img");
      img.className = "msgImage";
      img.alt = "图片消息";
      img.src = attachmentURL(msg.attachment_id);
      img.onclick = function(){ openImage(img.src); };
      div.appendChild(img);
    } else {
      div.textContent = msg.type === "image" ? "[图片消息]" : (msg.content || "");
    }
    messagesEl.appendChild(div);
    messagesEl.scrollTop = messagesEl.scrollHeight;
    if (msg.sender_type === "agent" && msg.seq > lastReadSeq) lastReadSeq = msg.seq;
  }
  function attachmentURL(id){ return api + "/api/v1/customer/attachments/" + encodeURIComponent(id) + "?token=" + encodeURIComponent(token); }
  function openImage(src){ lightboxImgEl.src = src; lightboxEl.style.display = "flex"; lightboxEl.classList.remove("hidden"); }
  lightboxEl.onclick = function(){ lightboxEl.style.display = "none"; lightboxEl.classList.add("hidden"); lightboxImgEl.src = ""; };
  function escapeHTML(text){ return String(text).replace(/[&<>"']/g, function(c){ return ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]); }); }
  function errorMessage(data, fallback){
    var code = data && data.error;
    if (code === "send_too_frequent") return "验证码发送过于频繁，请 60 秒后再试";
    if (code === "smtp_not_configured" || code === "email_send_failed") return "验证码邮件发送失败，请联系网站管理员检查 SMTP 配置";
    if (code === "site_not_found") return "客服站点不存在，请检查嵌入代码";
    if (code === "site_email_required") return "请输入正确邮箱";
    if (code === "rate_check_failed" || code === "code_store_failed") return "验证码服务暂时不可用，请稍后再试";
    return fallback;
  }
  function imageErrorMessage(data){
    var code = data && data.error;
    if (code === "unsupported_image_type") return "图片格式不支持，请选择 JPG、PNG 或 WebP 图片";
    if (code === "file_too_large") return "图片太大，请选择 10MB 以内的图片";
    if (code === "file_required" || code === "invalid_upload") return "请选择要发送的图片";
    if (code === "storage_prepare_failed" || code === "storage_write_failed") return "图片存储失败，请联系网站管理员检查上传目录权限";
    if (code === "customer_blocked") return "当前无法使用在线客服，请通过其他联系方式联系我们。";
    if (code === "customer_not_found") return "登录已失效，请重新验证邮箱";
    return "图片发送失败，请稍后再试";
  }
  function setLoading(btn, loading, text){
    if (!btn) return;
    if (text && !btn.dataset.originalText) btn.dataset.originalText = btn.textContent;
    if (loading) {
      btn.disabled = true;
      btn.classList.add("loading");
      if (text) btn.textContent = text;
      return;
    }
    btn.disabled = false;
    btn.classList.remove("loading");
    if (btn.dataset.originalText) btn.textContent = btn.dataset.originalText;
    delete btn.dataset.originalText;
  }
  function startCodeCountdown(seconds){
    codeCountdown = seconds;
    if (codeTimer) clearInterval(codeTimer);
    sendCodeBtn.disabled = true;
    sendCodeBtn.classList.remove("loading");
    sendCodeBtn.textContent = codeCountdown + "秒后重发";
    codeTimer = setInterval(function(){
      codeCountdown--;
      if (codeCountdown <= 0) {
        clearInterval(codeTimer);
        codeTimer = null;
        sendCodeBtn.disabled = false;
        sendCodeBtn.textContent = "发送验证码";
        return;
      }
      sendCodeBtn.textContent = codeCountdown + "秒后重发";
    }, 1000);
  }
  function authHeaders(){ return token ? {"Authorization":"Bearer " + token, "Content-Type":"application/json"} : {"Content-Type":"application/json"}; }
  function showAuthed(){ authEl.classList.add("hidden"); composerEl.classList.remove("hidden"); }
  function showAuth(){ composerEl.classList.add("hidden"); authEl.classList.remove("hidden"); }
  function loadConversation(){
    if (!token) { showAuth(); addSystem("请先验证邮箱"); return; }
    fetch(api + "/api/v1/customer/conversation", {headers: authHeaders()}).then(function(r){
      if (r.status === 401) { token = ""; localStorage.removeItem(tokenKey); showAuth(); addSystem("请先验证邮箱"); return null; }
      return r.json();
    }).then(function(data){
      if (!data) return;
      messagesEl.innerHTML = "";
      (data.messages || []).forEach(appendMessage);
      if ((data.messages || []).length === 0) addSystem("可以开始聊天了");
      showAuthed();
      sendRead();
    }).catch(function(){ addSystem("连接客服失败"); showAuth(); });
  }
  function sendRead(){
    if (!token || !lastReadSeq || document.hidden) return;
    fetch(api + "/api/v1/customer/read", {method:"POST", headers:authHeaders(), body:JSON.stringify({up_to_seq:lastReadSeq})}).catch(function(){});
  }
  document.addEventListener("visibilitychange", sendRead);
  setInterval(function(){ if(token && !document.hidden) loadConversation(); }, 3000);
  sendCodeBtn.onclick = function(){
    setLoading(sendCodeBtn, true, "发送中");
    fetch(api + "/api/v1/customer/email/send-code", {method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({site_key:site,email:emailEl.value})})
      .then(function(r){ return r.json().then(function(data){ if(!r.ok) throw data; return data; }); })
      .then(function(data){ addSystem("验证码已发送，请打开邮箱查看 6 位验证码。", "success"); startCodeCountdown(data.resend_after_seconds || 60); })
      .catch(function(data){ setLoading(sendCodeBtn, false); addSystem(errorMessage(data, "验证码发送失败或过于频繁"), "error"); if(data && data.error === "send_too_frequent") startCodeCountdown(60); });
  };
  verifyCodeBtn.onclick = function(){
    setLoading(verifyCodeBtn, true, "进入中");
    fetch(api + "/api/v1/customer/email/verify", {method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({site_key:site,email:emailEl.value,code:codeEl.value,last_support_entry_type:"web",last_support_entry_url:entryURL})})
      .then(function(r){ return r.json().then(function(data){ if(!r.ok) throw data; return data; }); })
      .then(function(data){ token = data.token; localStorage.setItem(tokenKey, token); loadConversation(); })
      .catch(function(){ addSystem("验证码错误或已过期"); })
      .finally(function(){ setLoading(verifyCodeBtn, false); });
  };
  sendBtn.onclick = function(){
    var content = textEl.value.trim();
    if (!content) return;
    textEl.value = "";
    setLoading(sendBtn, true, "发送中");
    fetch(api + "/api/v1/customer/messages", {method:"POST", headers:authHeaders(), body:JSON.stringify({type:"text",content:content})})
      .then(function(r){ return r.json().then(function(data){ if(!r.ok) throw data; return data; }); })
      .then(function(data){ appendMessage(data.message); })
      .catch(function(){ addSystem("消息发送失败"); textEl.value = content; })
      .finally(function(){ setLoading(sendBtn, false); });
  };
  imageBtn.onclick = function(){ imageInputEl.click(); };
  imageInputEl.onchange = function(){
    var file = imageInputEl.files && imageInputEl.files[0];
    imageInputEl.value = "";
    if (!file) return;
    var form = new FormData();
    form.append("file", file);
    setLoading(imageBtn, true, "上传中");
    fetch(api + "/api/v1/customer/images", {method:"POST", headers: token ? {"Authorization":"Bearer " + token} : {}, body: form})
      .then(function(r){ return r.json().then(function(data){ if(!r.ok) throw data; return data; }); })
      .then(function(data){ appendMessage(data.message); })
      .catch(function(data){ addSystem(imageErrorMessage(data), "error"); })
      .finally(function(){ setLoading(imageBtn, false); });
  };
  emergencyBtn.onclick = function(){
    setLoading(emergencyBtn, true, "呼叫中");
    fetch(api + "/api/v1/customer/emergency/start", {method:"POST", headers:authHeaders()})
      .then(function(r){ return r.json().then(function(data){ if(!r.ok) throw data; return data; }); })
      .then(function(){ addSystem("正在紧急呼叫客服..."); })
      .catch(function(){ addSystem("当前暂无客服值班，您可以先发送消息。"); })
      .finally(function(){ setLoading(emergencyBtn, false); });
  };
  window.addEventListener("message", function(ev){
    var data = ev.data || {};
    if (data.type !== "chat-v0:xboard-identity" || !data.identity) return;
    var id = data.identity;
    fetch(api + "/api/v1/customer/xboard-login", {method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({site_key:site,xboard_user_id:String(id.xboard_user_id || id.id || id.user_id || ""),email:id.email || "",plan:id.plan || id.subscription || "",expire_time:id.expire_time || id.expired_at || null,used_traffic:id.used_traffic || null,all_traffic:id.all_traffic || null,raw_profile:id,last_support_entry_type:"web",last_support_entry_url:entryURL})})
      .then(function(r){ return r.json().then(function(data){ if(!r.ok) throw data; return data; }); })
      .then(function(data){ token = data.token; localStorage.setItem(tokenKey, token); loadConversation(); })
      .catch(function(){ if(!token) showAuth(); });
  });
  loadConversation();
})();
</script>
</body>
</html>`
	page = strings.ReplaceAll(page, "__SITE__", site)
	page = strings.ReplaceAll(page, "__SITE_NAME__", siteName)
	page = strings.ReplaceAll(page, "__ENTRY_URL__", entryURL)
	_, _ = w.Write([]byte(page))
}
