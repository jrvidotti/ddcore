<script lang="ts">
  import type { FormController } from "$lib/form.svelte";
  import { __ } from "$lib/boot.svelte";
  import Icon from "./Icon.svelte";
  import { parseVersion, type ParsedVersion, type ParsedChange } from "./history";

  let {
    frm,
    versions = [],
    mode = "compact",
    onExpand,
  }: {
    frm: FormController;
    versions: any[];
    mode?: "compact" | "full";
    onExpand?: () => void;
  } = $props();

  let search = $state("");

  const parsedList = $derived.by((): ParsedVersion[] => {
    return (versions || []).map((v) => parseVersion(v, frm));
  });

  const filtered = $derived.by((): ParsedVersion[] => {
    const q = search.trim().toLowerCase();
    if (!q) return parsedList;
    return parsedList.filter((v) => {
      if (v.owner.toLowerCase().includes(q)) return true;
      if (v.relativeTime.toLowerCase().includes(q)) return true;
      return v.changes.some((c) => {
        if (c.label.toLowerCase().includes(q)) return true;
        if (c.field.toLowerCase().includes(q)) return true;
        if (String(c.formattedOld).toLowerCase().includes(q)) return true;
        if (String(c.formattedNew).toLowerCase().includes(q)) return true;
        if (c.tableDiff) {
          const inAdded = c.tableDiff.added.some((r) =>
            Object.values(r.fields).some((f) => f.label.toLowerCase().includes(q) || f.formatted.toLowerCase().includes(q))
          );
          if (inAdded) return true;
          const inMod = c.tableDiff.modified.some((m) =>
            m.changes.some(
              (ch) =>
                ch.label.toLowerCase().includes(q) ||
                ch.formattedFrom.toLowerCase().includes(q) ||
                ch.formattedTo.toLowerCase().includes(q)
            )
          );
          if (inMod) return true;
        }
        return false;
      });
    });
  });

  function avatarInitial(name: string): string {
    return (name || "U").trim().charAt(0).toUpperCase();
  }
</script>

