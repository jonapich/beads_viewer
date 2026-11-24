/** A controllable EventSource for tests (the real one is injected via startLive({ EventSource })). */
export class FakeEventSource {
  static instances: FakeEventSource[] = [];
  static reset(): void {
    FakeEventSource.instances = [];
  }
  static get last(): FakeEventSource {
    const s = FakeEventSource.instances[FakeEventSource.instances.length - 1];
    if (!s) throw new Error("no FakeEventSource was created");
    return s;
  }

  readonly url: string;
  /** 0 CONNECTING, 1 OPEN, 2 CLOSED */
  readyState = 0;
  closed = false;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  private handlers = new Map<string, Array<(e: { data: string }) => void>>();

  constructor(url: string) {
    this.url = url;
    FakeEventSource.instances.push(this);
  }

  addEventListener(type: string, fn: (e: { data: string }) => void): void {
    this.handlers.set(type, [...(this.handlers.get(type) ?? []), fn]);
  }

  close(): void {
    this.closed = true;
    this.readyState = 2;
  }

  /** The connection opened. */
  open(): void {
    this.readyState = 1;
    this.onopen?.();
  }

  /** The server sent an event. */
  send(type: string, data: unknown): void {
    const raw = typeof data === "string" ? data : JSON.stringify(data);
    for (const fn of this.handlers.get(type) ?? []) fn({ data: raw });
  }

  /** A connection error; closed=true is what the browser does after an HTTP error. */
  fail(closed: boolean): void {
    this.readyState = closed ? 2 : 0;
    this.onerror?.();
  }
}

export const asEventSource = FakeEventSource as unknown as typeof EventSource;
