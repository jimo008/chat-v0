import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

function fixture(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'autoreply-cli-test-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  fs.copyFileSync(new URL('../supportctl', import.meta.url), path.join(dir, 'supportctl'));
  fs.writeFileSync(path.join(dir, '.env'), 'APP_BASE_URL=https://support.invalid\nADMIN_TOKEN=test-admin\n', { mode: 0o600 });
  const bin = path.join(dir, 'bin');
  fs.mkdirSync(bin);
  fs.writeFileSync(path.join(bin, 'curl'), `#!/usr/bin/env node
const fs = require('node:fs');
const args = process.argv.slice(2);
const i = args.indexOf('--data');
const raw = args.includes('--data-binary') ? fs.readFileSync(0, 'utf8') : (i >= 0 ? args[i + 1] : '{}');
process.stdout.write(JSON.stringify({args, data:JSON.parse(raw)}));
`, { mode: 0o700 });
  return (mode, input = '') => {
    const result = spawnSync('bash', [path.join(dir, 'supportctl'), 'autoreply', mode], {
      input, encoding: 'utf8', env: { ...process.env, PATH: [bin, path.dirname(process.execPath), process.env.PATH].join(path.delimiter) },
    });
    assert.equal(result.status, 0, result.stderr);
    const start = result.stdout.indexOf('{"args":');
    assert.ok(start >= 0, result.stdout);
    return JSON.parse(result.stdout.slice(start));
  };
}

test('multiline auto-reply content is safely JSON encoded without shell expansion', (t) => {
  const run = fixture(t);
  const body = '第一行 "quotes" \\ path\n第二行 $(not_a_command) <script>literal</script>';
  const result = run('set', body + '\n.\n');
  assert.deepEqual(result.data, { content: body });
  assert.ok(result.args.includes('PATCH'));
  assert.ok(result.args.includes('https://support.invalid/api/v1/admin/auto-reply'));
  assert.ok(!result.args.includes(body));
});

test('on and off only change the enabled flag, preserving the configured body', (t) => {
  const run = fixture(t);
  assert.deepEqual(run('on').data, { enabled: true });
  assert.deepEqual(run('off').data, { enabled: false });
  assert.ok(run('status').args.includes('GET'));
});
