# V1 Technical Design

## 1. Scope

Chat V0 is a self-hosted customer support messaging system.

V1 supports:

- Web customer widget embedded by one script tag.
- Future Android customer SDK/app entry using the same Site model.
- Text messages.
- Image messages.
- Emergency call button, implemented as a high-priority agent alert, not VoIP.
- Android-only Agent App.
- Postal SMTP unread-message email reminders.
- XBoard trusted customer identity and profile import.
- Single-server Docker Compose deployment first, with a design that can later scale horizontally.

V1 explicitly does not support:

- FCM or vendor Push.
- Web agent console.
- Voice or video calls.
- Ticket system.
- AI bot.
- Customer message edit/revoke/delete.
- Customer account deletion.
- Site deletion.
- Site domain whitelist.
- Showing agent names or online state to customers.
- Inbound email replies.
- Emergency call cooldown.
- Conversation locking.

## 2. Architecture

```text
Web Site / XBoard / Future Customer App
              |
              | widget.js / customer SDK
              v
        Go Backend API
       /      |       \
 WebSocket  HTTP /sync  REST API
       \      |       /
           Event Layer
              |
       MySQL + Redis
              |
       Worker Processes
       - Postal email
       - cleanup
       - scheduled emergency duty

Android Agent App
  - Foreground Service
  - WebSocket
  - fixed 3-second HTTP sync
  - system default normal notification
  - continuous emergency ringtone
```

### Technology Choices

- Backend: Go.
- Database: MySQL 8.
- Realtime/cache/job state: Redis.
- Realtime transport: WebSocket.
- Fallback transport: fixed 3-second HTTP `/sync`.
- Customer Web: JavaScript widget + iframe isolation.
- Agent: Android, Kotlin, Jetpack Compose, Foreground Service.
- Image storage: local filesystem in V1, storage interface shaped for S3/R2/MinIO migration.
- Email: Postal SMTP.
- Deployment: Docker Compose.

## 3. Site Model

Site is the highest data isolation boundary.

A Site has:

- `site_id`
- `name`
- deployment code
- timestamps

No V1 Site fields:

- no allowed domains
- no origin whitelist
- no enable/disable flag
- no fixed email return URL
- no delete feature

Example deployment code:

```html
<script src="https://chat.example.com/widget.js" data-site="1001"></script>
```

Rules:

- `site_id` is public, not a secret.
- Same `site_id` across many domains shares one customer data space.
- Different `site_id` values fully isolate customers, messages, unread state, images, blocked state, emergency calls, email batches, and XBoard data.
- Same email in two Sites means two separate customers.

## 4. Customer Identity

Within one Site:

```text
site_id + normalized_email = one customer
```

Email is case-insensitive.

XBoard also has a stable user id:

```text
site_id + xboard_user_id = one XBoard identity
```

The actual XBoard user id field name is mapped during implementation according to the real XBoard response.

### Identity Priority

When a customer opens the widget:

1. Try XBoard trusted identity/profile.
2. If XBoard identity exists, enter support directly.
3. Otherwise check existing valid `customer_token`.
4. Otherwise require email verification.

Unauthenticated users cannot:

- send text
- upload images
- start emergency calls

### XBoard Identity

The XBoard page is treated as trusted for V1.

The expected flow follows the same idea as Crisp user data injection:

```text
XBoard logged-in page
  -> page/widget has current user info or calls /api/v1/user/info
  -> widget sends identity/profile to Support Backend
  -> Support Backend finds/creates customer
  -> binds xboard_user_id
  -> updates xboard profile snapshot
  -> issues customer_token
  -> opens conversation
```

The widget login path refreshes XBoard profile immediately. It is not split into separate login and profile refresh calls.

Expected profile fields:

- `xboard_user_id` mapped from actual XBoard response
- email
- plan / subscription name
- expire time
- used traffic
- all traffic

XBoard API reference candidate:

- `/api/v1/user/info`

The exact mapping must be verified against the deployed XBoard version during implementation.

### XBoard Email Change

If `xboard_user_id` is unchanged but email changes:

- keep the same customer
- update email
- handle any collision by customer merge/alias

