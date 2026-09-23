// Keep these response types aligned with the public Desk SDK.
export interface DeskNotification {
  id: string; title: string; message: string; creation: string; read: boolean;
  reference_doctype: string; reference_id: string;
}
export interface NotificationListOptions { limit?: number; offset?: number; read?: boolean }
export interface NotificationPage { data: DeskNotification[]; total: number }

export interface ToDoDoc {
  id: string;
  status: "Open" | "Closed" | "Cancelled";
  priority: "Low" | "Medium" | "High" | "Urgent";
  date?: string;
  allocated_to: string;
  assigned_by: string;
  description?: string;
  reference_type?: string;
  reference_id?: string;
  creation?: string;
  modified?: string;
}

/** One user's share on one document (SEC-03). */
export interface DocShare {
  id: string;
  user: string;
  share_doctype: string;
  share_id: string;
  read: boolean;
  write: boolean;
  share: boolean;
  override_scope: boolean;
  owner: string;
}
export interface DocSharesInfo {
  shares: DocShare[];
  canShare: boolean;
  canOverrideScope: boolean;
}
export interface ShareArgs {
  user: string;
  write?: boolean;
  share?: boolean;
  overrideScope?: boolean;
}

export interface AssignArgs {
  allocated_to: string;
  date?: string;
  priority?: "Low" | "Medium" | "High" | "Urgent";
  description?: string;
}

export interface PendingWorkOptions {
  limit?: number;
  offset?: number;
  status?: string;
  scope?: "assigned_to_me" | "assigned_by_me";
}

export interface PendingWorkPage {
  data: ToDoDoc[];
  total: number;
}
import { setMaintenance } from "./maintenance.svelte";

// Thin client for the ddcore HTTP API. Every error becomes a DDCoreError with
// type/title/message so the UI can show it the same way the server phrased it.
export class DDCoreError extends Error {
  constructor(
    public type: string,
    public title: string,
    message: string,
    public status: number,
    public extra?: any,
    /** English template with {0} placeholders, and its arguments. The message
     * already arrives translated; these travel for telemetry and grouping. */
    public key?: string,
    public args?: any[],
    /** The id the server knows this request by. It is on the response header
     * and in the error body, and it is what an operator greps for. */
    public requestId?: string,
  ) {
    super(message);
  }
}

export interface Message { message: string; title?: string; indicator?: string; alert?: boolean }

export const messages: { list: Message[]; push(m: Message): void } = {
  list: [],
  push(m) { this.list.push(m); listeners.forEach((l) => l(m)); },
};
const listeners: ((m: Message) => void)[] = [];
export function onMessage(fn: (m: Message) => void) { listeners.push(fn); return () => listeners.splice(listeners.indexOf(fn), 1); }

/**
 * Language sent as X-Lang on every request. Empty until /api/boot answers —
 * that first call is the one that resolves the language server-side.
 * Set through setRequestLang() in boot.svelte.ts.
 */
let requestLang = "";
export function setRequestLang(l: string) { requestLang = l || ""; }

async function request<T = any>(method: string, url: string, body?: any, opts: { raw?: boolean } = {}): Promise<T> {
  const headers: Record<string, string> = { "X-DDCore-CSRF": "1", "X-Requested-With": "ddcore" };
  if (requestLang) headers["X-Lang"] = requestLang;
  let payload: BodyInit | undefined;
  if (body instanceof FormData) payload = body;
  else if (body !== undefined) { headers["Content-Type"] = "application/json"; payload = JSON.stringify(body); }
  const res = await fetch(url, { method, headers, body: payload, credentials: "same-origin" });
  const text = await res.text();
  let data: any = null;
  const contentType = res.headers.get("Content-Type") || "";
  if (contentType.includes("application/json") || (!contentType.includes("text/html") && text.trim().startsWith("{"))) {
    try { data = text ? JSON.parse(text) : null; } catch { data = { error: { type: "InternalError", message: text.slice(0, 500) } }; }
  } else {
    data = text;
  }
  if (!res.ok || (data && typeof data === "object" && data.error)) {
    const e = data?.error || { type: "HTTPError", message: res.statusText };
    if (res.status === 401 && typeof window !== "undefined" && !location.pathname.startsWith("/login")) {
      location.href = "/login?redirect=" + encodeURIComponent(location.pathname + location.search);
    }
    // a paused site refuses every write; the banner should appear on the first
    // refusal rather than wait for the event stream to catch up
    if (e.type === "MaintenanceError") setMaintenance({ enabled: true, reason: e.extra?.reason });
    throw new DDCoreError(e.type, e.title || "", e.message, res.status, e.extra, e.key, e.args,
      e.requestId || res.headers.get("X-Request-Id") || undefined);
  }
  if (data?.messages) for (const m of data.messages) messages.push(m);
  if (typeof data === "string") return data as any;
  return opts.raw ? data : data?.data;
}

