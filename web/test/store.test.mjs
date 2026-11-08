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

function response(status = 204, body = status === 401 ? { error: 'authentication required' } : undefined) {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    status, headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
  });
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
      return response(fixtureState.sessionStatus, fixtureState.sessionStatus === 200 ? {
        username: fixtureState.username, binding: fixtureState.binding,
      } : undefined);
    }
    if (path !== '/api/login' && path !== '/api/logout' &&
        (fixtureState.sessionStatus !== 200 || new Headers(init.headers).get('X-Steady-Session') !== fixtureState.binding)) {
      return response(401);
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

function activeTimers(t) {
  const originalSet = globalThis.setTimeout;
  const originalClear = globalThis.clearTimeout;
  const active = new Set();
  globalThis.setTimeout = (callback, delay, ...args) => {
    const timer = originalSet(() => {
      active.delete(timer);
      callback(...args);
    }, delay);
    active.add(timer);
    return timer;
  };
  globalThis.clearTimeout = (timer) => {
    active.delete(timer);
    originalClear(timer);
  };
  t.after(() => {
    for (const timer of active) originalClear(timer);
    globalThis.setTimeout = originalSet;
    globalThis.clearTimeout = originalClear;
  });
  return active;
}

function strictBackend(server) {
  const backend = { active: true, cookieBinding: A_BINDING, logoutStatuses: [], revoked: false };
  server.override = ({ path, init }) => {
    if (path === '/api/login') {
      backend.active = true;
      backend.cookieBinding = B_BINDING;
      server.username = 'bob'; server.binding = B_BINDING; server.privateState = BOB;
      return response(204);
    }
    if (path === '/api/session') {
      return backend.active ? response(200, { username: server.username, binding: backend.cookieBinding }) : response(401);
    }
    const authorized = backend.active && new Headers(init.headers).get('X-Steady-Session') === backend.cookieBinding;
    if (path === '/api/logout') {
      const status = !backend.active || authorized ? 204 : 401;
      backend.logoutStatuses.push(status);
      if (status === 204) { backend.active = false; backend.revoked = true; }
      return response(status);
    }
    if (!authorized) return response(401);
    return path === '/api/state' ? response(200, server.privateState) : response(204);
  };
  return backend;
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
  const active = activeTimers(t);
  store.renameHabit('alice-habit', 'Unsent private name');
  assert.equal(active.size, 1, 'rename owns one live debounce resource');
  const leaving = store.logout();
  assert.equal(active.size, 0, 'logout must cancel the resource immediately');
  await leaving;
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
  assert.equal(store.auth.status, 'checking');
  assert.equal(store.auth.error, 'Unable to check your session. Try again.');
  assert.deepEqual(store.state, EMPTY);
  assert.equal(server.calls.some(({ path }) => path === '/api/state'), false);
});

test('strict backend logout revokes the captured session and reload cannot recover it', async (t) => {
  const { server } = fixture(t);
  const backend = strictBackend(server);
  const store = new Store(() => {});
  await store.load();
  await store.logout();
  assert.deepEqual(backend.logoutStatuses, [204]);
  assert.equal(backend.revoked, true);
  assert.equal(backend.active, false);
  assert.equal(store.auth.error, '');
  assertAnonymous(store);
  const reloaded = new Store(() => {});
  await reloaded.load();
  assertAnonymous(reloaded);
  assertAnonymous(store);
});

test('serialized login then logout uses the binding discovered by that login', async (t) => {
  const { server } = fixture(t);
  const backend = strictBackend(server);
  backend.active = false;
  const store = new Store(() => {});
  await store.load();
  const login = store.login('bob', 'another private password');
  const logout = store.logout();
  await Promise.all([login, logout]);
  const sent = server.calls.find(({ path }) => path === '/api/logout');
  assert.equal(new Headers(sent.init.headers).get('X-Steady-Session'), B_BINDING);
  assert.deepEqual(backend.logoutStatuses, [204]);
  assert.equal(backend.revoked, true);
  assert.equal(store.auth.error, '');
  assertAnonymous(store);
});

