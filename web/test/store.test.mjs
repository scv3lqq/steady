import assert from 'node:assert/strict';
import { test } from 'node:test';
import { Store, RENAME_DEBOUNCE_MS } from '../dist/store.js';

const EMPTY = { habits: [], view: 'week', accent: '#4FA872' };
const ALICE = {
  habits: [{ id: 'alice-habit', name: 'Alice private habit', done: ['2026-11-03'] }],
  view: 'year',
  accent: '#E07BB0',
};
const BOB = {
  habits: [{ id: 'bob-habit', name: 'Bob private habit', done: [] }],
  view: 'month',
  accent: '#5B8CEB',
};
const A_BINDING = 'a'.repeat(64);
const B_BINDING = 'b'.repeat(64);

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

function response(status = 204, body) {
  return { ok: status >= 200 && status < 300, status, json: async () => body };
}

async function settle() {
  // Fetch, response JSON and queue drain each occupy separate promise turns.
  for (let turn = 0; turn < 20; turn += 1) await Promise.resolve();
}

function fixture(t) {
  const originalFetch = globalThis.fetch;
  const originalChannel = globalThis.BroadcastChannel;
  const channels = [];
  class Channel {
    constructor(name) { this.name = name; this.listeners = []; channels.push(this); }
    addEventListener(type, listener) { if (type === 'message') this.listeners.push(listener); }
    postMessage(data) {
      fixtureState.messages.push(data);
      for (const channel of channels) {
        if (channel !== this && channel.name === this.name) channel.receive(data);
      }
    }
    receive(data) {
      this.onmessage?.({ data });
      for (const listener of this.listeners) listener({ data });
    }
    close() {}
  }
  const fixtureState = {
    username: 'alice', binding: A_BINDING, privateState: ALICE,
    sessionStatus: 200, calls: [], messages: [], override: undefined,
  };
  globalThis.BroadcastChannel = Channel;
  globalThis.fetch = async (path, init = {}) => {
    const call = { path, init };
    fixtureState.calls.push(call);
    const overridden = fixtureState.override?.(call);
    if (overridden !== undefined) return overridden;
    if (path === '/api/session') {
      return response(fixtureState.sessionStatus, {
        username: fixtureState.username, binding: fixtureState.binding,
      });
    }
    if (path === '/api/state') return response(200, fixtureState.privateState);
    return response();
  };
  t.after(() => {
    globalThis.fetch = originalFetch;
    globalThis.BroadcastChannel = originalChannel;
  });
  return {
    ...fixtureState,
    server: fixtureState,
    invalidate() {
      for (const channel of channels) channel.receive({ type: 'changed' });
    },
  };
}

function mutations(server) {
  return server.calls.filter(({ path, init }) =>
    path !== '/api/session' && path !== '/api/state' && path !== '/api/login' &&
    path !== '/api/logout' && init.method !== 'GET');
}

async function loaded(t) {
  const f = fixture(t);
  const store = new Store(() => {});
  await store.load();
  return { ...f, store };
}

function assertAnonymous(store) {
  assert.equal(store.auth.status, 'anonymous');
  assert.deepEqual(store.state, EMPTY);
}

test('initial session check precedes private state and an initial 401 keeps defaults', async (t) => {
  const { server } = fixture(t);
  server.sessionStatus = 401;
  const store = new Store(() => {});
  assert.equal(store.auth.status, 'checking');
  await store.load();
  assertAnonymous(store);
  assert.deepEqual(server.calls.map(({ path }) => path), ['/api/session']);
});

test('login fetches authenticated state with the captured session binding', async (t) => {
  const { server } = fixture(t);
  server.sessionStatus = 401;
  const store = new Store(() => {});
  await store.load();
  server.sessionStatus = 200;
  await store.login('alice', 'a private password');
  assert.equal(store.auth.status, 'authenticated');
  assert.equal(store.auth.busy, false);
  assert.equal(store.state.habits[0].name, 'Alice private habit');
  const login = server.calls.find(({ path }) => path === '/api/login');
  assert.deepEqual(JSON.parse(login.init.body), { username: 'alice', password: 'a private password' });
  const state = server.calls.find(({ path }) => path === '/api/state');
  assert.equal(new Headers(state.init.headers).get('X-Steady-Session'), A_BINDING);
  assert.equal(JSON.stringify(store.auth).includes('a private password'), false);
});

test('failed login exposes a generic error and no private state', async (t) => {
  const { server } = fixture(t);
  server.sessionStatus = 401;
  server.override = ({ path }) => path === '/api/login' ? response(401, { error: 'sensitive detail' }) : undefined;
  const store = new Store(() => {});
  await store.load();
  await store.login('alice', 'wrong private password');
  assertAnonymous(store);
  assert.equal(store.auth.busy, false);
  assert.equal(typeof store.auth.error, 'string');
  assert.ok(store.auth.error.length > 0);
  assert.equal(store.auth.error.includes('sensitive detail'), false);
});

