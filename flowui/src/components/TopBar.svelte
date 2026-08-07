<script lang="ts">
  import { app, activeFilterCount, toggleTheme } from '../lib/state.svelte';
  import { projectCounts } from '../lib/derive';
  import type { ViewName } from '../lib/state.svelte';
  import Logo from './Logo.svelte';

  const views: ViewName[] = ['table', 'lanes', 'graph'];
  const counts = $derived(projectCounts(app.nodes));
  const projectName = $derived(app.projects.find((p) => p.id === app.projectId)?.name ?? '—');
  const mobile = $derived(app.width < 860);
  const filters = $derived(activeFilterCount());
  const pct = (n: number) => (counts.total ? (n / counts.total) * 100 : 0);
  // Fixed-width, right-aligned slots: percent = 4 cols (% anchored last),
  // steps = 9 cols ("  3 /  5"), separated by " · " -> 16 chars, always.
  // Fixed 16 columns. Percent uses non-breaking-space padding (" 12%", not
  // "012%") so it looks clean yet stays full-width; steps use zero padding.
  const NBSP = '\u00A0';
  const ratioText = $derived(
    `${String(counts.pct).padStart(3, NBSP)}% · ${String(counts.done).padStart(3, '0')} / ${String(counts.total).padStart(3, '0')}`
  );
  const mobileRatio = $derived(`${String(counts.pct).padStart(3, NBSP)}%`);
</script>