test('stale logout keeps its invocation binding and cannot revoke a replacement cookie', async (t) => {
  const { server } = fixture(t);
  const backend = strictBackend(server);
  const store = new Store(() => {});
  await store.load();
  backend.cookieBinding = B_BINDING;
  const discoveries = server.calls.filter(({ path }) => path === '/api/session').length;
  await store.logout();
  const sent = server.calls.find(({ path }) => path === '/api/logout');
  assert.equal(new Headers(sent.init.headers).get('X-Steady-Session'), A_BINDING);
  assert.deepEqual(backend.logoutStatuses, [401]);
  assert.equal(backend.active, true);
  assert.equal(backend.revoked, false);
  assert.equal(server.calls.filter(({ path }) => path === '/api/session').length, discoveries);
  assertAnonymous(store);
});

for (const source of ['session', 'state', 'mutation', 'pagehide']) {
  test(`${source} 401 clears both tabs and announces once without reciprocal notifications`, async (t) => {
    const { server } = fixture(t);
    activeTimers(t);
    const first = new Store(() => {});
    const second = new Store(() => {});
    if (source !== 'session') await first.load();
    await second.load();
    const pending = deferred();
    server.override = ({ path, init }) => {
      if (path === '/api/settings') return pending.promise;
      if (source === 'session' && path === '/api/session') return response(401);
      if (source === 'state' && path === '/api/state') return response(401);
      if (source === 'mutation' && path.includes('/days/')) return response(401);
      if (source === 'pagehide' && init.keepalive) return response(401);
      return undefined;
    };
    second.setView('month');
    second.setAccent('#3FB8B0');
    second.renameHabit('alice-habit', 'Peer pending private name');
    await settle();
    if (source === 'session' || source === 'state') await first.load();
    else if (source === 'mutation') first.toggleDay('alice-habit', '2026-11-02');
    else {
      first.renameHabit('alice-habit', 'Expired pagehide name');
      first.flushPending();
    }
    await settle();
    assertAnonymous(first);
    assertAnonymous(second);
    assert.deepEqual(server.messages, [{ type: 'changed' }]);
    pending.resolve(response(401));
    await settle();
    second.flushPending();
    await new Promise((resolve) => setTimeout(resolve, RENAME_DEBOUNCE_MS + 20));
    assert.equal(server.calls.filter(({ path }) => path === '/api/settings').length, 1);
    assert.equal(server.calls.some(({ init }) => init.method === 'PATCH' && !init.keepalive), false);
    assert.deepEqual(server.messages, [{ type: 'changed' }], 'old peer response cannot rebroadcast');
  });

  test(`stale ${source} 401 cannot invalidate or notify for a later user`, async (t) => {
    const { server, invalidate } = fixture(t);
    const first = new Store(() => {});
    if (source !== 'session') await first.load();
    const late = deferred();
    let intercepted = false;
    server.override = ({ path, init }) => {
      const match = source === 'session' ? path === '/api/session' :
        source === 'state' ? path === '/api/state' :
          source === 'mutation' ? path.includes('/days/') : init.keepalive;
      if (intercepted || !match) return undefined;
      intercepted = true;
      return late.promise;
    };
    let loading;
    if (source === 'session' || source === 'state') loading = first.load();
    else if (source === 'mutation') first.toggleDay('alice-habit', '2026-11-02');
    else {
      first.renameHabit('alice-habit', 'Old pagehide name');
      first.flushPending();
    }
    await settle();
    invalidate();
    server.username = 'bob'; server.binding = B_BINDING; server.privateState = BOB;
    await first.load();
    const peer = new Store(() => {});
    await peer.load();
    late.resolve(response(401));
    if (loading !== undefined) await loading;
    await settle();
    assert.equal(first.auth.status, 'authenticated');
    assert.equal(peer.auth.status, 'authenticated');
    assert.equal(first.state.habits[0].name, 'Bob private habit');
    assert.equal(peer.state.habits[0].name, 'Bob private habit');
    assert.deepEqual(server.messages, []);
  });
}

test('login 429 displays the specified retry-later message', async (t) => {
  const { server } = fixture(t);
  server.sessionStatus = 401;
  server.override = ({ path }) => path === '/api/login' ? response(429) : undefined;
  const store = new Store(() => {});
  await store.load();
  await store.login('alice', 'a private password');
  assertAnonymous(store);
  assert.equal(store.auth.error, 'Too many login attempts. Try again later.');
});

