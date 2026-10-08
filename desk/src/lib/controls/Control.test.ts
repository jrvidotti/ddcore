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

  it("renders an Autocomplete as a combobox, not a select", () => {
    const field: Field = { fieldname: "tag", fieldtype: "Autocomplete", label: "Tag", options: ["Red", "Blue"] };
    const { body } = render(Control, { props: { field, value: "Green", onchange: vi.fn() } });
    expect(body).toContain('role="combobox"');
    expect(body).toContain('data-fieldtype="Autocomplete"');
    expect(body).not.toContain("<select");
  });

  it("renders a Barcode as a text box with its own control", () => {
    const field: Field = { fieldname: "gtin", fieldtype: "Barcode", label: "GTIN", options: "EAN-13" };
    const { body } = render(Control, { props: { field, value: "4006381333931", onchange: vi.fn() } });
    expect(body).toContain('data-fieldtype="Barcode"');
    expect(body).toContain('class="barcode"');
  });

  it("renders a Signature as its image, or as a pad when there is none", () => {
    const field: Field = { fieldname: "signed", fieldtype: "Signature", label: "Signed by" };
    const png = "data:image/png;base64,iVBORw0KGgo=";
    const signed = render(Control, { props: { field, value: png, onchange: vi.fn() } }).body;
    expect(signed).toContain('data-fieldtype="Signature"');
    expect(signed).toContain(`src="${png}"`);
    expect(signed).not.toContain("<canvas");
    const empty = render(Control, { props: { field, value: null, onchange: vi.fn() } }).body;
    expect(empty).toContain("<canvas");
    expect(empty).not.toContain("<input");
  });

  it("renders a Geolocation as a map with its tools and summary, and no tools when read-only", () => {
    const field: Field = { fieldname: "place", fieldtype: "Geolocation", label: "Place" };
    const value = { type: "FeatureCollection", features: [{ type: "Feature", geometry: { type: "Point", coordinates: [-46.6333, -23.5505] }, properties: {} }] };
    const body = render(Control, { props: { field, value, onchange: vi.fn() } }).body;
    expect(body).toContain('data-fieldtype="Geolocation"');
    expect(body).toContain('class="geo-map');
    for (const mode of ["point", "line", "polygon", "delete"]) expect(body).toContain(`data-mode="${mode}"`);
    expect(body).toContain("-23.55050, -46.63330");
    expect(body).not.toContain("<input");
    const ro = render(Control, { props: { field: { ...field, readOnly: true }, value, onchange: vi.fn() } }).body;
    expect(ro).toContain('class="geo-map');
    expect(ro).not.toContain("data-mode=");
  });

  it("renders a file picker, not a text input, for a dialog's File field", () => {
    const field: Field = { fieldname: "file", fieldtype: "File", label: "Certificate", options: ".pfx,.p12", reqd: true };
    const { body } = render(Control, { props: { field, value: null, onchange: vi.fn() } });
    expect(body).toContain('type="file"');
    expect(body).toContain('accept=".pfx,.p12"');
    expect(body).toContain("Choose file");
    expect(body).not.toContain('type="text"');
  });

  describe("a Select with optionColors/optionIcons", () => {
    const field: Field = {
      fieldname: "status", fieldtype: "Select", label: "Status",
      options: ["Not registered", "Enabled"],
      optionColors: { "Not registered": "gray", Enabled: "green" },
      optionIcons: { "Not registered": "x", Enabled: "check" },
      optionIconOnly: true,
    };

    it("draws its indicator, with the label, when read-only", () => {
      const { body } = render(Control, { props: { field, value: "Enabled", onchange: vi.fn(), readOnly: true } });
      expect(body).toContain("indicator select-indicator green");
      expect(body).toContain("has-icon");
      // iconOnly is for table cells: a field keeps its label
      expect(body).not.toContain("icon-only");
      expect(body).toContain('<span class="indicator-label">Enabled</span>');
      expect(body).not.toContain("<select");
    });

    it("stays a select when editable or empty", () => {
      expect(render(Control, { props: { field, value: "Enabled", onchange: vi.fn() } }).body).toContain("<select");
      expect(render(Control, { props: { field, value: null, onchange: vi.fn(), readOnly: true } }).body).toContain("<select");
    });

    it("stays a select without optionColors or optionIcons", () => {
      const plain: Field = { fieldname: "kind", fieldtype: "Select", label: "Kind", options: ["A", "B"] };
      expect(render(Control, { props: { field: plain, value: "A", onchange: vi.fn(), readOnly: true } }).body).toContain("<select");
    });
  });
});
