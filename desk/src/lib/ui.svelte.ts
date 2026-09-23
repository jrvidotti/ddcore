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
  /** leave out the secondary (Cancel) button, for a dialog whose primary action already cancels */
  hideSecondary?: boolean;
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
  setDfProperty(fieldname: string, prop: string, value: any): void;
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
  let body = String(e?.message || e);
  // Only for the errors the reader cannot act on. A validation message is
  // about what they typed; a 500 is about us, and the id is the one thing
  // they can carry into a support ticket that finds the server-side row.
  if (e?.requestId && e?.status >= 500) body += ` [${e.requestId}]`;
  toast(body, { title, indicator: "red", timeout: 9000 });
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
    setDfProperty(fieldname, prop, value) {
      const field = live().spec.fields?.find((f) => f.fieldname === fieldname);
      if (field) (field as any)[prop] = value;
    },
    hide() { const i = ui.dialogs.findIndex((d) => d.id === id); if (i >= 0) ui.dialogs.splice(i, 1); },
    show() { if (!ui.dialogs.find((d) => d.id === id)) ui.dialogs.push(h); },
  };
  const live = () => ui.dialogs.find((d) => d.id === id) || h;
  return h;
}

export interface ConfirmOptions {
  /**
   * The answer loses something (deletes, discards, revokes). "Yes" turns into
   * a danger button and "No" becomes the primary one, so Enter keeps the data.
   */
  destructive?: boolean;
}

export function confirm(message: string, title?: string, opts: ConfirmOptions = {}): Promise<boolean> {
  return new Promise((resolve) => {
    const base = { title: title ?? __("Confirm"), message, size: "sm" as const };
    const h = opts.destructive
      ? dialog({
        ...base, hideSecondary: true,
        primaryLabel: __("No"), primaryAction: () => { resolve(false); h.hide(); },
        dangerLabel: __("Yes"), dangerAction: () => { resolve(true); h.hide(); },
      })
      : dialog({
        ...base, primaryLabel: __("Yes"), secondaryLabel: __("No"),
        primaryAction: () => { resolve(true); h.hide(); },
      });
    (h as any).onCancel = () => resolve(false);
    h.show();
  });
}

export function prompt(title: string, fields: Field[], primaryLabel = __("OK")): Promise<Record<string, any> | null> {
  return new Promise((resolve) => {
    const h = dialog({ title, fields, primaryLabel, primaryAction: (v) => { resolve(v); h.hide(); } });
    (h as any).onCancel = () => resolve(null);
    h.show();
  });
}