for (const failure of ['503', 'network', 'proxy 401']) {
  test(`initial ${failure} discovery remains checking and refresh recovers`, async (t) => {
    const { server } = fixture(t);
    server.override = ({ path }) => {
      if (path !== '/api/session') return undefined;
      if (failure === 'network') return Promise.reject(new TypeError('offline'));
      return failure === '503' ? response(503) : new Response('<html>Proxy login</html>', { status: 401 });
    };
    const store = new Store(() => {});
    await store.load();
    assert.equal(store.auth.status, 'checking');
    assert.equal(store.auth.error, 'Unable to check your session. Try again.');
    assert.deepEqual(store.state, EMPTY);
    assert.deepEqual(server.messages, []);
    server.override = undefined;
    await store.refresh();
    assert.equal(store.auth.status, 'authenticated');
    assert.equal(store.auth.error, '');
    assert.equal(store.state.habits[0].id, 'alice-habit');
  });
}

for (const failure of ['503', 'network', 'proxy 401']) {
  test(`authenticated ${failure} read preserves private state and binding until recovery`, async (t) => {
    const { store, server } = await loaded(t);
    server.override = ({ path }) => {
      if (path !== '/api/state') return undefined;
      if (failure === 'network') return Promise.reject(new TypeError('offline'));
      return failure === '503' ? response(503) : new Response('<html>Proxy login</html>', { status: 401 });
    };
    await store.load();
    assert.equal(store.auth.status, 'authenticated');
    assert.equal(store.state.habits[0].id, 'alice-habit');
    assert.deepEqual(server.messages, []);
    server.override = undefined;
    server.privateState = { ...ALICE, accent: '#3FB8B0' };
    await store.refresh();
    assert.equal(store.state.accent, '#3FB8B0');
    const last = server.calls.at(-1);
    assert.equal(new Headers(last.init.headers).get('X-Steady-Session'), A_BINDING);
  });
}

for (const source of ['mutation', 'pagehide']) {
  test(`proxy 401 ${source} does not revoke the application session or notify peers`, async (t) => {
    const { store, server } = await loaded(t);
    activeTimers(t);
    server.override = ({ path, init }) => path.includes('/days/') || init.keepalive
      ? new Response('<html>Proxy login</html>', { status: 401 }) : undefined;
    if (source === 'mutation') store.toggleDay('alice-habit', '2026-11-02');
    else {
      store.renameHabit('alice-habit', 'Pending name');
      store.flushPending();
    }
    await settle();
    assert.equal(store.auth.status, 'authenticated');
    assert.equal(store.state.habits[0].id, 'alice-habit');
    assert.deepEqual(server.messages, []);
  });
}

test('successful login with transient discovery enters checking and recovers without another password', async (t) => {
  const { server } = fixture(t);
  server.sessionStatus = 401;
  const store = new Store(() => {});
  await store.load();
  server.override = ({ path }) => path === '/api/session' ? response(503) : undefined;
  await store.login('alice', 'a private password');
  assert.equal(store.auth.status, 'checking');
  assert.equal(store.auth.busy, false);
  assert.equal(store.auth.error, 'Unable to check your session. Try again.');
  server.override = undefined;
  server.sessionStatus = 200;
  await store.refresh();
  assert.equal(store.auth.status, 'authenticated');
  assert.equal(server.calls.filter(({ path }) => path === '/api/login').length, 1);
});

test('refresh skips an anonymous login screen and active authentication operations', async (t) => {
  const { server } = fixture(t);
  server.sessionStatus = 401;
  const store = new Store(() => {});
  await store.load();
  const baseline = server.calls.length;
  await store.refresh();
  assert.equal(server.calls.length, baseline);
  const login = deferred();
  server.override = ({ path }) => path === '/api/login' ? login.promise : undefined;
  const authenticating = store.login('alice', 'a private password');
  await settle();
  const duringLogin = server.calls.length;
  await store.refresh();
  assert.equal(server.calls.length, duringLogin);
  login.resolve(response());
  await authenticating;
});