<div class="bar" class:mobile>
  <div class="left">
    {#if mobile}
      <button class="logo" onclick={() => (app.projectMenuOpen = true)} aria-label="Projects">
        <Logo size={26} />
      </button>
    {/if}
    <button class="title" onclick={() => (app.projectMenuOpen = !app.projectMenuOpen)}>
      <span class="name">{projectName}</span>
      <span class="chev">▾</span>
    </button>
  </div>

  {#if !mobile}
    <div class="mid">
      <div class="tabs">
        {#each views as v}
          <button class="tab" class:on={app.view === v} onclick={() => (app.view = v)}>
            {v[0].toUpperCase() + v.slice(1)}
          </button>
        {/each}
      </div>
      <div class="progress">
        <div class="track">
          <div style:width="{pct(counts.done)}%" style:background="var(--done)"></div>
          <div style:width="{pct(counts.ready)}%" style:background="var(--ready)"></div>
          <div style:width="{pct(counts.blocked)}%" style:background="var(--blocked)"></div>
          <div style:width="{pct(counts.deferred)}%" style:background="var(--deferred)"></div>
        </div>
        <span class="mono ratio">{ratioText}</span>
      </div>
    </div>
  {/if}

  <div class="right">
    {#if mobile}
      <button class="icon" class:on={filters > 0} onclick={() => (app.filterOpen = true)} aria-label="Filter">
        ⚟{#if filters}<span class="badge">{filters}</span>{/if}
      </button>
      <button class="icon" onclick={() => (app.paletteOpen = true)} aria-label="Search">⌕</button>
      <button class="icon" onclick={toggleTheme} aria-label="Theme">{app.theme === 'dark' ? '☾' : '☀'}</button>
    {:else}
      <button class="search" onclick={() => (app.paletteOpen = true)}>
        <span>Search or jump…</span>
        <span class="mono kbd">⌘K</span>
      </button>
      <button class="ghost" class:on={filters > 0} onclick={() => (app.filterOpen = !app.filterOpen)}>
        Filter
        {#if filters}<span class="badge">{filters}</span>{/if}
      </button>
      <button
        class="primary"
        onclick={() => {
          const sel = app.nodes.find((n) => n.id === app.selectedId);
          const parent = sel?.type === 'TASK' ? sel.parentId : sel?.type === 'STEP' ? null : sel?.id ?? null;
          app.dialog = { kind: 'create', nodeType: 'TASK', parentId: parent, title: '' };
        }}>New task</button>
    {/if}
  </div>
</div>

{#if mobile}
  <div class="mprogress">
    <div class="track">
      <div style:width="{pct(counts.done)}%" style:background="var(--done)"></div>
      <div style:width="{pct(counts.ready)}%" style:background="var(--ready)"></div>
      <div style:width="{pct(counts.blocked)}%" style:background="var(--blocked)"></div>
      <div style:width="{pct(counts.deferred)}%" style:background="var(--deferred)"></div>
    </div>
    <span class="mono ratio">{mobileRatio}</span>
  </div>
{/if}

<style>
  .bar {
    height: 54px;
    flex: none;
    border-bottom: 1px solid var(--border);
    /* three columns keep the middle (tabs + ratio) centred and stable no matter
       how the project name length changes on the left. minmax(0,1fr) stops the
       side columns from growing with their content, so the centre is pinned. */
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
    align-items: center;
    gap: 14px;
    padding: 0 18px;
  }
  .left {
    display: flex;
    align-items: center;
    gap: 12px;
    justify-self: start;
    min-width: 0;
    overflow: hidden;
  }
  .mid {
    display: flex;
    align-items: center;
    gap: 14px;
    justify-self: center;
  }
  .right {
    display: flex;
    align-items: center;
    gap: 10px;
    justify-self: end;
    min-width: 0;
    overflow: hidden;
  }
  .bar.mobile {
    height: 52px;
    grid-template-columns: auto 1fr auto;
    gap: 8px;
    padding: 0 12px;
    border-bottom: 0;
  }
  .logo {
    width: 26px;
    height: 26px;
    border-radius: 6px;
    display: grid;
    place-items: center;
    color: var(--accent);
    background: transparent;
    border: 0;
    cursor: pointer;
    flex: none;
  }
  .title {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    background: transparent;
    border: 0;
    padding: 5px 7px;
    margin: 0 -7px;
    border-radius: 7px;
    cursor: pointer;
    font-family: inherit;
    color: var(--fg);
  }
  .title:hover {
    background: var(--hover);
  }
  .name {
    font-weight: 600;
    font-size: 13.5px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .chev {
    font-size: 9px;
    color: var(--fg3);
    flex: none;
  }
  .tabs {
    display: flex;
    background: var(--panel2);
    border: 1px solid var(--border);
    border-radius: 7px;
    padding: 2px;
    flex: none;
  }
  .tab {
    padding: 4px 12px;
    border-radius: 5px;
    border: 0;
    background: transparent;
    color: var(--fg2);
    font-size: 11.5px;
    font-family: inherit;
    cursor: pointer;
  }
  .tab.on {
    background: var(--hover);
    color: var(--fg);
    font-weight: 500;
  }
  .progress {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .track {
    width: 150px;
    height: 7px;
    border-radius: 4px;
    overflow: hidden;
    display: flex;
    background: var(--track);
  }
  /* The ratio is padded to a fixed character count AND given a reserved box
     width (widest string "100% · 100/100" = 14 monospace chars) so digit-count
     changes can never shift the layout. */
  .ratio {
    font-size: 11px;
    color: var(--fg2);
    font-variant-numeric: tabular-nums;
  }
  .progress .ratio {
    display: inline-block;
    min-width: 16ch;
  }
  .mprogress {
    flex: none;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 12px 10px;
    border-bottom: 1px solid var(--border2);
  }
  .mprogress .track {
    flex: 1;
    width: auto;
    height: 5px;
  }
  .spacer {
    flex: 1;
  }
  .icon {
    position: relative;
    min-width: 40px;
    min-height: 40px;
    border-radius: 8px;
    border: 0;
    background: transparent;
    color: var(--fg2);
    font-size: 15px;
    cursor: pointer;
    font-family: inherit;
    flex: none;
  }
  .icon.on {
    color: var(--accent);
  }
  .search {
    display: flex;
    align-items: center;
    gap: 8px;
    background: var(--panel2);
    border: 1px solid var(--border);
    border-radius: 7px;
    padding: 5px 10px;
    width: 190px;
    color: var(--fg3);
    font-size: 12px;
    font-family: inherit;
    cursor: pointer;
  }
  .search:hover {
    border-color: var(--fg3);
  }
  .kbd {
    margin-left: auto;
    font-size: 10px;
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 1px 5px;
  }
  .ghost,
  .primary {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 5px 12px;
    border-radius: 7px;
    font-size: 12px;
    font-family: inherit;
    cursor: pointer;
    flex: none;
  }
  .ghost {
    border: 1px solid var(--border);
    background: transparent;
    color: var(--fg2);
  }
  /* A filter you can't see is a filter you forget you set. */
  .ghost.on {
    border-color: var(--accent);
    background: color-mix(in oklab, var(--accent) 10%, transparent);
    color: var(--accent);
  }
  .primary {
    border: 0;
    background: var(--accent);
    color: var(--accent-fg);
    font-weight: 500;
  }
  .badge {
    font-family: 'IBM Plex Mono', monospace;
    font-size: 10px;
    font-weight: 500;
    background: var(--accent);
    color: var(--accent-fg);
    padding: 2px 6px;
    border-radius: 9px;
    line-height: 1;
  }
  .icon .badge {
    position: absolute;
    top: 4px;
    right: 2px;
    padding: 1px 4px;
    font-size: 9px;
  }
</style>
