import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("./boot.svelte", () => ({ __: (s: string) => s }));

import { dialog, ui } from "./ui.svelte";

afterEach(() => {
  ui.dialogs.splice(0);
});

describe("dialog field properties", () => {
  it("changes a live field property after the dialog is open", () => {
    const d = dialog({
      title: "Generate",
      fields: [{ fieldname: "apply_late_fee", fieldtype: "Check", label: "Apply late fee" }],
    });
    d.show();

    d.setDfProperty("apply_late_fee", "hidden", true);

    expect(ui.dialogs[0].spec.fields?.[0].hidden).toBe(true);
  });
});
