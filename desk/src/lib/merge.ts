// Merging a duplicate into the document that stays (#129): who sees the menu
// item, and the preview the dialog shows before anything moves.
import { escapeHtml } from "./ui.svelte";

export interface MergeCount { doctype: string; label: string; fieldname?: string; fieldLabel?: string; count: number }
export interface MergePreview { from: string; into: string; links: MergeCount[]; records: MergeCount[]; blockers: string[] }

/**
 * A merge deletes the document on screen, so it takes what the server asks
 * for: allowRename, write, and delete. The target's write is the server's to
 * check once one is chosen.
 */
export function canMerge(f: { isNew: boolean; allowRename?: boolean; perm: { write?: boolean; delete?: boolean }; docstatus: number }): boolean {
  return !f.isNew && !!f.allowRename && !!f.perm.write && !!f.perm.delete && f.docstatus !== 1;
}

/** The preview as dialog HTML: what moves to the target, then why it cannot. */
export function mergePreviewHtml(p: MergePreview, __: (s: string, a?: any[]) => string): string {
  const rows = (items: MergeCount[]) => items.map((c) =>
    `<tr><td>${escapeHtml(c.label)}${c.fieldLabel ? ` <span class="muted">· ${escapeHtml(c.fieldLabel)}</span>` : ""}</td>` +
    `<td style="text-align:right">${c.count}</td></tr>`).join("");
  let html = "";
  if (p.blockers.length) {
    html += `<p style="color:var(--red);margin:0 0 8px">${p.blockers.map(escapeHtml).join("<br>")}</p>`;
  }
  const moves = [...p.links, ...p.records];
  if (moves.length) {
    html += `<p style="margin:0 0 4px">${__("Moves to {0}:", [`<b>${escapeHtml(p.into)}</b>`])}</p>` +
      `<table style="width:100%">${rows(moves)}</table>`;
  } else {
    html += `<p class="muted" style="margin:0">${__("Nothing links to {0}.", [`<b>${escapeHtml(p.from)}</b>`])}</p>`;
  }
  html += `<p class="muted" style="margin:8px 0 0">${__("{0} is then deleted. This cannot be undone.", [`<b>${escapeHtml(p.from)}</b>`])}</p>`;
  return html;
}
