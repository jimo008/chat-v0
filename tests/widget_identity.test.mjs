import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const widget = readFileSync(new URL('../internal/http/widget.go', import.meta.url), 'utf8');
const parentScript = widget.match(/fmt\.Fprintf\(w, `([\s\S]*?)`, s\.cfg\.AppBaseURL/)[1].replace(/%s/g, 'https://support.example');
const frameScript = widget.match(/<script>\n([\s\S]*?)\n<\/script>/)[1]
  .replaceAll('__SITE__', 'st_test')
  .replaceAll('__ENTRY_URL__', 'https://xboard.example/');
const identity = (id = 'user-1') => ({ xboard_user_id: id, email: `${id}@example.com`, expire_time: 1791993599 });
const response = (data, status = 200) => ({ ok: status < 400, status, json: async () => data, clone() { return this; } });
const tick = () => new Promise(resolve => setImmediate(resolve));
const deferred = () => {
  let resolve;
  const promise = new Promise(done => { resolve = done; });
  return { promise, resolve };
};

function element() {
  const classes = new Set();
  const listeners = {};
  const result = {
    children: [], dataset: {}, style: {}, value: '', listeners,
    classList: { add: name => classes.add(name), remove: name => classes.delete(name), contains: name => classes.has(name), toggle(name, enabled) { if (enabled) classes.add(name); else classes.delete(name); } },
    addEventListener(name, fn) { listeners[name] = fn; },
    appendChild(child) { this.children.push(child); },
    get lastElementChild() { return this.children.at(-1); },
    set innerHTML(value) { this.children = []; },
    get innerHTML() { return ''; },
    scrollHeight: 100, clientHeight: 100, scrollTop: 0,
  };
  return result;
}

function parentHarness() {
  const posts = [], fetches = [], timers = [], listeners = {};
  let iframe;
  const window = {
    location: { href: 'https://xboard.example/' },
    addEventListener(name, fn) { listeners[name] = fn; },
    fetch(url) { fetches.push(url); return Promise.resolve(response({ status: 'success', data: { uuid: 'user-1', email: 'user-1@example.com', expired_at: 1791993599, transfer_enable: '123', u: 0, plan: { name: 'Basic' } } })); },
    XMLHttpRequest: class {
      listeners = {};
      addEventListener(name, fn) { this.listeners[name] = fn; }
      open() {}
    },
  };
  const document = {
    currentScript: { getAttribute: () => 'st_test' },
    getElementById: () => null,
    createElement() { iframe = element(); iframe.contentWindow = { postMessage: (data, origin) => posts.push({ data, origin }) }; return iframe; },
    body: { appendChild() {} },
  };
  vm.runInNewContext(parentScript, { window, document, URL, console: { info() {} }, setInterval() {}, setTimeout(fn) { timers.push(fn); return timers.length; }, clearTimeout() {} });
  const receive = data => listeners.message({ origin: 'https://support.example', source: iframe.contentWindow, data });
  return { window, iframe, posts, fetches, timers, receive };
}

function frameHarness(harnessOptions = {}) {
  const elements = new Map(), requests = [], posts = [], storage = new Map(), listeners = {};
  if (harnessOptions.token) storage.set('chat_v0_token_st_test', harnessOptions.token);
  const parent = { postMessage: (data, origin) => posts.push({ data, origin }) };
  const window = { parent, addEventListener(name, fn) { listeners[name] = fn; } };
  const document = {
    visibilityState: 'visible',
    getElementById(id) { if (!elements.has(id)) elements.set(id, element()); return elements.get(id); },
    querySelector: () => element(), createElement: () => element(), addEventListener() {},
  };
  const fetch = (url, options) => {
    if (url.startsWith('/api/v1/customer/conversation') && !harnessOptions.deferConversations) return Promise.resolve(response({ messages: [] }));
    const pending = deferred(); requests.push({ url, options, pending }); return pending.promise;
  };
  const instrumented = frameScript.replace('  setOpen(false);', '  window.testSession = { ensureGuestSession: ensureGuestSession, loadConversation: loadConversation };\n  setOpen(false);');
  vm.runInNewContext(instrumented, { window, document, URL, fetch, FormData: class {}, localStorage: { getItem: key => storage.get(key) || null, setItem: (key, value) => storage.set(key, value), removeItem: key => storage.delete(key) }, console: { info() {} }, setInterval() {}, clearInterval() {}, setTimeout() { return 1; }, clearTimeout() {}, requestAnimationFrame() {} });
  function receive(value, overrides = {}) { listeners.message({ source: parent, origin: 'https://xboard.example', data: { type: 'chat-v0:xboard-identity', identity: value }, ...overrides }); }
  return { window, storage, requests, posts, receive };
}

test('identity captured before iframe load is replayed, with no extra XBoard request', async () => {
  const h = parentHarness();
  await h.window.fetch('/api/v1/user/info?t=1');
  await tick();
  assert.equal(h.posts.length, 0);
  h.iframe.listeners.load();
  assert.equal(h.posts.length, 1);
  assert.equal(h.posts[0].data.identity.email, 'user-1@example.com');
  assert.equal(h.posts[0].data.identity.plan, 'Basic');
  assert.equal(h.posts[0].data.identity.used_traffic, 0);
  assert.equal(h.posts[0].data.identity.all_traffic, 123);
  assert.equal(h.fetches.length, 1);
});

test('only the exact user/info endpoint supplies identity', async () => {
  const h = parentHarness();
  h.iframe.listeners.load();
  await h.window.fetch('/other?target=/api/v1/user/info');
  await h.window.fetch('/api/v1/user/info-extra');
  await tick();
  assert.equal(h.posts.length, 0);
  await h.window.fetch('/api/v1/user/info');
  await tick();
  assert.equal(h.posts.length, 1);
});

test('a failed identity sync is retried and cached identity survives a ready handshake', async () => {
  const h = parentHarness();
  await h.window.fetch('/api/v1/user/info');
  await tick();
  h.receive({ type: 'chat-v0:ready' });
  const key = JSON.stringify({ id: 'user-1', email: 'user-1@example.com' });
  h.receive({ type: 'chat-v0:identity-result', key, ok: false });
  assert.equal(h.timers.length, 1);
  h.timers[0]();
  assert.equal(h.posts.length, 2);
});

test('XHR JSON responses provide the same exact-endpoint identity handoff', () => {
  const h = parentHarness();
  h.iframe.listeners.load();
  const xhr = new h.window.XMLHttpRequest();
  xhr.open('GET', '/api/v1/user/info?t=1');
  xhr.responseType = 'json';
  xhr.response = { status: 'success', data: { uuid: 'xhr-user', email: 'xhr@example.com' } };
  xhr.listeners.load();
  assert.equal(h.posts.length, 1);
  assert.equal(h.posts[0].data.identity.email, 'xhr@example.com');
});

test('iframe ignores identity from unrelated windows or origins', () => {
  const h = frameHarness();
  h.receive(identity(), { source: {} });
  h.receive(identity(), { origin: 'https://unrelated.example' });
  assert.equal(h.requests.length, 0);
  h.receive(identity());
  assert.equal(h.requests.length, 1);
});

test('duplicate ready/load delivery creates only one identity login', async () => {
  const h = frameHarness();
  h.receive(identity());
  h.receive(identity());
  assert.equal(h.requests.length, 1);
  h.requests[0].pending.resolve(response({ token: 'signed-in-token', customer: { email: 'user-1@example.com' } }));
  await tick();
  h.receive(identity());
  assert.equal(h.requests.length, 1);
  assert.equal(h.storage.get('chat_v0_token_st_test'), 'signed-in-token');
});

test('late guest login cannot replace an authenticated XBoard session', async () => {
  const h = frameHarness();
  const guest = h.window.testSession.ensureGuestSession();
  assert.equal(h.requests[0].url, '/api/v1/customer/guest-login');
  h.receive(identity());
  h.requests[1].pending.resolve(response({ token: 'signed-in-token', customer: {} }));
  await tick();
  h.requests[0].pending.resolve(response({ token: 'guest-token', customer: {} }));
  assert.equal(await guest, 'signed-in-token');
  assert.equal(h.storage.get('chat_v0_token_st_test'), 'signed-in-token');
});

test('sending while identity login is pending waits instead of creating a guest', async () => {
  const h = frameHarness();
  h.receive(identity());
  const session = h.window.testSession.ensureGuestSession();
  assert.equal(h.requests.length, 1);
  h.requests[0].pending.resolve(response({ token: 'signed-in-token', customer: {} }));
  assert.equal(await session, 'signed-in-token');
  assert.equal(h.requests.length, 1);
});

test('a late guest conversation 401 cannot clear the newly authenticated token', async () => {
  const h = frameHarness({ token: 'old-guest-token', deferConversations: true });
  h.receive(identity());
  h.window.testSession.loadConversation(true);
  const login = h.requests.find(request => request.url.endsWith('/xboard-login'));
  const guestPoll = h.requests.at(-1);
  login.pending.resolve(response({ token: 'signed-in-token', customer: {} }));
  await tick();
  guestPoll.pending.resolve(response({}, 401));
  await tick();
  assert.equal(h.storage.get('chat_v0_token_st_test'), 'signed-in-token');
});

test('rapid account switches keep the newest identity, even when requests finish out of order', async () => {
  const h = frameHarness();
  h.receive(identity('first'));
  h.requests[0].pending.resolve(response({ token: 'first-token', customer: {} }));
  await tick();
  h.receive(identity('second'));
  h.receive(identity('first'));
  assert.equal(h.requests.length, 3);
  h.requests[2].pending.resolve(response({ token: 'first-new-token', customer: {} }));
  await tick();
  h.requests[1].pending.resolve(response({ token: 'second-late-token', customer: {} }));
  await tick();
  assert.equal(h.storage.get('chat_v0_token_st_test'), 'first-new-token');
});