test('refresh coalesces concurrent checks and releases its slot after completion', async (t) => {
  const { store, server } = await loaded(t);
  const next = deferred();
  server.override = ({ path }) => path === '/api/state' ? next.promise : undefined;
  const first = store.refresh();
  const second = store.refresh();
  await settle();
  assert.equal(server.calls.filter(({ path }) => path === '/api/state').length, 2);
  next.resolve(response(200, ALICE));
  await Promise.all([first, second]);
  server.override = undefined;
  await store.refresh();
  assert.equal(server.calls.filter(({ path }) => path === '/api/state').length, 3);
});

test('a state body captured before a completed write cannot erase the local mark', async (t) => {
  const { store, server } = await loaded(t);
  const body = deferred();
  let intercepted = false;
  server.override = ({ path }) => {
    if (path !== '/api/state' || intercepted) return undefined;
    intercepted = true;
    return { ok: true, status: 200, json: () => body.promise };
  };
  const loading = store.load();
  await settle();
  store.toggleDay('alice-habit', '2026-11-02');
  await settle();
  server.privateState = { ...ALICE, habits: [{ ...ALICE.habits[0], done: ['2026-11-03', '2026-11-02'] }] };
  body.resolve(ALICE);
  await loading;
  assert.equal(store.state.habits[0].done['2026-11-02'], true);
});

test('a GET dispatched with an outstanding write cannot commit its older snapshot after the drain', async (t) => {
  const { store, server } = await loaded(t);
  const write = deferred();
  const body = deferred();
  let intercepted = false;
  server.override = ({ path }) => {
    if (path.includes('/days/')) return write.promise;
    if (path === '/api/state' && !intercepted) {
      intercepted = true;
      return { ok: true, status: 200, json: () => body.promise };
    }
    return undefined;
  };
  store.toggleDay('alice-habit', '2026-11-02');
  await settle();
  const loading = store.load();
  await settle();
  write.resolve(response());
  await settle();
  server.privateState = { ...ALICE, habits: [{ ...ALICE.habits[0], done: ['2026-11-03', '2026-11-02'] }] };
  body.resolve(ALICE);
  await loading;
  assert.equal(store.state.habits[0].done['2026-11-02'], true);
});

test('pending rename survives a remote snapshot and saves once with its session binding', async (t) => {
  const { store, server } = await loaded(t);
  activeTimers(t);
  store.renameHabit('alice-habit', 'Still typing');
  server.privateState = { ...ALICE, accent: '#3FB8B0' };
  await store.load();
  assert.equal(store.state.habits[0].name, 'Still typing');
  assert.equal(store.state.accent, '#3FB8B0');
  store.commitRename('alice-habit');
  await settle();
  const writes = mutations(server);
  assert.equal(writes.length, 1);
  assert.equal(new Headers(writes[0].init.headers).get('X-Steady-Session'), A_BINDING);
  assert.deepEqual(JSON.parse(writes[0].init.body), { name: 'Still typing' });
});

test('an older overlapping GET cannot overwrite a later server snapshot', async (t) => {
  const { store, server } = await loaded(t);
  const body = deferred();
  let reads = 0;
  server.override = ({ path }) => {
    if (path !== '/api/state') return undefined;
    reads += 1;
    return reads === 1 ? { ok: true, status: 200, json: () => body.promise }
      : response(200, { ...ALICE, accent: '#3FB8B0' });
  };
  const older = store.load();
  await settle();
  await store.load();
  assert.equal(store.state.accent, '#3FB8B0');
  body.resolve(ALICE);
  await older;
  assert.equal(store.state.accent, '#3FB8B0');
});

for (const phase of ['session', 'state']) {
  test(`${phase} reads have a four-second timeout and retry after cancellation`, async (t) => {
    const { server } = fixture(t);
    const store = new Store(() => {});
    if (phase === 'state') await store.load();
    const originalTimeout = AbortSignal.timeout;
    const deadlines = [];
    AbortSignal.timeout = (delay) => {
      const controller = new AbortController();
      deadlines.push({ delay, controller });
      return controller.signal;
    };
    t.after(() => { AbortSignal.timeout = originalTimeout; });
    server.override = ({ path, init }) => {
      if (path !== `/api/${phase}`) return undefined;
      return new Promise((resolve, reject) => {
        init.signal.addEventListener('abort', () => reject(init.signal.reason), { once: true });
      });
    };
    const loading = store.load();
    await settle();
    assert.deepEqual(deadlines.map(({ delay }) => delay), [4000]);
    deadlines[0].controller.abort(new DOMException('Timeout', 'TimeoutError'));
    await loading;
    assert.equal(store.auth.status, phase === 'session' ? 'checking' : 'authenticated');
    server.override = undefined;
    await store.refresh();
    assert.equal(store.auth.status, 'authenticated');
  });
}

