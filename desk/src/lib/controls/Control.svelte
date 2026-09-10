<script lang="ts">
  import { currencyBefore, currencySymbol } from "$lib/locale";
  // One control per fieldtype. `value`/`onchange` make it usable in forms,
  // dialogs, grids and filters alike.
  import type { Field } from "$lib/meta";
  import { selectLabels, selectOptions } from "$lib/meta";
  import { formatNumber, parseNumber } from "$lib/format";
  import LinkControl from "./LinkControl.svelte";
  import AttachControl from "./AttachControl.svelte";
  import MonthControl from "./MonthControl.svelte";
  import DateControl from "./DateControl.svelte";
  import { __ } from "$lib/boot.svelte";
  import { isSystemUserEmail, normalizeEmail, validEmail } from "$lib/email";
  import { toDatetimeLocal, fromDatetimeLocal } from "$lib/datetime";

  let {
    field, value, onchange, onbusychange = undefined, doc = {}, readOnly = false, mandatory = false, error = "", compact = false, query = undefined, inGrid = false,
  }: {
    field: Field; value: any; onchange: (v: any) => void; doc?: any; readOnly?: boolean; mandatory?: boolean; error?: string; compact?: boolean;
    query?: () => { filters?: any }; inGrid?: boolean; onbusychange?: (busy: boolean) => void;
  } = $props();

  const id = `f-${Math.random().toString(36).slice(2, 8)}`;
  const ft = $derived(field.fieldtype);
  const ro = $derived(readOnly || !!field.readOnly);
  const req = $derived(mandatory || !!field.reqd);
  let emailError = $state("");
  const shownError = $derived(error || emailError);

  // number inputs keep a local text buffer so "1.234,5" can be typed freely
  let text = $state("");
  let focused = $state(false);
  $effect(() => {
    if (!focused) text = value === null || value === undefined ? "" : ft === "Int" ? String(Math.round(Number(value))) : formatNumber(value, ft === "Currency" ? 2 : field.precision);
  });
  function commitNumber() {
    focused = false;
    const n = parseNumber(text);
    onchange(n === null ? null : ft === "Int" ? Math.round(n) : n);
  }
  function commitEmail(rawValue: string) {
    const normalized = normalizeEmail(rawValue);
    onchange(normalized || null);
    emailError = normalized && !validEmail(normalized) && !isSystemUserEmail(doc?.doctype, field.fieldname || "", normalized) ? __("Invalid email") : "";
  }
</script>

