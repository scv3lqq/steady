import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as d from '../dist/dates.js';

// `npm test` pins TZ=America/New_York: the zone is behind UTC and changes its clocks on
// 2026-03-08 and 2026-11-01, which several cases below depend on.

const doneOn = (...keys) => Object.fromEntries(keys.map((key) => [key, true]));

test('dayKey', () => {
  const cases = [
    { date: new Date(2026, 0, 5), want: '2026-01-05' },
    // Already 2026-11-02 in UTC.
    { date: new Date(2026, 10, 1, 23, 59), want: '2026-11-01' },
  ];
  for (const { date, want } of cases) {
    assert.equal(d.dayKey(date), want, date.toString());
  }
});

test('fromKey', () => {
  // Local midnight: parsing the key as UTC would give 2026-10-31 20:00 here.
  const date = d.fromKey('2026-11-01');
  assert.equal(date.getFullYear(), 2026);
  assert.equal(date.getMonth(), 10);
  assert.equal(date.getDate(), 1);
  assert.equal(date.getHours(), 0);
});

test('addDays across DST', () => {
  // Nov 1 has 25 hours, so 24 h after its midnight is still Nov 1.
  const cases = [
    { from: new Date(2026, 10, 1), n: 1, want: '2026-11-02' },
    { from: new Date(2026, 10, 1), n: -1, want: '2026-10-31' },
    { from: new Date(2026, 2, 7), n: 1, want: '2026-03-08' },
    { from: new Date(2026, 2, 8), n: 1, want: '2026-03-09' },
  ];
  for (const { from, n, want } of cases) {
    assert.equal(d.dayKey(d.addDays(from, n)), want, `${d.dayKey(from)} + ${n}`);
  }
});

test('addDays month/year ends', () => {
  const cases = [
    { from: new Date(2026, 11, 31), n: 1, want: '2027-01-01' },
    { from: new Date(2027, 0, 1), n: -1, want: '2026-12-31' },
    { from: new Date(2028, 1, 28), n: 1, want: '2028-02-29' },
    { from: new Date(2026, 1, 28), n: 1, want: '2026-03-01' },
  ];
  for (const { from, n, want } of cases) {
    assert.equal(d.dayKey(d.addDays(from, n)), want, `${d.dayKey(from)} + ${n}`);
  }
});

test('lastNDays', () => {
  const today = new Date(2026, 10, 1, 15, 30);

  const month = d.lastNDays(today, 30);
  assert.equal(month.length, 30);
  assert.equal(d.dayKey(month[0]), '2026-10-03');
  assert.equal(d.dayKey(month.at(-1)), '2026-11-01');
  assert.equal(month.at(-1).getHours(), 0);

  assert.equal(d.dayKey(d.lastNDays(today, 7)[0]), '2026-10-26');

  // The window spans the 2026-03-08 spring-forward day.
  const spring = d.lastNDays(new Date(2026, 2, 20), 30);
  assert.equal(d.dayKey(spring[0]), '2026-02-19');
  assert.equal(d.dayKey(spring.at(-1)), '2026-03-20');
});

test('mondayOf', () => {
  // Weeks start on Monday, so a Sunday belongs to the week that began six days earlier.
  const cases = [
    { name: 'Sunday', from: new Date(2026, 10, 1), want: '2026-10-26' },
    { name: 'Monday', from: new Date(2026, 9, 26), want: '2026-10-26' },
    { name: 'Wednesday', from: new Date(2026, 10, 4), want: '2026-11-02' },
  ];
  for (const { name, from, want } of cases) {
    assert.equal(d.dayKey(d.mondayOf(from)), want, name);
  }
});

test('yearGrid', () => {
  const cases = [
    { today: new Date(2026, 10, 1), first: '2025-10-27', last: '2026-11-01' },
    { today: new Date(2026, 10, 4), first: '2025-11-03', last: '2026-11-08' },
  ];
  for (const { today, first, last } of cases) {
    const label = `yearGrid(${d.dayKey(today)})`;
    const grid = d.yearGrid(today);
    assert.equal(grid.length, 371, `${label} length`);
    assert.equal(d.dayKey(grid[0]), first, `${label} first`);
    assert.equal(d.dayKey(grid[370]), last, `${label} last`);
    for (let i = 0; i < grid.length; i += 7) {
      assert.equal(grid[i].getDay(), 1, `${label} index ${i} is not a Monday`);
    }
  }
});

test('yearCaptionMonth', () => {
  const captionOf = (key) => d.yearCaptionMonth(d.fromKey(key));
  assert.equal(d.formatMonthYear(captionOf('2025-10-27')), 'November 2025');
  assert.equal(d.dayKey(captionOf('2025-12-01')), '2025-12-01');
  assert.equal(d.dayKey(captionOf('2025-12-29')), '2026-01-01');
});

test('streak', () => {
  const nov1 = new Date(2026, 10, 1);
  // The run may end today or yesterday (today can still be pending); anything older is 0.
  const cases = [
    { today: nov1, keys: ['2026-10-30', '2026-10-31', '2026-11-01'], want: 3 },
    { today: nov1, keys: ['2026-10-30', '2026-10-31'], want: 2 },
    { today: nov1, keys: ['2026-11-01'], want: 1 },
    { today: nov1, keys: [], want: 0 },
    { today: nov1, keys: ['2026-10-30'], want: 0 },
    { today: nov1, keys: ['2026-10-29', '2026-10-31', '2026-11-01'], want: 2 },
    // Both rows walk back over a DST change: fall back on 11-01, spring forward on 03-08.
    { today: new Date(2026, 10, 2), keys: ['2026-10-31', '2026-11-01', '2026-11-02'], want: 3 },
    { today: new Date(2026, 2, 9), keys: ['2026-03-07', '2026-03-08', '2026-03-09'], want: 3 },
  ];
  for (const { today, keys, want } of cases) {
    assert.equal(d.streak(doneOn(...keys), today), want, `${d.dayKey(today)} [${keys}]`);
  }
});

test('countDone', () => {
  const days = d.lastNDays(new Date(2026, 10, 1), 30);
  assert.equal(d.countDone(doneOn('2026-10-30', '2026-11-01', '2025-01-01'), days), 2);
});

test('pluralDays', () => {
  const cases = [
    { n: 0, want: '0 days' },
    { n: 1, want: '1 day' },
    { n: 2, want: '2 days' },
    { n: 21, want: '21 days' },
    { n: 214, want: '214 days' },
  ];
  for (const { n, want } of cases) {
    assert.equal(d.pluralDays(n), want, `pluralDays(${n})`);
  }
});

test('formatters', () => {
  assert.equal(d.formatHeaderDate(new Date(2026, 10, 1)), 'Sunday, November 1');
  assert.equal(d.formatShortDate(new Date(2026, 9, 3)), 'Oct 3');
  assert.equal(d.formatLongDate(new Date(2026, 9, 30)), 'October 30');
  assert.equal(d.formatMonthYear(new Date(2025, 10, 1)), 'November 2025');
  assert.equal(d.weekdayShort(new Date(2026, 9, 26)), 'Mo');
  assert.equal(d.weekdayShort(new Date(2026, 9, 29)), 'Th');
  assert.equal(d.weekdayShort(new Date(2026, 10, 1)), 'Su');
});
