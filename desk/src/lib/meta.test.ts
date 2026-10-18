import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { boot } from "./boot.svelte";
import { newDoc, type Field, type Meta } from "./meta";

vi.mock("./api", () => ({ api: {} }));

/** A Meta whose DocType has one field per default under test. */
const metaWith = (...fields: Partial<Field>[]) =>
  ({ doctype: { name: "Event", fields: fields.map((f, i) => ({ fieldname: `f${i}`, ...f })) } }) as unknown as Meta;

/** The site's timezone, the way a boot would set it. */
const siteZone = (timezone: string) => {
  boot.data = { user: "ana@x.com", site: { timezone } } as any;
};

// 23:30 UTC on the last day of March: already April in Kiritimati (UTC+14),
// still the 31st in Los Angeles (UTC-7) and in UTC.
const INSTANT = "2026-03-31T23:30:00Z";

describe("newDoc defaults", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(INSTANT));
  });
  afterEach(() => {
    vi.useRealTimers();
    boot.data = null;
  });

  it("resolves Today in the site's timezone, not the browser's", () => {
    const meta = metaWith({ fieldtype: "Date", default: "Today" });

    siteZone("Pacific/Kiritimati");
    expect(newDoc(meta).f0).toBe("2026-04-01");
    siteZone("America/Los_Angeles");
    expect(newDoc(meta).f0).toBe("2026-03-31");
    siteZone("UTC");
    expect(newDoc(meta).f0).toBe("2026-03-31");
  });

  it("starts a Month field, or This Month, on the site's month", () => {
    siteZone("Pacific/Kiritimati");
    const meta = metaWith(
      { fieldtype: "Month", default: "Today" },
      { fieldtype: "Date", default: "This Month" },
    );
    const doc = newDoc(meta);
    expect(doc.f0).toBe("2026-04-01");
    expect(doc.f1).toBe("2026-04-01");

    siteZone("America/Los_Angeles");
    expect(newDoc(meta).f0).toBe("2026-03-01");
  });

  it("accepts the keywords the server does: today and now are today for a Date", () => {
    siteZone("Pacific/Kiritimati");
    const doc = newDoc(metaWith({ fieldtype: "Date", default: "today" }, { fieldtype: "Date", default: "now" }));
    expect(doc.f0).toBe("2026-04-01");
    expect(doc.f1).toBe("2026-04-01");
  });

  it("gives a Datetime the current instant", () => {
    siteZone("Pacific/Kiritimati");
    const doc = newDoc(metaWith(
      { fieldtype: "Datetime", default: "Today" },
      { fieldtype: "Datetime", default: "today" },
      { fieldtype: "Datetime", default: "now" },
    ));
    for (const key of ["f0", "f1", "f2"]) expect(doc[key]).toBe("2026-03-31T23:30:00.000Z");
  });

  it("gives __user the signed-in user", () => {
    siteZone("UTC");
    expect(newDoc(metaWith({ fieldtype: "Link", options: "User", default: "__user" })).f0).toBe("ana@x.com");
  });

  it("keeps literals, empties and tables as they were", () => {
    siteZone("UTC");
    const doc = newDoc(metaWith(
      { fieldtype: "Select", default: "Open" },
      { fieldtype: "Check" },
      { fieldtype: "Data" },
      { fieldtype: "Table", options: "Row" },
      { fieldtype: "Section Break" },
    ));
    expect(doc).toMatchObject({ doctype: "Event", docstatus: 0, __islocal: true, f0: "Open", f1: false, f2: null, f3: [] });
    expect("f4" in doc).toBe(false);
  });
});
