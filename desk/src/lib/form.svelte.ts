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
/** Loads /assets/apps/<app>/forms/<snake>.js once (registers via defineForm). */
export async function loadFormScript(meta: Meta) {
  const d = meta.doctype;
  if (!d.hasForm) return;
  const url = `/assets/apps/${d.app}/forms/${snake(d.name)}.js`;
  if (loadedScripts.has(url)) return;
  loadedScripts.add(url);
  try {
    await import(/* @vite-ignore */ url + "?v=" + (window as any).__ddcoreLoaded);
  } catch (e) {
    console.error("form script", url, e);
    toast(__("Falha ao carregar o script do formulário: {0}", [String(e)]), { indicator: "red" });
  }
}

export const snake = (s: string) =>
  s.replace(/([a-z0-9])([A-Z])/g, "$1_$2").toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "");

export interface Button { label: string; group?: string; action: () => any; primary?: boolean }

export class FormController {
  doc = $state<any>({});
  original = "";
  meta: Meta;
  doctype: string;
  buttons = $state<Button[]>([]);
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
  get isNew() { return !!this.doc.__islocal || !this.doc.name; }
  isNewDoc() { return this.isNew; }
  get isDirty() { return JSON.stringify(this.doc) !== this.original; }
  get docstatus(): number { return Number(this.doc.docstatus || 0); }
  get isSubmittable() { return !!this.meta.doctype.submittable; }
  get readOnly() { return this.docstatus === 2 || (this.docstatus === 1 && !this.meta.doctype.fields.some((f) => f.allowOnSubmit)); }
  get perm() { return this.meta.permissions; }

  field(fieldname: string): Field | undefined {
    const f = this.meta.doctype.fields.find((x) => x.fieldname === fieldname);
    if (!f) return undefined;
    const props = { ...(this.dfProps[fieldname] || {}) };
    if (fieldname === "name" && !this.isNew && !props.description && this.meta.doctype.naming?.prompt) {
      props.description = __("Para alterar, clique no título acima ou em Renomear no menu.");
    }
    return { ...f, ...props };
  }

  /** Whether a field can be edited right now (docstatus, readOnly, allowOnSubmit, readOnlyDependsOn). */
  isFieldEditable(f: Field): boolean {
    if (f.fieldname === "name" && !this.isNew) return false;
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
      const rows = await api.list(target, { filters: { name: link }, fields, limit: 1 });
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
  clearButtons() { this.buttons = []; this.indicators = []; }
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
          this.fieldErrors[f.fieldname] = __("Obrigatório");
        }
      }
    }
    const emailErrors = validateEmailFields(this.meta.doctype.fields, this.doc, this.doctype);
    Object.assign(this.fieldErrors, emailErrors);
    if (missing.length) {
      toast(__("Preencha os campos obrigatórios: {0}", [missing.join(", ")]), { title: __("Campos obrigatórios"), indicator: "red" });
      return false;
    }
    if (Object.keys(emailErrors).length) {
      toast(__("Corrija os campos de e-mail inválidos."), { title: __("E-mail inválido"), indicator: "red" });
      return false;
    }
    return true;
  }

  async save(action: "save" | "submit" | "cancel" = "save"): Promise<boolean> {
    if (this.saving) return false;
    if (action !== "cancel" && !this.validateMandatory()) return false;
    for (const h of this.handlers) {
      try { if ((await h.validate?.(this)) === false) return false; await h.beforeSave?.(this); } catch (e) { showError(e); return false; }
    }
    this.saving = true;
    ui.busy++;
    try {
      let saved: any;
      const dt = this.doctype;
      if (this.isNew) {
        if (action === "submit") this.doc.docstatus = 1;
        saved = await api.insert(dt, this.doc);
      } else if (action === "save") {
        saved = await api.update(dt, this.doc.name, this.doc);
      } else {
        saved = await api.docMethod(dt, this.doc.name, action, { doc: this.doc });
      }
      const wasNew = this.isNew;
      this.load(saved);
      for (const h of this.handlers) { try { await h.afterSave?.(this); } catch (e) { showError(e); } }
      toast(action === "submit" ? __("Enviado") : action === "cancel" ? __("Cancelado") : __("Salvo"), { indicator: "green", timeout: 2000 });
      if (wasNew) goto(`/app/${encodeURIComponent(dt)}/${encodeURIComponent(saved.name)}`, { replaceState: true });
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
  submit() { return this.save("submit"); }
  cancel() { return this.save("cancel"); }

  loadedAt = 0;
  load(doc: any) {
    if (doc?._linkTitles) registerTitles(doc._linkTitles);
    this.doc = doc;
    this.original = JSON.stringify(doc);
    this.fieldErrors = {};
    this.loadedAt = Date.now();
  }

  async reload() {
    if (this.isNew) return;
    try {
      this.load(await api.getDoc(this.doctype, this.doc.name));
      await this.runRefresh();
    } catch (e) { showError(e); }
  }

  async delete() {
    await api.remove(this.doctype, this.doc.name);
    toast(__("Apagado"), { indicator: "green", timeout: 2000 });
    goto(`/app/${encodeURIComponent(this.doctype)}`);
  }

  async amend() {
    const doc = await api.docMethod(this.doctype, this.doc.name, "amend");
    this.load(doc);
    goto(`/app/${encodeURIComponent(this.doctype)}/new`, { state: { doc } });
  }

  /** Calls a controller method on this document; returns its result and reloads the doc. */
  async call(method: string, args: Record<string, any> = {}, opts: { freeze?: boolean; reload?: boolean } = {}): Promise<any> {
    if (this.isNew) { toast(__("Salve o documento antes"), { indicator: "orange" }); return; }
    ui.busy++;
    try {
      const res = await api.docMethod(this.doctype, this.doc.name, method, args);
      if (opts.reload !== false && res?.doc) { this.load(res.doc); await this.runRefresh(); }
      return res?.result;
    } catch (e) { showError(e); throw e; } finally { ui.busy--; }
  }
}

export async function createForm(doctype: string, name?: string, initial?: any): Promise<FormController> {
  const meta = await getMeta(doctype);
  await loadFormScript(meta);
  let doc: any;
  if (name && name !== "new") doc = await api.getDoc(doctype, name);
  else doc = { ...newDoc(meta), ...(initial || {}) };
  const frm = new FormController(meta, doc);
  await frm.runSetup();
  await frm.runRefresh();
  return frm;
}