<div class="doc-history" class:full={mode === "full"}>
  {#if mode === "full"}
    <div class="history-toolbar">
      <div class="search-wrap">
        <Icon name="search" size={14} />
        <input
          class="input sm"
          type="search"
          placeholder={__("Filtrar por campo, valor ou usuário...")}
          bind:value={search}
        />
        {#if search}
          <button class="btn icon sm clear-btn" onclick={() => (search = "")} aria-label="Limpar">
            <Icon name="x" size={12} />
          </button>
        {/if}
      </div>
      <div class="history-count muted small">
        {parsedList.length} {parsedList.length === 1 ? __("versão") : __("versões")}
      </div>
    </div>
  {/if}

  {#if filtered.length === 0}
    <div class="empty-state">
      <Icon name="history" size={20} />
      <span>{search ? __("Nenhuma alteração encontrada com o filtro") : __("Nenhum histórico registrado")}</span>
    </div>
  {:else}
    <div class="timeline">
      {#each filtered as v (v.id)}
        <div class="timeline-item">
          <div class="timeline-spine">
            <div class="timeline-avatar" title={v.owner}>
              {avatarInitial(v.owner)}
            </div>
            <div class="timeline-line"></div>
          </div>

          <div class="timeline-content">
            <div class="timeline-header">
              <span class="user-name">{v.owner}</span>
              <span class="dot">·</span>
              <span class="time" title={v.fullTime}>{v.relativeTime}</span>
            </div>

            <div class="changes-list">
              {#each v.changes as c}
                {#if !c.isTable}
                  <!-- Standard Field Change -->
                  <div class="change-row">
                    <span class="field-label">{c.label}</span>
                    <div class="diff-values">
                      {#if c.isAttach && c.oldUrl}
                        <span class="val val-old">
                          <Icon name="paperclip" size={11} />
                          <a href={c.oldUrl} target="_blank" rel="noopener">{c.formattedOld}</a>
                        </span>
                      {:else}
                        <span class="val val-old" class:is-empty={c.formattedOld === "—"}>{c.formattedOld}</span>
                      {/if}

                      <span class="arrow">→</span>

                      {#if c.isAttach && c.newUrl}
                        <span class="val val-new">
                          <Icon name="paperclip" size={11} />
                          <a href={c.newUrl} target="_blank" rel="noopener">{c.formattedNew}</a>
                        </span>
                      {:else}
                        <span class="val val-new" class:is-empty={c.formattedNew === "—"}>{c.formattedNew}</span>
                      {/if}
                    </div>
                  </div>
                {:else if c.tableDiff}
                  <!-- Child Table Diff (ex: Anexos) -->
                  <div class="table-change-block">
                    <div class="table-header">
                      <span class="field-label bold">{c.label}</span>
                    </div>

                    <!-- Added Rows -->
                    {#each c.tableDiff.added as row}
                      <div class="table-row-card added">
                        <div class="card-status-bar">
                          <span class="badge green">+{__("Linha adicionada")}</span>
                        </div>
                        <div class="card-fields">
                          {#each Object.entries(row.fields) as [fname, fdata]}
                            <div class="card-field-item">
                              <span class="cf-label">{fdata.label}:</span>
                              {#if fdata.isAttach && fdata.url}
                                <a href={fdata.url} target="_blank" rel="noopener" class="cf-attach">
                                  <Icon name="paperclip" size={11} />
                                  <span>{fdata.formatted}</span>
                                </a>
                              {:else}
                                <span class="cf-val">{fdata.formatted}</span>
                              {/if}
                            </div>
                          {/each}
                        </div>
                      </div>
                    {/each}

                    <!-- Modified Rows -->
                    {#each c.tableDiff.modified as mod}
                      <div class="table-row-card modified">
                        <div class="card-status-bar">
                          <span class="badge blue">{__("Linha #{0} alterada", [mod.rowIdx])}</span>
                        </div>
                        <div class="card-fields">
                          {#each mod.changes as ch}
                            <div class="card-field-diff">
                              <span class="cf-label">{ch.label}:</span>
                              <div class="diff-values">
                                {#if ch.isAttach && ch.fromUrl}
                                  <span class="val val-old">
                                    <Icon name="paperclip" size={11} />
                                    <a href={ch.fromUrl} target="_blank" rel="noopener">{ch.formattedFrom}</a>
                                  </span>
                                {:else}
                                  <span class="val val-old" class:is-empty={ch.formattedFrom === "—"}>{ch.formattedFrom}</span>
                                {/if}

                                <span class="arrow">→</span>

                                {#if ch.isAttach && ch.toUrl}
                                  <span class="val val-new">
                                    <Icon name="paperclip" size={11} />
                                    <a href={ch.toUrl} target="_blank" rel="noopener">{ch.formattedTo}</a>
                                  </span>
                                {:else}
                                  <span class="val val-new" class:is-empty={ch.formattedTo === "—"}>{ch.formattedTo}</span>
                                {/if}
                              </div>
                            </div>
                          {/each}
                        </div>
                      </div>
                    {/each}

                    <!-- Removed Rows -->
                    {#each c.tableDiff.removed as row}
                      <div class="table-row-card removed">
                        <div class="card-status-bar">
                          <span class="badge red">-{__("Linha removida")}</span>
                        </div>
                        <div class="card-fields">
                          {#each Object.entries(row.fields) as [fname, fdata]}
                            <div class="card-field-item">
                              <span class="cf-label">{fdata.label}:</span>
                              {#if fdata.isAttach && fdata.url}
                                <a href={fdata.url} target="_blank" rel="noopener" class="cf-attach">
                                  <Icon name="paperclip" size={11} />
                                  <span>{fdata.formatted}</span>
                                </a>
                              {:else}
                                <span class="cf-val">{fdata.formatted}</span>
                              {/if}
                            </div>
                          {/each}
                        </div>
                      </div>
                    {/each}
                  </div>
                {/if}
              {/each}
            </div>
          </div>
        </div>
      {/each}
    </div>
  {/if}

  {#if mode === "compact" && onExpand && parsedList.length > 0}
    <div class="compact-footer">
      <button class="btn sm expand-btn" onclick={onExpand}>
        <Icon name="maximize-2" size={12} />
        <span>{__("Ver histórico completo")}</span>
      </button>
    </div>
  {/if}
</div>

<style>
  .doc-history {
    display: flex;
    flex-direction: column;
    font-size: 13px;
  }

  /* Toolbar (Full Mode) */
  .history-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 16px;
    padding-bottom: 12px;
    border-bottom: 1px solid var(--border);
  }
  .search-wrap {
    position: relative;
    display: flex;
    align-items: center;
    flex: 1;
    max-width: 360px;
  }
  .search-wrap :global(svg) {
    position: absolute;
    left: 8px;
    color: var(--muted);
    pointer-events: none;
  }
  .search-wrap input {
    padding-left: 28px;
    padding-right: 26px;
    width: 100%;
  }
  .clear-btn {
    position: absolute;
    right: 4px;
    background: none;
    border: none;
    padding: 4px;
    cursor: pointer;
    color: var(--muted);
  }

  /* Empty State */
  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    padding: 24px 12px;
    color: var(--muted);
    font-size: 13px;
    text-align: center;
  }

  /* Timeline */
  .timeline {
    display: flex;
    flex-direction: column;
    position: relative;
  }
  .timeline-item {
    display: flex;
    gap: 10px;
    position: relative;
  }
  .timeline-item:not(:last-child) {
    padding-bottom: 18px;
  }
  .timeline-spine {
    display: flex;
    flex-direction: column;
    align-items: center;
    width: 24px;
    flex-shrink: 0;
  }
  .timeline-avatar {
    width: 22px;
    height: 22px;
    border-radius: 50%;
    background: #e0e7ff;
    color: var(--primary);
    font-size: 11px;
    font-weight: 700;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    box-shadow: 0 0 0 2px var(--surface);
  }
  .timeline-line {
    flex: 1;
    width: 2px;
    background: var(--border);
    margin-top: 4px;
    margin-bottom: -4px;
  }
  .timeline-item:last-child .timeline-line {
    display: none;
  }

  /* Content */
  .timeline-content {
    flex: 1;
    min-width: 0;
  }
  .timeline-header {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 12px;
    margin-bottom: 6px;
  }
  .user-name {
    font-weight: 600;
    color: var(--text);
  }
  .dot {
    color: var(--muted);
  }
  .time {
    color: var(--muted);
    cursor: default;
  }

  /* Changes List */
  .changes-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .change-row {
    display: flex;
    flex-direction: column;
    gap: 2px;
    background: #fafbfc;
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 6px 8px;
  }
  .field-label {
    font-size: 11px;
    font-weight: 600;
    color: var(--muted);
    text-transform: uppercase;
    letter-spacing: 0.03em;
  }
  .field-label.bold {
    font-weight: 700;
  }

  /* Diff values */
  .diff-values {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 4px;
    font-size: 12px;
    line-height: 1.4;
  }
  .val {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 1px 6px;
    border-radius: 4px;
    word-break: break-word;
    max-width: 100%;
  }
  .val a {
    color: inherit;
    text-decoration: underline;
    text-underline-offset: 2px;
  }
  .val-old {
    background: #fee2e2;
    color: #991b1b;
    text-decoration: line-through;
  }
  .val-old.is-empty {
    text-decoration: none;
    background: #f3f4f6;
    color: var(--muted);
  }
  .val-new {
    background: #dcfce7;
    color: #166534;
    font-weight: 500;
  }
  .val-new.is-empty {
    background: #f3f4f6;
    color: var(--muted);
    font-weight: normal;
  }
  .arrow {
    color: var(--muted);
    font-size: 11px;
  }

  /* Child Table Diff Card */
  .table-change-block {
    margin-top: 4px;
  }
  .table-header {
    margin-bottom: 4px;
  }
  .table-row-card {
    background: #fff;
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 7px 9px;
    margin-bottom: 6px;
    display: flex;
    flex-direction: column;
    gap: 5px;
  }
  .table-row-card.added {
    border-left: 3px solid var(--green);
    background: rgba(22, 163, 74, 0.03);
  }
  .table-row-card.modified {
    border-left: 3px solid var(--blue);
    background: rgba(37, 99, 235, 0.03);
  }
  .table-row-card.removed {
    border-left: 3px solid var(--red);
    background: rgba(220, 38, 38, 0.03);
  }

  .card-status-bar {
    display: flex;
    align-items: center;
  }
  .badge {
    display: inline-flex;
    align-items: center;
    font-size: 11px;
    font-weight: 600;
    padding: 1px 6px;
    border-radius: 4px;
  }
  .badge.green {
    background: #dcfce7;
    color: #166534;
  }
  .badge.blue {
    background: #dbeafe;
    color: #1e40af;
  }
  .badge.red {
    background: #fee2e2;
    color: #991b1b;
  }

  .card-fields {
    display: flex;
    flex-direction: column;
    gap: 3px;
    font-size: 12px;
  }
  .card-field-item,
  .card-field-diff {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px;
  }
  .cf-label {
    font-size: 11px;
    color: var(--muted);
    font-weight: 500;
  }
  .cf-val {
    font-weight: 500;
  }
  .cf-attach {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    color: var(--primary);
    text-decoration: underline;
    text-underline-offset: 2px;
    word-break: break-all;
  }

  /* Compact Mode specifics */
  .compact-footer {
    margin-top: 10px;
    padding-top: 8px;
    border-top: 1px dashed var(--border);
  }
  .expand-btn {
    width: 100%;
    justify-content: center;
    font-size: 12px;
  }

  /* Full Mode adjustments */
  .full .change-row {
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
    padding: 8px 12px;
  }
  .full .field-label {
    min-width: 140px;
  }
  .full .diff-values {
    flex: 1;
    justify-content: flex-end;
  }
  .full .table-row-card {
    padding: 10px 14px;
  }
  .full .card-fields {
    gap: 6px;
  }
</style>
