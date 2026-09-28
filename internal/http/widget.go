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
  iframe.addEventListener("load", function(){
    if (window.SupportChatIdentity) {
      iframe.contentWindow.postMessage({type:"chat-v0:xboard-identity", identity: window.SupportChatIdentity}, "%s");
    }
  });
  document.body.appendChild(iframe);
})();`, s.cfg.AppBaseURL, s.cfg.AppBaseURL)
}

func (s *Server) widgetFrame(w http.ResponseWriter, r *http.Request) {
	site := html.EscapeString(r.URL.Query().Get("site"))
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
    .system { align-self: center; color: #64748b; font-size: 12px; text-align: center; }
    .auth { padding: 14px; border-top: 1px solid #e5e7eb; display: grid; gap: 8px; }
    .composer { padding: 10px; border-top: 1px solid #e5e7eb; display: grid; gap: 8px; background: #fff; }
    .row { display: flex; gap: 8px; }
    input { flex: 1; min-width: 0; height: 38px; border: 1px solid #cbd5e1; border-radius: 6px; padding: 0 10px; font-size: 14px; }
    button { height: 38px; border: 0; border-radius: 6px; padding: 0 12px; background: #2563eb; color: #fff; font-weight: 650; cursor: pointer; }
    button:disabled { opacity: .55; cursor: not-allowed; }
    .secondary { background: #475569; }
    .emergency { background: #dc2626; width: 100%; }
    .muted { color: #64748b; font-size: 12px; }
    .hidden { display: none; }
  </style>
</head>
<body>
  <div class="panel">
    <div class="header"><span>在线客服</span><span class="site">__SITE__</span></div>
    <div id="messages" class="messages"><div class="system">正在初始化...</div></div>
    <div id="auth" class="auth hidden">
      <div class="muted">请输入邮箱验证后开始聊天。</div>
      <div class="row"><input id="email" placeholder="邮箱"><button id="sendCode">发送验证码</button></div>
      <div class="row"><input id="code" placeholder="6 位验证码"><button id="verifyCode">进入客服</button></div>
    </div>
    <div id="composer" class="composer hidden">
      <div class="row"><input id="text" placeholder="输入消息..."><button id="send">发送</button></div>
      <button id="emergency" class="emergency">紧急呼叫客服</button>
    </div>
  </div>
<script>
(function(){
  var site = "__SITE__";
  var entryURL = "__ENTRY_URL__";
  var api = "";
  var tokenKey = "chat_v0_token_" + site;
  var token = localStorage.getItem(tokenKey) || "";
  var messagesEl = document.getElementById("messages");
  var authEl = document.getElementById("auth");
  var composerEl = document.getElementById("composer");
  var emailEl = document.getElementById("email");
  var codeEl = document.getElementById("code");
  var textEl = document.getElementById("text");
  var lastReadSeq = 0;

  function addSystem(text){ messagesEl.innerHTML = '<div class="system">' + escapeHTML(text) + '</div>'; }
  function appendMessage(msg){
    var div = document.createElement("div");
    div.className = "msg " + (msg.sender_type === "customer" ? "customer" : "agent");
    div.textContent = msg.type === "image" ? "[图片消息]" : (msg.content || "");
    messagesEl.appendChild(div);
    messagesEl.scrollTop = messagesEl.scrollHeight;
    if (msg.sender_type === "agent" && msg.seq > lastReadSeq) lastReadSeq = msg.seq;
  }
  function escapeHTML(text){ return String(text).replace(/[&<>"']/g, function(c){ return ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]); }); }
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
  document.getElementById("sendCode").onclick = function(){
    fetch(api + "/api/v1/customer/email/send-code", {method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({site_key:site,email:emailEl.value})})
      .then(function(r){ return r.json().then(function(data){ if(!r.ok) throw data; return data; }); })
      .then(function(){ addSystem("验证码已发送，请查收邮箱"); })
      .catch(function(){ addSystem("验证码发送失败或过于频繁"); });
  };
  document.getElementById("verifyCode").onclick = function(){
    fetch(api + "/api/v1/customer/email/verify", {method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({site_key:site,email:emailEl.value,code:codeEl.value,last_support_entry_type:"web",last_support_entry_url:entryURL})})
      .then(function(r){ return r.json().then(function(data){ if(!r.ok) throw data; return data; }); })
      .then(function(data){ token = data.token; localStorage.setItem(tokenKey, token); loadConversation(); })
      .catch(function(){ addSystem("验证码错误或已过期"); });
  };
  document.getElementById("send").onclick = function(){
    var content = textEl.value.trim();
    if (!content) return;
    textEl.value = "";
    fetch(api + "/api/v1/customer/messages", {method:"POST", headers:authHeaders(), body:JSON.stringify({type:"text",content:content})})
      .then(function(r){ return r.json().then(function(data){ if(!r.ok) throw data; return data; }); })
      .then(function(data){ appendMessage(data.message); })
      .catch(function(){ addSystem("消息发送失败"); });
  };
  document.getElementById("emergency").onclick = function(){
    fetch(api + "/api/v1/customer/emergency/start", {method:"POST", headers:authHeaders()})
      .then(function(r){ return r.json().then(function(data){ if(!r.ok) throw data; return data; }); })
      .then(function(){ addSystem("正在紧急呼叫客服..."); })
      .catch(function(){ addSystem("当前暂无客服值班，您可以先发送消息。"); });
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
	page = strings.ReplaceAll(page, "__ENTRY_URL__", entryURL)
	_, _ = w.Write([]byte(page))
}
