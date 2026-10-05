// What the Feedback modal asks and sends: the three kinds of feedback and the
// fields each one adds, the checks before sending, the `data` part of
// POST /api/feedback, and the context a report may carry. Pure, so it is
// tested without a browser; every label is an English key the modal
// translates with __().
import type { FeedbackData, FeedbackStatus, FeedbackType } from "./api";
import type { ClientError } from "./client-errors";

export type FeedbackFieldName =
  | "steps_to_reproduce" | "expected_result" | "actual_result" | "severity"
  | "current_behavior" | "suggested_improvement" | "problem" | "expected_benefit";

export interface FeedbackFieldDef {
  name: FeedbackFieldName;
  label: string;
  kind: "text" | "textarea" | "select";
  options?: string[];
  /** Half a line on a wide modal, side by side with the next half. */
  half?: boolean;
}

export interface FeedbackTypeDef {
  value: FeedbackType;
  label: string;
  /** An Icon name. */
  icon: string;
  /** One line under the type buttons saying what this kind is for. */
  hint: string;
  fields: FeedbackFieldDef[];
}

export const SEVERITIES = ["Low", "Medium", "High", "Critical"] as const;

export const FEEDBACK_TYPES: FeedbackTypeDef[] = [
  {
    value: "Bug", label: "Bug", icon: "bug",
    hint: "Something is broken or does not work as it should.",
    fields: [
      { name: "severity", label: "Severity", kind: "select", options: [...SEVERITIES], half: true },
      { name: "steps_to_reproduce", label: "Steps to reproduce", kind: "textarea" },
      { name: "expected_result", label: "Expected result", kind: "textarea", half: true },
      { name: "actual_result", label: "Actual result", kind: "textarea", half: true },
    ],
  },
  {
    value: "Improvement", label: "Improvement", icon: "trending-up",
    hint: "Something works, but could work better.",
    fields: [
      { name: "current_behavior", label: "How it works today", kind: "textarea" },
      { name: "suggested_improvement", label: "Suggested improvement", kind: "textarea" },
    ],
  },
  {
    value: "Feature Request", label: "Feature Request", icon: "lightbulb",
    hint: "Something you need that does not exist yet.",
    fields: [
      { name: "problem", label: "Problem to solve", kind: "textarea" },
      { name: "expected_benefit", label: "Expected benefit", kind: "textarea" },
    ],
  },
];

export function feedbackTypeDef(type: string | undefined): FeedbackTypeDef {
  return FEEDBACK_TYPES.find((t) => t.value === type) ?? FEEDBACK_TYPES[0];
}

export function isFeedbackType(type: unknown): type is FeedbackType {
  return FEEDBACK_TYPES.some((t) => t.value === type);
}

/** Each status's indicator color (the desk's `.indicator.<color>` classes). */
export const FEEDBACK_STATUS_COLORS: Record<FeedbackStatus, string> = {
  New: "blue",
  "In Review": "orange",
  Planned: "purple",
  Done: "green",
  "Won't Do": "gray",
};

export function feedbackStatusColor(status: string): string {
  return FEEDBACK_STATUS_COLORS[status as FeedbackStatus] ?? "gray";
}

/** What the form holds: the type, title, description and every type's fields. */
export type FeedbackValues = { feedback_type: FeedbackType; title: string; description: string } & Partial<Record<FeedbackFieldName, string>>;

export function emptyFeedback(type: FeedbackType = "Bug", title = ""): FeedbackValues {
  return { feedback_type: type, title, description: "" };
}

/** The most files one feedback carries, the audio note included. */
export const MAX_FEEDBACK_FILES = 10;
/** The longest page address sent; the server refuses a longer one. */
export const MAX_URL = 2048;
/** The longest title the form accepts (the input's maxlength). */
export const MAX_TITLE = 140;

