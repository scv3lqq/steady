# Steady: design spec

Build **Steady**, a minimal single-page habit tracker. The design below is final: follow it exactly and don't add features, sections or decoration that aren't described.

## Deviations from the original prompt
- Storage: habits, the chosen view and the accent live in SQLite behind the Go server's JSON API (see AGENTS.md), not in localStorage; "survive a reload" means against the same server.
- Serving: the Go binary serves `web/index.html`, `web/styles.css` and `web/dist/` (embedded); `npm run build` still compiles the TypeScript; `make run` replaces "any static server".
- Frontend files live under `web/`; `src/store.ts` holds state plus API sync instead of localStorage access.
- The two em dashes the UI shows (zero streak, aria-label separator) are built in code with `String.fromCharCode(0x2014)`; this document names them instead of printing them.

## Stack (strict)
- **No runtime dependencies** and no frameworks: no React, Vue, Tailwind or UI kits.
- TypeScript (`strict: true`), compiled by `tsc` to ES modules (`target: ES2022`, `module: ES2022`). The only devDependency is `typescript`.
- Files: `index.html`, `styles.css`, `src/main.ts`, `src/store.ts` (state + localStorage), `src/dates.ts` (date helpers), `src/render.ts` (DOM). Output goes to `dist/`.
- `package.json` scripts: `build` (`tsc`), `dev` (`tsc --watch`), and serve the folder with any static server.
- Rendering: plain DOM with one `render()` function that redraws the list from state. Use **event delegation** on the root (`data-action`, `data-id`, `data-date`). Don't use `innerHTML` with user text; use `textContent`.
- CSS: custom properties for tokens, flex/grid with `gap`, no preprocessors.

## Visual tokens
- Font: **Onest** 400/500/600 (Google Fonts), fallback `system-ui, sans-serif`. Use `font-variant-numeric: tabular-nums` on all numbers.
- Dark theme only:
  - `--bg #161513`, `--surface #1F1D1A`, `--line #2C2A26`, `--hover #26241F`
  - `--ink #ECE7DC` (main text), `--muted #9C968A`, `--faint #847E72`
  - `--empty #3A3732` (empty dot), `--cell-border #55514A` (empty week square)
  - `--accent` is chosen by the user, default `#4FA872`
- Accent palette (10 colors): `#4FA872 #8FBF4A #3FB8B0 #5B8CEB #B07FE0 #E07BB0 #D96A6A #E08A45 #E3C24F #ECE7DC`.
- Checkmark on a filled square uses `--bg` (dark on a light accent).
- Radii: week square 9px, buttons 8px, segmented control 11px, toast 12px. Use no shadows except on the toast.

## Layout
A centered column, `max-width: 600px`, padding `56px 20px 96px`, vertical gap 36px between blocks. It must reflow down to 320px.

1. **Header**
   - Row: on the left the wordmark **"Steady"** (15px/600); on the right the date `Sunday, November 1` (14px, muted, `nowrap`) and a 32×32 button holding a 14px accent-colored circle. The button toggles the palette.
   - Palette (when open): right-aligned row of 32×32 buttons, each with a 22px circle. The selected one gets a ring: `box-shadow: 0 0 0 2px var(--bg), 0 0 0 4px <color>`. Use `role="radiogroup"`. Choosing a color persists it; the palette stays open until toggled.
   - H1 at 40px/500, letter-spacing −0.03em: `"X of N today"`, or `"All done today"` when X = N, or `"Let’s start"` when there are no habits.
   - Progress bar: 4px tall, track `--line`, fill `--accent`, width = done today / total, 0.3s width transition.

2. **Toolbar row** (space-between)
   - Segmented control on the left: **Week / Month / Year**, inside a `--surface` container with a 1px `--line` border and 3px padding. Each tab is 34px tall; the active tab has `--empty` background and `--ink` text, inactive tabs are muted. Use `role="tablist"`. Persist the chosen view.
   - A quiet text button on the right, `+ Habit` (14px, muted, transparent; hover background `--surface`, text `--ink`). It must **not** look like a primary CTA.
   - Clicking it shows an inline row under the toolbar: an underlined borderless input (17px, autofocus, placeholder `Name it, press Enter`, maxlength 60) plus a hint `Esc to cancel` (12px, faint). Enter adds the habit to the end and closes the row. Esc closes the row, and so does blur while the input is empty.

