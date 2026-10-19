<script lang="ts">
  import { __ } from "$lib/boot.svelte";
  import { today } from "$lib/datetime";
  import type { Field } from "$lib/meta";
  import MaskedPickerInput from "./MaskedPickerInput.svelte";
  import {
    monthLabelsShort,
    monthPlaceholder,
    formatMonth,
    maskMonthInput,
    parseMonth,
    stepMonth,
  } from "./month-format";

  let {
    field,
    value,
    onchange,
    readOnly = false,
    error = "",
    id = "",
  }: {
    field: Field;
    value: any;
    onchange: (v: any) => void;
    readOnly?: boolean;
    error?: string;
    id?: string;
  } = $props();

  const fmt = { format: formatMonth, parse: parseMonth, mask: maskMonthInput, step: stepMonth, digits: 6 };
  const currentParsed = $derived(parseMonth(value));

  // the site's day, not the browser's — see DateControl
  const todayIso = today();
  const todayYear = Number(todayIso.slice(0, 4));
  const todayMonth = Number(todayIso.slice(5, 7));
  let viewYear = $state(todayYear);
</script>

<MaskedPickerInput
  {field}
  fieldtype="Month"
  {value}
  {onchange}
  {readOnly}
  {error}
  {id}
  {fmt}
  placeholder={monthPlaceholder()}
  {todayIso}
  todayValue={`${todayIso.slice(0, 7)}-01`}
  todayLabel={__("This month")}
  openLabel={__("Open the month picker")}
  compact
  nav={{
    title: String(viewYear),
    prevLabel: __("Previous year"),
    nextLabel: __("Next year"),
    onprev: () => (viewYear -= 1),
    onnext: () => (viewYear += 1),
  }}
  onview={(iso) => (viewYear = Number(iso.slice(0, 4)))}
>
  {#snippet grid(choose)}
    <div class="popover-grid">
      {#each monthLabelsShort() as name, idx}
        {@const mNum = idx + 1}
        {@const isSelected = currentParsed?.year === viewYear && currentParsed?.month === mNum}
        {@const isToday = todayYear === viewYear && todayMonth === mNum}
        <button
          type="button"
          class="month-cell"
          class:selected={isSelected}
          class:is-today={isToday && !isSelected}
          onclick={() => choose(`${viewYear}-${String(mNum).padStart(2, "0")}-01`)}
        >
          {name}
        </button>
      {/each}
    </div>
  {/snippet}
</MaskedPickerInput>

<style>
  .popover-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 5px;
    margin-bottom: 8px;
  }

  .month-cell {
    border: 1px solid transparent;
    background: transparent;
    border-radius: 6px;
    padding: 6px 0;
    font-size: 13px;
    text-align: center;
    cursor: pointer;
    color: var(--text);
    transition: all 0.1s ease;
  }

  .month-cell:hover {
    background: var(--hover);
  }

  .month-cell.is-today {
    border-color: var(--border);
    font-weight: 500;
  }

  .month-cell.selected {
    background: var(--primary);
    color: var(--on-primary);
    font-weight: 600;
  }
</style>
