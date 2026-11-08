export const PALETTE = [
    '#4FA872',
    '#8FBF4A',
    '#3FB8B0',
    '#5B8CEB',
    '#B07FE0',
    '#E07BB0',
    '#D96A6A',
    '#E08A45',
    '#E3C24F',
    '#ECE7DC',
];
export const DEFAULT_ACCENT = '#4FA872';
export const RENAME_DEBOUNCE_MS = 300;
const DEFAULT_NAME = 'Untitled';
async function authenticationRequired(response) {
    if (response.status !== 401)
        return false;
    try {
        const body = await response.json();
        return typeof body === 'object' && body !== null &&
            'error' in body && body.error === 'authentication required';
    }
    catch {
        return false;
    }
}
function normalizeName(name) {
    return name.trim() || DEFAULT_NAME;
}
function habitPath(id) {
    return `/api/habits/${encodeURIComponent(id)}`;
}
function requestInit(method, body) {
    if (body === undefined) {
        return { method };
    }
    return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) };
}
function toDone(days) {
    const done = {};
    for (const day of days) {
        done[day] = true;
    }
    return done;
}
function toState(api) {
    return {
        habits: api.habits.map((h) => ({ id: h.id, name: h.name, createdAt: h.createdAt, done: toDone(h.done) })),
        view: api.view,
        accent: api.accent,
    };
}
export class Store {
    #onChange;
    #channel;
    #state = { habits: [], view: 'week', accent: DEFAULT_ACCENT };
    #auth = { status: 'checking', busy: false, error: '' };
    #generation = 0;
    #binding = '';
    #controller = new AbortController();
    #data = new Set();
    #authQueue = Promise.resolve();
    #authPending = 0;
    #queue = Promise.resolve();
    #queued = 0;
    #flushing = 0;
    #stale = false;
    #revision = 0;
    #readSequence = 0;
    #refresh;
    #timers = new Map();
    #pending = new Map();
    constructor(onChange) {
        this.#onChange = onChange;
        this.#channel = new BroadcastChannel('steady-auth');
        this.#channel.addEventListener('message', () => this.#transition('anonymous'));
    }
    get state() {
        return this.#state;
    }
    get auth() {
        return this.#auth;
    }
    load() {
        const generation = this.#generation;
        const work = this.#auth.status === 'authenticated'
            ? this.#loadState(generation)
            : this.#checkSession(generation);
        return this.#track(work);
    }
    refresh() {
        if (this.#authPending > 0 || this.#auth.status === 'anonymous')
            return Promise.resolve();
        if (this.#refresh?.generation === this.#generation)
            return this.#refresh.promise;
        const refresh = { generation: this.#generation, promise: Promise.resolve() };
        refresh.promise = this.load().finally(() => {
            if (this.#refresh === refresh)
                this.#refresh = undefined;
        });
        this.#refresh = refresh;
        return refresh.promise;
    }
    login(username, password) {
        return this.#authenticate('/api/login', { username, password });
    }
    logout() {
        return this.#authenticate('/api/logout');
    }
    #transition(status, binding = '') {
        const previousGeneration = this.#generation;
        this.#generation += 1;
        if (status === 'authenticated' && this.#refresh?.generation === previousGeneration) {
            this.#refresh.generation = this.#generation;
        }
        else {
            this.#refresh = undefined;
        }
        this.#controller.abort();
        this.#controller = new AbortController();
        this.#binding = binding;
        this.#auth = { status, busy: this.#authPending > 0, error: '' };
        this.#state = { habits: [], view: 'week', accent: DEFAULT_ACCENT };
        for (const timer of this.#timers.values())
            clearTimeout(timer);
        this.#timers.clear();
        this.#pending.clear();
        this.#queue = Promise.resolve();
        this.#queued = 0;
        this.#flushing = 0;
        this.#stale = false;
        this.#onChange();
    }
    #track(work) {
        const tracked = work.finally(() => this.#data.delete(tracked));
        this.#data.add(tracked);
        return tracked;
    }
    #sessionLost(generation) {
        if (generation !== this.#generation)
            return;
        this.#transition('anonymous');
        this.#channel.postMessage({ type: 'changed' });
    }
    async #waitForData() {
        while (this.#data.size > 0)
            await Promise.allSettled([...this.#data]);
    }
    #authenticate(path, body) {
        const invocationBinding = this.#binding;
        this.#authPending += 1;
        this.#transition('anonymous');
        this.#channel.postMessage({ type: 'changed' });
        const run = this.#authQueue.then(async () => {
            // A queued logout may follow a login that discovered its binding after this invocation.
            const expectedBinding = invocationBinding || this.#binding;
            this.#transition('anonymous');
            // A fetch may ignore abort until its response/body settles. Do not replace its cookie yet.
            await this.#waitForData();
            const generation = this.#generation;
            try {
                const init = requestInit('POST', body);
                if (path === '/api/logout')
                    init.headers = { 'X-Steady-Session': expectedBinding };
                const res = await fetch(path, init);
                this.#channel.postMessage({ type: 'changed' });
                if (generation !== this.#generation)
                    return;
                if (!res.ok) {
                    if (path === '/api/login' && res.status === 429) {
                        this.#auth.error = 'Too many login attempts. Try again later.';
                    }
                    else {
                        this.#auth.error = path === '/api/login' ? 'Unable to log in. Check your credentials and try again.' : 'Unable to log out. Try again.';
                    }
                }
                else if (path === '/api/login') {
                    this.#auth.status = 'checking';
                    this.#onChange();
                    await this.#track(this.#checkSession(generation));
                }
            }
            catch {
                if (generation === this.#generation)
                    this.#auth.error = 'Unable to connect. Try again.';
            }
        }).finally(() => {
            this.#authPending -= 1;
            this.#auth.busy = this.#authPending > 0;
            this.#onChange();
        });
        this.#authQueue = run.catch(() => undefined);
        return run;
    }
    #readSignal() {
        return AbortSignal.any([this.#controller.signal, AbortSignal.timeout(4000)]);
    }
    #sessionCheckFailed(generation) {
        if (generation !== this.#generation)
            return;
        this.#auth.error = 'Unable to check your session. Try again.';
        this.#onChange();
    }
    async #checkSession(generation) {
        this.#auth.status = 'checking';
        try {
            const res = await fetch('/api/session', { signal: this.#readSignal() });
            if (generation !== this.#generation)
                return;
            if (!res.ok) {
                const lost = await authenticationRequired(res);
                if (generation !== this.#generation)
                    return;
                if (lost)
                    this.#sessionLost(generation);
                else
                    this.#sessionCheckFailed(generation);
                return;
            }
            const session = await res.json();
            if (generation !== this.#generation)
                return;
            if (typeof session !== 'object' || session === null ||
                !('username' in session) || typeof session.username !== 'string' ||
                !('binding' in session) || typeof session.binding !== 'string' ||
                !/^[a-f0-9]{64}$/.test(session.binding)) {
                throw new Error('Invalid session response');
            }
            this.#transition('authenticated', session.binding);
            await this.#loadState(this.#generation);
        }
        catch {
            this.#sessionCheckFailed(generation);
        }
    }
    #privateInit(method, body) {
        const init = requestInit(method, body);
        const headers = new Headers(init.headers);
        headers.set('X-Steady-Session', this.#binding);
        return { ...init, headers, signal: this.#controller.signal };
    }
    async #loadState(generation) {
        const revision = this.#revision;
        const idleAtDispatch = this.#queued === 0 && this.#flushing === 0;
        const sequence = ++this.#readSequence;
        let next;
        try {
            const res = await fetch('/api/state', { ...this.#privateInit('GET'), signal: this.#readSignal() });
            if (generation !== this.#generation)
                return;
            if (!res.ok) {
                const lost = await authenticationRequired(res);
                if (lost)
                    this.#sessionLost(generation);
                return;
            }
            // The server is ours: a malformed body throws in toState and counts as a failed load.
            next = toState((await res.json()));
            if (generation !== this.#generation)
                return;
        }
        catch {
            return;
        }
        // A completed write can drain before this body arrives; revision and dispatch-idle checks
        // still reject its older snapshot. Polling or the queue drain supplies the next read.
        if (!idleAtDispatch || this.#queued > 0 || this.#flushing > 0 ||
            revision !== this.#revision || sequence !== this.#readSequence) {
            if (this.#queued > 0)
                this.#stale = true;
            return;
        }
        // Names typed but not yet sent are newer than the server's copy.
        for (const [id, name] of this.#pending) {
            const habit = next.habits.find((h) => h.id === id);
            if (habit !== undefined) {
                habit.name = name;
            }
        }
        this.#state = next;
        this.#auth.error = '';
        this.#onChange();
    }
    addHabit(name) {
        if (this.#auth.status !== 'authenticated')
            return;
        const id = crypto.randomUUID();
        this.#revision += 1;
        this.#state.habits.push({ id, name, createdAt: Math.floor(Date.now() / 1000), done: {} });
        this.#onChange();
        this.#enqueue('POST', '/api/habits', { id, name });
    }
    renameHabit(id, name) {
        const habit = this.#find(id);
        if (habit === undefined) {
            return;
        }
        this.#revision += 1;
        habit.name = name;
        this.#pending.set(id, name);
        clearTimeout(this.#timers.get(id));
        const timer = setTimeout(() => this.#sendPending(id), RENAME_DEBOUNCE_MS);
        this.#timers.set(id, timer);
    }
    commitRename(id) {
        this.#sendPending(id);
        const habit = this.#find(id);
        if (habit === undefined) {
            return;
        }
        this.#revision += 1;
        habit.name = normalizeName(habit.name);
        this.#onChange();
    }
    flushPending() {
        if (this.#auth.status !== 'authenticated')
            return;
        this.#revision += 1;
        for (const timer of this.#timers.values()) {
            clearTimeout(timer);
        }
        for (const [id, name] of this.#pending) {
            // keepalive lets the request outlive the page that is being closed.
            const init = { ...this.#privateInit('PATCH', { name }), keepalive: true };
            const generation = this.#generation;
            this.#flushing += 1;
            void this.#track(this.#flush(habitPath(id), init, generation));
        }
        this.#timers.clear();
        this.#pending.clear();
    }
    deleteHabit(id) {
        const habit = this.#find(id);
        if (habit === undefined) {
            return null;
        }
        this.#revision += 1;
        this.#sendPending(id);
        habit.name = normalizeName(habit.name);
        const index = this.#state.habits.indexOf(habit);
        this.#state.habits.splice(index, 1);
        this.#onChange();
        this.#enqueue('DELETE', habitPath(id));
        return { habit, index };
    }
    restoreHabit(d) {
        if (this.#auth.status !== 'authenticated')
            return;
        const { habits } = this.#state;
        if (habits.some((h) => h.id === d.habit.id)) {
            return;
        }
        this.#revision += 1;
        habits.splice(Math.min(d.index, habits.length), 0, d.habit);
        this.#onChange();
        this.#enqueue('POST', `${habitPath(d.habit.id)}/restore`);
    }
    toggleDay(id, day) {
        const habit = this.#find(id);
        if (habit === undefined) {
            return;
        }
        this.#revision += 1;
        let method;
        if (habit.done[day]) {
            delete habit.done[day];
            method = 'DELETE';
        }
        else {
            habit.done[day] = true;
            method = 'PUT';
        }
        this.#onChange();
        this.#enqueue(method, `${habitPath(id)}/days/${encodeURIComponent(day)}`);
    }
    setView(view) {
        if (this.#auth.status !== 'authenticated')
            return;
        if (view === this.#state.view) {
            return;
        }
        this.#revision += 1;
        this.#state.view = view;
        this.#onChange();
        this.#enqueue('PUT', '/api/settings', { view });
    }
    setAccent(accent) {
        if (this.#auth.status !== 'authenticated')
            return;
        if (accent === this.#state.accent || !PALETTE.includes(accent)) {
            return;
        }
        this.#revision += 1;
        this.#state.accent = accent;
        this.#onChange();
        this.#enqueue('PUT', '/api/settings', { accent });
    }
    #find(id) {
        return this.#state.habits.find((h) => h.id === id);
    }
    // Clears the debounce timer for id and sends its pending name, if any, through the queue.
    #sendPending(id) {
        clearTimeout(this.#timers.get(id));
        this.#timers.delete(id);
        const name = this.#pending.get(id);
        if (name === undefined) {
            return;
        }
        this.#revision += 1;
        this.#pending.delete(id);
        this.#enqueue('PATCH', habitPath(id), { name });
    }
    #enqueue(method, path, body) {
        this.#revision += 1;
        const generation = this.#generation;
        const init = this.#privateInit(method, body);
        this.#queued += 1;
        // The trailing catch keeps the chain alive: a failure in one step must not block the next.
        this.#queue = this.#track(this.#queue.then(async () => {
            if (generation !== this.#generation)
                return;
            await this.#send(path, init, generation);
        }).catch(() => undefined));
    }
    async #flush(path, init, generation) {
        try {
            const res = await fetch(path, init);
            if (await authenticationRequired(res))
                this.#sessionLost(generation);
        }
        catch {
            // The page is closing; abort and network failures cannot be recovered here.
        }
        finally {
            if (generation === this.#generation)
                this.#flushing -= 1;
        }
    }
    async #send(path, init, generation) {
        try {
            const res = await fetch(path, init);
            if (generation !== this.#generation)
                return;
            if (!res.ok) {
                const lost = await authenticationRequired(res);
                if (generation !== this.#generation)
                    return;
                if (lost) {
                    this.#sessionLost(generation);
                    return;
                }
                this.#stale = true;
            }
        }
        catch {
            if (generation !== this.#generation)
                return;
            this.#stale = true;
        }
        this.#queued -= 1;
        // Reloading earlier would overwrite optimistic edits whose requests are still queued.
        if (this.#stale && this.#queued === 0) {
            // Cleared first: load() sets it again when it discards a response that raced with an edit.
            this.#stale = false;
            await this.#loadState(generation);
        }
    }
}
