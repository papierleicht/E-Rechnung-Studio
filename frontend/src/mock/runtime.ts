/**
 * Browser stand-in for `@wailsio/runtime` in the live demo (`npm run build:demo`).
 *
 * The generated bindings call `Call.ByID(<id>, ...args)`. The IDs are read from
 * the binding sources at build time, so regenerated bindings keep working
 * without editing this file. Every call is answered by the in-memory backend in
 * `backend.ts`; nothing is sent over the network.
 */
import { CancellablePromise } from "@wailsio/runtime/cancellable";
import { call } from "./backend";
import { demoHint } from "./hints";

export { CancellablePromise };
export * from "@wailsio/runtime/cancellable";

const sources = import.meta.glob(
  "../../bindings/papierleicht.de/e-rechnung-studio/internal/app/*service.ts",
  {
    query: "?raw",
    import: "default",
    eager: true,
  },
) as Record<string, string>;

const methods = new Map<number, string>();
for (const [path, source] of Object.entries(sources)) {
  const file = path.split("/").pop()!.replace(/\.ts$/, "");
  const service = file[0].toUpperCase() + file.slice(1, -"service".length) + "Service";
  for (const match of source.matchAll(
    /export function (\w+)\([\s\S]*?\{\s*return \$Call\.ByID\((\d+)/g,
  )) {
    methods.set(Number(match[2]), `${service}.${match[1]}`);
  }
}

// Answers arrive asynchronously like real IPC, so UI busy states behave as in the app.
function respond<T>(produce: () => T | Promise<T>): CancellablePromise<T> {
  return new CancellablePromise<T>((resolve, reject) => {
    setTimeout(() => {
      Promise.resolve()
        .then(produce)
        .then((value) => resolve(JSON.parse(JSON.stringify(value ?? null))), reject);
    }, 40);
  });
}

export const Call = {
  ByID(id: number, ...args: unknown[]): CancellablePromise<any> {
    const method = methods.get(id);
    return respond(() => {
      if (!method) throw new Error(demoHint("dieser Schritt"));
      return call(method, JSON.parse(JSON.stringify(args)));
    });
  },
  ByName(name: string, ...args: unknown[]): CancellablePromise<any> {
    return respond(() => call(name.split(".").slice(-2).join("."), args));
  },
};

export { Events } from "./events";

export const Browser = {
  OpenURL(url: string | URL) {
    window.open(String(url), "_blank", "noopener,noreferrer");
    return Promise.resolve();
  },
};

export const Window = {
  Print() {
    window.print();
    return Promise.resolve();
  },
};

export const Clipboard = {
  SetText(text: string) {
    return navigator.clipboard.writeText(text).then(() => true);
  },
  Text() {
    return navigator.clipboard.readText();
  },
};

export const Dialogs = {
  SaveFile(_options?: unknown): Promise<string> {
    return Promise.reject(
      new Error(
        "Dateien speichert die Live-Demo nicht – in der Desktop-App wählst du hier den Speicherort aus.",
      ),
    );
  },
  OpenFile(_options?: unknown): Promise<string | string[]> {
    return Promise.reject(
      new Error(
        "Dateien öffnet die Live-Demo nicht – in der Desktop-App wählst du hier eine Datei aus.",
      ),
    );
  },
  Info: () => Promise.resolve(""),
  Warning: () => Promise.resolve(""),
  Error: () => Promise.resolve(""),
  Question: () => Promise.resolve(""),
};

const identity = (value: unknown) => value;
/** Minimal `Create` helpers used by generated model files. */
export const Create = {
  Any: identity,
  Array: () => identity,
  Map: () => identity,
  Nullable: () => identity,
  Struct: () => identity,
  ByteSlice: identity,
  Events: {} as Record<string, unknown>,
};
