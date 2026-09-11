// Session-wide state: who is logged in, what DocTypes/workspaces exist,
// translations. Loaded once from /api/boot.
import { api, setRequestLang } from "./api";
import { resetLocale } from "./locale";

const LANG_KEY = "ddcore_lang";

function rememberedLang(): string {
  try { return localStorage.getItem(LANG_KEY) || ""; } catch { return ""; }
}

function rememberLang(l: string) {
  try { localStorage.setItem(LANG_KEY, l); } catch { /* private mode */ }
}

export interface Boot {
  user: string;
  roles: string[];
  userDoc: { name: string; full_name: string; language?: string } | null;
  lang: string;
  /** The languages the site serves, each labelled with its own autonym. */
  langs: { code: string; label: string }[];
  apps: { name: string; title: string; desk: { include?: string[]; home?: string; logo?: string } | null; hasDeskInclude: boolean }[];
  workspaces: any[];
  doctypes: Record<string, { label: string; app: string; icon: string; module: string; titleField?: string }>;
  reports: Record<string, { label: string; refDoctype?: string; app: string }>;
  site: {
    name: string; currency: string; timezone: string; dev: boolean; scheduler: boolean; version: string;
    // resolved by the server, not derived again here: two independent
    // derivations that "should" agree is the bug nobody finds until a JPY
    // invoice is off by a yen
    currencyPrecision?: number; rounding?: "commercial" | "bankers";
  };
  loaded: number;
}

export const boot = $state<{ data: Boot | null; translations: Record<string, string>; ready: boolean }>({ data: null, translations: {}, ready: false });

export async function loadBoot(): Promise<Boot> {
  // No X-Lang on this first call: /api/boot is what resolves the language,
  // from User.language for a session and Accept-Language for a visitor. A
  // header here would outrank the user's own setting.
  setRequestLang("");
  const data = await api.boot();
  // Signed in, the server has the last word and we remember what it said;
  // signed out, the login screen shows the language of the last session.
  const lang = data.user === "Guest" ? rememberedLang() || data.lang : data.lang;
  if (data.user !== "Guest") rememberLang(lang);
  data.lang = lang;
  boot.data = data;
  setRequestLang(lang);
  // the memoised Intl formatters belong to the old language
  resetLocale();
  try { boot.translations = (await api.translations(lang)) || {}; } catch { boot.translations = {}; }
  boot.ready = true;
  return data;
}

/** Translate a string; {0} placeholders are filled from args. */
export function __(s: string, args?: any[]): string {
  let t = boot.translations[s] || s;
  if (args) t = t.replace(/\{(\d+)\}/g, (m, i) => (args[i] === undefined ? m : String(args[i])));
  return t;
}

export const isLoggedIn = () => !!boot.data && boot.data.user !== "Guest";
export const hasRole = (r: string) => !!boot.data?.roles.includes(r);
export const doctypeLabel = (dt: string) => boot.data?.doctypes[dt]?.label || dt;

/**
 * What the site is called, for a title or a heading: the title of the app the
 * desk is showing, resolved and translated by the server. The fallback only
 * covers the moment before the boot arrives.
 */
export const siteName = () => boot.data?.site?.name || "ddcore";

/**
 * The square mark shown beside the site's name, in the sidebar and on the
 * sign-in screens. An app declares it with `desk.logo` — the first app in load
 * order that does wins, the way `desk.home` is resolved; the core declares
 * none, so an app's always does. Without one the mark is the initial of the
 * name beside it, which is the app's title in the reader's language, so the
 * letter and the word cannot drift apart.
 */
export function siteLogo(): string {
  const declared = boot.data?.apps?.map((a) => a.desk?.logo).find(Boolean);
  if (declared) return declared;
  // spread, not charAt: a name starting with an emoji is one grapheme made of
  // two code units, and half a surrogate pair renders as a replacement square
  const [initial] = Array.from((boot.data?.site?.name || "").trim());
  return initial ? initial.toUpperCase() : "d";
}
