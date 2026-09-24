// FormController — the `frm` object form scripts receive. Holds the doc as
// reactive state, tracks dirtiness and exposes the API described in
// @ddcore/desk-sdk (setValue, addButton, call, setQuery...).
import { api } from "./api";
import { getMeta, newDoc, type Meta, type Field, isLayout } from "./meta";
import { evalExpr } from "./expr";
import { toast, showError, ui } from "./ui.svelte";
import { __ } from "./boot.svelte";
import { goto } from "$app/navigation";
import { validateEmailFields } from "./email";
import { registerTitles } from "./titles.svelte";
import { dynamicLinksOf } from "./doctype-selector";
import { getRememberedWorkspace } from "./components/sidebar-workspace";

function recordBase(frm: { basePath: string; doctype: string }): string {
  return frm.basePath || `${currentWsPrefix()}/${encodeURIComponent(frm.doctype)}`;
}

function currentWsPrefix(): string {
  const ws = getRememberedWorkspace();
  return ws ? `/app/${encodeURIComponent(ws)}` : "/app";
}

export interface FormHandlers {
  setup?: (frm: FormController) => void;
  onload?: (frm: FormController) => void;
  refresh?: (frm: FormController) => void;
  validate?: (frm: FormController) => void | boolean;
  beforeSave?: (frm: FormController) => void;
  afterSave?: (frm: FormController) => void;
  onChange?: Record<string, (frm: FormController, cdt?: string, cdn?: string) => void>;
  /** child table row handlers keyed by child doctype */
  children?: Record<string, Record<string, (frm: FormController, row: any) => void>>;
}

const registry = new Map<string, FormHandlers[]>();
export function registerForm(doctype: string, h: FormHandlers) {
  const list = registry.get(doctype) || [];
  list.push(h);
  registry.set(doctype, list);
}
export function formHandlers(doctype: string): FormHandlers[] { return registry.get(doctype) || []; }

const loadedScripts = new Set<string>();
/**
 * Loads /assets/apps/<app>/forms/<snake>.js once per app that ships one: the
 * DocType's own, then each app extending it. Their handlers accumulate — a
 * second script adds behaviour, it does not replace the first.
 */
export async function loadFormScript(meta: Meta) {
  const d = meta.doctype;
  for (const app of d.formApps || []) {
    const url = `/assets/apps/${app}/forms/${snake(d.name)}.js`;
    if (loadedScripts.has(url)) continue;
    loadedScripts.add(url);
    try {
      await import(/* @vite-ignore */ url + "?v=" + (window as any).__ddcoreLoaded);
    } catch (e) {
      console.error("form script", url, e);
      toast(__("Could not load the form script: {0}", [String(e)]), { indicator: "red" });
    }
  }
}

export const snake = (s: string) =>
  s.replace(/([a-z0-9])([A-Z])/g, "$1_$2").toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "");

export interface Button { label: string; group?: string; action: () => any; primary?: boolean }

/**
 * A button rendered inside a field's control, beside the input. The desk owns
 * the markup, so the label is escaped and the field's own `hidden`/`dependsOn`
 * decide whether the button shows at all.
 */
export interface FieldButton {
  /** visible text, or the accessible name and tooltip when `icon` is set */
  label: string;
  /** an Icon name: renders the button icon-only, with `label` as its tooltip */
  icon?: string;
  onClick: () => any;
  /** identity within the field; a second call with the same key replaces the button */
  key?: string;
}

export class FormController {
  doc = $state<any>({});
  original = "";
  meta: Meta;
  doctype: string;
  /**
   * Where this doctype's list lives and its records under it. A page with its
   * own routes (the To-Do page: /app/todo) sets it; otherwise the workspace's.
   */
  basePath = "";
  buttons = $state<Button[]>([]);
  fieldButtons = $state<Record<string, FieldButton[]>>({});
  primaryAction = $state<{ label: string; action: () => any } | null>(null);
  indicators = $state<{ label: string; color: string }[]>([]);
  dfProps = $state<Record<string, Partial<Field>>>({});
  queries = new Map<string, () => { filters?: any }>();
  saving = $state(false);
  loading = $state(true);
  fieldErrors = $state<Record<string, string>>({});
  handlers: FormHandlers[];
  private setupDone = false;