### Customer Merge

Merge can happen when:

- a visitor email identity later logs in through XBoard with same email
- XBoard user changes email to another existing visitor email

Merge rules:

- pick one canonical customer
- preserve old customer ids through aliases/redirects
- keep old customer tokens working
- merge recent three-month messages by timestamp
- merge unread/read state conservatively
- merge identities
- do not hard delete the old customer row in a way that breaks old devices

## 5. Customer Token

After email verification or XBoard login, issue a high-entropy random server-side token.

Rules:

- no fixed expiration
- revocable by server
- not a permanent JWT
- clearing cookies or changing browsers/devices requires re-verification unless XBoard identity is available
- knowing `site_id` must never allow token forgery

## 6. Email Verification

Rules:

- 6 digits
- valid for 10 minutes
- resend allowed after 60 seconds
- 5 wrong attempts invalidate the current code
- rate limit by IP
- rate limit by email

The exact rate limits are config values.

## 7. Conversation Model

A customer has one long-lived conversation per Site.

It is not a ticket system.

All support interactions across time stay in the same conversation, with messages older than three months cleaned up per-message.

## 8. Message Model

All messages are stored before delivery.

Minimum fields:

- `message_id`
- `site_id`
- `conversation_id`
- `sender_type` (`customer`, `agent`)
- `sender_id`
- `seq`
- `type` (`text`, `image`)
- `content`
- `attachment_id`
- `created_at`

WebSocket is a transport only, never the storage layer.

## 9. Image Messages

Both customers and agents can send images.

Allowed:

- jpg
- jpeg
- png
- webp
- <= 10 MB

Rejected:

- svg
- html
- exe
- zip
- pdf
- every unlisted file type

The server must validate real MIME/file content, not only file extension.

Upload flow:

```text
HTTP upload
  -> validate MIME and size
  -> store original locally
  -> create thumbnail
  -> create image attachment
  -> message references attachment_id
```

Image bytes must not be sent through WebSocket.

## 10. Read State

### Customer READ

Agent-to-customer messages support customer READ state.

READ is sent only when the customer likely really saw the message:

- page visible
- support widget/chat open
- message enters visible chat viewport

WebSocket delivery alone is `DELIVERED`, not `READ`.

Customer READ is account-level: if any customer device sees the message, it is READ for that customer.

Agent App can display customer READ.

### Agent READ

Customer-to-agent read state can be recorded internally for:

- unread counts
- red dots
- sorting

This state is never shown to the customer.

## 11. Event Model

Everything Android Agent needs to sync becomes an event.

Examples:

- `MESSAGE_CREATED`
- `MESSAGE_READ`
- `EMERGENCY_STARTED`
- `EMERGENCY_ACCEPTED`
- `EMERGENCY_CANCELLED`
- `EMERGENCY_EXPIRED`
- `CUSTOMER_BLOCKED`
- `CUSTOMER_UNBLOCKED`

Event fields:

- `event_id`
- `site_id`
- global or agent-visible `seq`
- `type`
- `payload`
- `created_at`

Android receives events through:

- WebSocket
- fixed `/sync` every 3 seconds

Both paths feed one idempotent event handler.

Client dedupes by `event_id` and checks gaps by `seq`.

If the client receives `1001`, then `1003`, it immediately calls `/sync?after_seq=1001` to fetch missing `1002`.

## 12. Agent Android Reliability

V1 does not use any third-party Push.

Agent App after login runs a long-lived Foreground Service.

The service keeps:

- WebSocket connection
- fixed 3-second HTTP `/sync`
- reconnect loop
- event ACK
- normal message notification
- emergency ringtone logic

The 3-second sync interval is fixed and does not depend on WebSocket health.

This means:

```text
WebSocket normal -> still sync every 3 seconds
WebSocket broken -> still sync every 3 seconds
```

The system must accept the Android reality: if Android fully kills the app/service, WebSocket and `/sync` both stop. The app must guide agents to allow:

- notifications
- background running
- battery optimization disabled/unrestricted
- auto-start
- vendor-specific background permissions

## 13. Agent Accounts and Devices

