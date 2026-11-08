const SYNC_INTERVAL_MS = 3000;

export function startSync(load: () => Promise<void>, page: Document, host: Window): () => void {
  let timer: number | undefined;
  let checking = false;
  let stopped = false;

  const pause = (): void => {
    if (timer !== undefined) {
      host.clearInterval(timer);
      timer = undefined;
    }
  };

  const check = async (): Promise<void> => {
    if (stopped || page.visibilityState !== 'visible' || checking) return;
    checking = true;
    try {
      await load();
    } catch {
      // The loader owns error reporting; a failed check must allow the next wake to retry.
    } finally {
      checking = false;
    }
  };

  const wake = (): void => {
    if (stopped) return;
    if (page.visibilityState !== 'visible') {
      pause();
      return;
    }
    if (timer === undefined) {
      timer = host.setInterval(() => { void check(); }, SYNC_INTERVAL_MS);
    }
    void check();
  };

  const hostSignals = ['focus', 'online', 'pageshow'] as const;
  for (const signal of hostSignals) host.addEventListener(signal, wake);
  host.addEventListener('pagehide', pause);
  page.addEventListener('visibilitychange', wake);
  page.addEventListener('resume', wake);
  wake();

  return (): void => {
    if (stopped) return;
    stopped = true;
    pause();
    for (const signal of hostSignals) host.removeEventListener(signal, wake);
    host.removeEventListener('pagehide', pause);
    page.removeEventListener('visibilitychange', wake);
    page.removeEventListener('resume', wake);
  };
}
