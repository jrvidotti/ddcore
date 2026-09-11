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
  try { data = text ? JSON.parse(text) : null; } catch { data = { error: { type: "InternalError", message: text.slice(0, 500) } }; }
  if (!res.ok || (data && data.error)) {
    const e = data?.error || { type: "HTTPError", message: res.statusText };
    if (res.status === 401 && typeof window !== "undefined" && !location.pathname.startsWith("/login")) {
      location.href = "/login?redirect=" + encodeURIComponent(location.pathname + location.search);
    }
    throw new DDCoreError(e.type, e.title || "", e.message, res.status, e.extra, e.key, e.args);
  }
  if (data?.messages) for (const m of data.messages) messages.push(m);
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

  login: (usr: string, pwd: string) => request("POST", "/api/login", { usr, pwd }),
  logout: () => request("POST", "/api/logout"),
  boot: () => request("GET", "/api/boot"),
  meta: (doctype: string) => request("GET", `/api/meta/${encodeURIComponent(doctype)}`),
  translations: (lang: string) => request<Record<string, string>>("GET", `/api/translations?lang=${lang}`),

  list: (doctype: string, params: { filters?: any; or_filters?: any; fields?: string[]; order_by?: string; limit?: number; start?: number; with_count?: boolean; group_by?: string } = {}) =>
    request("GET", `/api/resource/${encodeURIComponent(doctype)}` + q(params as any)),
  count: (doctype: string, filters?: any, or_filters?: any) => request<number>("GET", `/api/count/${encodeURIComponent(doctype)}` + q({ filters, or_filters })),
  getDoc: (doctype: string, name: string) => request("GET", `/api/resource/${encodeURIComponent(doctype)}/${encodeURIComponent(name)}`),
  insert: (doctype: string, doc: any) => request("POST", `/api/resource/${encodeURIComponent(doctype)}`, doc),
  update: (doctype: string, name: string, doc: any) => request("PUT", `/api/resource/${encodeURIComponent(doctype)}/${encodeURIComponent(name)}`, doc),
  remove: (doctype: string, name: string) => request("DELETE", `/api/resource/${encodeURIComponent(doctype)}/${encodeURIComponent(name)}`),
  docMethod: (doctype: string, name: string, method: string, args: any = {}) =>
    request("POST", `/api/resource/${encodeURIComponent(doctype)}/${encodeURIComponent(name)}/${encodeURIComponent(method)}`, args),
  call: (path: string, args: any = {}) => request("POST", `/api/method/${path}`, args),
  linkSearch: (doctype: string, txt: string, filters?: any, limit = 20) => request<any[]>("GET", "/api/search/link" + q({ doctype, txt, filters, limit })),
  linkTitles: (doctype: string, names: string[]) =>
    request<Record<string, Record<string, string>>>("GET", "/api/search/link-titles" + q({ doctype, names: names.join(",") })),
  report: (name: string, filters: any) => request("GET", `/api/report/${encodeURIComponent(name)}` + q({ filters })),
  numberCard: (ws: string, card: string) => request("GET", `/api/workspace/${encodeURIComponent(ws)}/card/${encodeURIComponent(card)}`),
  chart: (ws: string, chart: string) => request("GET", `/api/workspace/${encodeURIComponent(ws)}/chart/${encodeURIComponent(chart)}`),
  comments: (doctype: string, name: string) => request<any[]>("GET", `/api/comments/${encodeURIComponent(doctype)}/${encodeURIComponent(name)}`),
  versions: (doctype: string, name: string) => request<any[]>("GET", `/api/versions/${encodeURIComponent(doctype)}/${encodeURIComponent(name)}`),
  upload: (file: File, opts: { doctype?: string; docname?: string; fieldname?: string; isPrivate?: boolean } = {}) => {
    const fd = new FormData();
    fd.append("file", file);
    if (opts.doctype) fd.append("doctype", opts.doctype);
    if (opts.docname) fd.append("docname", opts.docname);
    if (opts.fieldname) fd.append("fieldname", opts.fieldname);
    fd.append("is_private", opts.isPrivate === false ? "0" : "1");
    return request("POST", "/api/upload", fd);
  },
};
