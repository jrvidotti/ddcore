import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("./boot.svelte", () => ({ __: (s: string) => s }));

import { confirm, dialog, escapeHtml, ui } from "./ui.svelte";

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

describe("confirm", () => {
  it("makes Yes the primary action of an ordinary question", async () => {
    const answer = confirm("Send it again?");
    const d = ui.dialogs[0];
    expect(d.spec.primaryLabel).toBe("Yes");
    expect(d.spec.dangerAction).toBeUndefined();
    d.spec.primaryAction!({}, d);
    expect(await answer).toBe(true);
  });

  it("makes No the primary action when the answer destroys something", async () => {
    const answer = confirm("Discard your unsaved changes?", "Discard changes", { destructive: true });
    const d = ui.dialogs[0];
    expect(d.spec.primaryLabel).toBe("No");
    expect(d.spec.dangerLabel).toBe("Yes");
    expect(d.spec.hideSecondary).toBe(true);
    d.spec.primaryAction!({}, d);
    expect(await answer).toBe(false);
    expect(ui.dialogs).toHaveLength(0);
  });

  it("answers yes to a destructive question only through the danger button", async () => {
    const answer = confirm("Delete TASK-1?", "Delete", { destructive: true });
    const d = ui.dialogs[0];
    d.spec.dangerAction!({}, d);
    expect(await answer).toBe(true);
  });
});

describe("escapeHtml", () => {
  it("keeps a typed title from becoming markup in a dialog message", () => {
    expect(escapeHtml('<img src=x onerror="alert(1)"> & \'x\''))
      .toBe("&lt;img src=x onerror=&quot;alert(1)&quot;&gt; &amp; &#39;x&#39;");
    expect(escapeHtml(null)).toBe("");
  });
});
