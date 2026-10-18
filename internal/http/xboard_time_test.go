package http

import (
	"encoding/json"
	"testing"
	"time"
)

func TestXBoardLoginExpiryFormats(t *testing.T) {
	for _, raw := range []string{"1791993599", "1791993599000", `"1791993599"`, `"1791993599000"`, `"2026-10-14T15:59:59Z"`, `"2026-10-14T23:59:59+08:00"`} {
		t.Run(raw, func(t *testing.T) {
			var req xboardLoginRequest
			payload := `{"site_key":"st_test","xboard_user_id":"uuid-test","email":"customer@example.com","plan":"Basic","expire_time":` + raw + `,"used_traffic":0,"all_traffic":214748364800}`
			if err := json.Unmarshal([]byte(payload), &req); err != nil {
				t.Fatal(err)
			}
			if req.ExpireTime == nil || req.ExpireTime.Unix() != 1791993599 {
				t.Fatalf("unexpected expiry: %v", req.ExpireTime)
			}
			if req.Email != "customer@example.com" || req.XBoardUserID != "uuid-test" || req.SiteKey != "st_test" || req.Plan != "Basic" || req.UsedTraffic == nil || *req.UsedTraffic != 0 || req.AllTraffic == nil || *req.AllTraffic != 214748364800 {
				t.Fatalf("identity/profile fields were lost: %+v", req)
			}
		})
	}
}

func TestXBoardLoginOptionalAndInvalidExpiry(t *testing.T) {
	for _, payload := range []string{`{}`, `{"expire_time":null}`, `{"expire_time":0}`, `{"expire_time":""}`} {
		old := time.Now()
		req := xboardLoginRequest{ExpireTime: &old}
		if err := json.Unmarshal([]byte(payload), &req); err != nil || req.ExpireTime != nil {
			t.Fatalf("optional expiry %s: value=%v error=%v", payload, req.ExpireTime, err)
		}
	}
	for _, raw := range []string{`"not-a-date"`, `true`, `{}`, `-1`, `1.5`, `999999999999999999`} {
		var req xboardLoginRequest
		if err := json.Unmarshal([]byte(`{"expire_time":`+raw+`}`), &req); err == nil {
			t.Fatalf("accepted invalid expiry: %s", raw)
		}
	}
}
