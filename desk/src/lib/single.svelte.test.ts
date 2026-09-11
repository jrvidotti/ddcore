import { describe, expect, it, vi } from "vitest";
import { FormController, createForm } from "./form.svelte";
import { api } from "./api";
import { getMeta, type Meta } from "./meta";

vi.mock("./api", () => ({ api: { getSingle: vi.fn(), update: vi.fn(), insert: vi.fn() } }));
vi.mock("./meta", async (original) => ({ ...(await original<any>()), getMeta: vi.fn() }));
vi.mock("./ui.svelte", () => ({ toast: vi.fn(), showError: vi.fn(), ui: { busy: 0 } }));
vi.mock("./boot.svelte", () => ({ __: (s: string) => s }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));
const meta = (write: boolean) => ({ doctype: { name: "Settings", label: "Settings", isSingle: true, fields: [{fieldname:"enabled", fieldtype:"Check"}], formApps: [] }, permissions: { read: true, write, create: true } }) as unknown as Meta;

describe("Single forms", () => {
 it("loads defaults from the server even on the new route", async () => {
  vi.mocked(getMeta).mockResolvedValue(meta(true));
  vi.mocked(api.getSingle).mockResolvedValue({doctype:"Settings",name:"singleton",__islocal:true,enabled:true});
  const frm = await createForm("Settings", "new", {enabled:false});
  expect(frm.doc.enabled).toBe(true);
 });
 it("keeps a reader's unsaved settings read-only even with create", () => {
  const frm = new FormController(meta(false), {name:"singleton",__islocal:true});
  expect(frm.readOnly).toBe(true);
  expect(frm.isFieldEditable(frm.meta.doctype.fields[0])).toBe(false);
 });
 it("uses PUT for the first save", async () => {
  const frm = new FormController(meta(true), {doctype:"Settings",name:"singleton",__islocal:true,enabled:false});
  vi.mocked(api.update).mockResolvedValue({doctype:"Settings",name:"singleton",enabled:false});
  expect(await frm.save()).toBe(true);
  expect(api.update).toHaveBeenCalledWith("Settings", "singleton", expect.objectContaining({enabled:false}));
  expect(frm.isNew).toBe(false);
 });
});
