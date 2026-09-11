import { describe, it, expect } from "vitest";
import { describeDevice, isExpired, languageField, passwordProblem, sortSessions } from "./profile";

const t = (s: string, args?: any[]) =>
  args ? args.reduce<string>((acc, v, i) => acc.replaceAll(`{${i}}`, String(v)), s) : s;

describe("passwordProblem", () => {
  it("asks for a password before anything else", () => {
    expect(passwordProblem("", "", 8, t)).toBe("Choose a password");
  });

  it("enforces server minimum length and states what it is", () => {
    expect(passwordProblem("curta", "curta", 8, t)).toBe("The password must have at least 8 characters");
    expect(passwordProblem("curta", "curta", 4, t)).toBe("");
  });

  it("only complains about confirmation after the password itself is valid", () => {
    // otherwise the user fixes the confirmation only to discover the length requirement then
    expect(passwordProblem("abc", "outra", 8, t)).toBe("The password must have at least 8 characters");
    expect(passwordProblem("senhaboa1", "outra", 8, t)).toBe("The two passwords do not match");
  });

  it("accepts a valid pair", () => {
    expect(passwordProblem("senhaboa1", "senhaboa1", 8, t)).toBe("");
  });
});

describe("describeDevice", () => {
  it("recognizes enough for the user to recognize their device", () => {
    const chrome = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36";
    expect(describeDevice(chrome, "?")).toBe("Chrome · macOS");

    const iphone = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1";
    expect(describeDevice(iphone, "?")).toBe("Safari · iOS");

    const edge = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36 Edg/120.0";
    expect(describeDevice(edge, "?")).toBe("Edge · Windows");
  });

  it("does not guess when unknown", () => {
    expect(describeDevice(null, "Unknown device")).toBe("Unknown device");
    expect(describeDevice("", "Unknown device")).toBe("Unknown device");
    expect(describeDevice("curl/8.4.0", "Unknown device")).toBe("Unknown device");
  });
});

describe("sortSessions", () => {
  it("puts current session first, then sorts rest by most recent access", () => {
    const rows = [
      { id: "a", current: false, lastSeen: "2026-09-01T10:00:00Z" },
      { id: "b", current: false, lastSeen: "2026-09-10T10:00:00Z" },
      { id: "c", current: true, lastSeen: "2026-08-01T10:00:00Z" },
    ];
    expect(sortSessions(rows).map((r) => r.id)).toEqual(["c", "b", "a"]);
  });

  it("does not mutate the input array", () => {
    const rows = [
      { id: "a", current: false, lastSeen: "2026-09-01T10:00:00Z" },
      { id: "c", current: true, lastSeen: "2026-08-01T10:00:00Z" },
    ];
    sortSessions(rows);
    expect(rows.map((r) => r.id)).toEqual(["a", "c"]);
  });
});

describe("isExpired", () => {
  const now = new Date("2026-09-11T12:00:00Z");

  it("a key without expiration is never expired", () => {
    expect(isExpired(null, now)).toBe(false);
    expect(isExpired(undefined, now)).toBe(false);
    expect(isExpired("", now)).toBe(false);
  });

  it("compares against the given instant", () => {
    expect(isExpired("2026-09-10T12:00:00Z", now)).toBe(true);
    expect(isExpired("2026-09-12T12:00:00Z", now)).toBe(false);
  });

  it("an unparseable date does not become 'expired'", () => {
    // marking a working key as expired confuses more than it helps
    expect(isExpired("nem data é", now)).toBe(false);
  });
});

describe("languageField", () => {
  const langs = [{ code: "en", label: "English" }, { code: "pt-BR", label: "Português" }];

  it("is a Select over the site's languages, labelled with their autonyms", () => {
    const f = languageField(langs, t);
    expect(f.fieldtype).toBe("Select");
    expect(f.fieldname).toBe("language");
    expect(f.options).toEqual(["en", "pt-BR"]);
    expect(f.optionLabels).toEqual(["English", "Português"]);
  });

  it("leaves the blank entry to the control: empty means 'follow the site'", () => {
    expect(languageField(langs, t).options).not.toContain("");
  });

  it("translates its own caption", () => {
    const shout = (s: string) => s.toUpperCase();
    const f = languageField(langs, shout);
    expect(f.label).toBe("LANGUAGE");
    expect(f.description).toBe("LEAVE BLANK TO FOLLOW THE SITE LANGUAGE.");
  });

  it("survives a site that reported no languages", () => {
    expect(languageField([], t).options).toEqual([]);
  });
});
