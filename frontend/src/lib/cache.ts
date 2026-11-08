// Last loaded data per key. Pages show it right away when opened and still
// reload in the background, so a tab switch never starts from an empty page.
const store = new Map<string, unknown>();

export const cached = <T>(key: string): T | undefined => store.get(key) as T | undefined;

export function remember<T>(key: string, value: T): T {
  store.set(key, value);
  return value;
}