V1 has no Web agent console.

The first agent/admin account is created during deployment/init.

Agent Android login uses:

- email or username
- password

One agent account can log in on multiple Android devices.

Each device stores independently:

- `device_id`
- login token
- `last_sync_at`
- WebSocket status
- foreground/background state
- `accept_emergency`

## 14. Emergency Duty

Normal messages and emergency duty are separate.

Normal messages are always received if the Agent App is alive.

`accept_emergency` only controls emergency calls.

Daily schedule:

- timezone: Asia/Shanghai
- 08:00 sets `accept_emergency=true`
- 22:00 does nothing
- agents can manually start/stop emergency duty
- stopping duty does not stop Foreground Service, WebSocket, `/sync`, or normal message notifications

Emergency button is enabled for customer only when:

```text
exists agent_device where
  accept_emergency = true
  and last_sync_at within 60 seconds
```

If no eligible device exists, the emergency button is gray but clickable to show a reason.

## 15. Emergency Call State Machine

Emergency calls are not voice calls or VoIP.

They are high-priority alerts that push agents into text/image chat.

States:

- `RINGING`
- `ACCEPTED`
- `CANCELLED`
- `EXPIRED`

Rules:

- Customer must be authenticated.
- Customer clicks emergency call and enters full-screen calling UI.
- No return-to-chat button during ringing.
- Customer can only cancel.
- Cancel sets `CANCELLED` and sends `STOP_RING` to agents.
- If no agent responds within 3 minutes, set `EXPIRED`.
- After expiry, customer can immediately call again. No cooldown.
- Multiple emergency calls play one ringtone set, not multiple overlapping sounds.
- Agent App shows count, for example `3 emergency calls`.
- If Agent App is foreground and a new emergency call starts, it is immediately `ACCEPTED`; no ringtone.
- If Agent App moves from background/lockscreen to foreground while calls are `RINGING`, all current ringing calls become `ACCEPTED`.
- `ACCEPTED` stops ringtone on all agent devices.
- `ACCEPTED` does not create exclusive chat ownership.
- Any agent can still enter and reply to the conversation.
- Customer sees `客服已接通` and manually taps return-to-chat.

All transitions must be server-side and race-safe.

Examples of races:

- customer cancels while agent foregrounds app
- expiry worker runs while agent accepts
- two agents accept at nearly the same time

Only one final state transition can win.

## 16. Notifications

### Normal Customer Message

Every Customer-to-Agent normal message triggers system default notification sound and vibration on every logged-in, alive agent device.

This applies even if:

- app is foreground
- app is background
- agent is currently viewing this same customer
- `accept_emergency=false`

Ten customer messages produce ten normal notifications.

### Emergency

Emergency uses a continuous phone-like ringtone until stopped by:

- accepted
- cancelled
- expired

## 17. Postal Email

Postal SMTP is used only for Agent-to-Customer unread reminders.

Customer-to-Agent messages never send email to agents.

Email is asynchronous:

```text
agent message stored
  -> realtime push to customer
  -> create/update email pending batch
  -> API returns
  -> worker sends through Postal later
```

Postal failure must not block chat APIs.

### Send Rule

If an Agent-to-Customer message is still not customer READ after 5 minutes, send one email batch.

Batch rule:

- first unread agent message starts the 5-minute timer
- later unread agent messages before send time are merged into the same batch
- before SMTP send, worker checks READ again
- if read, skip
- once sent, messages in that batch are marked email-notified
- changing browser/device/re-verifying email never resends old notified messages
- later new agent messages can create a new email batch

### Email Content

Sender display name is dynamic by Site:

- `寒山云客服`
- `锐雯加速客服`

Content:

- unread text summary
- image messages displayed as `[图片消息]`
- button: `查看客服消息`
- note: do not reply directly to this email

Email return target:

- no fixed Site return URL
- when customer opens support, record `last_support_entry`
- if web, record current page URL
- email button returns to that URL
- if last entry is Android App, email says to open the App; no deep link required in V1

## 18. Blocked Customers

Agents can block/unblock customers.

Blocked customers cannot:

- send text
- upload images
- start emergency calls

