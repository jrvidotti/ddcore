import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import CalendarView from "./CalendarView.svelte";
import type { Meta } from "$lib/meta";

vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s, boot: { data: null } }));
const metas: Record<string, { fields: { fieldname: string; fieldtype: string }[]; create: boolean }> = {
  "Marketing Action": { fields: [{ fieldname: "start_date", fieldtype: "Date" }], create: true },
  "External Event": { fields: [{ fieldname: "starts_at", fieldtype: "Datetime" }], create: true },
  "Marketing Log": { fields: [{ fieldname: "log_date", fieldtype: "Date" }], create: false },
};
vi.mock("$lib/meta", async (importOriginal) => ({
  ...(await importOriginal<typeof import("$lib/meta")>()),
  getMeta: async (dt: string) => {
    const m = metas[dt];
    if (!m) throw new Error("forbidden");
    return { doctype: { name: dt, label: dt, fields: m.fields }, permissions: { create: m.create } };
  },
}));

const meta = {
  doctype: {
    name: "Campaign", app: "base", label: "Campaign", idGeneration: {}, titleField: "title",
    fields: [{ fieldname: "title", fieldtype: "Data" }, { fieldname: "start_date", fieldtype: "Date" }, { fieldname: "end_date", fieldtype: "Date" }],
  },
  permissions: {},
} as unknown as Meta;

describe("CalendarView spans", () => {
  it("draws a record over its days, one focusable labelled piece per week", () => {
    const target = document.createElement("div");
    const view = mount(CalendarView, { target, props: {
      rows: [{ id: "BF", title: "Black Friday 2026", start_date: "2026-11-20", end_date: "2026-11-23" }],
      meta, doctype: "Campaign", wsPrefix: "/app/Marketing", viewYear: 2026, viewMonth: 11, onMonthChange: () => {},
      calendar: { field: "start_date", endField: "end_date" },
    } });
    flushSync();
    const pieces = [...target.querySelectorAll<HTMLAnchorElement>("a.event")];
    expect(pieces).toHaveLength(4); // Fri, Sat | Sun, Mon
    expect(pieces.map((a) => a.textContent?.trim())).toEqual(["Black Friday 2026", "", "Black Friday 2026", ""]);
    expect(pieces.map((a) => a.className.match(/continues-\w+/g)?.join(" ") ?? "")).toEqual([
      "continues-after", "continues-before continues-after", "continues-before continues-after", "continues-before",
    ]);
    expect(pieces.filter((a) => a.getAttribute("tabindex") !== "-1")).toHaveLength(2);
    expect(pieces.every((a) => a.getAttribute("href") === "/app/Marketing/Campaign/BF")).toBe(true);
    unmount(view);
  });

  it("paints a record with the colour a Color field holds", () => {
    const target = document.createElement("div");
    const colored = { ...meta, doctype: { ...meta.doctype, fields: [...meta.doctype.fields, { fieldname: "color", fieldtype: "Color" }] } } as unknown as Meta;
    const view = mount(CalendarView, { target, props: {
      rows: [{ id: "BF", title: "Black Friday 2026", start_date: "2026-11-20", end_date: "2026-11-20", color: "#f97316" }],
      meta: colored, doctype: "Campaign", wsPrefix: "/app/Marketing", viewYear: 2026, viewMonth: 11, onMonthChange: () => {},
      calendar: { field: "start_date", endField: "end_date", colorField: "color" },
    } });
    flushSync();
    const piece = target.querySelector<HTMLAnchorElement>("a.event")!;
    expect(piece.getAttribute("style")).toContain("rgb(249, 115, 22)"); // #f97316, as the DOM spells it
    unmount(view);
  });
});

describe("CalendarView day +", () => {
  const settle = async () => { await new Promise((r) => setTimeout(r)); flushSync(); };
  const mountCalendar = (props: Record<string, any>) => {
    const target = document.createElement("div");
    document.body.append(target);
    const view = mount(CalendarView, { target, props: {
      rows: [], meta, doctype: "Campaign", wsPrefix: "/app/Marketing", viewYear: 2026, viewMonth: 11, onMonthChange: () => {},
      calendar: { field: "start_date" }, ...props,
    } });
    return { target, done: () => { unmount(view); target.remove(); } };
  };

  it("creates a record of the list's DocType when the user may", () => {
    const { target, done } = mountCalendar({ meta: { ...meta, permissions: { create: true } } });
    flushSync();
    const plus = target.querySelectorAll<HTMLAnchorElement>("a.day-new");
    expect(plus).toHaveLength(35);
    expect(plus[5].getAttribute("href")).toBe("/app/Marketing/Campaign/new?start_date=2026-11-06");
    done();
  });

  it("offers the newOptions the user may create, in a menu", async () => {
    const { target, done } = mountCalendar({ calendar: { field: "start_date", newOptions: [
      { label: "Action", doctype: "Marketing Action", field: "start_date" },
      { doctype: "External Event", field: "starts_at" },
      { label: "Diary", doctype: "Marketing Log", field: "log_date" }, // no create
      { label: "Hidden", doctype: "Secret", field: "day" }, // unreadable
    ] } });
    await settle();
    expect(target.querySelector("a.day-new")).toBeNull();
    const buttons = target.querySelectorAll<HTMLButtonElement>(".day-new button");
    expect(buttons).toHaveLength(35);
    buttons[5].click();
    flushSync();
    const links = [...target.querySelectorAll<HTMLAnchorElement>(".day-new .menu a")];
    expect(links.map((a) => [a.textContent, a.getAttribute("href")])).toEqual([
      ["Action", "/app/Marketing/MarketingAction/new?start_date=2026-11-06"],
      ["External Event", expect.stringMatching(/^\/app\/Marketing\/ExternalEvent\/new\?starts_at=2026-11-0[56]T/)],
    ]);
    document.body.click();
    flushSync();
    expect(target.querySelector(".day-new .menu")).toBeNull();
    done();
  });

  it("goes straight to a lone option, and hides the + when none remains", async () => {
    const lone = mountCalendar({ calendar: { field: "start_date", newOptions: [
      { label: "Action", doctype: "Marketing Action", field: "start_date" },
      { label: "Diary", doctype: "Marketing Log", field: "log_date" },
    ] } });
    await settle();
    expect(lone.target.querySelectorAll("a.day-new")[5].getAttribute("href")).toBe("/app/Marketing/MarketingAction/new?start_date=2026-11-06");
    lone.done();

    const none = mountCalendar({ meta: { ...meta, permissions: { create: true } }, calendar: { field: "start_date", newOptions: [
      { label: "Diary", doctype: "Marketing Log", field: "log_date" },
    ] } });
    await settle();
    expect(none.target.querySelector(".day-new")).toBeNull();
    none.done();
  });
});