/** Field name → English error key; empty when the feedback can be sent. */
export function validateFeedback(values: Partial<FeedbackValues>): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!isFeedbackType(values.feedback_type)) errors.feedback_type = "Required";
  // the input's maxlength keeps the title within MAX_TITLE
  if (!String(values.title ?? "").trim()) errors.title = "Required";
  if (!String(values.description ?? "").trim()) errors.description = "Required";
  return errors;
}

export interface PayloadOptions {
  sendUrl: boolean;
  url?: string;
  sendContext: boolean;
  context?: Record<string, any>;
}

/**
 * The `data` part of POST /api/feedback: the type, title and description,
 * the chosen type's own fields (trimmed, the empty ones left out), and the
 * page address and context only when the person agreed to send them.
 */
export function feedbackPayload(values: FeedbackValues, opts: PayloadOptions): FeedbackData {
  const def = feedbackTypeDef(values.feedback_type);
  const data: FeedbackData = {
    feedback_type: def.value,
    title: String(values.title ?? "").trim(),
    description: String(values.description ?? "").trim(),
  };
  for (const f of def.fields) {
    const v = String(values[f.name] ?? "").trim();
    if (v) (data as any)[f.name] = v;
  }
  if (opts.sendUrl && opts.url) data.page_url = opts.url.slice(0, MAX_URL);
  if (opts.sendContext && opts.context) data.context = opts.context;
  return data;
}

export interface ContextInputs {
  boot: {
    user?: string;
    roles?: string[];
    lang?: string;
    apps?: { name: string; title?: string }[];
    site?: { name?: string; version?: string; tenant?: { id?: string; title?: string } | null };
  } | null | undefined;
  path?: string;
  params?: Record<string, string | undefined>;
  viewport?: { width: number; height: number };
  userAgent?: string;
  timezone?: string;
  errors?: ClientError[];
}

/**
 * What "Send context data" sends: who is reporting and where, the site and
 * its apps, the browser, and the last errors this tab saw. Everything comes
 * in through `inputs`, so it is the same object the preview shows.
 */
export function buildContext(inputs: ContextInputs): Record<string, any> {
  const b = inputs.boot;
  const ctx: Record<string, any> = {
    user: b?.user ?? "",
    roles: [...(b?.roles ?? [])],
    lang: b?.lang ?? "",
    site: { name: b?.site?.name ?? "", version: b?.site?.version ?? "" },
  };
  const tenant = b?.site?.tenant;
  if (tenant) ctx.tenant = { id: tenant.id ?? "", title: tenant.title ?? "" };
  ctx.apps = (b?.apps ?? []).map((a) => a.name);
  if (inputs.path) ctx.path = inputs.path;
  const p = inputs.params ?? {};
  for (const k of ["workspace", "doctype", "id"] as const) if (p[k]) ctx[k] = p[k];
  if (inputs.viewport) ctx.viewport = `${inputs.viewport.width}x${inputs.viewport.height}`;
  if (inputs.userAgent) ctx.userAgent = inputs.userAgent;
  if (inputs.timezone) ctx.timezone = inputs.timezone;
  ctx.errors = (inputs.errors ?? []).map((e) => ({ ...e }));
  return ctx;
}

/** Files added beyond the limit are dropped; `added` says how many were kept. */
export function addFiles(current: File[], incoming: File[], max = MAX_FEEDBACK_FILES): { files: File[]; added: number; dropped: number } {
  const room = Math.max(0, max - current.length);
  const kept = incoming.slice(0, room);
  return { files: [...current, ...kept], added: kept.length, dropped: incoming.length - kept.length };
}

/**
 * A pasted image arrives as `image.png` (or with no name at all), so each one
 * is named `screenshot-<n>.<ext>`, n counting the ones pasted before.
 */
export function screenshotName(n: number, type: string): string {
  const sub = (type.split("/")[1] || "png").split(/[;+]/)[0].toLowerCase();
  const ext = sub === "jpeg" ? "jpg" : sub || "png";
  return `screenshot-${n}.${ext}`;
}