Customer-facing text must be neutral:

```text
当前无法使用在线客服，请通过其他联系方式联系我们。
```

Do not show `you are blocked`.

Agents can still view history.

## 19. Data Retention

Customer accounts are permanent.

Text and image messages roll for three months by message timestamp:

```text
message.created_at < now - 3 months
```

Cleanup deletes:

- messages
- image originals
- thumbnails
- direct message-linked records without retention value

Cleanup does not delete:

- customers
- sites
- identities
- blocked state
- required statistics
- required email-notification dedupe data

Emergency logs and security logs can have separate retention policies.

## 20. Proposed MySQL Tables

### sites

- `id` bigint pk
- `site_key` varchar unique, public deployment id
- `name` varchar
- `created_at`
- `updated_at`

### customers

- `id` bigint pk
- `site_id` bigint
- `normalized_email` varchar
- `email_original` varchar
- `xboard_user_id` varchar null
- `canonical_customer_id` bigint null
- `blocked` bool
- `last_support_entry_type` enum(`web`,`app`) null
- `last_support_entry_url` text null
- `created_at`
- `updated_at`
- `last_active_at`

Unique considerations:

- unique active/canonical `(site_id, normalized_email)`
- unique active/canonical `(site_id, xboard_user_id)` when non-null
- aliases must allow redirects after merge

### customer_aliases

- `id`
- `site_id`
- `from_customer_id`
- `to_customer_id`
- `reason`
- `created_at`

### customer_tokens

- `id`
- `site_id`
- `customer_id`
- `token_hash`
- `device_label`
- `revoked_at`
- `created_at`
- `last_used_at`

### xboard_profiles

- `id`
- `site_id`
- `customer_id`
- `xboard_user_id`
- `email`
- `plan`
- `expire_time`
- `used_traffic`
- `all_traffic`
- `raw_json`
- `profile_updated_at`
- `created_at`
- `updated_at`

### agents

- `id`
- `username`
- `email`
- `password_hash`
- `created_at`
- `updated_at`

### agent_devices

- `id`
- `agent_id`
- `device_id`
- `token_hash`
- `last_sync_at`
- `ws_connected`
- `app_foreground`
- `accept_emergency`
- `created_at`
- `updated_at`

### conversations

- `id`
- `site_id`
- `customer_id`
- `last_message_id`
- `last_message_at`
- `created_at`
- `updated_at`

Unique:

- `(site_id, customer_id)`

### messages

- `id`
- `site_id`
- `conversation_id`
- `sender_type`
- `sender_customer_id` null
- `sender_agent_id` null
- `seq`
- `type`
- `content`
- `attachment_id` null
- `customer_read_at` null
- `agent_read_at` null or separate receipt table
- `email_notified_at` null
- `created_at`

### attachments

- `id`
- `site_id`
- `uploader_type`
- `uploader_id`
- `mime_type`
- `size_bytes`
- `storage_key`
- `thumbnail_key`
- `created_at`

### events

- `id`
- `event_id` unique
- `seq` unique
- `site_id`
- `type`
- `payload_json`
- `created_at`

### emergency_calls

- `id`
- `site_id`
- `customer_id`
- `conversation_id`
- `status`
- `accepted_by_agent_id` null
- `accepted_by_device_id` null
- `created_at`
- `accepted_at`
- `cancelled_at`
- `expired_at`

### email_batches

- `id`
- `site_id`
- `customer_id`
- `conversation_id`
- `status` (`pending`,`sent`,`skipped`,`failed`)
- `send_after`
- `sent_at`
- `return_entry_type`
- `return_url`
- `retry_count`
- `last_error`
- `created_at`
- `updated_at`

### email_batch_messages

- `batch_id`
- `message_id`

### verification_codes

- `id`
- `site_id`
- `normalized_email`
- `code_hash`
- `attempt_count`
- `expires_at`
- `used_at`
- `invalidated_at`
- `created_ip`
- `created_at`

## 21. Redis Use

Redis can store:

- recent agent presence
- WebSocket node mapping
- recent event stream/cache
- verification rate limit counters
- emergency realtime locks
- distributed locks
- email job queue

