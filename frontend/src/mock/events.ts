/** Wails-style events inside the demo tab. */
type Listener = (event: { name: string; data: unknown }) => void;
const listeners = new Map<string, Set<Listener>>();

export const Events = {
  On(name: string, callback: Listener) {
    const set = listeners.get(name) ?? new Set();
    set.add(callback);
    listeners.set(name, set);
    return () => void set.delete(callback);
  },
  Emit(name: string, data?: unknown) {
    for (const callback of listeners.get(name) ?? []) callback({ name, data });
    return Promise.resolve(false);
  },
};

/** Lets the demo backend publish status events like the Go services do. */
export function emit(name: string, data: unknown) {
  void Events.Emit(name, data);
}
