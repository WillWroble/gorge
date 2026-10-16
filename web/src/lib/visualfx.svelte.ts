import { safeStorage } from './storage';
import { FX_OFF_CLASS, fxFromURL, loadFxMode, saveFxMode, type FxMode } from './visualfx';

/**
 * FxStore is the one reactive visual-effects setting (lib/visualfx). It
 * mirrors the mode onto <html> as FX_OFF_CLASS, which is all the filtered
 * CSS reads, so no component has to import it.
 */
export class FxStore {
  mode = $state<FxMode>('on');
  #storage: Storage | null;
  #root: HTMLElement | null;

  constructor(
    storage: Storage | null = safeStorage(),
    search: string = typeof location === 'undefined' ? '' : location.search,
    root: HTMLElement | null = typeof document === 'undefined' ? null : document.documentElement,
  ) {
    this.#storage = storage;
    this.#root = root;
    const fromURL = fxFromURL(search);
    if (fromURL) saveFxMode(storage, fromURL);
    this.mode = fromURL ?? loadFxMode(storage);
    this.#apply();
  }

  setMode(mode: FxMode): void {
    this.mode = mode;
    saveFxMode(this.#storage, mode);
    this.#apply();
  }

  #apply(): void {
    this.#root?.classList.toggle(FX_OFF_CLASS, this.mode === 'off');
  }
}

export const fxStore = new FxStore();
