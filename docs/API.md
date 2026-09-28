# API Reference

Base URL: `APP_BASE_URL`.

## Admin CLI APIs

All admin APIs require:

```http
Authorization: Bearer <ADMIN_TOKEN>
```

### Initialize first agent

```http
POST /api/v1/admin/agents/init
Content-Type: application/json

{"username":"admin","email":"admin@example.com","password":"change-me-strong"}
```

Only works when no agent exists.

### Create Site

```http
POST /api/v1/admin/sites
Content-Type: application/json

{"name":"寒山云"}
```

Returns deployment code.

### List Sites

```http
GET /api/v1/admin/sites
```

## Customer APIs

### XBoard trusted login

```http
POST /api/v1/customer/xboard-login
Content-Type: application/json

{
  "site_key":"st_xxxxxxxx",
  "xboard_user_id":"123",
  "email":"user@example.com",
  "plan":"Pro",
  "expire_time":"2027-01-01T00:00:00+08:00",
  "used_traffic":123,
  "all_traffic":456,
  "raw_profile":{},
  "last_support_entry_type":"web",
  "last_support_entry_url":"https://example.com/dashboard"
}
```

### Email verification

```http
POST /api/v1/customer/email/send-code
POST /api/v1/customer/email/verify
```

### Customer auth

Customer APIs after login require:

```http
Authorization: Bearer <customer_token>
```

Available endpoints:

```http
GET  /api/v1/customer/me
GET  /api/v1/customer/conversation
POST /api/v1/customer/messages
POST /api/v1/customer/images
POST /api/v1/customer/read
GET  /api/v1/customer/emergency/status
POST /api/v1/customer/emergency/start
POST /api/v1/customer/emergency/cancel
```

## Agent APIs

### Login

```http
POST /api/v1/agent/login
Content-Type: application/json

{"login":"admin","password":"change-me-strong","device_id":"android-device-id"}
```

Agent APIs after login require:

```http
Authorization: Bearer <agent_token>
```

Available endpoints:

```http
GET  /api/v1/agent/me
GET  /api/v1/agent/sync?after_seq=0
GET  /api/v1/agent/customers
GET  /api/v1/agent/conversations/{id}
POST /api/v1/agent/conversations/{id}/read
POST /api/v1/agent/conversations/{id}/messages
POST /api/v1/agent/conversations/{id}/images
POST /api/v1/agent/devices/foreground
POST /api/v1/agent/devices/emergency-duty
POST /api/v1/agent/emergency/{id}/accept
POST /api/v1/agent/customers/{id}/block
POST /api/v1/agent/customers/{id}/unblock
```

### Agent WebSocket

```text
GET /ws/agent?token=<agent_token>&after_seq=0
```

The Android Agent must still call `/api/v1/agent/sync` every 3 seconds as fallback.
