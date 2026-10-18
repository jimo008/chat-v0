# Agent device pairing

Build and sign APKs in GitHub Actions. Deploy prebuilt server images only.
The release signing material is stored in Actions Secrets, never in Git.

## Rollout

1. Update the server first. Both enforcement flags default to false so old
   phones continue working during the upgrade.
2. Download `support-agent-latest.apk` from the `android-latest` release.
   The first private-signed release cannot replace the old debug-signed APK:
   uninstall the old test app before installing this release. Server messages
   are retained; local caches will be rebuilt.
3. On the server run `./supportctl device pair-code`. Enter the code together
   with the HTTPS server address and agent credentials in the new app.
   Each code expires after 20 minutes and can be used once. Generate a separate
   code for each phone. The phone generates its own P-256 Android Keystore key;
   the private device key is not uploaded.
4. Test messages and emergency notifications on every phone. Run
   `./supportctl security status` to inspect paired and recently unpaired devices.
5. Only after upgrading all active phones, run `./supportctl security enable`.
   This refuses to enable while recently active unpaired devices remain.
   `./supportctl security disable` restores legacy-client compatibility.

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
clock skew, and temporary storage errors can occur for legitimate clients.