function independentClients(t) {
  fixture(t);
  const accounts = new Map([['alice', structuredClone(ALICE)], ['bob', structuredClone(BOB)]]);
  const sessions = new Map([
    ['cookie-alice-one', { username: 'alice', binding: A_BINDING }],
    ['cookie-alice-two', { username: 'alice', binding: 'c'.repeat(64) }],
    ['cookie-bob', { username: 'bob', binding: B_BINDING }],
  ]);
  const backend = { cookie: '', calls: [], deferWrite: undefined, successful: [] };
  globalThis.fetch = async (path, init = {}) => {
    const cookie = backend.cookie;
    const session = sessions.get(cookie);
    backend.calls.push({ path, init, cookie });
    if (session === undefined) return response(401);
    if (path === '/api/session') return response(200, session);
    if (new Headers(init.headers).get('X-Steady-Session') !== session.binding) return response(401);
    const state = accounts.get(session.username);
    if (path === '/api/state') return response(200, structuredClone(state));
    const match = /^\/api\/habits\/([^/]+)\/days\/([^/]+)$/.exec(path);
    if (match === null) throw new Error(`Unexpected request ${path}`);
    const habit = state.habits.find(({ id }) => id === decodeURIComponent(match[1]));
    if (habit === undefined) return response(404);
    const held = backend.deferWrite?.({ path, init, cookie });
    if (held !== undefined) await held;
    const day = decodeURIComponent(match[2]);
    habit.done = habit.done.filter((saved) => saved !== day);
    if (init.method === 'PUT') habit.done.push(day);
    backend.successful.push({ cookie, method: init.method });
    return response();
  };
  function client(cookie) {
    const store = new Store(() => {});
    return {
      store,
      async run(operation) {
        backend.cookie = cookie;
        await operation(store);
        await settle();
      },
    };
  }
  return { backend, accounts, first: client('cookie-alice-one'), second: client('cookie-alice-two'), bob: client('cookie-bob') };
}

test('independent cookie clients converge on remote mark and unmark without replaying saves', async (t) => {
  const { backend, first, second } = independentClients(t);
  await first.run((store) => store.load());
  await second.run((store) => store.load());
  await first.run((store) => store.toggleDay('alice-habit', '2026-11-02'));
  await second.run((store) => store.refresh());
  assert.equal(second.store.state.habits[0].done['2026-11-02'], true);
  await second.run((store) => store.toggleDay('alice-habit', '2026-11-02'));
  await first.run((store) => store.refresh());
  assert.equal(first.store.state.habits[0].done['2026-11-02'], undefined);
  await second.run((store) => store.refresh());
  assert.equal(backend.successful.length, 2);
  assert.deepEqual(backend.successful.map(({ method }) => method), ['PUT', 'DELETE']);
});

test('independent concurrent cookie saves converge to the last server-successful mark', async (t) => {
  const { backend, first, second } = independentClients(t);
  await first.run((store) => store.load());
  await second.run((store) => store.load());
  const delayed = deferred();
  backend.deferWrite = ({ cookie }) => cookie === 'cookie-alice-one' ? delayed.promise : undefined;
  await first.run((store) => store.toggleDay('alice-habit', '2026-11-02'));
  await second.run((store) => store.toggleDay('alice-habit', '2026-11-02'));
  await second.run((store) => store.toggleDay('alice-habit', '2026-11-02'));
  delayed.resolve();
  await settle();
  assert.deepEqual(backend.successful.map(({ method }) => method), ['PUT', 'DELETE', 'PUT']);
  await first.run((store) => store.refresh());
  await second.run((store) => store.refresh());
  assert.equal(first.store.state.habits[0].done['2026-11-02'], true);
  assert.equal(second.store.state.habits[0].done['2026-11-02'], true);
  assert.equal(backend.successful.length, 3, 'refresh cannot replay an already successful write');
});

