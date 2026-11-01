import { MONTH_DAYS, countDone, dayKey, formatHeaderDate, formatLongDate, formatMonthYear, formatShortDate, lastNDays, pluralDays, streak, weekdayShort, yearCaptionMonth, yearGrid, } from './dates.js';
const EM = String.fromCharCode(0x2014);
const SVG_NS = 'http://www.w3.org/2000/svg';
const WEEK_DAYS = 7;
const NAME_MAX_LENGTH = 60;
const VIEW_SPECS = {
    week: { rowClass: 'habit week-row', gridClass: 'days', dayClass: 'sq' },
    month: { rowClass: 'habit block month-block', gridClass: 'month-grid', dayClass: 'dot' },
    year: { rowClass: 'habit block year-block', gridClass: 'year-grid', dayClass: 'cell' },
};
export function render(root, state, ui, now) {
    const todayKey = dayKey(now);
    const total = state.habits.length;
    const done = state.habits.filter((habit) => habit.done[todayKey] === true).length;
    setText(required(root, '#today'), formatHeaderDate(now));
    document.documentElement.style.setProperty('--accent', state.accent);
    renderPalette(root, state.accent, ui.paletteOpen);
    setText(required(root, '#title'), titleText(done, total));
    required(root, '#progress').style.width = `${total === 0 ? 0 : (done / total) * 100}%`;
    for (const tab of root.querySelectorAll('.tab')) {
        setAttr(tab, 'aria-selected', String(tab.dataset.value === state.view));
    }
    // Only visibility: the add input keeps its own value and focus.
    required(root, '#add-row').hidden = !ui.adding;
    renderToast(root, ui.toast);
    renderList(required(root, '#list'), state, now);
}
function titleText(done, total) {
    if (total === 0) {
        return 'Let’s start';
    }
    return done === total ? 'All done today' : `${done} of ${total} today`;
}
function renderPalette(root, accent, open) {
    required(root, '#palette').hidden = !open;
    setAttr(required(root, '#accent-toggle'), 'aria-expanded', String(open));
    for (const color of root.querySelectorAll('.palette-color')) {
        setAttr(color, 'aria-checked', String(color.dataset.value === accent));
    }
}
function renderToast(root, toast) {
    required(root, '#toast').hidden = toast === null;
    if (toast !== null) {
        setText(required(root, '#toast-text'), `“${toast.habit.name}” deleted`);
    }
}
function renderList(list, state, now) {
    const existing = new Map();
    for (const child of list.children) {
        if (child instanceof HTMLElement && child.dataset.view === state.view) {
            const id = child.dataset.id;
            if (id !== undefined) {
                existing.set(id, child);
            }
        }
    }
    const wanted = [];
    if (state.habits.length === 0) {
        wanted.push(createEmpty());
    }
    else {
        const frame = buildFrame(state.view, now);
        if (state.view === 'week') {
            wanted.push(createWeekHead(frame));
        }
        for (const habit of state.habits) {
            const row = existing.get(habit.id) ?? createRow(state.view, habit.id);
            updateRow(row, habit, frame);
            wanted.push(row);
        }
    }
    const keep = new Set(wanted);
    for (const child of Array.from(list.children)) {
        if (!keep.has(child)) {
            child.remove();
        }
    }
    // A node that is already in place is left alone: moving a row would blur its focused input.
    wanted.forEach((node, i) => {
        const current = list.children[i];
        if (current !== node) {
            list.insertBefore(node, current ?? null);
        }
    });
}
function buildFrame(view, now) {
    const todayKey = dayKey(now);
    const dates = visibleDates(view, now);
    const days = dates.map((date) => ({ date, key: dayKey(date), label: formatLongDate(date) }));
    const [first = now] = dates;
    return {
        view,
        now,
        todayKey,
        days,
        countable: days.filter((day) => day.key <= todayKey).map((day) => day.date),
        captionStart: view === 'year' ? formatMonthYear(yearCaptionMonth(first)) : formatShortDate(first),
    };
}
function visibleDates(view, now) {
    switch (view) {
        case 'week':
            return lastNDays(now, WEEK_DAYS);
        case 'month':
            return lastNDays(now, MONTH_DAYS);
        case 'year':
            return yearGrid(now);
    }
}
function createEmpty() {
    const empty = create('div', 'empty');
    empty.append(create('p', 'empty-title', 'Nothing here yet'), create('p', 'empty-text', 'Tap “+ Habit” above. Starting small is fine.'));
    return empty;
}
function createWeekHead(frame) {
    const days = create('div', 'days');
    for (const day of frame.days) {
        const wd = create('span', day.key === frame.todayKey ? 'wd is-today' : 'wd');
        wd.append(create('span', 'wd-name', weekdayShort(day.date)), create('span', 'wd-date', String(day.date.getDate())));
        days.append(wd);
    }
    const head = create('div', 'week-head');
    head.setAttribute('aria-hidden', 'true');
    head.append(create('span', 'name-spacer'), days, create('span', 'side-spacer'));
    return head;
}
function createRow(view, id) {
    const spec = VIEW_SPECS[view];
    const row = create('div', spec.rowClass);
    row.dataset.id = id;
    row.dataset.view = view;
    const name = createNameInput(id);
    const del = createDeleteButton(id);
    const grid = create('div', spec.gridClass);
    if (view === 'week') {
        const side = create('div', 'side');
        side.append(create('span', 'streak'), del);
        row.append(name, grid, side);
        return row;
    }
    const head = create('div', 'block-head');
    head.append(name, create('span', 'count'), del);
    const caption = create('div', 'caption');
    caption.append(create('span'), create('span', undefined, 'today'));
    row.append(head, grid, caption);
    return row;
}
function createNameInput(id) {
    const input = create('input', 'name');
    input.type = 'text';
    input.setAttribute('maxlength', String(NAME_MAX_LENGTH));
    input.dataset.action = 'rename';
    input.dataset.id = id;
    input.setAttribute('aria-label', 'Habit name');
    input.setAttribute('autocomplete', 'off');
    input.setAttribute('spellcheck', 'false');
    return input;
}
function createDeleteButton(id) {
    const button = create('button', 'del', '×');
    button.type = 'button';
    button.dataset.action = 'delete';
    button.dataset.id = id;
    return button;
}
function updateRow(row, habit, frame) {
    const input = required(row, 'input.name');
    // Assigning the value of the focused input would move the caret while the user types.
    if (input !== document.activeElement && input.value !== habit.name) {
        input.value = habit.name;
    }
    setAttr(required(row, 'button.del'), 'aria-label', `Delete ${habit.name}`);
    if (frame.view === 'week') {
        const run = streak(habit.done, frame.now);
        const streakText = required(row, '.streak');
        setText(streakText, run === 0 ? EM : pluralDays(run));
        streakText.classList.toggle('is-zero', run === 0);
    }
    else {
        const count = countDone(habit.done, frame.countable);
        const text = frame.view === 'month' ? `${count} of ${MONTH_DAYS}` : pluralDays(count);
        setText(required(row, '.count'), text);
        setText(required(row, '.caption > span:first-child'), frame.captionStart);
    }
    syncDays(required(row, `.${VIEW_SPECS[frame.view].gridClass}`), habit, frame);
}
function syncDays(grid, habit, frame) {
    const { days, view } = frame;
    // Same dates in the same order: patch in place so a focused day button keeps its focus.
    const intact = grid.children.length === days.length &&
        days.every((day, i) => dateOf(grid.children[i]) === day.key);
    if (!intact) {
        grid.replaceChildren(...days.map((day) => createDayButton(view, habit.id, day.key)));
    }
    days.forEach((day, i) => {
        const button = grid.children[i];
        if (button instanceof HTMLButtonElement) {
            patchDay(button, habit, day, frame);
        }
    });
}
function dateOf(node) {
    return node instanceof HTMLElement ? node.dataset.date : undefined;
}
function createDayButton(view, id, key) {
    const button = create('button', VIEW_SPECS[view].dayClass);
    button.type = 'button';
    button.dataset.action = 'toggle';
    button.dataset.id = id;
    button.dataset.date = key;
    return button;
}
function patchDay(button, habit, day, frame) {
    const done = habit.done[day.key] === true;
    button.classList.toggle('is-done', done);
    button.classList.toggle('is-today', day.key === frame.todayKey);
    setAttr(button, 'aria-pressed', String(done));
    setAttr(button, 'aria-label', `${habit.name}, ${day.label} ${EM} ${done ? 'done' : 'not done'}`);
    if (frame.view === 'week') {
        syncCheck(button, done);
    }
    else if (frame.view === 'year') {
        const future = day.key > frame.todayKey;
        button.classList.toggle('is-future', future);
        button.disabled = future;
    }
}
function syncCheck(button, done) {
    const icon = button.querySelector('svg');
    if (!done) {
        icon?.remove();
    }
    else if (icon === null) {
        button.append(createCheck());
    }
}
function createCheck() {
    const svg = document.createElementNS(SVG_NS, 'svg');
    svg.setAttribute('class', 'check');
    svg.setAttribute('viewBox', '0 0 16 16');
    svg.setAttribute('aria-hidden', 'true');
    const path = document.createElementNS(SVG_NS, 'path');
    path.setAttribute('d', 'M3.5 8.5l3 3 6-7');
    path.setAttribute('fill', 'none');
    path.setAttribute('stroke', 'currentColor');
    path.setAttribute('stroke-width', '2');
    path.setAttribute('stroke-linecap', 'round');
    path.setAttribute('stroke-linejoin', 'round');
    svg.append(path);
    return svg;
}
function create(tag, className, text) {
    const node = document.createElement(tag);
    if (className !== undefined) {
        node.className = className;
    }
    if (text !== undefined) {
        node.textContent = text;
    }
    return node;
}
// The skeleton in index.html is static, so a miss is a broken page rather than a runtime case.
function required(parent, selector) {
    const node = parent.querySelector(selector);
    if (node === null) {
        throw new Error(`render: ${selector} not found`);
    }
    return node;
}
// Skipping identical writes keeps the toast live region from re-announcing on every render.
function setText(node, text) {
    if (node.textContent !== text) {
        node.textContent = text;
    }
}
function setAttr(node, name, value) {
    if (node.getAttribute(name) !== value) {
        node.setAttribute(name, value);
    }
}
