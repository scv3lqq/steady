export type View = 'week' | 'month' | 'year';

export interface Habit {
  id: string;
  name: string;
  done: Record<string, true>;
}

export interface State {
  habits: Habit[];
  view: View;
  accent: string;
}

export interface Deleted {
  habit: Habit;
  index: number;
}

export const PALETTE: readonly string[] = [
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

interface ApiHabit {
  id: string;
  name: string;
  done: string[];
}

interface ApiState {
  habits: ApiHabit[];
  view: View;
  accent: string;
}

function normalizeName(name: string): string {
  return name.trim() || DEFAULT_NAME;
}

function habitPath(id: string): string {
  return `/api/habits/${encodeURIComponent(id)}`;
}

function requestInit(method: string, body?: unknown): RequestInit {
  if (body === undefined) {
    return { method };
  }
  return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) };
}

function toDone(days: readonly string[]): Record<string, true> {
  const done: Record<string, true> = {};
  for (const day of days) {
    done[day] = true;
  }
  return done;
}

function toState(api: ApiState): State {
  return {
    habits: api.habits.map((h) => ({ id: h.id, name: h.name, done: toDone(h.done) })),
    view: api.view,
    accent: api.accent,
  };
}

export class Store {
  readonly #onChange: () => void;
  #state: State = { habits: [], view: 'week', accent: DEFAULT_ACCENT };
  #queue: Promise<void> = Promise.resolve();
  #queued = 0;
  #stale = false;
  #timers = new Map<string, number>();
  #pending = new Map<string, string>();

  constructor(onChange: () => void) {
    this.#onChange = onChange;
  }

  get state(): State {
    return this.#state;
  }

  async load(): Promise<void> {
    let next: State;
    try {
      const res = await fetch('/api/state');
      if (!res.ok) {
        return;
      }
      // The server is ours: a malformed body throws in toState and counts as a failed load.
      next = toState((await res.json()) as ApiState);
    } catch {
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
    this.#onChange();
  }

  addHabit(name: string): void {
    const id = crypto.randomUUID();
    this.#state.habits.push({ id, name, done: {} });
    this.#onChange();
    this.#enqueue('POST', '/api/habits', { id, name });
  }

  renameHabit(id: string, name: string): void {
    const habit = this.#find(id);
    if (habit === undefined) {
      return;
    }
    habit.name = name;
    this.#pending.set(id, name);
    clearTimeout(this.#timers.get(id));
    const timer = setTimeout(() => this.#sendPending(id), RENAME_DEBOUNCE_MS);
    this.#timers.set(id, timer);
  }

  commitRename(id: string): void {
    this.#sendPending(id);
    const habit = this.#find(id);
    if (habit === undefined) {
      return;
    }
    habit.name = normalizeName(habit.name);
    this.#onChange();
  }

  flushPending(): void {
    for (const timer of this.#timers.values()) {
      clearTimeout(timer);
    }
    for (const [id, name] of this.#pending) {
      // keepalive lets the request outlive the page that is being closed.
      const init: RequestInit = { ...requestInit('PATCH', { name }), keepalive: true };
      void fetch(habitPath(id), init).catch(() => undefined);
    }
    this.#timers.clear();
    this.#pending.clear();
  }

  deleteHabit(id: string): Deleted | null {
    const habit = this.#find(id);
    if (habit === undefined) {
      return null;
    }
    this.#sendPending(id);
    habit.name = normalizeName(habit.name);
    const index = this.#state.habits.indexOf(habit);
    this.#state.habits.splice(index, 1);
    this.#onChange();
    this.#enqueue('DELETE', habitPath(id));
    return { habit, index };
  }

  restoreHabit(d: Deleted): void {
    const { habits } = this.#state;
    if (habits.some((h) => h.id === d.habit.id)) {
      return;
    }
    habits.splice(Math.min(d.index, habits.length), 0, d.habit);
    this.#onChange();
    this.#enqueue('POST', `${habitPath(d.habit.id)}/restore`);
  }

  toggleDay(id: string, day: string): void {
    const habit = this.#find(id);
    if (habit === undefined) {
      return;
    }
    let method: 'PUT' | 'DELETE';
    if (habit.done[day]) {
      delete habit.done[day];
      method = 'DELETE';
    } else {
      habit.done[day] = true;
      method = 'PUT';
    }
    this.#onChange();
    this.#enqueue(method, `${habitPath(id)}/days/${encodeURIComponent(day)}`);
  }

  setView(view: View): void {
    if (view === this.#state.view) {
      return;
    }
    this.#state.view = view;
    this.#onChange();
    this.#enqueue('PUT', '/api/settings', { view });
  }

  setAccent(accent: string): void {
    if (accent === this.#state.accent || !PALETTE.includes(accent)) {
      return;
    }
    this.#state.accent = accent;
    this.#onChange();
    this.#enqueue('PUT', '/api/settings', { accent });
  }

  #find(id: string): Habit | undefined {
    return this.#state.habits.find((h) => h.id === id);
  }

  // Clears the debounce timer for id and sends its pending name, if any, through the queue.
  #sendPending(id: string): void {
    clearTimeout(this.#timers.get(id));
    this.#timers.delete(id);
    const name = this.#pending.get(id);
    if (name === undefined) {
      return;
    }
    this.#pending.delete(id);
    this.#enqueue('PATCH', habitPath(id), { name });
  }

  #enqueue(method: string, path: string, body?: unknown): void {
    const init = requestInit(method, body);
    this.#queued += 1;
    // The trailing catch keeps the chain alive: a failure in one step must not block the next.
    this.#queue = this.#queue.then(() => this.#send(path, init)).catch(() => undefined);
  }

  async #send(path: string, init: RequestInit): Promise<void> {
    try {
      const res = await fetch(path, init);
      if (!res.ok) {
        this.#stale = true;
      }
    } catch {
      this.#stale = true;
    }
    this.#queued -= 1;
    // Reloading earlier would overwrite optimistic edits whose requests are still queued.
    if (this.#stale && this.#queued === 0) {
      try {
        await this.load();
      } finally {
        this.#stale = false;
      }
    }
  }
}
