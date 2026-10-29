<script lang="ts">
  import { __ } from "$lib/boot.svelte";
  import { nowTime } from "$lib/datetime";
  import type { Field } from "$lib/meta";
  import { timePlaceholder } from "$lib/locale";
  import MaskedPickerInput from "./MaskedPickerInput.svelte";
  import { formatTime, maskTimeInput, parseTime, stepTime, timeSlots } from "./time-format";

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

  const fmt = { format: formatTime, parse: parseTime, mask: maskTimeInput, step: stepTime, digits: 4, sep: ":" };
  const slots = timeSlots(30);
  const selected = $derived(parseTime(value)?.iso);

  // the site's clock, not the browser's — see DateControl
  let now = $state(nowTime());
  // the time the list opens on: the value, or now
  let viewIso = $state(nowTime());
  // the last slot at or before viewIso, which the list scrolls to
  const anchor = $derived(slots.filter((s) => s <= viewIso).at(-1) ?? slots[0]);

  function scrollToAnchor(list: HTMLElement) {
    const row = list.querySelector<HTMLElement>('[data-anchor="true"]');
    if (row) list.scrollTop = row.offsetTop - list.clientHeight / 2 + row.offsetHeight / 2;
  }
</script>

<MaskedPickerInput
  {field}
  fieldtype="Time"
  {value}
  {onchange}
  {readOnly}
  {error}
  {id}
  {fmt}
  placeholder={timePlaceholder()}
  todayIso={now}
  todayValue={now}
  todayLabel={__("Now")}
  openLabel={__("Open the time picker")}
  icon="clock"
  openOnClick={false}
  compact
  onview={(iso) => {
    now = nowTime();
    viewIso = iso;
  }}
>
  {#snippet grid(choose)}
    <div class="slot-list" use:scrollToAnchor>
      {#each slots as slot}
        {@const isSelected = selected === slot}
        <button
          type="button"
          class="slot"
          class:selected={isSelected}
          data-anchor={slot === anchor}
          onclick={() => choose(slot)}
        >
          {formatTime(slot)}
        </button>
      {/each}
    </div>
  {/snippet}
</MaskedPickerInput>

<style>
  .slot-list {
    /* the rows' offsetTop is measured from the list, which scrollToAnchor needs */
    position: relative;
    display: flex;
    flex-direction: column;
    gap: 2px;
    max-height: 216px;
    overflow-y: auto;
    margin-bottom: 8px;
  }

  .slot {
    border: 1px solid transparent;
    background: transparent;
    border-radius: 6px;
    padding: 5px 8px;
    font-size: 13px;
    text-align: left;
    font-variant-numeric: tabular-nums;
    cursor: pointer;
    color: var(--text);
    transition: all 0.1s ease;
  }

  .slot:hover {
    background: var(--hover);
  }

  .slot.selected {
    background: var(--primary);
    color: var(--on-primary);
    font-weight: 600;
  }
</style>
