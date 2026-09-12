<script lang="ts">
  import { ui, type DialogHandle } from "$lib/ui.svelte";
  import Control from "$lib/controls/Control.svelte";
  import Icon from "./Icon.svelte";
  import { __ } from "$lib/boot.svelte";
  import { showError } from "$lib/ui.svelte";
  import { cellWidthClass, formRows, LINE_SLOTS } from "./form-layout";
  import { runDialogAction } from "./dialog-actions";

  function cancel(d: DialogHandle) { (d as any).onCancel?.(); d.hide(); }
  async function primary(d: DialogHandle) {
    if (!d.spec.primaryAction) return d.hide();
    // client-side reqd check
    for (const f of d.spec.fields || []) {
      if (f.reqd && f.fieldname && (d.values[f.fieldname] === null || d.values[f.fieldname] === undefined || d.values[f.fieldname] === "")) {
        showError({ title: __("Required fields"), message: __("Fill in {0}", [f.label || f.fieldname]) });
        return;
      }
    }
    try { await runDialogAction(d.spec.primaryAction, d.values, d); } catch (e) { showError(e); }
  }
  async function danger(d: DialogHandle) {
    if (!d.spec.dangerAction) return;
    try { await runDialogAction(d.spec.dangerAction, d.values, d); } catch (e) { showError(e); }
  }
  // sections/columns inside dialogs: split fields into columns on Column Break
  function layout(fields: any[]) {
    const sections: any[][][] = [[[]]];
    for (const f of fields) {
      if (f.fieldtype === "Section Break") sections.push([[]]);
      else if (f.fieldtype === "Column Break") sections[sections.length - 1].push([]);
      else { const s = sections[sections.length - 1]; s[s.length - 1].push(f); }
    }
    return sections;
  }

  /** A modal is too narrow for quarter-line cells, so a dialog column always owns two slots. */
  const DIALOG_SLOTS = LINE_SLOTS / 2;
</script>

{#each ui.dialogs as d (d.id)}
  <div class="modal-bg" role="dialog" aria-modal="true" onkeydown={(e) => e.key === "Escape" && cancel(d)} tabindex="-1">
    <div class="modal {d.spec.size || 'md'}">
      <div class="head"><h3>{d.spec.title}</h3><button class="btn icon" onclick={() => cancel(d)} aria-label="Fechar"><Icon name="x" /></button></div>
      <div class="body">
        {#if d.spec.message}<p style="margin:0 0 12px">{@html d.spec.message}</p>{/if}
        {#each layout(d.spec.fields || []) as cols}
          {#each formRows(cols, undefined, DIALOG_SLOTS) as row}
            <div class="form-columns form-row" style="--cols:{cols.length}">
              {#each row as cells}
                <div class="form-column">
                  {#each cells as cell, i (cell.field?.fieldname ?? i)}
                    {@const f = cell.field}
                    <div class="form-cell {cellWidthClass(cell.slots, DIALOG_SLOTS)}">
                      {#if !f}
                        <!-- alignment spacer -->
                      {:else if f.fieldtype === "HTML"}
                        <div class="field">{@html (f.fieldname ? d.html[f.fieldname] : "") || f.options || ""}</div>
                      {:else if f.fieldname}
                        <Control field={f} value={d.values[f.fieldname]} onchange={(v) => d.setValue(f.fieldname!, v)} onbusychange={(busy) => (d.busy = busy)} doc={d.values} />
                      {/if}
                    </div>
                  {/each}
                </div>
              {/each}
            </div>
          {/each}
        {/each}
      </div>
      <div class="foot">
        {#if d.spec.dangerAction}<button class="btn danger" disabled={d.busy} onclick={() => danger(d)}>{d.spec.dangerLabel || __("Delete")}</button><span class="spacer"></span>{/if}
        <button class="btn" onclick={() => cancel(d)}>{d.spec.secondaryLabel || __("Cancel")}</button>
        {#if d.spec.primaryAction || d.spec.primaryLabel}
          <button class="btn primary" disabled={d.busy} onclick={() => primary(d)}>{d.spec.primaryLabel || "OK"}</button>
        {/if}
      </div>
    </div>
  </div>
{/each}
