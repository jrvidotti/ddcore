// Prelude: runs once per runtime before any app bundle. Defines the registry
// (`__ddcore`), the `ddcore` bridge API and the Document class. Every call to
// Go goes through __host(op, jsonArgs) -> jsonResult.
(function () {
  "use strict";
  const host = globalThis.__host;

  class DDCoreError extends Error {
    constructor(type, title, message, extra) {
      super(message);
      this.name = type || "DDCoreError";
      this.ddcoreType = type || "ValidationError";
      this.title = title || "";
      this.extra = extra;
    }
  }
  globalThis.DDCoreError = DDCoreError;

  function toError(e) {
    if (e instanceof DDCoreError) return e;
    const msg = String((e && e.message) || e);
    const i = msg.indexOf("ddcore:{");
    if (i >= 0) {
      try {
        // "ddcore:" is seven characters. Slicing at six left the colon in
        // front of the JSON, the parse always failed, and every typed error
        // raised in Go reached the border as a 500 ScriptError.
        const o = JSON.parse(msg.slice(i + "ddcore:".length));
        const err = new DDCoreError(o.type, o.title, o.message, o.extra);
        // The English template and its arguments, so the HTTP border can still
        // translate the message into the reader's language.
        if (o.key) { err.key = o.key; err.args = o.args; }
        if (o.titleKey) err.titleKey = o.titleKey;
        return err;
      } catch (_) {}
    }
    return e;
  }

  function call(op, args) {
    let r;
    try {
      r = host(op, JSON.stringify(args === undefined ? {} : args));
    } catch (e) {
      throw toError(e);
    }
    return r === "" || r === undefined ? undefined : JSON.parse(r);
  }

  // ---------------------------------------------------------------- registry
  const reg = {
    app: "",
    // translation catalogue per language, mirrored from the host on first use
    __cat: {},
    // the site's currency, precision, rounding rule and timezone, mirrored for
    // the same reason and on the same terms: immutable inside a State
    __site: null,
    current: "",
    doctypes: {},
    controllers: {},
    // one entry per extendDoctype call, in load order; Go merges them into the
    // meta so DDL, typegen, the API and the desk all see one truth
    extensions: [],
    // the app that registered each controller, so a second app's
    // defineController is refused instead of silently replacing it
    controllerApp: {},
    reports: {},
    workspaces: {},
    mailTemplates: {},
    printTemplates: {},
    notifications: Object.create(null),
    workflows: Object.create(null),
    workflowsByDoctype: Object.create(null),
    apps: {},
    modules: {},
    modulesByApp: {},
    tests: [],
    register(kind, value) {
      switch (kind) {
        case "doctype": {
          const prev = reg.doctypes[value.name];
          if (prev) {
            throw new DDCoreError("ValidationError", "", "DocType " + value.name + " is defined twice: " +
              prev.app + " (" + prev.sourceFile + ") and " + reg.app + " (" + reg.current + "). " +
              "To add fields to another app's DocType use extendDoctype.");
          }
          value.app = value.app || reg.app;
          value.sourceFile = reg.current;
          reg.doctypes[value.name] = value;
          break;
        }
        case "controller": {
          const owner = reg.controllerApp[value.doctype];
          if (owner !== undefined) {
            throw new DDCoreError("ValidationError", "", "DocType " + value.doctype + " already has a controller, from " +
              owner + ". Use extendDoctype (permissions) or defineApp.docEvents (hooks) to add behaviour from " + reg.app + ".");
          }
          reg.controllerApp[value.doctype] = reg.app;
          reg.controllers[value.doctype] = value.controller;
          break;
        }
        case "extension":
          reg.extensions.push({ doctype: value.doctype, app: reg.app, sourceFile: reg.current, ext: value.ext });
          break;
        case "mail": {
          const prev = reg.mailTemplates[value.name];
          if (prev) {
            throw new DDCoreError("ValidationError", "", "Mail template " + value.name + " is defined twice: " +
              prev.app + " (" + prev.sourceFile + ") and " + reg.app + " (" + reg.current + "). " +
              "A template name is unique across every installed app, like a DocType name.");
          }
          value.app = reg.app;
          value.sourceFile = reg.current;
          reg.mailTemplates[value.name] = value;
          break;
        }
        case "print": {
          const prev = reg.printTemplates[value.name];
          if (prev) {
            throw new DDCoreError("ValidationError", "", "Print template " + value.name + " is defined twice: " +
              prev.app + " (" + prev.sourceFile + ") and " + reg.app + " (" + reg.current + "). " +
              "A template name is unique across every installed app, like a DocType name.");
          }
          if (!value || typeof value.name !== "string" || !value.name.trim()) throw new DDCoreError("ValidationError", "", "Print template: name is required");
          if (!value.doctype || typeof value.doctype !== "string") throw new DDCoreError("ValidationError", "", "Print template " + value.name + ": doctype is required");
          if (typeof value.body !== "function") throw new DDCoreError("ValidationError", "", "Print template " + value.name + ": body must be a function");
          value.app = reg.app;
          value.sourceFile = reg.current;
          reg.printTemplates[value.name] = value;
          break;
        }
        case "notification": {
          const fail = (message) => { throw new DDCoreError("ValidationError", "", "Notification " + (value?.name || "<unnamed>") + ": " + message); };
          if (!value || typeof value.name !== "string" || !value.name.trim()) fail("name is required");
          if (reg.notifications[value.name]) fail("name is defined twice");
          if (typeof value.doctype !== "string" || !value.doctype.trim()) fail("doctype is required");
          if ((value.event !== undefined) === (value.date !== undefined)) fail("declare exactly one event or date trigger");
          if (value.event !== undefined && !["on_insert", "on_update", "on_submit", "on_cancel"].includes(value.event)) fail("invalid event");
          if (value.date !== undefined && (!value.date || typeof value.date.field !== "string" || !value.date.field || !Number.isSafeInteger(value.date.days))) fail("date requires field and integer days");
          const fn = (v, label) => {
            if (typeof v !== "function" || v.constructor?.name === "AsyncFunction" || v.constructor?.name === "GeneratorFunction") fail(label + " must be a synchronous function");
          };
          if (value.condition !== undefined) fn(value.condition, "condition");
          fn(value.recipients, "recipients");
          if (!value.desk && !value.email) fail("declare at least one channel");
          if (value.desk) { fn(value.desk.title, "desk.title"); fn(value.desk.message, "desk.message"); }
          if (value.email) {
            if (typeof value.email.template !== "string" || !value.email.template.trim()) fail("email.template is required");
            fn(value.email.args, "email.args");
          }
          value.app = reg.app;
          value.sourceFile = reg.current;
          reg.notifications[value.name] = value;
          break;
        }
        case "workflow": {
          const fail = (msg) => { throw new DDCoreError("ValidationError", "", "Workflow " + (value?.name || "<unnamed>") + ": " + msg); };
          if (!value || typeof value.name !== "string" || !value.name.trim()) fail("name is required");
          if (reg.workflows[value.name]) fail("name is defined twice");
          if (typeof value.doctype !== "string" || !value.doctype.trim()) fail("doctype is required");
          if (reg.workflowsByDoctype[value.doctype]) fail("DocType " + value.doctype + " already has a workflow defined");
          if (typeof value.initialState !== "string" || !value.initialState.trim()) fail("initialState is required");
          if (!Array.isArray(value.states) || value.states.length === 0) fail("states array is required");
          if (!Array.isArray(value.transitions) || value.transitions.length === 0) fail("transitions array is required");
          const stateNames = new Set(value.states.map((s) => s.state));
          if (!stateNames.has(value.initialState)) fail("initialState must exist in states");
          value.transitions.forEach((tr, i) => {
            if (!stateNames.has(tr.state)) fail("transition " + i + " state '" + tr.state + "' does not exist in states");
            if (!stateNames.has(tr.nextState)) fail("transition " + i + " nextState '" + tr.nextState + "' does not exist in states");
            if (!tr.action) fail("transition " + i + " action is required");
            if (!tr.allowed) fail("transition " + i + " allowed role is required");
            if (tr.condition !== undefined) {
              if (typeof tr.condition !== "function" || tr.condition.constructor?.name === "AsyncFunction" || tr.condition.constructor?.name === "GeneratorFunction") {
                fail("transition " + i + " condition must be a function");
              }
            }
          });
          value.app = reg.app;
          value.sourceFile = reg.current;
          value.stateField = value.stateField || "workflow_state";
          reg.workflows[value.name] = value;
          reg.workflowsByDoctype[value.doctype] = value.name;
          break;
        }
        case "report":
          value.app = reg.app;
          reg.reports[value.name] = value;
          break;
        case "workspace":
          value.app = reg.app;
          reg.workspaces[value.name] = value;
          break;
        case "app":
          reg.apps[reg.app] = value;
          break;
        case "module":
          reg.modules[value.path] = value;
          (reg.modulesByApp[reg.app] ||= []).push(value.path);
          break;
      }
    },
  };
  globalThis.__ddcore = reg;

  const stripFns = (o) =>
    JSON.parse(JSON.stringify(o, (k, v) => (typeof v === "function" ? undefined : v)));

  const extensionsOf = (doctype) => reg.extensions.filter((x) => x.doctype === doctype);

  // Go merges the extensions into the meta and hands the result back, so that
  // `ddcore.getMeta()` and the Document internals read the same DocType the
  // database, the API and the desk see — not the one this app happened to
  // declare. Called once per runtime, before it serves anything.
  reg.applyMeta = function (json) {
    const merged = JSON.parse(json);
    for (const n in merged) reg.doctypes[n] = merged[n];
  };

  // Snapshot of everything Go needs to know (no functions).
  reg.meta = function () {
    const whitelisted = [];
    const patches = [];
    for (const path in reg.modules) {
      const m = reg.modules[path];
      const ex = m.exports || {};
      for (const k of Object.keys(ex)) {
        const fn = ex[k];
        if (typeof fn === "function" && fn.__whitelisted) whitelisted.push({ path: path + "." + k, opts: fn.__whitelisted });
      }
      if (/\.patches\.[^.]+$/.test(path)) {
        const def = patchDef(m);
        if (def) {
          patches.push({
            app: path.split(".")[0], name: path.split(".").pop(), path,
            phase: def.phase === "beforeSchema" ? "beforeSchema" : "afterSchema",
            description: def.description || "",
          });
        }
      }
    }
    const reports = {};
    for (const n in reg.reports) reports[n] = stripFns(reg.reports[n]);
    const workspaces = {};
    for (const n in reg.workspaces) workspaces[n] = stripFns(reg.workspaces[n]);
    // stripFns drops `subject` and `body`: Go never renders a template, it only
    // needs to know one exists and whether its arguments may be stored.
    const mailTemplates = {};
    for (const n in reg.mailTemplates) mailTemplates[n] = stripFns(reg.mailTemplates[n]);
    const printTemplates = {};
    for (const n in reg.printTemplates) printTemplates[n] = stripFns(reg.printTemplates[n]);
    const notifications = Object.create(null);
    for (const n in reg.notifications) notifications[n] = { ...stripFns(reg.notifications[n]), desk: !!reg.notifications[n].desk };
    const workflows = {};
    for (const n in reg.workflows) {
      const wf = reg.workflows[n];
      workflows[n] = {
        name: wf.name,
        doctype: wf.doctype,
        stateField: wf.stateField || "workflow_state",
        initialState: wf.initialState,
        app: wf.app,
        sourceFile: wf.sourceFile,
        states: wf.states.map((s) => ({
          state: s.state,
          docstatus: s.docstatus !== undefined ? s.docstatus : 0,
          allowEdit: s.allowEdit || "",
          updateFields: s.updateFields || null,
        })),
        transitions: wf.transitions.map((t, idx) => ({
          index: idx,
          state: t.state,
          action: t.action,
          nextState: t.nextState,
          allowed: Array.isArray(t.allowed) ? t.allowed : [t.allowed],
          allowSelfApproval: t.allowSelfApproval !== false,
          hasCondition: typeof t.condition === "function",
        })),
      };
    }
    const apps = {};
    for (const n in reg.apps) {
      const a = stripFns(reg.apps[n]);
      a.hasAfterInstall = typeof reg.apps[n].afterInstall === "function";
      a.hasAfterMigrate = typeof reg.apps[n].afterMigrate === "function";
      apps[n] = a;
    }
    const doctypes = {};
    for (const n in reg.doctypes) {
      const d = stripFns(reg.doctypes[n]);
      d.hasController = !!reg.controllers[n];
      const c = reg.controllers[n] || {};
      d.methods = Object.keys(c.methods || {});
      // an extension's permission hooks count too: the flags are what Go uses
      // to decide whether to cross the bridge at all (meta.HasController)
      d.hasPermissionHook = typeof c.hasPermission === "function" || extensionsOf(n).some((x) => typeof x.ext.hasPermission === "function");
      d.hasPermissionQuery = typeof c.permissionQuery === "function" || extensionsOf(n).some((x) => typeof x.ext.permissionQuery === "function");
      doctypes[n] = d;
    }
    // flattened for Go, and `doctype` at this level is the *target* — the
    // DocType-level overrides travel as `props` so the two cannot collide
    const extensions = reg.extensions.map((x) => {
      const ext = stripFns(x.ext);
      return { doctype: x.doctype, app: x.app, sourceFile: x.sourceFile,
        fields: ext.fields, set: ext.set, props: ext.doctype, permissions: ext.permissions };
    });
    return JSON.stringify({ doctypes, reports, workspaces, mailTemplates, printTemplates, notifications, workflows, apps, whitelisted, patches, extensions });
  };

  const notificationCall = (fn, doc, before) => {
    const result = fn(doc, before);
    if (result && typeof result.then === "function") throw new DDCoreError("ValidationError", "", "Notification functions must be synchronous");
    return result;
  };
  const notificationInput = (name, docJSON, beforeJSON) => {
    const rule = reg.notifications[name];
    if (!rule) throw new DDCoreError("NotFoundError", "", "Notification rule does not exist: " + name);
    return [rule, JSON.parse(docJSON), beforeJSON ? JSON.parse(beforeJSON) : null];
  };
  reg.evaluateNotification = function (name, docJSON, beforeJSON) {
    const [rule, doc, before] = notificationInput(name, docJSON, beforeJSON);
    if (rule.condition) {
      const matches = notificationCall(rule.condition, doc, before);
      if (typeof matches !== "boolean") throw new DDCoreError("ValidationError", "", "Notification condition must return a boolean");
      if (!matches) return JSON.stringify({ matches: false, recipients: [] });
    }
    const recipients = notificationCall(rule.recipients, doc, before);
    if (!Array.isArray(recipients) || recipients.some((v) => typeof v !== "string" || !v.trim())) throw new DDCoreError("ValidationError", "", "Notification recipients must return User names");
    return JSON.stringify({ matches: true, recipients: [...new Set(recipients)] });
  };
  reg.renderNotification = function (name, docJSON, beforeJSON) {
    const [rule, doc, before] = notificationInput(name, docJSON, beforeJSON);
    const result = {};
    if (rule.desk) {
      result.title = notificationCall(rule.desk.title, doc, before);
      result.message = notificationCall(rule.desk.message, doc, before);
      if (typeof result.title !== "string" || typeof result.message !== "string") throw new DDCoreError("ValidationError", "", "Notification title and message must be plain text strings");
    }
    if (rule.email) {
      result.emailArgs = notificationCall(rule.email.args, doc, before);
      if (!result.emailArgs || typeof result.emailArgs !== "object" || Array.isArray(result.emailArgs)) throw new DDCoreError("ValidationError", "", "Notification email args must be an object");
    }
    return JSON.stringify(result);
  };

  reg.evaluateWorkflowCondition = function (name, transitionIndex, docJSON) {
    const wf = reg.workflows[name];
    if (!wf) throw new DDCoreError("ValidationError", "", "Unknown workflow: " + name);
    const tr = wf.transitions[transitionIndex];
    if (!tr) throw new DDCoreError("ValidationError", "", "Unknown transition index: " + transitionIndex);
    if (!tr.condition) return "true";
    const doc = docJSON ? JSON.parse(docJSON) : {};
    const res = tr.condition(doc);
    if (res && typeof res.then === "function") throw new DDCoreError("ValidationError", "", "Workflow condition must be synchronous");
    return res ? "true" : "false";
  };

  // -------------------------------------------------------------------- mail

  // The only vocabulary an app has for a message body. Everything is coerced to
  // a string here so a template cannot hand the renderer an object and discover
  // "[object Object]" in someone's inbox.
  const mailBlocks = {
    p: (text) => ({ type: "p", text: String(text ?? "") }),
    h: (text) => ({ type: "h", text: String(text ?? "") }),
    button: (text, url) => ({ type: "button", text: String(text ?? ""), url: String(url ?? "") }),
    table: (head, rows) => ({
      type: "table",
      head: (head || []).map((c) => String(c ?? "")),
      rows: (rows || []).map((r) => (r || []).map((c) => String(c ?? ""))),
    }),
    rule: () => ({ type: "rule" }),
  };

  // Renders a template in the reader's language rather than the caller's. The
  // catalogue `_()` reads is chosen by __ddcoreLang, so swapping it around the
  // call is all it takes — and restoring it in a finally is what keeps an
  // exception inside a template from leaving the whole VM speaking Portuguese.
  reg.renderMail = function (name, args, lang) {
    const t = reg.mailTemplates[name];
    if (!t) {
      throw new DDCoreError("NotFoundError", "", "Mail template " + String(name) + " does not exist");
    }
    const previous = globalThis.__ddcoreLang;
    if (lang) globalThis.__ddcoreLang = lang;
    try {
      return {
        subject: String(t.subject ? t.subject(args) : ""),
        blocks: (t.body ? t.body(args, mailBlocks) : []) || [],
      };
    } finally {
      globalThis.__ddcoreLang = previous;
    }
  };

  const printBlocks = {
    header: (title, opts = {}) => ({
      type: "header",
      title: String(title ?? ""),
      subtitle: opts && opts.subtitle ? String(opts.subtitle) : undefined,
      badge: opts && opts.badge ? String(opts.badge) : undefined,
      badgeColor: opts && opts.badgeColor ? String(opts.badgeColor) : undefined,
    }),
    keyValues: (pairs, opts = {}) => ({
      type: "keyValues",
      columns: Number(opts && opts.columns) || 2,
      pairs: (pairs || []).map(([k, v]) => [String(k ?? ""), String(v ?? "")]),
    }),
    section: (title, blocks = []) => ({
      type: "section",
      title: title ? String(title) : undefined,
      blocks: Array.isArray(blocks) ? blocks : [],
    }),
    table: (headers, rows, opts = {}) => ({
      type: "table",
      headers: (headers || []).map((h) => String(h ?? "")),
      rows: (rows || []).map((r) => (r || []).map((c) => String(c ?? ""))),
      aligns: (opts && opts.aligns || []).map((a) => String(a)),
    }),
    totals: (rows) => ({
      type: "totals",
      pairs: (rows || []).map(([k, v]) => [String(k ?? ""), String(v ?? "")]),
    }),
    p: (text) => ({ type: "p", text: String(text ?? "") }),
    h: (level, text) => ({ type: "h", level: Number(level) || 2, text: String(text ?? "") }),
    h1: (text) => ({ type: "h", level: 1, text: String(text ?? "") }),
    h2: (text) => ({ type: "h", level: 2, text: String(text ?? "") }),
    h3: (text) => ({ type: "h", level: 3, text: String(text ?? "") }),
    rule: () => ({ type: "rule" }),
    divider: () => ({ type: "rule" }),
    pageBreak: () => ({ type: "pageBreak" }),
    raw: (html) => ({ type: "raw", html: String(html ?? "") }),
    html: (html) => ({ type: "raw", html: String(html ?? "") }),
    columns: (cols) => ({ type: "columns", columns: Array.isArray(cols) ? cols : [] }),
  };

  const formatNumberHelper = (val, decimals, lang) => {
    const n = Number(val) || 0;
    const dec = decimals !== undefined ? decimals : 2;
    const parts = n.toFixed(dec >= 0 ? dec : 2).split(".");
    let intPart = parts[0];
    const fracPart = parts[1];
    const isPT = (lang || "").toLowerCase().startsWith("pt");
    const thousandsSep = isPT ? "." : ",";
    const decimalSep = isPT ? "," : ".";
    const sign = intPart.startsWith("-") ? "-" : "";
    if (sign) intPart = intPart.slice(1);
    const grouped = intPart.replace(/\B(?=(\d{3})+(?!\d))/g, thousandsSep);
    if (dec === 0) return sign + grouped;
    return sign + grouped + decimalSep + (fracPart || "00");
  };

  const makePrintContext = (lang) => {
    const l = lang || globalThis.__ddcoreLang || "en";
    return {
      formatCurrency: (val) => formatNumberHelper(val, 2, l),
      formatDate: (val) => {
        if (!val) return "";
        const s = String(val).trim();
        const m = s.match(/^(\d{4})-(\d{2})-(\d{2})/);
        if (m) {
          if (l.toLowerCase().startsWith("pt")) return `${m[3]}/${m[2]}/${m[1]}`;
          return `${m[1]}-${m[2]}-${m[3]}`;
        }
        return s;
      },
      formatDateTime: (val) => {
        if (!val) return "";
        return String(val);
      },
      formatNumber: (val, decimals = 2) => formatNumberHelper(val, decimals, l),
    };
  };

  reg.renderPrint = function (name, docJSON, lang) {
    const t = reg.printTemplates[name];
    if (!t) {
      throw new DDCoreError("NotFoundError", "", "Print template " + String(name) + " does not exist");
    }
    const previous = globalThis.__ddcoreLang;
    if (lang) globalThis.__ddcoreLang = lang;
    try {
      const ctx = makePrintContext(globalThis.__ddcoreLang);
      const doc = typeof docJSON === "string" ? JSON.parse(docJSON) : docJSON;
      const wrappedDoc = doc instanceof Document ? doc : new Document(doc);
      const blocks = (t.body ? t.body(wrappedDoc, printBlocks, ctx) : []) || [];
      return JSON.stringify({ blocks });
    } finally {
      globalThis.__ddcoreLang = previous;
    }
  };

  // ---------------------------------------------------------------- Document
  const DOC_INTERNAL = new Set(["flags", "__before"]);

  class Document {
    constructor(data) {
      Object.defineProperty(this, "flags", { value: {}, enumerable: false, writable: true });
      Object.defineProperty(this, "__before", { value: undefined, enumerable: false, writable: true });
      Object.assign(this, data);
      this._wrapChildren();
    }
    _wrapChildren() {
      for (const k of Object.keys(this)) {
        const v = this[k];
        if (Array.isArray(v)) {
          for (const row of v) if (row && typeof row === "object" && !(row instanceof ChildRow)) Object.setPrototypeOf(row, ChildRow.prototype);
        }
      }
    }
    toJSON() {
      const o = {};
      for (const k of Object.keys(this)) if (!DOC_INTERNAL.has(k)) o[k] = this[k];
      return o;
    }
    _apply(data) {
      for (const k of Object.keys(this)) delete this[k];
      Object.assign(this, data);
      this._wrapChildren();
      return this;
    }
    insert(opts) { return this._apply(call("doc.insert", { doc: this, opts })); }
    save(opts) { return this._apply(call("doc.save", { doc: this, opts })); }
    submit() { this.docstatus = 1; return this.save(); }
    cancel() { return this._apply(call("doc.cancel", { doc: this })); }
    applyWorkflow(action) { return this._apply(call("doc.applyWorkflow", { doctype: this.doctype, name: this.name, action })); }
    delete(opts) { call("doc.delete", { doctype: this.doctype, name: this.name, opts }); }
    reload() { return this._apply(call("getDoc", { doctype: this.doctype, name: this.name })); }
    dbSet(field, value) {
      const values = typeof field === "object" ? field : { [field]: value };
      const res = call("doc.dbSet", { doctype: this.doctype, name: this.name, values });
      Object.assign(this, values);
      // the bridge also updated `modified`: without this, the next save()
      // would fail with TimestampMismatch without anyone having edited the document.
      if (res && res.modified) this.modified = res.modified;
      return this;
    }
    append(fieldname, row) {
      const rows = (this[fieldname] ||= []);
      const meta = reg.doctypes[this.doctype];
      const f = meta && meta.fields.find((x) => x.fieldname === fieldname);
      const r = Object.assign(Object.create(ChildRow.prototype), {
        doctype: f ? f.options : undefined,
        parent: this.name, parenttype: this.doctype, parentfield: fieldname,
        idx: rows.length + 1, docstatus: this.docstatus || 0,
      }, row || {});
      rows.push(r);
      return r;
    }
    get(f) { return this[f]; }
    set(f, v) { this[f] = v; return this; }
    getDoc() { return this.toJSON(); }
    isNew() { return !!this.__islocal; }
    getDocBeforeSave() { return this.__before; }
    hasValueChanged(f) {
      const b = this.__before;
      if (!b) return true;
      return JSON.stringify(b[f] === undefined ? null : b[f]) !== JSON.stringify(this[f] === undefined ? null : this[f]);
    }
    runMethod(name, args) { return reg.runMethodOn(this, name, args || {}); }
    getTitle() {
      const meta = reg.doctypes[this.doctype];
      return (meta && meta.titleField && this[meta.titleField]) || this.name;
    }
  }
  class ChildRow {
    toJSON() {
      const o = {};
      for (const k of Object.keys(this)) o[k] = this[k];
      return o;
    }
  }
  globalThis.Document = Document;
  reg.makeDoc = (data) => new Document(data);

  // ---------------------------------------------------------------- ddcore API
  function makeContext() { return call("session"); }

  // ------------------------------------------------- the decimal rounding rule
  //
  // Shared with internal/num (Go) and desk/src/lib/round.ts. All three are
  // asserted against internal/num/testdata/rounding.json: two copies of a table
  // drift, and the day they drift is the day a form and the database disagree
  // about a cent.

  /**
   * The number's shortest decimal representation, as [negative, int, frac].
   *
   * String(n) is that representation already, except that JS switches to
   * exponent notation at 1e21 and below 1e-6 where Go's %f never does. Those
   * magnitudes are past anything a money column holds, so they are expanded
   * rather than handled: the answer has to be the same in both languages.
   */
  function decimalDigits(n) {
    let s = String(n);
    const neg = s[0] === "-";
    if (neg) s = s.slice(1);
    const e = s.indexOf("e");
    if (e >= 0) {
      const exp = Number(s.slice(e + 1));
      let [i, f = ""] = s.slice(0, e).split(".");
      if (exp >= 0) {
        const pad = exp - f.length;
        s = pad >= 0 ? i + f + "0".repeat(pad) : i + f.slice(0, exp) + "." + f.slice(exp);
      } else {
        s = "0." + "0".repeat(-exp - i.length) + i + f;
      }
    }
    const dot = s.indexOf(".");
    return dot < 0 ? [neg, s, ""] : [neg, s.slice(0, dot), s.slice(dot + 1)];
  }

  // Decides on the discarded digits alone: a leading digit above or below 5
  // settles it, and a leading 5 with anything non-zero after it is above half,
  // not at it.
  function roundsUp(kept, rest, mode) {
    if (!rest || rest[0] < "5") return false;
    if (rest[0] > "5" || /[1-9]/.test(rest.slice(1))) return true;
    if (mode === "bankers") return (kept.charCodeAt(kept.length - 1) - 48) % 2 === 1;
    return true;
  }

  function increment(d) {
    const b = d.split("");
    for (let i = b.length - 1; i >= 0; i--) {
      if (b[i] !== "9") { b[i] = String.fromCharCode(b[i].charCodeAt(0) + 1); return b.join(""); }
      b[i] = "0";
    }
    return "1" + b.join("");
  }

  function insertPoint(d, p) {
    if (p === 0) return d;
    while (d.length <= p) d = "0" + d;
    return d.slice(0, d.length - p) + "." + d.slice(d.length - p);
  }

  function site() {
    return reg.__site || (reg.__site = call("site") || { currency: "USD", currencyPrecision: 2, rounding: "commercial", timezone: "UTC" });
  }

  const utils = {
    flt(v, precision) {
      if (v === null || v === undefined || v === "") return 0;
      let n = typeof v === "number" ? v : parseFloat(String(v).replace(/,/g, ""));
      if (isNaN(n)) n = 0;
      if (precision !== undefined) n = utils.roundTo(n, precision);
      return n;
    },
    cint(v) { const n = parseInt(v, 10); return isNaN(n) ? 0 : n; },
    cstr(v) { return v === null || v === undefined ? "" : String(v); },
    /**
     * Rounds n to p decimal places under the site's rule.
     *
     * The rounding is defined over the number's shortest decimal
     * representation — the digits String(n) prints — and not over the binary
     * double, because someone who types 1.005 stores 1.00499999999999989 and
     * expects to get 1.01 back. This is the same algorithm internal/num
     * implements in Go, asserted against the same table of values, which is
     * what lets a total computed here agree with the total the server stores.
     *
     * What was here multiplied by a power of ten and called Math.round. That
     * is asymmetric: it turned 1.005 into 1.01 and -1.005 into -1.00, so a
     * credit and a debit of the same size did not cancel.
     */
    roundTo(n, p = 2, mode) {
      if (typeof n !== "number" || !isFinite(n) || p < 0) return n;
      const digits = decimalDigits(n);
      if (digits === null) return n;
      let [neg, intPart, frac] = digits;
      if (frac.length <= p) return n;
      let kept = intPart + frac.slice(0, p);
      const rest = frac.slice(p);
      if (roundsUp(kept, rest, mode || site().rounding)) kept = increment(kept);
      const out = (neg ? "-" : "") + insertPoint(kept, p);
      const v = Number(out);
      return v === 0 ? 0 : v;
    },
    /** How many decimal places a Currency value has on this site. */
    currencyPrecision() { return site().currencyPrecision; },
    /** Rounds a value the way the server is about to store it. */
    roundCurrency(v) { return utils.roundTo(utils.flt(v), site().currencyPrecision); },
    /**
     * Splits a total into n parts at the site's currency precision whose sum
     * is exactly the total.
     *
     * Rounding each of three thirds of 100.00 gives 33.33 three times, and the
     * invoice is a cent short of itself. The residue is placed on the earliest
     * parts — a decision, not an accident: the instalments a customer pays
     * first absorb it, and the schedule is the same every time it is computed.
     */
    splitAmount(total, n) {
      n = utils.cint(n);
      if (n < 1) throw new Error("splitAmount: n must be at least 1");
      const p = site().currencyPrecision;
      const unit = Math.pow(10, p);
      const cents = Math.round(utils.roundCurrency(total) * unit);
      const base = Math.trunc(cents / n);
      let residue = cents - base * n; // carries the sign of the total
      const step = residue < 0 ? -1 : 1;
      const out = [];
      for (let i = 0; i < n; i++) {
        let c = base;
        if (residue !== 0) { c += step; residue -= step; }
        out.push(utils.roundTo(c / unit, p));
      }
      return out;
    },
    getdate(v) {
      if (v instanceof Date) return new Date(Date.UTC(v.getUTCFullYear(), v.getUTCMonth(), v.getUTCDate()));
      if (!v) return utils.getdate(utils.nowdate());
      const s = String(v).slice(0, 10);
      const parts = s.split("-").map(Number);
      const y = parts[0], m = parts[1] || 1, d = parts[2] !== undefined && !isNaN(parts[2]) ? parts[2] : 1;
      return new Date(Date.UTC(y, m - 1, d));
    },
    formatDate(v, fmt) {
      if (!v) return "";
      const d = utils.getdate(v);
      const y = d.getUTCFullYear(), m = String(d.getUTCMonth() + 1).padStart(2, "0"), dd = String(d.getUTCDate()).padStart(2, "0");
      if (fmt === "dd/mm/yyyy") return `${dd}/${m}/${y}`;
      return `${y}-${m}-${dd}`;
    },
    nowdate() { return call("nowdate"); },
    today() { return call("nowdate"); },
    now() { return call("now"); },
    addDays(d, n) { const x = utils.getdate(d); x.setUTCDate(x.getUTCDate() + n); return utils.formatDate(x); },
    addMonths(d, n) {
      const x = utils.getdate(d);
      const y = x.getUTCFullYear(), m = x.getUTCMonth() + n, day = x.getUTCDate();
      const last = new Date(Date.UTC(y, m + 1, 0)).getUTCDate();
      return utils.formatDate(new Date(Date.UTC(y, m, Math.min(day, last))));
    },
    addYears(d, n) { return utils.addMonths(d, 12 * n); },
    getFirstDay(d) { const x = utils.getdate(d); x.setUTCDate(1); return utils.formatDate(x); },
    getLastDay(d) { const x = utils.getdate(d); const l = new Date(Date.UTC(x.getUTCFullYear(), x.getUTCMonth() + 1, 0)); return utils.formatDate(l); },
    dateDiff(a, b) { return Math.round((utils.getdate(a) - utils.getdate(b)) / 86400000); },
    monthDiff(a, b) { const x = utils.getdate(a), y = utils.getdate(b); return (x.getUTCFullYear() - y.getUTCFullYear()) * 12 + x.getUTCMonth() - y.getUTCMonth(); },
    formatCurrency(v, currency) { return call("formatCurrency", { value: utils.flt(v), currency }); },
    randomString(n = 10) {
      const chars = "abcdefghijklmnopqrstuvwxyz0123456789";
      let s = "";
      for (let i = 0; i < n; i++) s += chars[Math.floor(Math.random() * chars.length)];
      return s;
    },
  };

  const api = {
    version: "0.1.0",
    utils,
    db: {
      getValue(doctype, name, fields) { return call("db.getValue", { doctype, name, fields }); },
      getList(doctype, args) { return call("db.getList", { doctype, args: args || {} }); },
      getAll(doctype, args) { return call("db.getList", { doctype, args: Object.assign({}, args || {}, { ignorePermissions: true }) }); },
      setValue(doctype, name, field, value) {
        const values = typeof field === "object" ? field : { [field]: value };
        call("db.setValue", { doctype, name, values });
      },
      count(doctype, filters, orFilters) { return call("db.count", { doctype, filters, orFilters }); },
      exists(doctype, name) { return call("db.exists", { doctype, name }); },
      sql(query, params) { return call("db.sql", { query, params: params || [] }); },
      lock(key) { call("db.lock", { key: String(key) }); },
      getSingleValue(doctype, field) { return call("db.getSingleValue", { doctype, field }); },
    },
    get session() { return makeContext(); },
    user() { return call("session").user; },
    getRoles(user) { return call("getRoles", { user }); },
    getDoc(doctype, name) {
      if (typeof doctype === "object") return new Document(doctype);
      return new Document(call("getDoc", { doctype, name }));
    },
    newDoc(doctype, values) {
      return new Document(call("newDoc", { doctype, values: values || {} }));
    },
    deleteDoc(doctype, name, opts) { call("doc.delete", { doctype, name, opts }); },
    getMeta(doctype) { return reg.doctypes[doctype] || call("getMeta", { doctype }); },
    hasPermission(doctype, ptype, doc, user) { return call("hasPermission", { doctype, ptype: ptype || "read", doc, user }); },
    redact(doctype, doc) { return call("redact", { doctype, doc }); },
    throw(message, opts) {
      opts = opts || {};
      throw new DDCoreError(opts.type || "ValidationError", opts.title, message, opts.extra);
    },
    msgprint(message, opts) { call("msgprint", { message, opts: opts || {} }); },
    _(text, args) {
      // The catalogue is immutable inside a State, and a reload builds new
      // VMs, so it can be mirrored here: one round-trip per (VM, language)
      // instead of one per call. `validate` and a report's `execute` call
      // this inside loops, where a marshal each way is the whole cost.
      const lang = globalThis.__ddcoreLang || "";
      const cat = reg.__cat[lang] || (reg.__cat[lang] = call("catalogue", { lang }) || {});
      let s = cat[text] || text;
      if (args) s = s.replace(/\{(\d+)\}/g, (m, i) => (args[i] === undefined ? m : String(args[i])));
      return s;
    },
    bold(v) { return "<b>" + String(v) + "</b>"; },
    cache: {
      get(key) { return call("cache.get", { key }); },
      set(key, value, ttl) { call("cache.set", { key, value, ttl }); },
      del(key) { call("cache.del", { key }); },
    },
    http: {
      get(url, opts) { return wrapHttp(call("http", Object.assign({}, opts || {}, { method: "GET", url }))); },
      post(url, body, opts) { return wrapHttp(call("http", Object.assign({}, opts || {}, { method: "POST", url, body }))); },
      put(url, body, opts) { return wrapHttp(call("http", Object.assign({}, opts || {}, { method: "PUT", url, body }))); },
      patch(url, body, opts) { return wrapHttp(call("http", Object.assign({}, opts || {}, { method: "PATCH", url, body }))); },
      del(url, opts) { return wrapHttp(call("http", Object.assign({}, opts || {}, { method: "DELETE", url }))); },
    },
    siteName() { return site().name || ""; },
    enqueue(method, args, opts) { return call("enqueue", { method, args: args || {}, opts: opts || {} }); },
    sendMail(args) {
      args = args || {};
      const template = reg.mailTemplates[args.template];
      if (!template) {
        // Deliberately here and not in the worker: a template nobody declared
        // is a mistake in the caller's own code, and it should fail in the
        // caller's own transaction.
        throw new DDCoreError("NotFoundError", "", "Mail template " + String(args.template) + " does not exist");
      }
      const to = Array.isArray(args.to) ? args.to : [args.to];
      const lang = args.lang || call("mail.prepare", { to }).lang;
      const payload = args.args || {};
      const rendered = reg.renderMail(args.template, payload, lang);
      return call("mail.queue", {
        template: args.template,
        to,
        subject: rendered.subject,
        lang,
        // A sensitive template's arguments are a credential: they go to the job
        // and nowhere else, so nothing durable is left holding a live token.
        args: template.sensitive ? null : payload,
        jobArgs: template.sensitive ? payload : null,
        attach: args.attach || [],
        reference: args.reference || null,
        key: args.key || "",
      });
    },
    webhooks: {
      // Written on this transaction, like sendMail: a request that rolls back
      // has told no receiver anything.
      emit(event, data, opts) {
        opts = opts || {};
        return call("webhook.emit", { event: String(event), data: data === undefined ? null : data, key: opts.key || "", reference: opts.reference || null });
      },
    },
    publish(event, payload, opts) { call("publish", { event, payload, opts: opts || {} }); },
    log: {
      info: (...a) => call("log", { level: "info", args: a.map(String) }),
      warn: (...a) => call("log", { level: "warn", args: a.map(String) }),
      error: (...a) => call("log", { level: "error", args: a.map(String) }),
      debug: (...a) => call("log", { level: "debug", args: a.map(String) }),
    },
    isTest() { return !!globalThis.__ddcoreTest; },
    isJob() { return !call("session").request; },
    form: {},
    callMethod(method, args) { return reg.callModule(method, args || {}); },
    rename(doctype, oldName, newName) { return call("rename", { doctype, oldName, newName }); },
    // An integration credential comes from the environment (.env in
    // development, the platform in production), never from a column: a secret
    // in the database is a secret in every backup, export and Version diff.
    // Returns null when the site was not given it.
    secret(name) { return call("secret", { text: name }); },
    vault: {
      set(name, value) { return call("vault.set", { key: String(name), value: String(value) }); },
      get(name) {
        const res = call("vault.get", { key: String(name) });
        return res ? res.value : null;
      },
      del(name) { return call("vault.del", { key: String(name) }); },
      list(prefix) { return call("vault.list", { prefix: prefix ? String(prefix) : "" }) || []; },
    },
    audit(action, targetDoctype, targetName, detail) {
      return call("audit", {
        action: String(action),
        targetDoctype: String(targetDoctype || ""),
        targetName: String(targetName || ""),
        detail: detail || null,
      });
    },
    auditDenied(action, targetDoctype, targetName, detail) {
      return call("auditDenied", {
        action: String(action),
        targetDoctype: String(targetDoctype || ""),
        targetName: String(targetName || ""),
        detail: detail || null,
      });
    },
    // Self-service auth. These exist because ddcore.db.sql is read-only: no TS
    // can write to ddcore_session or ddcore_auth_token, so every one of these
    // has to cross the bridge. They take the user explicitly, and the services
    // in core/ check it is the session's own before calling.
    __auth: {
      sessions(user) { return call("auth.sessions", { user }); },
      revokeSessions(user, opts) { return call("auth.revokeSessions", { user, id: (opts || {}).id || "", exceptSid: (opts || {}).exceptSid || "" }); },
      currentSid() { return call("auth.currentSid", {}); },
      checkPassword(user, password) { return call("auth.checkPassword", { user, password }); },
      setPassword(user, password, exceptSid) { call("auth.setPassword", { user, password, exceptSid: exceptSid || "" }); },
      startRecovery(user, kind) { return call("auth.startRecovery", { user, kind: kind || "reset" }); },
      throttle(key, limit, minutes) { call("auth.throttle", { key, limit: limit || 0, minutes: minutes || 0 }); },
      clearAttempts(key) { return call("auth.clearAttempts", { key }); },
      createAPIKey(user, label, days) { return call("auth.createAPIKey", { user, label: label || "", days: days || 0 }); },
      apiKeys(user) { return call("auth.apiKeys", { user }); },
      revokeAPIKey(user, name) { return call("auth.revokeAPIKey", { user, name }); },
    },
    // The delivery half of the mail service, reached only from
    // core/services/mail.ts as a job target. Not on DDCoreAPI: an app queues a
    // message with ddcore.sendMail and never touches the wire itself.
    __mail: {
      load(delivery) { return call("mail.load", { delivery }); },
      deliver(delivery, subject, blocks) { return call("mail.deliver", { delivery, subject, blocks }); },
      result(delivery, status, error) { call("mail.result", { delivery, status, error: error || "" }); },
    },
    // The delivery half of webhooks, reached only from core/services/webhooks.ts
    // and the Webhook controller. Not on DDCoreAPI.
    __webhooks: {
      validate(doc) { call("webhook.validate", { doc }); },
      deliver(delivery) { call("webhook.deliver", { delivery }); },
      replay(delivery) { return call("webhook.replay", { delivery }); },
      sweep() { return call("webhook.sweep", {}); },
    },
    __authSweep() { return call("authSweep", {}); },
    __jobSweep() { return call("jobSweep", {}); },
    __auditSweep() { return call("auditSweep", {}); },
    // `user` lets the policy refuse a password that is the account name. It
    // throws when the password is below the site's minimum.
    __hashPassword(password, user) { return call("hashPassword", { text: password, user: user || "" }); },
    __dropSessions(user) { call("dropSessions", { user }); },
  };
  function wrapHttp(r) {
    r.json = function () { return JSON.parse(r.body); };
    return r;
  }
  globalThis.ddcore = api;
  globalThis.console = {
    log: (...a) => call("log", { level: "info", args: a.map(fmtArg) }),
    error: (...a) => call("log", { level: "error", args: a.map(fmtArg) }),
    warn: (...a) => call("log", { level: "warn", args: a.map(fmtArg) }),
    info: (...a) => call("log", { level: "info", args: a.map(fmtArg) }),
    debug: (...a) => call("log", { level: "debug", args: a.map(fmtArg) }),
  };
  function fmtArg(a) { return typeof a === "string" ? a : JSON.stringify(a); }

  // ------------------------------------------------------------- entry points
  // Go calls these with JSON in and JSON out.

  function hooksFor(doctype, event) {
    const fns = [];
    const c = reg.controllers[doctype];
    if (c && typeof c[event] === "function") fns.push(c[event]);
    for (const appName in reg.apps) {
      const ev = reg.apps[appName].docEvents || {};
      for (const key of [doctype, "*"]) {
        const h = ev[key] && ev[key][event];
        if (typeof h === "function") fns.push(h);
      }
    }
    return fns;
  }

  // Runs one lifecycle event; returns the (possibly mutated) doc.
  reg.runHook = function (doctype, event, docJSON, beforeJSON) {
    const fns = hooksFor(doctype, event);
    if (fns.length === 0) return docJSON;
    const doc = new Document(JSON.parse(docJSON));
    if (beforeJSON) doc.__before = JSON.parse(beforeJSON);
    const ctx = makeContext();
    for (const fn of fns) fn.call(doc, doc, ctx);
    return JSON.stringify(doc);
  };

  reg.hasHook = function (doctype, event) { return hooksFor(doctype, event).length > 0; };

  reg.runMethodOn = function (doc, name, args) {
    const c = reg.controllers[doc.doctype];
    const fn = c && c.methods && c.methods[name];
    if (typeof fn !== "function") throw new DDCoreError("NotFound", "", "Method " + name + " does not exist on " + doc.doctype);
    return fn.call(doc, doc, args || {}, makeContext());
  };

  // Runs a whitelisted controller method; returns { doc, result }.
  reg.runMethod = function (doctype, name, docJSON, argsJSON) {
    const doc = new Document(JSON.parse(docJSON));
    const result = reg.runMethodOn(doc, name, JSON.parse(argsJSON));
    // returns the persisted version: the method may have saved via dbSet/save and
    // the caller (desk, API) needs the document with the current timestamp.
    if (!doc.__islocal && doc.name) {
      try {
        doc._apply(call("getDoc", { doctype: doc.doctype, name: doc.name }));
      } catch (e) {
        // document deleted by the method itself: retain what is in memory
      }
    }
    return JSON.stringify({ doc, result: result === undefined ? null : result });
  };

  // The host's controller first, then every extension in load order. A denial
  // wins over an allow: an app that extends a DocType can restrict access to
  // it, never widen what the host already refused.
  reg.hasPermission = function (doctype, docJSON, ptype, user) {
    const c = reg.controllers[doctype];
    const fns = [];
    if (c && typeof c.hasPermission === "function") fns.push(c.hasPermission);
    for (const x of extensionsOf(doctype)) if (typeof x.ext.hasPermission === "function") fns.push(x.ext.hasPermission);
    if (fns.length === 0) return "";
    const doc = docJSON ? JSON.parse(docJSON) : null;
    let allowed;
    for (const fn of fns) {
      const r = fn(doc, ptype, user);
      if (r === false) return "false";
      if (r === true) allowed = true;
    }
    return allowed ? "true" : "";
  };

  // Every filter set is AND-ed: each extension narrows the rows the host
  // already allows. Objects are normalised to the [field, op, value] form so
  // two apps filtering the same field intersect instead of overwriting.
  reg.permissionQuery = function (doctype, user) {
    const c = reg.controllers[doctype];
    const fns = [];
    if (c && typeof c.permissionQuery === "function") fns.push(c.permissionQuery);
    for (const x of extensionsOf(doctype)) if (typeof x.ext.permissionQuery === "function") fns.push(x.ext.permissionQuery);
    if (fns.length === 0) return "";
    let out = [];
    for (const fn of fns) {
      const r = fn(user);
      if (r) out = out.concat(asFilterList(r));
    }
    return out.length ? JSON.stringify(out) : "";
  };

  // `{ field: v }` and `{ field: [op, v] }` become [[field, op, v], ...]; a
  // list is already in that form.
  function asFilterList(f) {
    if (Array.isArray(f)) return f;
    const out = [];
    for (const k in f) {
      const v = f[k];
      if (Array.isArray(v) && v.length === 2) out.push([k, v[0], v[1]]);
      else out.push([k, "=", v]);
    }
    return out;
  }

  // Resolves "app.dir.file.fn" into the exported function.
  function resolve(path) {
    const i = path.lastIndexOf(".");
    const m = reg.modules[path.slice(0, i)];
    const fn = m && m.exports && m.exports[path.slice(i + 1)];
    if (typeof fn !== "function") throw new DDCoreError("NotFound", "", "Function " + path + " not found");
    return fn;
  }
  reg.callModule = function (path, args) {
    return resolve(path)(args || {}, makeContext());
  };
  // Whitelisted call from the API: args are passed as a single object.
  reg.callWhitelisted = function (path, argsJSON) {
    const fn = resolve(path);
    if (!fn.__whitelisted) throw new DDCoreError("PermissionError", "", "Function " + path + " is not whitelisted");
    const r = fn(JSON.parse(argsJSON), makeContext());
    return JSON.stringify(r === undefined ? null : r);
  };
  reg.callFunction = function (path, argsJSON) {
    const r = resolve(path)(JSON.parse(argsJSON), makeContext());
    return JSON.stringify(r === undefined ? null : r);
  };

  reg.runReport = function (name, filtersJSON) {
    const r = reg.reports[name];
    if (!r) throw new DDCoreError("NotFound", "", "Report " + name + " does not exist");
    return JSON.stringify(r.execute(JSON.parse(filtersJSON) || {}, makeContext()));
  };

  reg.numberCard = function (workspace, name) {
    const ws = reg.workspaces[workspace];
    const card = ws && (ws.numberCards || []).find((c) => c.name === name);
    if (!card || typeof card.method !== "function") throw new DDCoreError("NotFound", "", "Card " + name + " has no method");
    return JSON.stringify(card.method());
  };
  reg.chart = function (workspace, name) {
    const ws = reg.workspaces[workspace];
    const ch = ws && (ws.charts || []).find((c) => c.name === name);
    if (!ch) throw new DDCoreError("NotFound", "", "Chart " + name + " does not exist");
    return JSON.stringify(ch.method());
  };

  reg.appHook = function (app, hook) {
    const a = reg.apps[app];
    if (a && typeof a[hook] === "function") a[hook](makeContext());
  };

  // A patch is `export default definePatch({...})` or, still supported, a bare
  // `export function execute`. The bare form has no phase and gets the default.
  function patchDef(m) {
    const ex = (m && m.exports) || {};
    if (ex.default && typeof ex.default.execute === "function") return ex.default;
    if (typeof ex.execute === "function") return ex;
    return null;
  }

  reg.runPatch = function (path) {
    const def = patchDef(reg.modules[path]);
    if (!def) throw new DDCoreError("NotFound", "", "Patch " + path + " has no execute()");
    const ctx = makeContext();
    // The only write-SQL there is, and it exists only here: a backfill cannot
    // be a row-by-row walk through the lifecycle, which rewrites `modified` on
    // every row and writes a Version per row — an audit trail a migration has
    // no business forging.
    ctx.sql = function (query, params) { return call("patchSQL", { query: query, params: params || [] }); };
    def.execute(ctx);
  };

  reg.eval = function (code) {
    const r = (0, eval)(code);
    return JSON.stringify(r === undefined ? null : r, (k, v) => (typeof v === "function" ? "[function]" : v));
  };


  // Evaluates a dependsOn-style expression: "eval:doc.x > 1", "doc.x", or a fieldname.
  reg.evalExpr = function (expr, docJSON) {
    const doc = JSON.parse(docJSON);
    let e = String(expr).trim();
    if (e.startsWith("eval:")) e = e.slice(5);
    if (/^[a-z_][a-z0-9_]*$/.test(e)) return doc[e] ? "true" : "false";
    try {
      return new Function("doc", "ddcore", "return (" + e + ")")(doc, api) ? "true" : "false";
    } catch (err) {
      return "false";
    }
  };

  // ------------------------------------------------------------------- tests
  const suites = [];
  let current = null;
  const root = { name: "", app: "", tests: [], beforeEach: [], afterEach: [], beforeAll: [], children: [] };
  current = root;
  globalThis.describe = (name, fn) => {
    const s = { name, app: reg.app, tests: [], beforeEach: [], afterEach: [], beforeAll: [], children: [], file: reg.current };
    current.children.push(s);
    const prev = current;
    current = s;
    try { fn(); } finally { current = prev; }
  };
  globalThis.it = globalThis.test = (name, fn) => current.tests.push({ name, fn, file: reg.current, app: reg.app });
  globalThis.beforeEach = (fn) => current.beforeEach.push({ fn, app: reg.app });
  globalThis.afterEach = (fn) => current.afterEach.push({ fn, app: reg.app });
  globalThis.beforeAll = (fn) => current.beforeAll.push({ fn, app: reg.app });

  function deepEqual(a, b) {
    if (a === b) return true;
    if (typeof a !== typeof b || a === null || b === null || typeof a !== "object") return false;
    if (Array.isArray(a) !== Array.isArray(b)) return false;
    const ka = Object.keys(a), kb = Object.keys(b);
    if (ka.length !== kb.length) return false;
    return ka.every((k) => deepEqual(a[k], b[k]));
  }
  const show = (v) => { try { return JSON.stringify(v); } catch (_) { return String(v); } };
  globalThis.expect = (actual) => {
    const make = (neg) => {
      const check = (ok, msg) => {
        if (neg ? ok : !ok) throw new Error((neg ? "not " : "") + msg);
      };
      const e = {
        toBe: (v) => check(Object.is(actual, v), `esperado ${show(v)}, recebido ${show(actual)}`),
        toEqual: (v) => check(deepEqual(actual, v), `esperado ${show(v)}, recebido ${show(actual)}`),
        toBeTruthy: () => check(!!actual, `esperado truthy, recebido ${show(actual)}`),
        toBeFalsy: () => check(!actual, `esperado falsy, recebido ${show(actual)}`),
        toBeNull: () => check(actual === null, `esperado null, recebido ${show(actual)}`),
        toBeUndefined: () => check(actual === undefined, `esperado undefined, recebido ${show(actual)}`),
        toBeDefined: () => check(actual !== undefined, `esperado definido`),
        toContain: (v) => check(typeof actual === "string" ? actual.includes(v) : Array.isArray(actual) && actual.some((x) => deepEqual(x, v)), `esperado que ${show(actual)} contenha ${show(v)}`),
        toBeGreaterThan: (n) => check(actual > n, `esperado ${show(actual)} > ${n}`),
        toBeGreaterThanOrEqual: (n) => check(actual >= n, `esperado ${show(actual)} >= ${n}`),
        toBeLessThan: (n) => check(actual < n, `esperado ${show(actual)} < ${n}`),
        toBeLessThanOrEqual: (n) => check(actual <= n, `esperado ${show(actual)} <= ${n}`),
        toBeCloseTo: (n, digits = 2) => check(Math.abs(actual - n) < Math.pow(10, -digits) / 2, `esperado ${show(actual)} ≈ ${n}`),
        toHaveLength: (n) => check(actual != null && actual.length === n, `esperado length ${n}, recebido ${actual && actual.length}`),
        toMatch: (re) => check(typeof re === "string" ? String(actual).includes(re) : re.test(String(actual)), `esperado ${show(actual)} ~ ${re}`),
        toThrow: (match) => {
          let err = null;
          try { actual(); } catch (x) { err = x; }
          if (neg) { if (err) throw new Error("expected no error, got: " + (err.message || err)); return; }
          if (!err) throw new Error("esperava erro" + (match ? " ~ " + match : ""));
          if (match) {
            const text = (err.title ? err.title + ": " : "") + (err.message || String(err));
            const ok = typeof match === "string" ? text.includes(match) : match.test(text);
            if (!ok) throw new Error(`error ${show(text)} does not match ${match}`);
          }
        },
      };
      return e;
    };
    const pos = make(false);
    pos.not = make(true);
    return pos;
  };

  reg.runTests = function (filter, app) {
    const results = [];
    const re = filter ? new RegExp(filter, "i") : null;
    function walk(suite, path, befores, afters) {
      if (app && suite.app && suite.app !== app) return;
      const selectedHooks = (hooks) => app ? hooks.filter((hook) => hook.app === app) : hooks;
      for (const b of selectedHooks(suite.beforeAll)) b.fn();
      const be = selectedHooks(befores).concat(selectedHooks(suite.beforeEach));
      const af = selectedHooks(suite.afterEach).concat(selectedHooks(afters));
      for (const t of suite.tests) {
        const full = (path ? path + " > " : "") + t.name;
        if (app && t.app !== app) continue;
        if (re && !re.test(full) && !re.test(t.file || "")) continue;
        const r = { name: full, file: t.file, app: t.app, ok: true };
        const t0 = Date.now();
        call("test.begin");
        try {
          for (const b of be) b.fn();
          t.fn();
          for (const a of af) a.fn();
        } catch (e) {
          r.ok = false;
          r.error = (e && e.title ? e.title + ": " : "") + (e && e.message ? e.message : String(e));
          if (e && e.stack) r.stack = String(e.stack).split("\n").slice(0, 6).join("\n");
        }
        call("test.rollback");
        r.ms = Date.now() - t0;
        results.push(r);
      }
      for (const c of suite.children) walk(c, (path ? path + " > " : "") + c.name, be, af);
    }
    walk(root, "", [], []);
    return JSON.stringify(results);
  };
})();
