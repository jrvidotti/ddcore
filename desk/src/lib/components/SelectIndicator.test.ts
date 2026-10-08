import { render } from "svelte/server";
import { describe, expect, it, vi } from "vitest";
import SelectIndicator from "./SelectIndicator.svelte";

vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s, boot: { data: {} } }));

const field = {
  fieldtype: "Select", options: ["Not registered", "Enabled", "Disabled"],
  optionColors: { "Not registered": "gray", Enabled: "green", Disabled: "red" },
  optionIcons: { "Not registered": "x", Enabled: "check" },
};

describe("SelectIndicator", () => {
  it("draws the icon in place of the dot, then the label", () => {
    const { body } = render(SelectIndicator, { props: { value: "Enabled", field } });
    expect(body).toContain("indicator select-indicator green has-icon");
    expect(body).toContain("<svg");
    expect(body).toContain('<span class="indicator-label">Enabled</span>');
  });

  it("keeps the dot for a value without an icon", () => {
    const { body } = render(SelectIndicator, { props: { value: "Disabled", field } });
    expect(body).not.toContain("has-icon");
    expect(body).not.toContain("<svg");
  });

  it("icon-only moves the label into the tooltip and the accessible name", () => {
    const { body } = render(SelectIndicator, { props: { value: "Not registered", field, iconOnly: true } });
    expect(body).toContain("icon-only");
    expect(body).toContain('title="Not registered"');
    expect(body).toContain('aria-label="Not registered"');
    expect(body).not.toContain("indicator-label");
  });

  it("icon-only keeps the label for a value without an icon", () => {
    const { body } = render(SelectIndicator, { props: { value: "Disabled", field, iconOnly: true } });
    expect(body).not.toContain("icon-only");
    expect(body).toContain('<span class="indicator-label">Disabled</span>');
  });

  it("draws nothing for an empty value", () => {
    expect(render(SelectIndicator, { props: { value: "", field } }).body).not.toContain("indicator");
  });
});
