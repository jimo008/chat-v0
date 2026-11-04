# Server auto reply

Global for all registered sites; disabled by default. The phone app needs no
new settings and can be closed while the worker handles replies.

```
cd /root/chat-v0
./supportctl autoreply set
./supportctl autoreply on
./supportctl autoreply cooldown 20
./supportctl autoreply status
./supportctl autoreply off
```

`set` accepts multiple lines. Finish with a line containing only `.`. Enter
only the body. It does not enable the feature automatically. Run it again
to change the text without restarting services. Empty text cannot be enabled.
Text is limited to 2,000 Unicode characters. The fixed footer is appended
after a blank line:

> 此条消息由系统自动回复，有问题请留言，客服上线为您解决！

Only newly submitted customer text and image messages schedule replies.
Opening a page, replaying historical messages, calls, or automatic messages do
not schedule replies. Dispatch waits at least three seconds. Messages received
within a conversation's cooldown are discarded, not queued for later
delivery. The cooldown starts when an automatic message is actually sent.
It defaults to one minute; `./supportctl autoreply cooldown 20` changes it to
20 minutes. The supported range is 1-1440 whole minutes. Changes apply immediately
and preserve the body, enabled state, and last-send timestamps. This is not a
periodic timer: without a new incoming message, expiry sends nothing. The
initial three-second delay does not change.
Human replies sent after the triggering customer message cancel that pending
reply. Marking a message read without replying does not cancel it.

The database stores the pending jobs, cooldown, and settings. Restarting the
worker does not reset cooldown or duplicate messages. Multiple workers share
database locks. Turning the feature off invalidates pending jobs; turning it
back on cannot resurrect them. Editing the body applies to still-pending jobs.
After enabling, only new customer messages trigger replies; old conversations
are not scanned or answered retroactively.

Automatic messages have `is_auto_reply=true` and no human sender ID. They do not
accept calls, mark customer messages read, or create unread-email batches.
They travel through the ordinary chat event stream so existing clients display
them, including the fixed footer. SMTP runs separately and cannot delay the
three-second reply scheduler. Build server images in GitHub Actions, not on the
production server.
