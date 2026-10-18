package http

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Legacy embed tags carry data-site in the DOM, not in the script request URL.
func (s *Server) widgetBootstrap(w http.ResponseWriter, r *http.Request) {
	endpoint, _ := json.Marshal(strings.TrimRight(s.cfg.AppBaseURL, "/") + "/widget.js?site_key=")
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	_, _ = w.Write([]byte(`(function(){
var current=document.currentScript;
var site=current && current.getAttribute("data-site");
if(!site) return;
var script=document.createElement("script");
script.src=` + string(endpoint) + `+encodeURIComponent(site);
script.setAttribute("data-site",site);
(document.head || document.body).appendChild(script);
})();`))
}
