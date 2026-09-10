// Session-wide state: who is logged in, what DocTypes/workspaces exist,
// translations. Loaded once from /api/boot.
import { api } from "./api";

export interface Boot {
  user: string;
  roles: string[];
  userDoc: { name: string; full_name: string; language?: string } | null;
  lang: string;
  apps: { name: string; title: string; desk: any; hasDeskInclude: boolean }[];
  workspaces: any[];
  doctypes: Record<string, { label: string; app: string; icon: string; module: string; titleField?: string }>;
  reports: Record<string, { label: string; refDoctype?: string; app: string }>;
  site: { name: string; currency: string; dev: boolean; scheduler: boolean; version: string };
  loaded: number;
}

export const boot = $state<{ data: Boot | null; translations: Record<string, string>; ready: boolean }>({ data: null, translations: {}, ready: false });

export async function loadBoot(): Promise<Boot> {
  const data = await api.boot();
  boot.data = data;
  try { boot.translations = (await api.translations(data.lang)) || {}; } catch { boot.translations = {}; }
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
