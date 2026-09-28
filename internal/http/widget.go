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
  iframe.style.width = "118px";
  iframe.style.height = "48px";
  iframe.style.maxWidth = "calc(100vw - 24px)";
  iframe.style.maxHeight = "calc(100vh - 24px)";
  iframe.style.border = "0";
  iframe.style.zIndex = "2147483647";
  iframe.style.background = "transparent";
  iframe.allow = "clipboard-write";
  var lastPostedIdentity = "";
  function postIdentity(identity, source){
    if (!identity) return;
    var identityKey = "";
    try { identityKey = JSON.stringify({id: identity.xboard_user_id || identity.id || identity.user_id || "", email: identity.email || ""}); } catch(e) {}
    if (identityKey && identityKey === lastPostedIdentity) return;
    lastPostedIdentity = identityKey;
    console.info("[chat-v0] xboard identity detected from " + source, identity);
    iframe.contentWindow.postMessage({type:"chat-v0:xboard-identity", identity: identity, source: source}, "%s");
  }
  function normalizeXBoardInfo(raw){
    var root = raw && (raw.data || raw.user || raw);
    if (!root) return null;
    var id = root.xboard_user_id || root.user_id || root.id || root.uuid;
    var email = root.email || root.mail || root.account || root.username;
    if (!id || !email) return null;
    return {
      xboard_user_id: String(id),
      email: String(email),
      plan: root.plan || root.plan_name || root.group || "",
      expire_time: root.expire_time || root.expired_at || root.expiredAt || null,
      used_traffic: root.used_traffic || root.u || null,
      all_traffic: root.all_traffic || root.transfer_enable || null,
      raw_profile: raw
    };
  }
  function sendIdentity(){
    if (window.SupportChatIdentity) {
      postIdentity(window.SupportChatIdentity, "SupportChatIdentity");
    }
  }
  iframe.addEventListener("load", sendIdentity);
  var lastIdentity = "";
  var lastInfoCheck = 0;
  function pollXBoardInfo(){
    if (Date.now() - lastInfoCheck < 10000) return;
    lastInfoCheck = Date.now();
    fetch("/api/v1/user/info", {credentials:"include"}).then(function(r){
      if (!r.ok) throw new Error("status " + r.status);
      return r.json();
    }).then(function(data){
      var identity = normalizeXBoardInfo(data);
      if (identity) postIdentity(identity, "/api/v1/user/info");
      else console.info("[chat-v0] /api/v1/user/info returned without usable id/email", data);
    }).catch(function(err){
      console.info("[chat-v0] /api/v1/user/info unavailable", err && err.message ? err.message : err);
    });
  }
  setInterval(function(){
    var next = "";
    try { next = JSON.stringify(window.SupportChatIdentity || null); } catch(e) {}
    if (next && next !== lastIdentity) {
      lastIdentity = next;
      sendIdentity();
    }
    pollXBoardInfo();
  }, 2000);
  setTimeout(pollXBoardInfo, 1000);
  window.addEventListener("message", function(ev){
    if (ev.origin !== "%s") return;
    var data = ev.data || {};
    if (data.type !== "chat-v0:resize") return;
    iframe.style.width = data.open ? "360px" : "118px";
    iframe.style.height = data.open ? "560px" : "48px";
  });
  document.body.appendChild(iframe);
})();`, s.cfg.AppBaseURL, s.cfg.AppBaseURL, s.cfg.AppBaseURL)
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
    .launcher { width: 118px; height: 48px; border: 0; border-radius: 999px; background: #2563eb; color: #fff; font-size: 15px; font-weight: 750; box-shadow: 0 10px 28px rgba(37,99,235,.35); position: relative; }
    .launcher.notify { animation: shake .8s ease-in-out infinite; }
    @keyframes shake { 0%, 100% { transform: translateX(0); } 20% { transform: translateX(-3px); } 40% { transform: translateX(3px); } 60% { transform: translateX(-2px); } 80% { transform: translateX(2px); } }
    .badge { position: absolute; right: -4px; top: -5px; min-width: 20px; height: 20px; padding: 0 6px; border-radius: 999px; background: #ef4444; color: #fff; font-size: 12px; line-height: 20px; text-align: center; border: 2px solid #fff; }
    .panel { width: 100vw; height: 100vh; border: 1px solid #d7dde8; border-radius: 8px; overflow: hidden; background: #fff; box-shadow: 0 16px 50px rgba(15,23,42,.18); display: flex; flex-direction: column; }
    .header { height: 52px; padding: 0 14px; display: flex; align-items: center; justify-content: space-between; background: #0f172a; color: #fff; font-size: 15px; font-weight: 650; }
    .headerMain { display: flex; align-items: center; gap: 8px; }
    .minBtn { width: 34px; height: 34px; padding: 0; background: rgba(255,255,255,.16); color: #fff; font-size: 20px; line-height: 34px; }
    .site { font-size: 12px; color: #cbd5e1; max-width: 150px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .messages { flex: 1; padding: 14px; overflow-y: auto; background: #f8fafc; display: flex; flex-direction: column; gap: 10px; }
    .msg { max-width: 82%; padding: 9px 10px; border-radius: 8px; font-size: 13px; line-height: 1.45; white-space: pre-wrap; word-break: break-word; }
    .msgBody { white-space: pre-wrap; word-break: break-word; }
    .msgMeta { margin-top: 5px; font-size: 11px; opacity: .76; display: flex; gap: 6px; align-items: center; justify-content: flex-end; }
    .agent .msgMeta { justify-content: flex-start; color: #64748b; }
    .retryLink { border: 0; height: auto; padding: 0; background: transparent; color: #fecaca; font-size: 11px; font-weight: 700; }
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
    .emergencyState { border-top: 1px solid #fecaca; background: #fff1f2; color: #991b1b; padding: 10px; display: grid; gap: 8px; font-size: 13px; font-weight: 650; }
    .cancelEmergency { background: #991b1b; width: 100%; }
    .muted { color: #64748b; font-size: 12px; }
    .hidden { display: none; }
  </style>
</head>
<body>
  <button id="launcher" class="launcher">在线客服<span id="badge" class="badge hidden">0</span></button>
  <div class="panel">
    <div class="header"><div class="headerMain"><button id="minimize" class="minBtn" title="最小化">×</button><span>在线客服</span></div><span class="site">__SITE_NAME__</span></div>
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
    <div id="emergencyState" class="emergencyState hidden">
      <div id="emergencyText">☎ 正在呼叫客服 0 秒</div>
      <button id="cancelEmergency" class="cancelEmergency">取消呼叫</button>
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
  var launcherEl = document.getElementById("launcher");
  var badgeEl = document.getElementById("badge");
  var minimizeBtn = document.getElementById("minimize");
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
  var emergencyStateEl = document.getElementById("emergencyState");
  var emergencyTextEl = document.getElementById("emergencyText");
  var cancelEmergencyBtn = document.getElementById("cancelEmergency");
  var lastReadSeq = 0;
  var seenAgentSeq = Number(localStorage.getItem("chat_v0_seen_agent_seq_" + site) || "0");
  var unreadCount = 0;
  var opened = false;
  var emergencySeconds = 0;
  var emergencyTimer = null;
  var emergencyStatusTimer = null;
  var emergencyActive = false;
  var codeTimer = null;
  var codeCountdown = 0;

  function addSystem(text, kind){
    var cls = "system" + (kind ? " " + kind : "");
    messagesEl.innerHTML = '<div class="' + cls + '">' + escapeHTML(text) + '</div>';
  }
  function appendMessage(msg){
    var div = document.createElement("div");
    div.className = "msg " + (msg.sender_type === "customer" ? "customer" : "agent");
    var body = document.createElement("div");
    body.className = "msgBody";
    if (msg.type === "image" && msg.attachment_id) {
      div.className += " imageMsg";
      var img = document.createElement("img");
      img.className = "msgImage";
      img.alt = "图片消息";
      img.src = attachmentURL(msg.attachment_id);
      img.onclick = function(){ openImage(img.src); };
      body.appendChild(img);
    } else {
      body.textContent = msg.type === "image" ? "[图片消息]" : (msg.content || "");
    }
    div.appendChild(body);
    div.appendChild(messageMeta(msg));
    messagesEl.appendChild(div);
    messagesEl.scrollTop = messagesEl.scrollHeight;
    if (msg.sender_type === "agent" && msg.seq > lastReadSeq) {
      lastReadSeq = msg.seq;
      if (!opened && msg.seq > seenAgentSeq) unreadCount++;
    }
    updateBadge();
  }
  function messageMeta(msg){
    var meta = document.createElement("div");
    meta.className = "msgMeta";
    var time = document.createElement("span");
    time.textContent = formatTime(msg.created_at);
    meta.appendChild(time);
    if (msg.sender_type === "customer") {
      var state = document.createElement("span");
      state.textContent = msg.local_status || "已发送";
      meta.appendChild(state);
      if (msg.local_status === "发送失败" && msg.retry) {
        var retry = document.createElement("button");
        retry.className = "retryLink";
        retry.textContent = "重发";
        retry.onclick = msg.retry;
        meta.appendChild(retry);
      }
    }
    return meta;
  }
  function formatTime(value){
    if (!value) return "刚刚";
    var d = new Date(value);
    if (isNaN(d.getTime())) return String(value).slice(11,16) || "刚刚";
    return d.toLocaleTimeString("zh-CN", {hour:"2-digit", minute:"2-digit", hour12:false, timeZone:"Asia/Shanghai"});
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
    if (code === "file_too_large") return "图片太大，请选择 30MB 以内的图片";
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
  function canMarkRead(){ return token && lastReadSeq && opened && !document.hidden && document.hasFocus(); }
  function setOpen(next){
    opened = next;
    launcherEl.classList.toggle("hidden", opened);
    document.querySelector(".panel").classList.toggle("hidden", !opened);
    window.parent.postMessage({type:"chat-v0:resize", open: opened}, "*");
    if (opened && canMarkRead()) {
      unreadCount = 0;
      if (lastReadSeq) {
        seenAgentSeq = lastReadSeq;
        localStorage.setItem("chat_v0_seen_agent_seq_" + site, String(seenAgentSeq));
      }
      updateBadge();
      sendRead();
    }
  }
  function updateBadge(){
    if (unreadCount > 0) { badgeEl.textContent = unreadCount > 99 ? "99+" : String(unreadCount); badgeEl.classList.remove("hidden"); }
    else { badgeEl.classList.add("hidden"); }
    launcherEl.classList.toggle("notify", unreadCount > 0 && !opened);
  }
  launcherEl.onclick = function(){ setOpen(true); };
  minimizeBtn.onclick = function(){ setOpen(false); };
  function showAuthed(){ authEl.classList.add("hidden"); composerEl.classList.toggle("hidden", emergencyActive); }
  function showAuth(){ composerEl.classList.add("hidden"); authEl.classList.remove("hidden"); }
  function loadConversation(){
    if (!token) { showAuth(); addSystem("请先验证邮箱"); return; }
    fetch(api + "/api/v1/customer/conversation", {headers: authHeaders()}).then(function(r){
      if (r.status === 401) { token = ""; localStorage.removeItem(tokenKey); showAuth(); addSystem("请先验证邮箱"); return null; }
      return r.json();
    }).then(function(data){
      if (!data) return;
      messagesEl.innerHTML = "";
      unreadCount = 0;
      (data.messages || []).forEach(appendMessage);
      if ((data.messages || []).length === 0) addSystem("可以开始聊天了");
      showAuthed();
      if (canMarkRead()) {
        if (lastReadSeq) {
          seenAgentSeq = lastReadSeq;
          localStorage.setItem("chat_v0_seen_agent_seq_" + site, String(seenAgentSeq));
        }
        updateBadge();
        sendRead();
      }
    }).catch(function(){ addSystem("连接客服失败"); showAuth(); });
  }
  function sendRead(){
    if (!canMarkRead()) return;
    fetch(api + "/api/v1/customer/read", {method:"POST", headers:authHeaders(), body:JSON.stringify({up_to_seq:lastReadSeq})}).catch(function(){});
  }
  document.addEventListener("visibilitychange", sendRead);
  window.addEventListener("focus", sendRead);
  setInterval(function(){ if(token) loadConversation(); }, 3000);
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
    var localKey = "local-" + Date.now();
    var pendingEl = null;
    var pending = {id:localKey, sender_type:"customer", type:"text", content:content, created_at:new Date().toISOString(), local_status:"发送中..."};
    var originalAppendMessage = appendMessage;
    appendMessage(pending);
    pendingEl = messagesEl.lastElementChild;
    fetch(api + "/api/v1/customer/messages", {method:"POST", headers:authHeaders(), body:JSON.stringify({type:"text",content:content})})
      .then(function(r){ return r.json().then(function(data){ if(!r.ok) throw data; return data; }); })
      .then(function(){ loadConversation(); })
      .catch(function(){
        pending.local_status = "发送失败";
        pending.retry = function(){ textEl.value = content; sendBtn.click(); };
        if (pendingEl) {
          var replacement = document.createElement("div");
          originalAppendMessage(pending);
          replacement = messagesEl.lastElementChild;
          messagesEl.insertBefore(replacement, pendingEl);
          pendingEl.remove();
          messagesEl.scrollTop = messagesEl.scrollHeight;
        } else {
          appendMessage(pending);
        }
      })
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
      .then(function(){ startEmergencyCalling(); })
      .catch(function(){ addSystem("当前暂无客服值班，您可以先发送消息。"); })
      .finally(function(){ setLoading(emergencyBtn, false); });
  };
  function startEmergencyCalling(){
    emergencyActive = true;
    emergencySeconds = 0;
    composerEl.classList.add("hidden");
    emergencyStateEl.classList.remove("hidden");
    emergencyTextEl.textContent = "☎ 正在呼叫客服 0 秒";
    if (emergencyTimer) clearInterval(emergencyTimer);
    emergencyTimer = setInterval(function(){
      emergencySeconds++;
      emergencyTextEl.textContent = "☎ 正在呼叫客服 " + emergencySeconds + " 秒";
    }, 1000);
    if (emergencyStatusTimer) clearInterval(emergencyStatusTimer);
    emergencyStatusTimer = setInterval(checkEmergencyStatus, 3000);
  }
  function stopEmergencyCalling(){
    emergencyActive = false;
    if (emergencyTimer) clearInterval(emergencyTimer);
    if (emergencyStatusTimer) clearInterval(emergencyStatusTimer);
    emergencyTimer = null;
    emergencyStatusTimer = null;
    emergencyStateEl.classList.add("hidden");
    if (token) composerEl.classList.remove("hidden");
  }
  function checkEmergencyStatus(){
    if (!token) return;
    fetch(api + "/api/v1/customer/emergency/status", {headers:authHeaders()})
      .then(function(r){ return r.json(); })
      .then(function(data){
        if (data.call_status === "ACCEPTED") { stopEmergencyCalling(); addSystem("客服已接听紧急呼叫", "success"); }
        if (data.call_status === "CANCELLED" || data.call_status === "EXPIRED") { stopEmergencyCalling(); addSystem("紧急呼叫已结束"); }
      }).catch(function(){});
  }
  cancelEmergencyBtn.onclick = function(){
    setLoading(cancelEmergencyBtn, true, "取消中");
    fetch(api + "/api/v1/customer/emergency/cancel", {method:"POST", headers:authHeaders()})
      .then(function(r){ return r.json().then(function(data){ if(!r.ok) throw data; return data; }); })
      .then(function(){ stopEmergencyCalling(); addSystem("已取消紧急呼叫"); })
      .catch(function(){ addSystem("取消呼叫失败，请稍后再试", "error"); })
      .finally(function(){ setLoading(cancelEmergencyBtn, false); });
  };
  window.addEventListener("message", function(ev){
    var data = ev.data || {};
    if (data.type !== "chat-v0:xboard-identity" || !data.identity) return;
    var id = data.identity;
    console.info("[chat-v0] iframe received xboard identity", data.source || "unknown", id);
    fetch(api + "/api/v1/customer/xboard-login", {method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({site_key:site,xboard_user_id:String(id.xboard_user_id || id.id || id.user_id || ""),email:id.email || "",plan:id.plan || id.subscription || "",expire_time:id.expire_time || id.expired_at || null,used_traffic:id.used_traffic || null,all_traffic:id.all_traffic || null,raw_profile:id.raw_profile || id,last_support_entry_type:"web",last_support_entry_url:entryURL})})
      .then(function(r){ return r.json().then(function(data){ if(!r.ok) throw data; return data; }); })
      .then(function(data){ console.info("[chat-v0] xboard login success", data.customer); token = data.token; localStorage.setItem(tokenKey, token); if (id.email) emailEl.value = id.email; loadConversation(); })
      .catch(function(err){ console.info("[chat-v0] xboard login failed", err); if(!token) showAuth(); });
  });
  setOpen(false);
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
