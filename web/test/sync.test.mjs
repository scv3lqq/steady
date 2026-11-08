import assert from 'node:assert/strict';
import { test } from 'node:test';
import { startSync } from '../dist/sync.js';

function deferred() {
  let resolve;
  const promise = new Promise((done) => { resolve = done; });
  return { promise, resolve };
}

async function settle() {
  for (let turn = 0; turn < 10; turn += 1) await Promise.resolve();
}

class Surface extends EventTarget {
  visibilityState = 'visible';
  get hidden() { return this.visibilityState === 'hidden'; }
  fire(type) { this.dispatchEvent(new Event(type)); }
}

function fixture() {
  const page = new Surface();
  const host = new Surface();
  const intervals = new Map();
  let nextTimer = 0;
  let calls = 0;
  let pending;
  host.setInterval = (callback, delay) => {
    const id = ++nextTimer;
    intervals.set(id, { callback, delay });
    return id;
  };
  host.clearInterval = (id) => { intervals.delete(id); };
  const load = async () => { calls += 1; await pending?.promise; };
  return {
    page, host, intervals, load,
    get calls() { return calls; },
    hold(value) { pending = value; },
    tick() { for (const { callback } of [...intervals.values()]) callback(); },
    hide() { page.visibilityState = 'hidden'; page.fire('visibilitychange'); },
    show() { page.visibilityState = 'visible'; page.fire('visibilitychange'); },
  };
}

test('sync starts with an immediate check and one three-second visible interval', async () => {
  const f = fixture();
  const stop = startSync(f.load, f.page, f.host);
  await settle();
  assert.equal(f.calls, 1);
  assert.equal(f.intervals.size, 1);
  assert.deepEqual([...f.intervals.values()].map(({ delay }) => delay), [3000]);
  f.tick();
  await settle();
  assert.equal(f.calls, 2);
  stop();
});

test('sync begins paused while hidden and resumes immediately on visibility restoration', async () => {
  const f = fixture();
  f.page.visibilityState = 'hidden';
  const stop = startSync(f.load, f.page, f.host);
  await settle();
  assert.equal(f.calls, 0);
  assert.equal(f.intervals.size, 0);
  f.show();
  await settle();
  assert.equal(f.calls, 1);
  assert.equal(f.intervals.size, 1);
  f.hide();
  f.tick();
  await settle();
  assert.equal(f.calls, 1);
  assert.equal(f.intervals.size, 0);
  stop();
});

for (const [surface, signal] of [['host', 'focus'], ['host', 'online'], ['host', 'pageshow'], ['page', 'resume']]) {
  test(`${signal} wakes a visible client immediately without duplicating its interval`, async () => {
    const f = fixture();
    const stop = startSync(f.load, f.page, f.host);
    await settle();
    f[surface].fire(signal);
    await settle();
    assert.equal(f.calls, 2);
    assert.equal(f.intervals.size, 1);
    f[surface].fire(signal);
    await settle();
    assert.equal(f.calls, 3);
    assert.equal(f.intervals.size, 1);
    stop();
  });

  test(`${signal} cannot start checks while hidden`, async () => {
    const f = fixture();
    const stop = startSync(f.load, f.page, f.host);
    await settle();
    f.hide();
    f[surface].fire(signal);
    await settle();
    assert.equal(f.calls, 1);
    assert.equal(f.intervals.size, 0);
    stop();
  });
}

test('simultaneous lifecycle and interval wakes coalesce into one in-flight check', async () => {
  const f = fixture();
  const held = deferred();
  f.hold(held);
  const stop = startSync(f.load, f.page, f.host);
  f.host.fire('focus');
  f.host.fire('online');
  f.host.fire('pageshow');
  f.page.fire('resume');
  f.show();
  f.tick();
  await settle();
  assert.equal(f.calls, 1);
  assert.equal(f.intervals.size, 1);
  held.resolve();
  await settle();
  assert.equal(f.calls, 1, 'coalesced events must not schedule a trailing burst');
  f.tick();
  await settle();
  assert.equal(f.calls, 2);
  stop();
});

test('visibility changes never accumulate intervals and an in-flight check stays single', async () => {
  const f = fixture();
  const held = deferred();
  f.hold(held);
  const stop = startSync(f.load, f.page, f.host);
  for (let index = 0; index < 4; index += 1) {
    f.hide();
    assert.equal(f.intervals.size, 0);
    f.show();
    assert.equal(f.intervals.size, 1);
  }
  await settle();
  assert.equal(f.calls, 1);
  held.resolve();
  await settle();
  f.tick();
  await settle();
  assert.equal(f.calls, 2);
  stop();
});

test('idempotent stop removes scheduling and all wake listeners even after an in-flight check', async () => {
  const f = fixture();
  const held = deferred();
  f.hold(held);
  const stop = startSync(f.load, f.page, f.host);
  await settle();
  stop();
  stop();
  assert.equal(f.intervals.size, 0);
  held.resolve();
  await settle();
  f.hide();
  f.show();
  f.host.fire('focus');
  f.host.fire('online');
  f.host.fire('pageshow');
  f.page.fire('resume');
  f.tick();
  await settle();
  assert.equal(f.calls, 1);
  assert.equal(f.intervals.size, 0);
});
