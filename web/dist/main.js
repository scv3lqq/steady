import { render } from './render.js';
import { Store } from './store.js';
import { startSync } from './sync.js';
const TOAST_MS = 5000;
const NAME_INPUT = 'input.name';
const ADD_INPUT = '#add-input';
const FONTS_URL = 'https://fonts.googleapis.com/css2?family=Onest:wght@400;500;600&display=swap';
function isView(value) {
    return value === 'week' || value === 'month' || value === 'year';
}
function start() {
    const root = document.getElementById('app');
    if (root === null) {
        throw new Error('start: #app not found');
    }
    const ui = { paletteOpen: false, adding: false, toast: null };
    let toastTimer;
    // The store calls draw on every change, and draw reads the store only when it runs.
    const draw = () => {
        if (store.auth.status !== 'authenticated') {
            clearTimeout(toastTimer);
            ui.toast = null;
            ui.paletteOpen = false;
            ui.adding = false;
            const addInput = root.querySelector(ADD_INPUT);
            if (addInput !== null)
                addInput.value = '';
        }
        render(root, store.state, ui, new Date(), store.auth);
    };
    const store = new Store(draw);
    draw();
    root.addEventListener('submit', (event) => {
        if (!(event.target instanceof HTMLFormElement) || event.target.id !== 'login-form')
            return;
        event.preventDefault();
        if (store.auth.busy)
            return;
        const username = event.target.querySelector('#username');
        const password = event.target.querySelector('#password');
        if (username === null || password === null)
            return;
        const secret = password.value;
        password.value = '';
        void store.login(username.value, secret);
    });
    const dismissToast = () => {
        clearTimeout(toastTimer);
        ui.toast = null;
        draw();
    };
    const deleteHabit = (id) => {
        const deleted = store.deleteHabit(id);
        if (deleted === null) {
            return;
        }
        ui.toast = deleted;
        // A new delete replaces the toast and gives it a full window again.
        clearTimeout(toastTimer);
        toastTimer = setTimeout(dismissToast, TOAST_MS);
        draw();
    };
    const undoDelete = () => {
        if (ui.toast !== null) {
            store.restoreHabit(ui.toast);
            dismissToast();
        }
    };
    // The row stays in the DOM while hidden and render never touches its input, so clear it here.
    const closeAddRow = (input) => {
        input.value = '';
        ui.adding = false;
        draw();
    };
    root.addEventListener('click', (event) => {
        const target = event.target;
        if (!(target instanceof Element)) {
            return;
        }
        const el = target.closest('[data-action]');
        if (!(el instanceof HTMLElement)) {
            return;
        }
        const { action, id, date, value } = el.dataset;
        if (action === 'logout') {
            void store.logout();
            return;
        }
        if (store.auth.status !== 'authenticated')
            return;
        switch (action) {
            case 'toggle-palette':
                ui.paletteOpen = !ui.paletteOpen;
                draw();
                break;
            case 'accent':
                if (value !== undefined) {
                    store.setAccent(value);
                }
                break;
            case 'view':
                if (isView(value)) {
                    store.setView(value);
                }
                break;
            case 'show-add':
                ui.adding = true;
                draw();
                // A hidden input cannot take focus, so this has to follow the draw that shows the row.
                root.querySelector(ADD_INPUT)?.focus();
                break;
            case 'toggle':
                if (id !== undefined && date !== undefined) {
                    store.toggleDay(id, date);
                }
                break;
            case 'delete':
                if (id !== undefined) {
                    deleteHabit(id);
                }
                break;
            case 'undo':
                undoDelete();
                break;
        }
    });
    root.addEventListener('input', (event) => {
        const input = event.target;
        if (input instanceof HTMLInputElement && input.matches(NAME_INPUT)) {
            const { id } = input.dataset;
            if (id !== undefined) {
                store.renameHabit(id, input.value);
            }
        }
    });
    // focusout bubbles, which blur does not, so one listener on root covers every input.
    root.addEventListener('focusout', (event) => {
        const input = event.target;
        if (!(input instanceof HTMLInputElement)) {
            return;
        }
        if (input.matches(NAME_INPUT)) {
            const { id } = input.dataset;
            if (id !== undefined) {
                store.commitRename(id);
            }
            // Apply a deferred view after the browser finishes transferring focus.
            queueMicrotask(draw);
        }
        else if (input.matches(ADD_INPUT) && input.value.trim() === '') {
            closeAddRow(input);
        }
    });
    root.addEventListener('keydown', (event) => {
        const input = event.target;
        if (!(input instanceof HTMLInputElement) || !input.matches(ADD_INPUT)) {
            return;
        }
        if (event.key === 'Enter') {
            const name = input.value.trim();
            if (name !== '') {
                store.addHabit(name);
            }
            closeAddRow(input);
        }
        else if (event.key === 'Escape') {
            closeAddRow(input);
        }
    });
    window.addEventListener('pagehide', () => store.flushPending());
    // Not in index.html: a stylesheet link in <head> holds this module until the font host answers.
    const fonts = document.createElement('link');
    fonts.rel = 'stylesheet';
    fonts.href = FONTS_URL;
    document.head.append(fonts);
    startSync(() => store.refresh(), document, window);
}
start();
