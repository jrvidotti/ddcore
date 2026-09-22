import { render } from "svelte/server";
import { describe, expect, it, vi } from "vitest";
import Control from "./Control.svelte";
import type { Field } from "$lib/meta";

vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s, boot: { data: {} } }));
vi.mock("$lib/api", () => ({ api: { linkSearch: vi.fn() } }));
vi.mock("$app/state", () => ({ page: { params: {} } }));

describe("Control", () => {
  it("renders description for Check field when not inGrid", () => {
    const field: Field = {
      fieldname: "is_active",
      fieldtype: "Check",
      label: "Active",
      description: "Mark this person as active in the system",
    };
    const { body } = render(Control, { props: { field, value: true, onchange: vi.fn() } });
    expect(body).toContain('class="desc"');
    expect(body).toContain("Mark this person as active in the system");
  });

  it("does not render description for Check field when inGrid is true", () => {
    const field: Field = {
      fieldname: "is_active",
      fieldtype: "Check",
      label: "Active",
      description: "Mark this person as active in the system",
    };
    const { body } = render(Control, { props: { field, value: true, onchange: vi.fn(), inGrid: true } });
    expect(body).not.toContain('class="desc"');
    expect(body).not.toContain("Mark this person as active in the system");
  });

  it("renders error for Check field when error is passed", () => {
    const field: Field = {
      fieldname: "accept_terms",
      fieldtype: "Check",
      label: "Accept Terms",
      description: "You must accept the terms",
    };
    const { body } = render(Control, { props: { field, value: false, onchange: vi.fn(), error: "Required field" } });
    expect(body).toContain('class="err"');
    expect(body).toContain("Required field");
    // error takes precedence over description
    expect(body).not.toContain('class="desc"');
  });

  it("renders mandatory indicator for required Check field", () => {
    const field: Field = {
      fieldname: "agree",
      fieldtype: "Check",
      label: "I agree",
      reqd: true,
    };
    const { body } = render(Control, { props: { field, value: false, onchange: vi.fn(), mandatory: true } });
    expect(body).toContain('class="req"');
    expect(body).toContain("*");
  });

  it("applies bold class when field.bold is true", () => {
    const field: Field = {
      fieldname: "is_primary",
      fieldtype: "Check",
      label: "Primary",
      bold: true,
    };
    const { body } = render(Control, { props: { field, value: false, onchange: vi.fn() } });
    expect(body).toContain("bold");
  });

  it("renders field buttons alongside checkbox", () => {
    const field: Field = {
      fieldname: "verified",
      fieldtype: "Check",
      label: "Verified",
    };
    const onClick = vi.fn();
    const { body } = render(Control, {
      props: {
        field,
        value: true,
        onchange: vi.fn(),
        buttons: [{ label: "Verify Now", onClick }],
      },
    });
    expect(body).toContain("Verify Now");
    expect(body).toContain('class="btn field-btn"');
  });
});