3. **List: Week view**
   - Header row: an empty flexible spacer (`flex: 1 1 140px`), then 7 columns 34px wide with weekday + date (`Mo`/`27`, 11px). Today's column is ink/600, the others faint. Then an 84px spacer.
   - Each habit row is a wrapping flex row with a 1px `--line` bottom border and 14px vertical padding:
     - Inline-editable name input (17px, transparent, 1px ink underline on focus). Saving happens on input; an empty name on blur becomes `Untitled`.
     - 7 squares, 34×34, 6px gap. Done = accent fill + accent border + check icon. Today, not done = `--surface` fill + 1.5px ink border. Past, not done = transparent + `--cell-border`. Clicking toggles the day.
     - Right side (84px): the streak (`5 days`, ink when > 0, an em dash (U+2014), faint, when 0), then a × delete button (32×32, faint; hover `--hover` / ink).

4. **List: Month view (last 30 days)**
   - Each habit is a block with a top border and 16px/18px padding. Top line: name input, the count `18 of 30` (13px, muted), then ×.
   - Below it, a grid of `repeat(30, minmax(0,1fr))` with a 4px gap holding round dots (`aspect-ratio: 1`). Done = accent, otherwise `--empty`; today gets an inset 1.5px ink ring. Clicking toggles the day.
   - Caption row (11px, faint, space-between): the start date `Oct 3` and `today`.

5. **List: Year view (GitHub-style, 53 weeks)**
   - Same block header, with the count `214 days`.
   - Grid: `grid-template-columns: repeat(53, minmax(0,1fr)); grid-template-rows: repeat(7, auto); grid-auto-flow: column; gap: 3px`. Columns are weeks and rows run Monday→Sunday. The last column is the current week; cells after today are `visibility: hidden` and disabled. Squares are radius 2px; today gets an inset 1px ink ring. Clicking toggles the day.
   - Caption: `November 2025` … `today`.

6. **Empty state**: `Nothing here yet` (17px) plus `Tap “+ Habit” above. Starting small is fine.` (14px, muted).

7. **Footer**: `Tap a square or dot to mark a day. Missed one? You can still mark it later.` (13px, faint).

8. **Undo toast**
   - Deleting a habit happens immediately, with **no confirm dialog**. A toast is fixed at bottom center, 28px from the bottom, with `--ink` background, `--bg` text and `role="status"`.
   - It reads `“<name>” deleted` with an **Undo** button (`#D9D3C7`, hover `#C9C2B5`). Undo restores the habit at its original index.
   - The toast auto-hides after 5 seconds; a new delete resets the timer.

## Data and logic
```ts
type Habit = { id: string; name: string; done: Record<string, true> }; // keys: local "YYYY-MM-DD"
```
- Persistence: see Deviations (SQLite via the JSON API).
- Dates use the **local timezone**: build keys from `getFullYear/getMonth/getDate`, never `toISOString`.
- **Streak**: start at today if today is done, otherwise at yesterday, then count consecutive done days backwards.
- Format dates with `Intl.DateTimeFormat('en-US', …)`. Pluralize as `1 day` / `N days`.
- First-run seed: `Drink 2 L of water`, `Walk 30 minutes`, `Read 10 pages`, with some random history over the last year so the Month and Year views aren't empty.
- Re-render on every change. Don't rebuild the input that has focus: patch in place, or skip the name input while it's focused, so the caret doesn't jump while the user types.

## Accessibility
- Every interactive element is a real `<button>` or `<input>`.
- Day buttons have `aria-pressed` and an `aria-label` like `Read 10 pages, October 30`, a space, an em dash (U+2014), a space, `done`.
- Show `:focus-visible` rings: 2px ink outline with a 2px offset.
- Keep hit targets at least 32px; the year/month dots are the exception.
- Under `prefers-reduced-motion`, disable transitions.

## Acceptance
- `npm run build` passes with zero TS errors, and the app opens from `index.html` with no console errors.
- Add, rename, delete + undo, toggling a day in all three views, switching views and changing the accent all work and survive a reload.
- There is no horizontal scroll at 320px wide.