  constructor(meta: Meta, doc: any) {
    this.meta = meta;
    this.doctype = meta.doctype.name;
    if (doc?._linkTitles) registerTitles(doc._linkTitles);
    this.doc = doc;
    this.original = JSON.stringify(doc);
    this.handlers = formHandlers(this.doctype);
    this.loading = false;
  }

  // ------------------------------------------------------------- doc state
  get isSingle() { return !!this.meta.doctype.isSingle; }
  /**
   * A Single is never new: before its first save the server already answers with
   * the declared defaults, which are the settings in effect (`__islocal` only says
   * no row was written yet). Treating it as new labelled it "New" and hid its menu
   * and sidebar.
   */
  get isNew() { return !this.isSingle && (!!this.doc.__islocal || !this.doc.id); }
  isNewDoc() { return this.isNew; }
  get isDirty() { return JSON.stringify(this.doc) !== this.original; }
  get docstatus(): number { return Number(this.doc.docstatus || 0); }
  get isSubmittable() { return !!this.meta.doctype.submittable; }
  get workflow() { return this.doc?._workflow; }
  get readOnly() {
    if (this.workflow && (this.workflow.allowEdit === false || !this.perm?.write)) {
      return true;
    }
    return (this.isSingle && !this.perm?.write) || this.docstatus === 2 || (this.docstatus === 1 && !this.meta.doctype.fields.some((f) => f.allowOnSubmit));
  }
  get perm() { return this.meta.permissions; }

  field(fieldname: string): Field | undefined {
    const f = this.meta.doctype.fields.find((x) => x.fieldname === fieldname);
    if (!f) return undefined;
    const props = { ...(this.dfProps[fieldname] || {}) };
    if (fieldname === "id" && !this.isNew && !props.description && this.meta.doctype.idGeneration?.prompt) {
      props.description = __("To change it, click the title above or Rename in the menu.");
    }
    return { ...f, ...props };
  }

  /** Whether a field can be edited right now (docstatus, readOnly, allowOnSubmit, readOnlyDependsOn). */
  isFieldEditable(f: Field): boolean {
    if (this.workflow && (this.workflow.allowEdit === false || !this.perm?.write)) return false;
    if (this.isSingle && (!this.perm.write || f.fieldname === "id")) return false;
    if (f.fieldname === "id" && !this.isNew) return false;
    if (f.readOnly) return false;
    if (this.docstatus === 2) return false;
    if (this.docstatus === 1 && !f.allowOnSubmit) return false;
    if (f.readOnlyDependsOn && evalExpr(f.readOnlyDependsOn, this.doc)) return false;
    return true;
  }
  isFieldVisible(f: Field): boolean {
    if (f.hidden) return false;
    if (f.dependsOn && !evalExpr(f.dependsOn, this.doc)) return false;
    return true;
  }
  isFieldMandatory(f: Field): boolean {
    return !!f.reqd || (!!f.mandatoryDependsOn && evalExpr(f.mandatoryDependsOn, this.doc));
  }

  getValue(f: string) { return this.doc[f]; }
  setValue(f: string | Record<string, any>, v?: any) {
    const values = typeof f === "string" ? { [f]: v } : f;
    for (const [k, val] of Object.entries(values)) {
      if (this.doc[k] === val) continue;
      this.doc[k] = val;
      if (this.fieldErrors[k]) delete this.fieldErrors[k];
      this.trigger(k);
      // a document of the old DocType means nothing under the new one
      for (const link of dynamicLinksOf(this.meta.doctype, k)) {
        if (!(link in values) && this.doc[link]) this.setValue(link, null);
      }
    }
    return this;
  }
  set(f: string, v: any) { return this.setValue(f, v); }

