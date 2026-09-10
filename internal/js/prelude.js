// Prelude: runs once per runtime before any app bundle. Defines the registry
// (`__cerne`), the `cerne` bridge API and the Document class. Every call to
// Go goes through __host(op, jsonArgs) -> jsonResult.
(function () {
  "use strict";
  const host = globalThis.__host;

  class CerneError extends Error {
    constructor(type, title, message, extra) {
      super(message);
      this.name = type || "CerneError";
      this.cerneType = type || "ValidationError";
      this.title = title || "";
      this.extra = extra;
    }
  }
  globalThis.CerneError = CerneError;

  function toError(e) {
    if (e instanceof CerneError) return e;
    const msg = String((e && e.message) || e);
    const i = msg.indexOf("cerne:{");
    if (i >= 0) {
      try {
        const o = JSON.parse(msg.slice(i + 6));
        return new CerneError(o.type, o.title, o.message, o.extra);
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
    current: "",
    doctypes: {},
    controllers: {},
    reports: {},
    workspaces: {},
    apps: {},
    modules: {},
    modulesByApp: {},
    tests: [],
    register(kind, value) {
      switch (kind) {
        case "doctype":
          value.app = value.app || reg.app;
          value.sourceFile = reg.current;
          reg.doctypes[value.name] = value;
          break;
        case "controller":
          reg.controllers[value.doctype] = value.controller;
          break;
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
  globalThis.__cerne = reg;

  const stripFns = (o) =>
    JSON.parse(JSON.stringify(o, (k, v) => (typeof v === "function" ? undefined : v)));

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
      if (/\.patches\.[^.]+$/.test(path) && typeof ex.execute === "function") {
        patches.push({ app: path.split(".")[0], name: path.split(".").pop(), path });
      }
    }
    const reports = {};
    for (const n in reg.reports) reports[n] = stripFns(reg.reports[n]);
    const workspaces = {};
    for (const n in reg.workspaces) workspaces[n] = stripFns(reg.workspaces[n]);
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
      d.hasPermissionHook = typeof c.hasPermission === "function";
      d.hasPermissionQuery = typeof c.permissionQuery === "function";
      doctypes[n] = d;
    }
    return JSON.stringify({ doctypes, reports, workspaces, apps, whitelisted, patches });
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
    delete(opts) { call("doc.delete", { doctype: this.doctype, name: this.name, opts }); }
    reload() { return this._apply(call("getDoc", { doctype: this.doctype, name: this.name })); }
    dbSet(field, value) {
      const values = typeof field === "object" ? field : { [field]: value };
      const res = call("doc.dbSet", { doctype: this.doctype, name: this.name, values });
      Object.assign(this, values);
      // o bridge também atualizou `modified`: sem isso o próximo save()
      // falharia com TimestampMismatch sem ninguém ter editado o documento.
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

  // ---------------------------------------------------------------- cerne API
  function makeContext() { return call("session"); }

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
    roundTo(n, p = 2) { const m = Math.pow(10, p); return Math.round((n + Number.EPSILON) * m) / m; },
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
    throw(message, opts) {
      opts = opts || {};
      throw new CerneError(opts.type || "ValidationError", opts.title, message, opts.extra);
    },
    msgprint(message, opts) { call("msgprint", { message, opts: opts || {} }); },
    _(text, args) {
      let s = call("translate", { text });
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
      get(url, opts) { return wrapHttp(call("http", Object.assign({ method: "GET", url }, opts || {}))); },
      post(url, body, opts) { return wrapHttp(call("http", Object.assign({ method: "POST", url, body }, opts || {}))); },
    },
    enqueue(method, args, opts) { return call("enqueue", { method, args: args || {}, opts: opts || {} }); },
    publish(event, payload, opts) { call("publish", { event, payload, opts: opts || {} }); },
    log: {
      info: (...a) => call("log", { level: "info", args: a.map(String) }),
      warn: (...a) => call("log", { level: "warn", args: a.map(String) }),
      error: (...a) => call("log", { level: "error", args: a.map(String) }),
      debug: (...a) => call("log", { level: "debug", args: a.map(String) }),
    },
    isTest() { return !!globalThis.__cerneTest; },
    isJob() { return !call("session").request; },
    form: {},
    callMethod(method, args) { return reg.callModule(method, args || {}); },
    rename(doctype, oldName, newName) { return call("rename", { doctype, oldName, newName }); },
    __hashPassword(password) { return call("hashPassword", { text: password }); },
    __dropSessions(user) { call("dropSessions", { user }); },
  };
  function wrapHttp(r) {
    r.json = function () { return JSON.parse(r.body); };
    return r;
  }
  globalThis.cerne = api;
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
    if (typeof fn !== "function") throw new CerneError("NotFound", "", "Método " + name + " não existe em " + doc.doctype);
    return fn.call(doc, doc, args || {}, makeContext());
  };

  // Runs a whitelisted controller method; returns { doc, result }.
  reg.runMethod = function (doctype, name, docJSON, argsJSON) {
    const doc = new Document(JSON.parse(docJSON));
    const result = reg.runMethodOn(doc, name, JSON.parse(argsJSON));
    // devolve a versão persistida: o método pode ter gravado por dbSet/save e
    // quem chamou (desk, API) precisa do documento com o timestamp atual.
    if (!doc.__islocal && doc.name) {
      try {
        doc._apply(call("getDoc", { doctype: doc.doctype, name: doc.name }));
      } catch (e) {
        // documento apagado pelo próprio método: mantém o que está em memória
      }
    }
    return JSON.stringify({ doc, result: result === undefined ? null : result });
  };

  reg.hasPermission = function (doctype, docJSON, ptype, user) {
    const c = reg.controllers[doctype];
    if (!c || typeof c.hasPermission !== "function") return "";
    const r = c.hasPermission(docJSON ? JSON.parse(docJSON) : null, ptype, user);
    return r === undefined ? "" : r ? "true" : "false";
  };

  reg.permissionQuery = function (doctype, user) {
    const c = reg.controllers[doctype];
    if (!c || typeof c.permissionQuery !== "function") return "";
    const r = c.permissionQuery(user);
    return r ? JSON.stringify(r) : "";
  };

  // Resolves "app.dir.file.fn" into the exported function.
  function resolve(path) {
    const i = path.lastIndexOf(".");
    const m = reg.modules[path.slice(0, i)];
    const fn = m && m.exports && m.exports[path.slice(i + 1)];
    if (typeof fn !== "function") throw new CerneError("NotFound", "", "Função " + path + " não encontrada");
    return fn;
  }
  reg.callModule = function (path, args) {
    return resolve(path)(args || {}, makeContext());
  };
  // Whitelisted call from the API: args are passed as a single object.
  reg.callWhitelisted = function (path, argsJSON) {
    const fn = resolve(path);
    if (!fn.__whitelisted) throw new CerneError("PermissionError", "", "Função " + path + " não é whitelisted");
    const r = fn(JSON.parse(argsJSON), makeContext());
    return JSON.stringify(r === undefined ? null : r);
  };
  reg.callFunction = function (path, argsJSON) {
    const r = resolve(path)(JSON.parse(argsJSON), makeContext());
    return JSON.stringify(r === undefined ? null : r);
  };

  reg.runReport = function (name, filtersJSON) {
    const r = reg.reports[name];
    if (!r) throw new CerneError("NotFound", "", "Relatório " + name + " não existe");
    return JSON.stringify(r.execute(JSON.parse(filtersJSON) || {}, makeContext()));
  };

  reg.numberCard = function (workspace, name) {
    const ws = reg.workspaces[workspace];
    const card = ws && (ws.numberCards || []).find((c) => c.name === name);
    if (!card || typeof card.method !== "function") throw new CerneError("NotFound", "", "Card " + name + " não tem method");
    return JSON.stringify(card.method());
  };
  reg.chart = function (workspace, name) {
    const ws = reg.workspaces[workspace];
    const ch = ws && (ws.charts || []).find((c) => c.name === name);
    if (!ch) throw new CerneError("NotFound", "", "Chart " + name + " não existe");
    return JSON.stringify(ch.method());
  };

  reg.appHook = function (app, hook) {
    const a = reg.apps[app];
    if (a && typeof a[hook] === "function") a[hook](makeContext());
  };

  reg.runPatch = function (path) {
    const m = reg.modules[path];
    if (!m || typeof m.exports.execute !== "function") throw new CerneError("NotFound", "", "Patch " + path + " não tem execute()");
    m.exports.execute(makeContext());
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
      return new Function("doc", "cerne", "return (" + e + ")")(doc, api) ? "true" : "false";
    } catch (err) {
      return "false";
    }
  };

  // ------------------------------------------------------------------- tests
  const suites = [];
  let current = null;
  const root = { name: "", tests: [], beforeEach: [], afterEach: [], beforeAll: [], children: [] };
  current = root;
  globalThis.describe = (name, fn) => {
    const s = { name, tests: [], beforeEach: [], afterEach: [], beforeAll: [], children: [], file: reg.current };
    current.children.push(s);
    const prev = current;
    current = s;
    try { fn(); } finally { current = prev; }
  };
  globalThis.it = globalThis.test = (name, fn) => current.tests.push({ name, fn, file: reg.current, app: reg.app });
  globalThis.beforeEach = (fn) => current.beforeEach.push(fn);
  globalThis.afterEach = (fn) => current.afterEach.push(fn);
  globalThis.beforeAll = (fn) => current.beforeAll.push(fn);

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
          if (neg) { if (err) throw new Error("não esperava erro, recebido: " + (err.message || err)); return; }
          if (!err) throw new Error("esperava erro" + (match ? " ~ " + match : ""));
          if (match) {
            const text = (err.title ? err.title + ": " : "") + (err.message || String(err));
            const ok = typeof match === "string" ? text.includes(match) : match.test(text);
            if (!ok) throw new Error(`erro ${show(text)} não bate com ${match}`);
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
      for (const b of suite.beforeAll) b();
      const be = befores.concat(suite.beforeEach), af = suite.afterEach.concat(afters);
      for (const t of suite.tests) {
        const full = (path ? path + " > " : "") + t.name;
        if (app && t.app !== app) continue;
        if (re && !re.test(full) && !re.test(t.file || "")) continue;
        const r = { name: full, file: t.file, app: t.app, ok: true };
        const t0 = Date.now();
        call("test.begin");
        try {
          for (const b of be) b();
          t.fn();
          for (const a of af) a();
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
