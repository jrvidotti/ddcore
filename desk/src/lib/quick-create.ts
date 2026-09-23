// Quick create from a Link field: a dialog with the target's required fields,
// which inserts the document and hands its id back to the field.
import { api } from "./api";
import { __, boot } from "./boot.svelte";
import { getMeta, isLayout, type DocTypeMeta, type Field } from "./meta";
import { dialog, escapeHtml } from "./ui.svelte";
import { getRememberedWorkspace } from "./components/sidebar-workspace";

/**
 * The fields a quick-create dialog asks for: the prompted id, the title field and
 * every required field a person fills in. `null` when a required field cannot be
 * filled in a dialog (a child table), so only the full form can create the document.
 */
export function quickEntryFields(d: DocTypeMeta): Field[] | null {
  const out: Field[] = [];
  for (const f of d.fields) {
    if (!f.fieldname || isLayout(f)) continue;
    if (f.fieldtype === "Table") {
      if (f.reqd) return null;
      continue;
    }
    if (f.hidden || f.readOnly || f.fetchFrom) continue;
    if (f.reqd || f.fieldname === "id" || f.fieldname === d.titleField) out.push(f);
  }
  return out;
}

/** The field the text typed in the Link goes into: the title, else the first text field asked for. */
export function prefillField(d: DocTypeMeta, fields: Field[]): string | undefined {
  if (d.titleField && fields.some((f) => f.fieldname === d.titleField)) return d.titleField;
  return fields.find((f) => f.fieldtype === "Data")?.fieldname;
}

function newFormUrl(doctype: string, prefill?: Record<string, string>): string {
  const ws = getRememberedWorkspace();
  const qs = prefill && Object.keys(prefill).length ? "?" + new URLSearchParams(prefill).toString() : "";
  return `${ws ? `/app/${encodeURIComponent(ws)}` : "/app"}/${encodeURIComponent(doctype)}/new${qs}`;
}

/**
 * Asks for the new document in a dialog and inserts it. Resolves to the new
 * document's id and title, or null when cancelled or sent to the full form.
 */
export async function quickCreate(doctype: string, text = ""): Promise<{ id: string; title: string } | null> {
  const meta = await getMeta(doctype);
  const d = meta.doctype;
  const label = boot.data?.doctypes[doctype]?.label || d.label || doctype;
  const fields = quickEntryFields(d);
  const target = fields ? prefillField(d, fields) : d.titleField;
  const prefill = target && text ? { [target]: text } : undefined;
  if (!fields || !fields.length) {
    window.open(newFormUrl(doctype, prefill), "_blank");
    return null;
  }
  const fullForm = __("Open the full form");
  return new Promise((resolve) => {
    const h = dialog({
      title: __("New {0}", [label]),
      fields: fields.map((f) => ({ ...f })),
      values: prefill,
      primaryLabel: __("Create"),
      message: `<a href="${escapeHtml(newFormUrl(doctype, prefill))}" target="_blank" rel="noopener">${escapeHtml(fullForm)}</a>`,
      primaryAction: async (values) => {
        const saved: any = await api.insert(doctype, { ...values, doctype });
        resolve({ id: saved.id, title: String((d.titleField && saved[d.titleField]) || saved.id) });
        h.hide();
      },
    });
    (h as any).onCancel = () => resolve(null);
    h.show();
  });
}
