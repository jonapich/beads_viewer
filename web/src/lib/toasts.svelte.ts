export type ToastKind = "success" | "danger" | "warning";

export interface Toast {
  id: number;
  kind: ToastKind;
  text: string;
}

/** How long a toast stays: a danger toast carries bd's stderr, which takes longer to read. */
export const TOAST_MS: Record<ToastKind, number> = { success: 4000, warning: 8000, danger: 8000 };

/** The one-line messages the write actions raise. */
export class Toasts {
  items = $state.raw<Toast[]>([]);
  private seq = 0;
  private timers = new Map<number, ReturnType<typeof setTimeout>>();

  push(kind: ToastKind, text: string): number {
    const id = ++this.seq;
    this.items = [...this.items, { id, kind, text }];
    this.timers.set(
      id,
      setTimeout(() => this.dismiss(id), TOAST_MS[kind]),
    );
    return id;
  }

  dismiss(id: number): void {
    clearTimeout(this.timers.get(id));
    this.timers.delete(id);
    this.items = this.items.filter((t) => t.id !== id);
  }
}

export const toasts = new Toasts();
