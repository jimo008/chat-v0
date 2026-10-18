# Agent device pairing

Build and sign APKs in GitHub Actions. Deploy prebuilt server images only.
The release signing material is stored in Actions Secrets, never in Git.

## Rollout

1. Update the server first. Both request-proof enforcement flags default to
   false so existing logged-in phones continue working during the upgrade.
   All new logins require the fixed code regardless of those flags. Missing
   codes are classified for IP banning, so upgrade old apps before logging in.
2. Download `support-agent-latest.apk` from the `android-latest` release.
   The first private-signed release cannot replace the old debug-signed APK:
   uninstall the old test app before installing this release. Server messages
   are retained; local caches will be rebuilt.
3. On the server run `./supportctl device pair-code`. Enter and confirm your
   own fixed code at the hidden prompts (8-64 bytes, preferably 16 or more).
   It is reusable across phones and does not expire automatically. Run the
   same command to change it; the old value immediately stops working for new
   logins. The server stores only a salted bcrypt hash, never returns the code,
   and does not embed it in the APK. The CLI keeps it out of shell history and
   curl process arguments. Do not send it to a developer or paste it in chat.
4. Enter the fixed code, HTTPS server address, and agent credentials in the
   new app. All manual logins, including previously paired phones, require
   the current fixed code. Each phone also generates its own P-256 Android
   Keystore key; the private device key is not uploaded. The app does not
   persist the fixed code. Changing the code does not revoke existing sessions
   or device keys; it controls new logins, not remote logout.
5. Test messages and emergency notifications on every phone. Run
   `./supportctl security status` to inspect paired and recently unpaired devices.
6. Only after upgrading all active phones, run `./supportctl security enable`.
   This refuses to enable while recently active unpaired devices remain.
   `./supportctl security disable` disables request-proof enforcement, but does
   not remove the fixed-code requirement for new logins.

Keep valid HTTPS certificate chains and automatic phone time enabled. Do not
disable certificate verification. Clock skew and nonce replay errors do not
qualify for automatic IP banning. A paired phone signs HTTP requests and the
WebSocket handshake; signing proves possession of its device key, not APK
binary attestation.

## Customer sites

New embed codes include `site_key` in the Widget script URL. Existing embeds
with `data-site` load a small bootstrap that adds it automatically. Requests
carry `X-Chat-Site-Key`; image URLs carry `site_key`. The server checks the site
exists and matches the authenticated customer token. The key is public, not an
identity secret. XBoard identity still requires an independently trusted source.

Old open pages with tokens created before enforcement receive a reload error
instead of being classified as attackers. `/id/` is an independent Nginx proxy
and is outside these application authentication gates.

The server emits `X-Chat-Security-Reject` only for explicit site/device policy
violations. A reverse proxy may use it to feed the dedicated Fail2ban log.
Do not ban arbitrary 400/401/403 responses: expired sessions, invalid passwords,
clock skew, wrong fixed codes, and temporary storage errors can occur for
legitimate clients. Missing pairing codes are an explicit policy violation.
