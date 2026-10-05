import { describe, expect, it } from "vitest";
import {
  FEEDBACK_TYPES, MAX_FEEDBACK_FILES, addFiles, buildContext, emptyFeedback, feedbackPayload,
  feedbackStatusColor, feedbackTypeDef, isFeedbackType, screenshotName, validateFeedback,
} from "./feedback";

describe("FEEDBACK_TYPES", () => {
  it("offers Bug, Improvement and Feature Request, each with its own fields", () => {
    expect(FEEDBACK_TYPES.map((t) => t.value)).toEqual(["Bug", "Improvement", "Feature Request"]);
    expect(feedbackTypeDef("Bug").fields.map((f) => f.name)).toEqual(["severity", "steps_to_reproduce", "expected_result", "actual_result"]);
    expect(feedbackTypeDef("Improvement").fields.map((f) => f.name)).toEqual(["current_behavior", "suggested_improvement"]);
    expect(feedbackTypeDef("Feature Request").fields.map((f) => f.name)).toEqual(["problem", "expected_benefit"]);
    expect(feedbackTypeDef("Bug").fields[0]).toMatchObject({ kind: "select", options: ["Low", "Medium", "High", "Critical"] });
  });

  it("falls back to Bug for an unknown type", () => {
    expect(feedbackTypeDef("Nope").value).toBe("Bug");
    expect(isFeedbackType("Nope")).toBe(false);
    expect(isFeedbackType("Feature Request")).toBe(true);
  });
});

describe("validateFeedback", () => {
  it("asks for a title and a description", () => {
    expect(validateFeedback(emptyFeedback())).toEqual({ title: "Required", description: "Required" });
    expect(validateFeedback({ feedback_type: "Bug", title: "  ", description: "x" })).toEqual({ title: "Required" });
  });

  it("passes a complete feedback", () => {
    expect(validateFeedback({ feedback_type: "Improvement", title: "Faster list", description: "It is slow" })).toEqual({});
  });

  it("refuses an unknown type", () => {
    expect(validateFeedback({ feedback_type: "Other" as any, title: "a", description: "b" })).toEqual({ feedback_type: "Required" });
  });
});

describe("feedbackPayload", () => {
  const values = {
    feedback_type: "Bug" as const,
    title: "  Save fails ",
    description: " Clicking save does nothing ",
    severity: "High",
    steps_to_reproduce: "1. open\n2. save ",
    expected_result: "",
    actual_result: "   ",
    // left over from switching types: not the Bug's fields
    current_behavior: "slow",
    problem: "none",
  };

  it("sends the chosen type's own fields, trimmed, without the empty ones", () => {
    expect(feedbackPayload(values, { sendUrl: false, sendContext: false })).toEqual({
      feedback_type: "Bug",
      title: "Save fails",
      description: "Clicking save does nothing",
      severity: "High",
      steps_to_reproduce: "1. open\n2. save",
    });
  });

  it("adds the page address and the context only when asked to", () => {
    const ctx = { user: "a@b.c" };
    const both = feedbackPayload(values, { sendUrl: true, url: "http://x/app/crm", sendContext: true, context: ctx });
    expect(both.page_url).toBe("http://x/app/crm");
    expect(both.context).toEqual(ctx);
    const none = feedbackPayload(values, { sendUrl: false, url: "http://x/app/crm", sendContext: false, context: ctx });
    expect("page_url" in none).toBe(false);
    expect("context" in none).toBe(false);
  });

  it("switching the type switches the fields sent", () => {
    const p = feedbackPayload({ ...values, feedback_type: "Improvement", suggested_improvement: " cache it " }, { sendUrl: false, sendContext: false });
    expect(p).toEqual({ feedback_type: "Improvement", title: "Save fails", description: "Clicking save does nothing", current_behavior: "slow", suggested_improvement: "cache it" });
  });
});

describe("buildContext", () => {
  const boot = {
    user: "ana@example.com",
    roles: ["System Manager", "All"],
    lang: "pt-BR",
    apps: [{ name: "core", title: "Core" }, { name: "crm", title: "CRM" }],
    site: { name: "Acme", version: "0.27.0", tenant: { id: "acme", title: "Acme Ltd", platform: false } },
  };

  it("collects who, where, the site, the browser and the recent errors", () => {
    const ctx = buildContext({
      boot,
      path: "/app/crm/lead/L-1",
      params: { workspace: "crm", doctype: "lead", id: "L-1" },
      viewport: { width: 390, height: 844 },
      userAgent: "UA",
      timezone: "America/Sao_Paulo",
      errors: [{ at: "2026-01-01T00:00:00.000Z", kind: "request", message: "boom", status: 500, requestId: "r1" }],
    });
    expect(ctx).toEqual({
      user: "ana@example.com",
      roles: ["System Manager", "All"],
      lang: "pt-BR",
      site: { name: "Acme", version: "0.27.0" },
      tenant: { id: "acme", title: "Acme Ltd" },
      apps: ["core", "crm"],
      path: "/app/crm/lead/L-1",
      workspace: "crm",
      doctype: "lead",
      id: "L-1",
      viewport: "390x844",
      userAgent: "UA",
      timezone: "America/Sao_Paulo",
      errors: [{ at: "2026-01-01T00:00:00.000Z", kind: "request", message: "boom", status: 500, requestId: "r1" }],
    });
  });

  it("leaves out what is not known", () => {
    const ctx = buildContext({ boot: { ...boot, site: { name: "Acme", version: "1" } } });
    expect(ctx).toEqual({ user: "ana@example.com", roles: ["System Manager", "All"], lang: "pt-BR", site: { name: "Acme", version: "1" }, apps: ["core", "crm"], errors: [] });
  });

  it("survives a missing boot", () => {
    expect(buildContext({ boot: null })).toEqual({ user: "", roles: [], lang: "", site: { name: "", version: "" }, apps: [], errors: [] });
  });
});

describe("files", () => {
  const f = (n: string) => new File(["x"], n);

  it("keeps at most the limit", () => {
    const current = Array.from({ length: MAX_FEEDBACK_FILES - 2 }, (_, i) => f(`a${i}`));
    const r = addFiles(current, [f("b"), f("c"), f("d")]);
    expect(r.files).toHaveLength(MAX_FEEDBACK_FILES);
    expect(r.added).toBe(2);
    expect(r.dropped).toBe(1);
    expect(addFiles([], [f("x")], 0)).toEqual({ files: [], added: 0, dropped: 1 });
  });

  it("names pasted screenshots in order", () => {
    expect(screenshotName(1, "image/png")).toBe("screenshot-1.png");
    expect(screenshotName(2, "image/jpeg")).toBe("screenshot-2.jpg");
    expect(screenshotName(3, "image/svg+xml")).toBe("screenshot-3.svg");
    expect(screenshotName(4, "")).toBe("screenshot-4.png");
  });
});

describe("feedbackStatusColor", () => {
  it("paints each status", () => {
    expect(["New", "In Review", "Planned", "Done", "Won't Do", "?"].map(feedbackStatusColor)).toEqual(["blue", "orange", "purple", "green", "gray", "gray"]);
  });
});