{#if ft === "Check"}
  <div class="field check" class:compact>
    <input {id} type="checkbox" checked={!!value} disabled={ro} onchange={(e) => onchange((e.target as HTMLInputElement).checked)} />
    {#if !inGrid}<label for={id}>{field.label}</label>{/if}
  </div>
{:else}
  <div class="field" class:compact class:bold={field.bold}>
    {#if !inGrid && field.label}
      <label for={id}>{field.label}{#if req && !ro}<span class="req">*</span>{/if}</label>
    {/if}
    {#if ft === "Select"}
      <select {id} class="input" class:error={!!shownError} disabled={ro} value={value ?? ""} onchange={(e) => onchange((e.target as HTMLSelectElement).value || null)}>
        {#if !selectOptions(field).includes("")}<option value=""></option>{/if}
        {#each selectOptions(field) as o, i}<option value={o}>{selectLabels(field)[i] ?? o}</option>{/each}
      </select>
    {:else if ft === "Link" || ft === "Dynamic Link"}
      <LinkControl {field} {value} {onchange} {doc} readOnly={ro} {query} {error} {id} />
    {:else if ft === "Month" || (ft === "Date" && (field.options === "month" || (field as any).format === "mm/yyyy"))}
      <MonthControl {field} {value} {onchange} {id} readOnly={ro} error={shownError} {inGrid} />
    {:else if ft === "Date"}
      <DateControl {field} {value} {onchange} {id} readOnly={ro} error={shownError} {inGrid} />
    {:else if ft === "Datetime"}
      <input {id} type="datetime-local" class="input" class:error={!!shownError} readonly={ro} value={toDatetimeLocal(value)}
        onclick={(e) => { if (!ro) { try { (e.target as HTMLInputElement).showPicker?.(); } catch {} } }}
        onchange={(e) => onchange(fromDatetimeLocal((e.target as HTMLInputElement).value))} />
    {:else if ft === "Time"}
      <input {id} type="time" class="input" readonly={ro} value={value || ""}
        onclick={(e) => { if (!ro) { try { (e.target as HTMLInputElement).showPicker?.(); } catch {} } }}
        onchange={(e) => onchange((e.target as HTMLInputElement).value || null)} />
    {:else if ft === "Int" || ft === "Float" || ft === "Currency" || ft === "Percent"}
      <div style="position:relative">
        {#if ft === "Currency" && !inGrid && currencyBefore()}<span class="muted" style="position:absolute;left:8px;top:50%;transform:translateY(-50%);font-size:12px">{currencySymbol()}</span>{/if}
        {#if ft === "Currency" && !inGrid && !currencyBefore()}<span class="muted" style="position:absolute;right:8px;top:50%;transform:translateY(-50%);font-size:12px">{currencySymbol()}</span>{/if}
        <input {id} class="input num" class:error={!!shownError} style:padding-left={ft === "Currency" && !inGrid && currencyBefore() ? "30px" : undefined} style:padding-right={ft === "Percent" ? "24px" : ft === "Currency" && !inGrid && !currencyBefore() ? "30px" : undefined} style="text-align:right" readonly={ro}
          value={text} onfocus={() => (focused = true)} oninput={(e) => (text = (e.target as HTMLInputElement).value)} onblur={commitNumber}
          onkeydown={(e) => e.key === "Enter" && commitNumber()} inputmode="decimal" />
        {#if ft === "Percent"}<span class="muted" style="position:absolute;right:8px;top:50%;transform:translateY(-50%);font-size:12px">%</span>{/if}
      </div>
    {:else if ft === "Small Text" || ft === "Text" || ft === "Text Editor"}
      <textarea {id} class="input" class:error={!!shownError} readonly={ro} rows={ft === "Small Text" ? 2 : 5} value={value ?? ""} onchange={(e) => onchange((e.target as HTMLTextAreaElement).value || null)}></textarea>
    {:else if ft === "Attach"}
      <AttachControl {value} {onchange} {onbusychange} readOnly={ro} mandatory={req} {doc} fieldname={field.fieldname || ""} />
    {:else if ft === "JSON"}
      <textarea {id} class="input" readonly={ro} rows={4} value={typeof value === "string" ? value : JSON.stringify(value ?? null, null, 2)} onchange={(e) => { try { onchange(JSON.parse((e.target as HTMLTextAreaElement).value)); } catch { onchange((e.target as HTMLTextAreaElement).value); } }}></textarea>
    {:else if ft === "Password"}
      <input {id} type="password" class="input" readonly={ro} value={value ?? ""} onchange={(e) => onchange((e.target as HTMLInputElement).value || null)} autocomplete="new-password" />
    {:else if ft === "Email"}
      <input {id} type="email" class="input" class:error={!!shownError} readonly={ro} maxlength={Math.min(field.length || 254, 254)} value={value ?? ""} data-fieldname={field.fieldname} data-fieldtype={ft}
        oninput={(e) => { emailError = ""; onchange((e.target as HTMLInputElement).value || null); }} onblur={(e) => commitEmail((e.target as HTMLInputElement).value)} autocomplete="email" />
    {:else}
      <input {id} type="text" class="input" class:error={!!shownError} readonly={ro} maxlength={field.length || undefined} value={value ?? ""} data-fieldname={field.fieldname} data-fieldtype={ft}
        oninput={(e) => onchange((e.target as HTMLInputElement).value || null)} />
    {/if}
    {#if shownError}<div class="err">{shownError}</div>{:else if field.description && !inGrid}<div class="desc">{field.description}</div>{/if}
  </div>
{/if}

<style>
  .compact { margin-bottom: 0; }
</style>
