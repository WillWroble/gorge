/**
 * Visual effects (GPU filters): a per-browser view setting, on / off,
 * default on. Off drops the board's backdrop-filters and the zoomed card's
 * drop-shadow, the shader-heavy paints a spectator board re-runs on every
 * repaint. It exists to isolate a GPU hang: on 2026-10-13 Chrome's GPU
 * process on an AMD iGPU raised amdgpu page faults and ring timeouts that
 * wedged the desktop while the demo was open, and backdrop-filter is the
 * prime suspect.
 *
 * `?fx=off` / `?fx=on` in the URL sets the value and saves it, so one link
 * reproduces either side of an A/B. Client-side only: the value lives in
 * this browser under `gorge.fx.v1`.
 */
export type FxMode = 'on' | 'off';

export const FX_MODES: readonly FxMode[] = ['on', 'off'];
export const FX_MODE_LABELS: Record<FxMode, string> = { on: 'On', off: 'Off' };
export const DEFAULT_FX_MODE: FxMode = 'on';
export const FX_KEY = 'gorge.fx.v1';
/** FX_OFF_CLASS goes on <html> while effects are off; the filtered rules are scoped under it. */
export const FX_OFF_CLASS = 'fx-off';

export function isFxMode(v: unknown): v is FxMode {
  return typeof v === 'string' && (FX_MODES as readonly string[]).includes(v);
}

/** fxFromURL reads a `fx=` override from a query string; anything else is no override. */
export function fxFromURL(search: string): FxMode | null {
  try {
    const v = new URLSearchParams(search).get('fx');
    return isFxMode(v) ? v : null;
  } catch {
    return null;
  }
}

/** loadFxMode reads the saved mode; a missing, unreadable or foreign blob is the default. */
export function loadFxMode(storage: Storage | null): FxMode {
  try {
    const raw = storage?.getItem(FX_KEY);
    if (!raw) return DEFAULT_FX_MODE;
    const blob = JSON.parse(raw) as { version?: unknown; mode?: unknown };
    return blob && blob.version === 1 && isFxMode(blob.mode) ? blob.mode : DEFAULT_FX_MODE;
  } catch {
    return DEFAULT_FX_MODE;
  }
}

export function saveFxMode(storage: Storage | null, mode: FxMode): void {
  try {
    storage?.setItem(FX_KEY, JSON.stringify({ version: 1, mode }));
  } catch {
    // A browser refusing site data keeps the choice for this page only.
  }
}