  /** Fires the onChange handler for a field (and fetchFrom updates). */
  async trigger(fieldname: string, cdt?: string, cdn?: string) {
    const f = this.field(fieldname);
    if (f && (f.fieldtype === "Link" || f.fieldtype === "Dynamic Link")) await this.applyFetchFrom(fieldname);
    for (const h of this.handlers) {
      try { await h.onChange?.[fieldname]?.(this, cdt, cdn); } catch (e) { showError(e); }
    }
  }

  private async applyFetchFrom(linkField: string) {
    const targets = this.meta.doctype.fields.filter((x) => x.fetchFrom && x.fetchFrom.split(".")[0] === linkField);
    if (!targets.length) return;
    const lf = this.field(linkField)!;
    const link = this.doc[linkField];
    const target = lf.fieldtype === "Dynamic Link" ? this.doc[lf.options] : lf.options;
    if (!link || !target) { for (const t of targets) if (t.readOnly) this.doc[t.fieldname!] = null; return; }
    const fields = targets.map((t) => t.fetchFrom!.split(".")[1]);
    try {
      const rows = await api.list(target, { filters: { id: link }, fields, limit: 1 });
      const row = rows[0] || {};
      for (const t of targets) {
        const src = t.fetchFrom!.split(".")[1];
        if (t.readOnly || !this.doc[t.fieldname!]) this.doc[t.fieldname!] = row[src] ?? null;
      }
    } catch (e) { showError(e); }
  }

  // ------------------------------------------------------------- children
  addChild(fieldname: string, values: any = {}) {
    const f = this.field(fieldname);
    const rows = (this.doc[fieldname] ||= []);
    const row = { doctype: f?.options, parentfield: fieldname, parenttype: this.doctype, idx: rows.length + 1, __islocal: true, ...values };
    rows.push(row);
    return row;
  }
  removeChild(fieldname: string, idx: number) {
    const rows = this.doc[fieldname] || [];
    rows.splice(idx, 1);
    rows.forEach((r: any, i: number) => (r.idx = i + 1));
  }

  // ------------------------------------------------------------- ui api
  addButton(label: string, action: () => any, group?: string) {
    if (!this.buttons.find((b) => b.label === label && b.group === group)) this.buttons.push({ label, action, group });
    return this;
  }
  removeButton(label: string) { this.buttons = this.buttons.filter((b) => b.label !== label); }
  clearButtons() { this.buttons = []; this.indicators = []; this.clearFieldButtons(); }

  /**
   * Attaches a button to a field's control. Adding twice for the same field and
   * `key` replaces the button, so a label carrying a count can be refreshed.
   */
  addFieldButton(fieldname: string, button: FieldButton) {
    const key = button.key || "";
    const list = [...(this.fieldButtons[fieldname] || [])];
    const i = list.findIndex((b) => (b.key || "") === key);
    if (i >= 0) list[i] = { ...button };
    else list.push({ ...button });
    this.fieldButtons[fieldname] = list;
    return this;
  }
  /** Removes one button by `key`, or every button on the field when no key is given. */
  removeFieldButton(fieldname: string, key?: string) {
    if (key === undefined) { delete this.fieldButtons[fieldname]; return; }
    const list = (this.fieldButtons[fieldname] || []).filter((b) => (b.key || "") !== key);
    if (list.length) this.fieldButtons[fieldname] = list;
    else delete this.fieldButtons[fieldname];
  }
  clearFieldButtons() { this.fieldButtons = {}; }
  setPrimaryAction(label: string, action: () => any) { this.primaryAction = { label, action }; }
  setInnerGroupAsPrimary(group: string) { this.buttons = this.buttons.map((b) => (b.group === group ? { ...b, primary: true } : b)); }
  addIndicator(label: string, color = "blue") { this.indicators.push({ label, color }); }
  setDfProperty(fieldname: string, prop: string, value: any) { this.dfProps[fieldname] = { ...(this.dfProps[fieldname] || {}), [prop]: value }; }
  setQuery(fieldname: string, fn: () => { filters?: any }) { this.queries.set(fieldname, fn); }
  toggleDisplay(fieldname: string, show: boolean) { this.setDfProperty(fieldname, "hidden", !show); }
  toggleReqd(fieldname: string, reqd: boolean) { this.setDfProperty(fieldname, "reqd", reqd); }
  toggleEnable(fieldname: string, enable: boolean) { this.setDfProperty(fieldname, "readOnly", !enable); }

