import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import CalendarView from "./CalendarView.svelte";
import type { Meta } from "$lib/meta";

vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s, boot: { data: null } }));

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