test('independent cookie clients keep account state isolated and reject a foreign binding', async (t) => {
  const { backend, accounts, first, bob } = independentClients(t);
  await first.run((store) => store.load());
  await bob.run((store) => store.load());
  await first.run((store) => store.toggleDay('alice-habit', '2026-11-02'));
  await bob.run((store) => store.refresh());
  assert.deepEqual(bob.store.state.habits.map(({ id }) => id), ['bob-habit']);
  assert.deepEqual(accounts.get('bob').habits[0].done, []);
  // The transport supplies Bob's cookie while the request still carries Alice's captured binding.
  await bob.run(() => first.store.toggleDay('alice-habit', '2026-11-01'));
  assertAnonymous(first.store);
  assert.equal(accounts.get('alice').habits[0].done.includes('2026-11-01'), false);
  assert.equal(backend.calls.at(-1).cookie, 'cookie-bob');
});

for (const completion of ['before', 'after']) {
  test(`pagehide snapshot completing ${completion} the keepalive write preserves the local name until convergence`, async (t) => {
    const { store, server } = await loaded(t);
    activeTimers(t);
    const write = deferred();
    const body = deferred();
    t.after(() => { write.resolve(response()); body.resolve(ALICE); });
    server.override = ({ path, init }) => {
      if (init.keepalive) {
        assert.equal(new Headers(init.headers).get('X-Steady-Session'), A_BINDING);
        assert.equal(init.method, 'PATCH');
        assert.deepEqual(JSON.parse(init.body), { name: 'Still typing after wake' });
        return write.promise;
      }
      return path === '/api/state' ? { ok: true, status: 200, json: () => body.promise } : undefined;
    };
    store.renameHabit('alice-habit', 'Still typing after wake');
    store.flushPending();
    await settle();
    const loading = store.refresh();
    await settle();
    if (completion === 'after') {
      write.resolve(response());
      await settle();
    }
    body.resolve(ALICE);
    await loading;
    assert.equal(store.state.habits[0].name, 'Still typing after wake');
    write.resolve(response());
    await settle();
    server.privateState = {
      ...ALICE, accent: '#3FB8B0',
      habits: [{ ...ALICE.habits[0], name: 'Still typing after wake' }],
    };
    server.override = undefined;
    await store.refresh();
    assert.equal(store.state.habits[0].name, 'Still typing after wake');
    assert.equal(store.state.accent, '#3FB8B0', 'a later authoritative snapshot must become eligible');
    assert.equal(mutations(server).length, 1, 'polling must not replay the completed keepalive write');
  });
}

test('an old-generation pagehide completion cannot release a new-generation pending flush', async (t) => {
  const { store, server, invalidate } = await loaded(t);
  activeTimers(t);
  const oldWrite = deferred();
  const newWrite = deferred();
  t.after(() => { oldWrite.resolve(response()); newWrite.resolve(response()); });
  server.override = ({ init }) => {
    if (!init.keepalive) return undefined;
    assert.equal(new Headers(init.headers).get('X-Steady-Session'), A_BINDING);
    return oldWrite.promise;
  };
  store.renameHabit('alice-habit', 'Old account pending name');
  store.flushPending();
  await settle();
  invalidate();
  server.username = 'bob'; server.binding = B_BINDING; server.privateState = BOB;
  await store.load();
  assert.equal(store.state.habits[0].name, 'Bob private habit', 'old flush tracking resets at an auth boundary');
  server.override = ({ init }) => {
    if (!init.keepalive) return undefined;
    assert.equal(new Headers(init.headers).get('X-Steady-Session'), B_BINDING);
    return newWrite.promise;
  };
  store.renameHabit('bob-habit', 'Still typing after wake');
  store.flushPending();
  await settle();
  oldWrite.resolve(response());
  await settle();
  await store.refresh();
  assert.equal(store.auth.status, 'authenticated');
  assert.equal(store.state.habits[0].name, 'Still typing after wake');
  newWrite.resolve(response());
  await settle();
  server.privateState = {
    ...BOB, accent: '#3FB8B0',
    habits: [{ ...BOB.habits[0], name: 'Still typing after wake' }],
  };
  server.override = undefined;
  await store.refresh();
  assert.equal(store.state.habits[0].name, 'Still typing after wake');
  assert.equal(store.state.accent, '#3FB8B0', 'new flush completion must release exactly its own accounting');
});
