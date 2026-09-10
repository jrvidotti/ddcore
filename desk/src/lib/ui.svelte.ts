// Global UI state: toasts and modal dialogs, driven from anywhere (controls,
// form scripts, API errors).
import type { Field } from "./meta";
import { __ } from "./boot.svelte";

export interface Toast { id: number; message: string; title?: string; indicator?: string; timeout?: number }
export interface DialogSpec {
  title: string;
  fields?: Field[];
  values?: Record<string, any>;
  primaryLabel?: string;
  secondaryLabel?: string;
  primaryAction?: (values: Record<string, any>, dialog: DialogHandle) => any;
  dangerLabel?: string;
  dangerAction?: (values: Record<string, any>, dialog: DialogHandle) => any;
  onChange?: (fieldname: string, values: Record<string, any>, dialog: DialogHandle) => void;
  /** free-form message shown above the fields */
  message?: string;
  size?: "sm" | "md" | "lg";
}
export interface DialogHandle {
  id: number;
  spec: DialogSpec;
  values: Record<string, any>;
  html: Record<string, string>;
  setValue(f: string, v: any): void;
  getValue(f: string): any;
  setHtml(f: string, html: string): void;
  hide(): void;
  show(): void;
  busy: boolean;
}

export const ui = $state<{ toasts: Toast[]; dialogs: DialogHandle[]; busy: number }>({ toasts: [], dialogs: [], busy: 0 });

let seq = 0;

export function toast(message: string, opts: { title?: string; indicator?: string; timeout?: number } = {}) {
  const t: Toast = { id: ++seq, message, ...opts };
  ui.toasts.push(t);
  setTimeout(() => { const i = ui.toasts.findIndex((x) => x.id === t.id); if (i >= 0) ui.toasts.splice(i, 1); }, opts.timeout ?? 5000);
}

/** A readable heading per error type, so a toast never shows "PermissionError". */
const ERROR_TITLES: Record<string, string> = {
  ValidationError: "Please check the form",
  MandatoryError: "Required fields",
  PermissionError: "Not allowed",
  DoesNotExistError: "Not found",
  LinkExistsError: "Still in use",
  TimestampMismatchError: "Changed by someone else",
  DuplicateEntryError: "Already exists",
  AuthenticationError: "Sign in to continue",
  InternalError: "Something went wrong",
};

export function showError(e: any) {
  const title = e?.title || __(ERROR_TITLES[e?.type] || "Error");
  toast(String(e?.message || e), { title, indicator: "red", timeout: 9000 });
}

export function dialog(spec: DialogSpec): DialogHandle {
  // ui.dialogs is deep-reactive state, so values/html become reactive once pushed;
  // build the handle as a plain object and read it back from the proxy.
  const id = ++seq;
  const values: Record<string, any> = { ...(spec.values || {}) };
  for (const f of spec.fields || []) if (f.fieldname && values[f.fieldname] === undefined && f.default !== undefined) values[f.fieldname] = f.default;
  const h: DialogHandle = {
    id, spec, values, html: {}, busy: false,
    setValue(f, v) { const me = live(); me.values[f] = v; spec.onChange?.(f, me.values, me); },
    getValue(f) { return live().values[f]; },
    setHtml(f, html) { live().html[f] = html; },
    hide() { const i = ui.dialogs.findIndex((d) => d.id === id); if (i >= 0) ui.dialogs.splice(i, 1); },
    show() { if (!ui.dialogs.find((d) => d.id === id)) ui.dialogs.push(h); },
  };
  const live = () => ui.dialogs.find((d) => d.id === id) || h;
  return h;
}

export function confirm(message: string, title = "Confirmar"): Promise<boolean> {
  return new Promise((resolve) => {
    const h = dialog({
      title, message, primaryLabel: "Sim", secondaryLabel: "Não", size: "sm",
      primaryAction: () => { resolve(true); h.hide(); },
    });
    (h as any).onCancel = () => resolve(false);
    h.show();
  });
}

export function prompt(title: string, fields: Field[], primaryLabel = "OK"): Promise<Record<string, any> | null> {
  return new Promise((resolve) => {
    const h = dialog({ title, fields, primaryLabel, primaryAction: (v) => { resolve(v); h.hide(); } });
    (h as any).onCancel = () => resolve(null);
    h.show();
  });
}