  // ------------------------------------------------------------- lifecycle
  async runSetup() {
    if (this.setupDone) return;
    this.setupDone = true;
    for (const h of this.handlers) { try { await h.setup?.(this); } catch (e) { showError(e); } }
    for (const h of this.handlers) { try { await h.onload?.(this); } catch (e) { showError(e); } }
  }
  async runRefresh() {
    this.clearButtons();
    this.primaryAction = null;
    for (const h of this.handlers) { try { await h.refresh?.(this); } catch (e) { showError(e); } }
  }

  /** Client-side mandatory check before hitting the server. */
  validateMandatory(): boolean {
    this.fieldErrors = {};
    const missing: string[] = [];
    for (const f of this.meta.doctype.fields) {
      if (!f.fieldname || isLayout(f) || !this.isFieldVisible(f)) continue;
      const fx = this.field(f.fieldname)!;
      if (this.isFieldMandatory(fx)) {
        const v = this.doc[f.fieldname];
        if (v === null || v === undefined || v === "" || (Array.isArray(v) && !v.length)) {
          missing.push(fx.label || f.fieldname);
          this.fieldErrors[f.fieldname] = __("Required");
        }
      }
    }
    const emailErrors = validateEmailFields(this.meta.doctype.fields, this.doc, this.doctype);
    Object.assign(this.fieldErrors, emailErrors);
    if (missing.length) {
      toast(__("Fill in the required fields: {0}", [missing.join(", ")]), { title: __("Required fields"), indicator: "red" });
      return false;
    }
    if (Object.keys(emailErrors).length) {
      toast(__("Fix the invalid email fields."), { title: __("Invalid email"), indicator: "red" });
      return false;
    }
    return true;
  }

  async save(action: "save" | "submit" | "cancel" = "save"): Promise<boolean> {
    if (this.saving || (this.isSingle && (this.readOnly || action !== "save"))) return false;
    if (this.workflow && (action === "submit" || action === "cancel")) return false;
    if (this.readOnly && action === "save") return false;
    if (action !== "cancel" && !this.validateMandatory()) return false;
    for (const h of this.handlers) {
      try { if ((await h.validate?.(this)) === false) return false; await h.beforeSave?.(this); } catch (e) { showError(e); return false; }
    }
    this.saving = true;
    ui.busy++;
    try {
      let saved: any;
      const dt = this.doctype;
      if (this.isSingle) {
        saved = await api.update(dt, "singleton", this.doc);
      } else if (this.isNew) {
        if (action === "submit") this.doc.docstatus = 1;
        saved = await api.insert(dt, this.doc);
      } else if (action === "save") {
        saved = await api.update(dt, this.doc.id, this.doc);
      } else {
        saved = await api.docMethod(dt, this.doc.id, action, { doc: this.doc });
      }
      const wasNew = this.isNew;
      this.load(saved);
      for (const h of this.handlers) { try { await h.afterSave?.(this); } catch (e) { showError(e); } }
      toast(action === "submit" ? __("Submitted") : action === "cancel" ? __("Cancelled") : __("Saved"), { indicator: "green", timeout: 2000 });
      if (wasNew && !this.isSingle) goto(`${recordBase(this)}/${encodeURIComponent(saved.id)}`, { replaceState: true });
      else await this.runRefresh();
      return true;
    } catch (e: any) {
      showError(e);
      if (action === "submit" && this.isNew) this.doc.docstatus = 0;
      return false;
    } finally {
      this.saving = false;
      ui.busy--;
    }
  }
  submit() {
    if (this.workflow) return Promise.resolve(false);
    return this.save("submit");
  }
  cancel() {
    if (this.workflow) return Promise.resolve(false);
    return this.save("cancel");
  }