test('session expiry during a mutation clears habits and nondefault settings', async (t) => {
  const { store, server } = await loaded(t);
  server.override = ({ path }) => path.includes('/days/') ? response(401) : undefined;
  store.toggleDay('alice-habit', '2026-11-02');
  await settle();
  assertAnonymous(store);
  assert.equal(server.calls.filter(({ path }) => path === '/api/state').length, 1);
});

test('logout clears private state immediately and prevents anonymous edits', async (t) => {
  const { store, server } = await loaded(t);
  const logout = deferred();
  server.override = ({ path }) => path === '/api/logout' ? logout.promise : undefined;
  const done = store.logout();
  await settle();
  assertAnonymous(store);
  store.setView('year');
  store.setAccent('#E07BB0');
  store.addHabit('Do not retain');
  await settle();
  assert.deepEqual(store.state, EMPTY);
  assert.equal(mutations(server).length, 0);
  logout.resolve(response());
  await done;
});

test('late GET response cannot restore private state after auth invalidation', async (t) => {
  const { store, server, invalidate } = await loaded(t);
  const late = deferred();
  server.override = ({ path }) => path === '/api/state' ? late.promise : undefined;
  const loading = store.load();
  await settle();
  invalidate();
  late.resolve(response(200, ALICE));
  await loading;
  assertAnonymous(store);
});

test('late JSON body cannot overwrite a newly authenticated user', async (t) => {
  const { store, server, invalidate } = await loaded(t);
  const body = deferred();
  let intercepted = false;
  server.override = ({ path }) => {
    if (path !== '/api/state' || intercepted) return undefined;
    intercepted = true;
    return { ok: true, status: 200, json: () => body.promise };
  };
  const loading = store.load();
  await settle();
  invalidate();
  server.username = 'bob'; server.binding = B_BINDING; server.privateState = BOB;
  // Session discovery can finish while the old body ignores its aborted signal.
  await store.load();
  assert.equal(store.state.habits[0].name, 'Bob private habit');
  body.resolve(ALICE);
  await loading;
  assert.equal(store.state.habits[0].name, 'Bob private habit');
});

test('old queued mutation is skipped before dispatch after a cookie switch', async (t) => {
  const { store, server, invalidate } = await loaded(t);
  const first = deferred();
  server.override = ({ path }) => path.includes('/days/') ? first.promise : undefined;
  store.toggleDay('alice-habit', '2026-11-02');
  store.setView('month');
  await settle();
  assert.equal(mutations(server).length, 1);
  invalidate();
  first.resolve(response());
  await settle();
  assert.equal(mutations(server).length, 1, 'queued settings must not use the replacement cookie');
  assertAnonymous(store);
});

test('stale failures cannot decrement new queue counters or trigger a new-user reload', async (t) => {
  const { store, server, invalidate } = await loaded(t);
  const old = deferred();
  server.override = ({ path }) => path.includes('/days/') ? old.promise : undefined;
  store.toggleDay('alice-habit', '2026-11-02');
  await settle();
  invalidate();
  server.username = 'bob'; server.binding = B_BINDING; server.privateState = BOB;
  await store.load();
  const baseline = server.calls.filter(({ path }) => path === '/api/state').length;
  const next = deferred();
  server.override = ({ path }) => path === '/api/settings' ? next.promise : undefined;
  store.setView('year');
  await settle();
  old.reject(new Error('old network failure'));
  await settle();
  assert.equal(server.calls.filter(({ path }) => path === '/api/state').length, baseline);
  next.resolve(response(500));
  await settle();
  assert.equal(server.calls.filter(({ path }) => path === '/api/state').length, baseline + 1);
  assert.equal(store.state.habits[0].name, 'Bob private habit');
});

test('logout clears pending rename, debounce timer and pagehide flush', async (t) => {
  const { store, server } = await loaded(t);
  store.renameHabit('alice-habit', 'Unsent private name');
  await store.logout();
  store.flushPending();
  await new Promise((resolve) => setTimeout(resolve, RENAME_DEBOUNCE_MS + 20));
  assertAnonymous(store);
  assert.equal(mutations(server).length, 0);
});

test('pagehide sends current pending rename with its signal and binding', async (t) => {
  const { store, server } = await loaded(t);
  store.renameHabit('alice-habit', 'Saved before leaving');
  store.flushPending();
  await settle();
  const [rename] = mutations(server);
  assert.equal(rename.init.keepalive, true);
  assert.equal(new Headers(rename.init.headers).get('X-Steady-Session'), A_BINDING);
  assert.ok(rename.init.signal instanceof AbortSignal);
  assert.deepEqual(JSON.parse(rename.init.body), { name: 'Saved before leaving' });
});

