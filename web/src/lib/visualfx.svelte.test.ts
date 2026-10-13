import { describe, expect, it } from 'vitest';
import { FxStore } from './visualfx.svelte';
import { FX_KEY, FX_OFF_CLASS, fxFromURL, loadFxMode } from './visualfx';

function memStorage() {
  const saved = new Map<string, string>();
  return { saved, storage: { getItem: (k: string) => saved.get(k) ?? null, setItem: (k: string, v: string) => saved.set(k, v) } as unknown as Storage };
}

// fakeRoot stands in for <html>: these tests run without a DOM.
function fakeRoot() {
  const cls = new Set<string>();
  const classList = { toggle: (c: string, on: boolean) => (on ? cls.add(c) : cls.delete(c), on), contains: (c: string) => cls.has(c) };
  return { classList } as unknown as HTMLElement;
}

describe('visual effects setting', () => {
  it('defaults on, and a foreign blob reads as the default', () => {
    const { saved, storage } = memStorage();
    expect(loadFxMode(storage)).toBe('on');
    saved.set(FX_KEY, '{"version":1,"mode":"sparkly"}');
    expect(loadFxMode(storage)).toBe('on');
    expect(loadFxMode(null)).toBe('on');
  });

  it('reads only fx=on|off from the URL', () => {
    expect(fxFromURL('?fx=off')).toBe('off');
    expect(fxFromURL('?table=t2&fx=on')).toBe('on');
    expect(fxFromURL('?fx=maybe')).toBeNull();
    expect(fxFromURL('')).toBeNull();
  });

  it('a URL override wins over storage and is saved', () => {
    const { storage } = memStorage();
    new FxStore(storage, '', null).setMode('on');
    const root = fakeRoot();
    const s = new FxStore(storage, '?fx=off', root);
    expect(s.mode).toBe('off');
    expect(root.classList.contains(FX_OFF_CLASS)).toBe(true);
    expect(new FxStore(storage, '', null).mode).toBe('off');
  });

  it('setMode persists and toggles the root class', () => {
    const { storage } = memStorage();
    const root = fakeRoot();
    const s = new FxStore(storage, '', root);
    expect(root.classList.contains(FX_OFF_CLASS)).toBe(false);
    s.setMode('off');
    expect(root.classList.contains(FX_OFF_CLASS)).toBe(true);
    s.setMode('on');
    expect(root.classList.contains(FX_OFF_CLASS)).toBe(false);
    expect(new FxStore(storage, '', null).mode).toBe('on');
  });
});