MySQL remains the source of truth for important durable state.

## 22. API Surface Draft

### Widget / Customer

- `GET /widget.js`
- `GET /widget/frame?site=...`
- `POST /api/customer/xboard-login`
- `POST /api/customer/email/send-code`
- `POST /api/customer/email/verify`
- `GET /api/customer/me`
- `GET /api/customer/conversation`
- `POST /api/customer/messages`
- `POST /api/customer/images`
- `POST /api/customer/read`
- `GET /api/customer/emergency/status`
- `POST /api/customer/emergency/start`
- `POST /api/customer/emergency/cancel`
- `GET /ws/customer`

### Agent

- `POST /api/agent/login`
- `POST /api/agent/logout`
- `GET /api/agent/sync?after_seq=...`
- `POST /api/agent/events/ack`
- `GET /api/agent/customers`
- `GET /api/agent/conversations/{id}`
- `POST /api/agent/conversations/{id}/messages`
- `POST /api/agent/images`
- `POST /api/agent/messages/read`
- `POST /api/agent/emergency/{id}/accept`
- `POST /api/agent/devices/foreground`
- `POST /api/agent/devices/emergency-duty`
- `POST /api/agent/customers/{id}/block`
- `POST /api/agent/customers/{id}/unblock`
- `GET /ws/agent`

### CLI/Admin

No Web admin in V1. Use `supportctl`:

- `supportctl init-agent`
- `supportctl site create`
- `supportctl site list`
- `supportctl site code`
- `supportctl status`

## 23. Security

Minimum requirements:

- HTTPS/WSS everywhere
- high-entropy customer tokens
- high-entropy agent tokens
- password hash with a modern password hashing algorithm
- email verification rate limits
- agent login rate limits
- real MIME validation for images
- randomized storage keys
- path traversal protection
- SQL parameterization
- Site isolation checks on every customer-scoped query
- customers cannot read other customers' chats
- agent API authentication
- upload size limits
- WebSocket authentication
- `/sync` authentication
- duplicate email prevention
- idempotent event handling

## 24. Docker Compose Deployment

V1 will deploy with:

- `support-api`
- `support-worker`
- `mysql:8`
- `redis`
- local upload volume
- optional reverse proxy configuration

The repository will include:

- `.env.example`
- `docker-compose.yml`
- `supportctl`
- database migrations
- deployment docs

## 25. Development Phases

1. Technical design and repository skeleton.
2. Go backend foundation: config, DB, Redis, auth, migrations.
3. Site and CLI management.
4. Customer identity and XBoard login.
5. Messaging, events, WebSocket, `/sync`, READ.
6. Web widget.
7. Android Agent app.
8. Emergency call state machine.
9. Image upload and thumbnailing.
10. Postal email batch worker.
11. Cleanup jobs.
12. Reliability and race tests.

## 26. Test Plan

Must test:

- WebSocket disconnect and `/sync` recovery.
- duplicate events from WebSocket + `/sync`.
- out-of-order event arrival.
- missing seq gap recovery.
- Wi-Fi to mobile network switch.
- Android lockscreen and background runtime.
- Foreground Service survival.
- customer multi-device sync.
- agent multi-device sync.
- READ race with email worker.
- Postal failure retry.
- emergency cancel/accept race.
- emergency expire/accept race.
- multiple emergency calls.
- app foreground auto-accept.
- three-month cleanup.
- customer merge.
- XBoard email change.
- same email in different Sites remains isolated.

## 27. Current Decisions From Product Owner

- XBoard-transmitted identity/profile is trusted.
- Widget login should refresh/import XBoard profile immediately using available XBoard user info, especially `/api/v1/user/info` data.
- Agent Android login uses email/username + password.
- Local image storage path can be implementation-defined.
- Postal sender display name uses Site name dynamically.
- Email shows unread text summary; image messages display `[图片消息]`.
- Normal message notification uses Android system default sound.
- Emergency uses continuous phone-like ringtone.
- Site creation and deployment code are handled by interactive CLI, not Web admin.
- The system must support one-command deployment to arbitrary servers.