const q = (params: Record<string, any>) => {
  const s = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === "") continue;
    s.set(k, typeof v === "object" ? JSON.stringify(v) : String(v));
  }
  const str = s.toString();
  return str ? "?" + str : "";
};

export const api = {
  get: <T = any>(url: string, params: Record<string, any> = {}) => request<T>("GET", url + q(params)),
  post: <T = any>(url: string, body?: any) => request<T>("POST", url, body),
  put: <T = any>(url: string, body?: any) => request<T>("PUT", url, body),
  delete: <T = any>(url: string) => request<T>("DELETE", url),

  notifications: {
    list: (options: NotificationListOptions = {}) => request<NotificationPage>("GET", "/api/notifications" + q(options)),
    count: () => request<number>("GET", "/api/notifications/count"),
    setRead: (id: string, read: boolean) => request<DeskNotification>("PATCH", "/api/notifications/" + encodeURIComponent(id), { read }),
  },

  assignments: {
    assign: (doctype: string, id: string, args: AssignArgs) =>
      request<ToDoDoc>("POST", "/api/assignments/assign", { doctype, id, ...args }),
    complete: (id: string) =>
      request<ToDoDoc>("POST", "/api/assignments/complete", { id }),
    revoke: (id: string) =>
      request<{ success: boolean }>("POST", "/api/assignments/revoke", { id }),
    forDoc: (doctype: string, id: string) =>
      request<ToDoDoc[]>("GET", `/api/assignments/${encodeURIComponent(doctype)}/${encodeURIComponent(id)}`),
    pending: (options: PendingWorkOptions = {}) =>
      request<PendingWorkPage>("GET", "/api/todo/pending" + q(options)),
  },

  shares: {
    forDoc: (doctype: string, id: string) =>
      request<DocSharesInfo>("GET", `/api/shares/${encodeURIComponent(doctype)}/${encodeURIComponent(id)}`),
    add: (doctype: string, id: string, args: ShareArgs) =>
      request<DocShare>("POST", "/api/shares/add", { doctype, id, read: true, ...args }),
    remove: (doctype: string, id: string, user: string) =>
      request<{ ok: boolean }>("POST", "/api/shares/remove", { doctype, id, user }),
  },

  print: {
    formats: (doctype: string) =>
      request<{ name: string; label: string; default?: boolean }[]>("GET", `/api/print/formats/${encodeURIComponent(doctype)}`),
    letterheads: () =>
      request<{ id: string; is_default?: boolean; disabled?: boolean }[]>("GET", "/api/letterheads"),
    html: (doctype: string, id: string, params: { format?: string; letterhead?: string; lang?: string } = {}) =>
      request<string>("GET", `/api/print/${encodeURIComponent(doctype)}/${encodeURIComponent(id)}` + q(params)),
    pdfUrl: (doctype: string, id: string, params: { format?: string; letterhead?: string; lang?: string; download?: boolean } = {}) =>
      `/api/print/${encodeURIComponent(doctype)}/${encodeURIComponent(id)}/pdf` + q(params),
  },

  login: (usr: string, pwd: string) => request("POST", "/api/login", { usr, pwd }),
  logout: () => request("POST", "/api/logout"),

  // Recovery and invitation. These answer for Guest, so they are the only
  // calls here that work before signing in.
  forgotPassword: (usr: string) => request("POST", "/api/auth/forgot-password", { usr }),
  /** What a link is for, without spending it. */
  authToken: (token: string) =>
    request<{ kind: string; user: string; fullName: string; expires: string }>("POST", "/api/auth/token", { token }),
  resetPassword: (token: string, password: string) =>
    request("POST", "/api/auth/reset-password", { token, password }),
  acceptInvite: (token: string, password: string, fullName?: string) =>
    request("POST", "/api/auth/accept-invite", { token, password, fullName }),
  boot: () => request("GET", "/api/boot"),
  meta: (doctype: string) => request("GET", `/api/meta/${encodeURIComponent(doctype)}`),
  translations: (lang: string) => request<Record<string, string>>("GET", `/api/translations?lang=${lang}`),

  list: (doctype: string, params: { filters?: any; or_filters?: any; fields?: string[]; order_by?: string; limit?: number; start?: number; with_count?: boolean; group_by?: string } = {}) =>
    request("GET", `/api/resource/${encodeURIComponent(doctype)}` + q(params as any)),
  count: (doctype: string, filters?: any, or_filters?: any) => request<number>("GET", `/api/count/${encodeURIComponent(doctype)}` + q({ filters, or_filters })),
  getSingle: (doctype: string) => request("GET", `/api/resource/${encodeURIComponent(doctype)}/singleton`),
  getDoc: (doctype: string, id: string) => request("GET", `/api/resource/${encodeURIComponent(doctype)}/${encodeURIComponent(id)}`),
  insert: (doctype: string, doc: any) => request("POST", `/api/resource/${encodeURIComponent(doctype)}`, doc),
  update: (doctype: string, id: string, doc: any) => request("PUT", `/api/resource/${encodeURIComponent(doctype)}/${encodeURIComponent(id)}`, doc),
  remove: (doctype: string, id: string) => request("DELETE", `/api/resource/${encodeURIComponent(doctype)}/${encodeURIComponent(id)}`),
  docMethod: (doctype: string, id: string, method: string, args: any = {}) =>
    request("POST", `/api/resource/${encodeURIComponent(doctype)}/${encodeURIComponent(id)}/${encodeURIComponent(method)}`, args),
  call: (path: string, args: any = {}) => request("POST", `/api/method/${path}`, args),
  /** One level of a tree DocType (DAT-07); an empty parent asks for the roots. */
  treeChildren: (doctype: string, parent = "", limit = 500) =>
    request<{ nodes: { id: string; title: string; parent: string; is_group: boolean; children: number }[]; hasMore: boolean }>(
      "GET", `/api/tree/${encodeURIComponent(doctype)}` + q({ parent, limit })),
  linkSearch: (doctype: string, txt: string, filters?: any, limit = 20) => request<any[]>("GET", "/api/search/link" + q({ doctype, txt, filters, limit })),
  globalSearch: (txt: string, limit = 20) =>
    request<{ doctype: string; label: string; id: string; title: string }[]>("GET", "/api/search/global" + q({ txt, limit })),
  linkTitles: (doctype: string, ids: string[]) =>
    request<Record<string, Record<string, string>>>("GET", "/api/search/link-titles" + q({ doctype, ids: ids.join(",") })),
  report: (name: string, filters: any) => request("GET", `/api/report/${encodeURIComponent(name)}` + q({ filters })),
  numberCard: (ws: string, card: string) => request("GET", `/api/workspace/${encodeURIComponent(ws)}/card/${encodeURIComponent(card)}`),
  chart: (ws: string, chart: string) => request("GET", `/api/workspace/${encodeURIComponent(ws)}/chart/${encodeURIComponent(chart)}`),
  comments: (doctype: string, id: string) => request<any[]>("GET", `/api/comments/${encodeURIComponent(doctype)}/${encodeURIComponent(id)}`),
  versions: (doctype: string, id: string) => request<any[]>("GET", `/api/versions/${encodeURIComponent(doctype)}/${encodeURIComponent(id)}`),
  upload: (file: File, opts: { doctype?: string; docId?: string; fieldname?: string; isPrivate?: boolean } = {}) => {
    const fd = new FormData();
    fd.append("file", file);
    if (opts.doctype) fd.append("doctype", opts.doctype);
    if (opts.docId) fd.append("doc_id", opts.docId);
    if (opts.fieldname) fd.append("fieldname", opts.fieldname);
    fd.append("is_private", opts.isPrivate === false ? "0" : "1");
    return request("POST", "/api/upload", fd);
  },
  /** Data Import: checks (dryRun) or loads a CSV/XLSX file into `doctype`. */
  dataImport: (doctype: string, file: File, opts: DataImportOptions) => {
    const fd = new FormData();
    fd.append("file", file);
    fd.append("mode", opts.mode);
    fd.append("dry_run", opts.dryRun ? "1" : "0");
    if (opts.sep) fd.append("sep", opts.sep);
    fd.append("decimal", opts.decimal);
    fd.append("date_order", opts.dateOrder);
    if (opts.columns && Object.keys(opts.columns).length) fd.append("columns", JSON.stringify(opts.columns));
    return request<DataImportResult>("POST", `/api/data-import/${encodeURIComponent(doctype)}`, fd);
  },
};

export interface DataImportOptions {
  mode: "insert" | "update";
  dryRun: boolean;
  /** CSV separator; empty lets the server read it from the header line. */
  sep?: string;
  decimal: "." | ",";
  dateOrder: "dmy" | "mdy" | "ymd";
  /** header → fieldname, or "" to ignore the column */
  columns?: Record<string, string>;
}

export interface DataImportColumn {
  index: number; header: string; fieldname?: string; label?: string;
  status: "mapped" | "ignored" | "unknown"; reason?: string;
}

export interface DataImportRow {
  row: number; status: "inserted" | "updated" | "error"; id?: string;
  type?: string; message?: string; cells?: string[];
}

export interface DataImportResult {
  doctype: string; mode: "insert" | "update"; dryRun: boolean;
  file: { name: string; format: "csv" | "xlsx"; sep?: string; sha256: string };
  headers: string[];
  columns: DataImportColumn[];
  fields: { fieldname: string; label: string; reqd?: boolean }[];
  counts: { rows: number; inserted: number; updated: number; errors: number };
  rows: DataImportRow[];
}