test('optimistic failure reload waits until the current queue drains', async (t) => {
  const { store, server } = await loaded(t);
  const first = deferred();
  const second = deferred();
  let sent = 0;
  server.override = ({ path }) => path === '/api/settings' ? (++sent === 1 ? first.promise : second.promise) : undefined;
  store.setView('month');
  store.setAccent('#3FB8B0');
  await settle();
  assert.equal(store.state.view, 'month');
  assert.equal(store.state.accent, '#3FB8B0');
  first.resolve(response(500));
  await settle();
  assert.equal(server.calls.filter(({ path }) => path === '/api/state').length, 1);
  second.resolve(response());
  await settle();
  assert.equal(server.calls.filter(({ path }) => path === '/api/state').length, 2);
  assert.equal(store.state.view, 'year');
});

test('auth requests serialize and busy stays set until the last operation settles', async (t) => {
  const { store, server } = await loaded(t);
  const login = deferred();
  const logout = deferred();
  server.override = ({ path }) => path === '/api/login' ? login.promise : path === '/api/logout' ? logout.promise : undefined;
  const first = store.login('alice', 'a private password');
  const second = store.logout();
  await settle();
  assert.equal(store.auth.busy, true);
  assert.deepEqual(server.calls.filter(({ path }) => path === '/api/login' || path === '/api/logout').map(({ path }) => path), ['/api/login']);
  login.resolve(response());
  await first;
  await settle();
  assert.equal(store.auth.busy, true);
  assert.deepEqual(server.calls.filter(({ path }) => path === '/api/login' || path === '/api/logout').map(({ path }) => path), ['/api/login', '/api/logout']);
  logout.resolve(response());
  await second;
  assert.equal(store.auth.busy, false);
  assertAnonymous(store);
});

test('login waits for aborted data work to settle before changing the cookie', async (t) => {
  const { store, server } = await loaded(t);
  const pending = deferred();
  server.override = ({ path }) => path.includes('/days/') ? pending.promise : undefined;
  store.toggleDay('alice-habit', '2026-11-02');
  await settle();
  const done = store.login('bob', 'another private password');
  await settle();
  const mutation = mutations(server)[0];
  assert.equal(mutation.init.signal.aborted, true);
  assert.equal(server.calls.some(({ path }) => path === '/api/login'), false);
  pending.reject(new DOMException('Aborted', 'AbortError'));
  await done;
  assert.equal(server.calls.some(({ path }) => path === '/api/login'), true);
});

test('BroadcastChannel invalidates another tab without sharing identity or credentials', async (t) => {
  const { store: first, server } = await loaded(t);
  const second = new Store(() => {});
  await second.load();
  await first.logout();
  assertAnonymous(second);
  assert.ok(server.messages.length > 0);
  const payload = JSON.stringify(server.messages);
  for (const secret of ['alice', A_BINDING, 'password', 'Alice private habit']) {
    assert.equal(payload.includes(secret), false);
  }
});

test('private mutations bind to the captured session even when cross-tab delivery is delayed', async (t) => {
  const { store, server } = await loaded(t);
  // Simulate a cookie replacement in another tab before its broadcast arrives.
  server.username = 'bob'; server.binding = B_BINDING; server.privateState = BOB;
  server.override = ({ path, init }) => {
    if (!path.includes('/days/')) return undefined;
    const binding = new Headers(init.headers).get('X-Steady-Session');
    assert.equal(binding, A_BINDING);
    return response(binding === server.binding ? 204 : 401);
  };
  store.toggleDay('alice-habit', '2026-11-02');
  await settle();
  assertAnonymous(store);
  assert.equal(server.calls.filter(({ path }) => path === '/api/state').length, 1);
});

test('another tab login invalidates an in-flight mutation and skips its queued successor', async (t) => {
  const { store: first, server } = await loaded(t);
  const second = new Store(() => {});
  await second.load();
  const pending = deferred();
  server.override = ({ path }) => path.includes('/days/') ? pending.promise : undefined;
  first.toggleDay('alice-habit', '2026-11-02');
  first.setView('month');
  await settle();
  server.username = 'bob'; server.binding = B_BINDING; server.privateState = BOB;
  await second.login('bob', 'another private password');
  assertAnonymous(first);
  assert.equal(second.state.habits[0].name, 'Bob private habit');
  const [mutation] = mutations(server);
  assert.equal(new Headers(mutation.init.headers).get('X-Steady-Session'), A_BINDING);
  assert.equal(mutation.init.signal.aborted, true);
  pending.resolve(response(401));
  await settle();
  assert.equal(mutations(server).length, 1);
  assertAnonymous(first);
  assert.equal(second.state.habits[0].name, 'Bob private habit');
});

test('a session response without a valid binding cannot expose private state', async (t) => {
  const { server } = fixture(t);
  server.binding = undefined;
  const store = new Store(() => {});
  await store.load();
  assertAnonymous(store);
  assert.equal(server.calls.some(({ path }) => path === '/api/state'), false);
});
