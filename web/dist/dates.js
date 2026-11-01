export const MONTH_DAYS = 30;
export const YEAR_WEEKS = 53;
const DAYS_PER_WEEK = 7;
const headerFormat = new Intl.DateTimeFormat('en-US', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
});
const shortFormat = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric' });
const longFormat = new Intl.DateTimeFormat('en-US', { month: 'long', day: 'numeric' });
const monthYearFormat = new Intl.DateTimeFormat('en-US', { month: 'long', year: 'numeric' });
const weekdayFormat = new Intl.DateTimeFormat('en-US', { weekday: 'short' });
export function startOfDay(d) {
    return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}
export function dayKey(d) {
    const year = String(d.getFullYear()).padStart(4, '0');
    const month = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    return `${year}-${month}-${day}`;
}
export function fromKey(key) {
    const year = Number(key.slice(0, 4));
    const month = Number(key.slice(5, 7));
    const day = Number(key.slice(8, 10));
    return new Date(year, month - 1, day);
}
// Calendar arithmetic on the local date parts: adding 24 h would land on the
// wrong day around a DST change.
export function addDays(d, n) {
    return new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
}
export function lastNDays(today, n) {
    const last = startOfDay(today);
    return Array.from({ length: n }, (_, i) => addDays(last, i - (n - 1)));
}
export function mondayOf(d) {
    return addDays(startOfDay(d), -((d.getDay() + 6) % DAYS_PER_WEEK));
}
export function yearGrid(today) {
    const start = addDays(mondayOf(today), -DAYS_PER_WEEK * (YEAR_WEEKS - 1));
    return Array.from({ length: YEAR_WEEKS * DAYS_PER_WEEK }, (_, i) => addDays(start, i));
}
// The month of the first column's Sunday. Moving to the next month whenever the grid does not
// start on the 1st would caption a grid that starts on 2025-11-03 as December.
export function yearCaptionMonth(gridStart) {
    const sunday = addDays(gridStart, DAYS_PER_WEEK - 1);
    return new Date(sunday.getFullYear(), sunday.getMonth(), 1);
}
export function streak(done, today) {
    let d = startOfDay(today);
    if (!done[dayKey(d)]) {
        d = addDays(d, -1);
    }
    let count = 0;
    while (done[dayKey(d)]) {
        count += 1;
        d = addDays(d, -1);
    }
    return count;
}
export function countDone(done, days) {
    let count = 0;
    for (const d of days) {
        if (done[dayKey(d)]) {
            count += 1;
        }
    }
    return count;
}
export function pluralDays(n) {
    return n === 1 ? '1 day' : `${n} days`;
}
export function formatHeaderDate(d) {
    return headerFormat.format(d);
}
export function formatShortDate(d) {
    return shortFormat.format(d);
}
export function formatLongDate(d) {
    return longFormat.format(d);
}
export function formatMonthYear(d) {
    return monthYearFormat.format(d);
}
export function weekdayShort(d) {
    return weekdayFormat.format(d).slice(0, 2);
}