  async applyWorkflowAction(action: string): Promise<boolean> {
    // applying reloads the document: unsaved edits would be dropped silently
    if (this.saving || this.isDirty) return false;
    this.saving = true;
    ui.busy++;
    try {
      const res = await api.post("/api/workflow/apply", {
        doctype: this.doctype,
        id: this.doc.id,
        action,
      });
      this.load(res);
      for (const h of this.handlers) {
        try { await h.afterSave?.(this); } catch (e) { showError(e); }
      }
      toast(__("Action '{0}' applied", [__(action)]), { indicator: "green", timeout: 2000 });
      await this.runRefresh();
      return true;
    } catch (e: any) {
      showError(e);
      return false;
    } finally {
      this.saving = false;
      ui.busy--;
    }
  }

  loadedAt = 0;
  load(doc: any) {
    if (doc?._linkTitles) registerTitles(doc._linkTitles);
    this.doc = doc;
    this.original = JSON.stringify(doc);
    this.fieldErrors = {};
    this.loadedAt = Date.now();
  }

  /**
   * Puts a recovered draft in place. The opposite of `load`: `original` is left
   * as it was, so the form stays dirty and the user still has to save.
   */
  applyDraft(doc: any) {
    if (doc?._linkTitles) registerTitles(doc._linkTitles);
    this.doc = doc;
    this.fieldErrors = {};
  }

  /** Throws the local edits away and goes back to the document as it was loaded. */
  async discardChanges() {
    this.load(JSON.parse(this.original));
    await this.runRefresh();
  }

  async reload() {
    if (this.isNew && !this.isSingle) return;
    try {
      this.load(await api.getDoc(this.doctype, this.doc.id));
      await this.runRefresh();
    } catch (e) { showError(e); }
  }

  async delete() {
    await api.remove(this.doctype, this.doc.id);
    toast(__("Deleted"), { indicator: "green", timeout: 2000 });
    goto(recordBase(this));
  }

  async amend() {
    const doc = await api.docMethod(this.doctype, this.doc.id, "amend");
    this.load(doc);
    goto(`${recordBase(this)}/new`, { state: { doc } });
  }

  /** Calls a controller method on this document; returns its result and reloads the doc. */
  async call(method: string, args: Record<string, any> = {}, opts: { freeze?: boolean; reload?: boolean } = {}): Promise<any> {
    if (this.isNew) { toast(__("Save the document first"), { indicator: "orange" }); return; }
    ui.busy++;
    try {
      const res = await api.docMethod(this.doctype, this.doc.id, method, args);
      if (opts.reload !== false && res?.doc) { this.load(res.doc); await this.runRefresh(); }
      return res?.result;
    } catch (e) { showError(e); throw e; } finally { ui.busy--; }
  }
}

export async function createForm(doctype: string, id?: string, initial?: any, basePath = ""): Promise<FormController> {
  const meta = await getMeta(doctype);
  await loadFormScript(meta);
  let doc: any;
  if (meta.doctype.isSingle) {
    if (id === "new") await goto(`${currentWsPrefix()}/${encodeURIComponent(doctype)}`, { replaceState: true });
    doc = await api.getSingle(doctype);
  }
  else if (id && id !== "new") doc = await api.getDoc(doctype, id);
  else doc = { ...newDoc(meta), ...(initial || {}) };
  const frm = new FormController(meta, doc);
  frm.basePath = basePath;
  await frm.runSetup();
  await frm.runRefresh();
  return frm;
}
