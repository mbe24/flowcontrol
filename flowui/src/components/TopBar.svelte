<script lang="ts">
  import { app, load, toggleTheme, openTaskDialog } from '../lib/state.svelte';
  import { projectCounts } from '../lib/derive';
  import type { ViewName } from '../lib/state.svelte';

  const views: ViewName[] = ['table', 'lanes', 'graph'];
  const counts = $derived(projectCounts(app.nodes));
  const projectName = $derived(app.projects.find((p) => p.id === app.projectId)?.name ?? '—');
  const mobile = $derived(app.width < 860);
  const pct = (n: number) => (counts.total ? (n / counts.total) * 100 : 0);
</script>

<div class="bar" class:mobile>
  {#if mobile}
    <span class="logo mono">fc</span>
  {/if}
  <div class="title">
    <span class="name">{projectName}</span>
    {#if !mobile}<span class="mono id">{app.projectId}</span>{/if}
  </div>

  <div class="tabs">
    {#each views as v}
      <button class="tab" class:on={app.view === v} onclick={() => (app.view = v)}>
        {v[0].toUpperCase() + v.slice(1)}
      </button>
    {/each}
  </div>

  {#if !mobile}
    <div class="progress">
      <div class="track">
        <div style:width="{pct(counts.done)}%" style:background="var(--done)"></div>
        <div style:width="{pct(counts.ready)}%" style:background="var(--ready)"></div>
        <div style:width="{pct(counts.blocked)}%" style:background="var(--blocked)"></div>
        <div style:width="{pct(counts.deferred)}%" style:background="var(--deferred)"></div>
      </div>
      <span class="mono ratio">{counts.pct}% · {counts.done}/{counts.total}</span>
    </div>
  {/if}

  <div class="spacer"></div>

  {#if mobile}
    <button class="icon" onclick={() => (app.paletteOpen = true)} aria-label="Search">⌕</button>
    <button class="icon" onclick={toggleTheme} aria-label="Theme">{app.theme === 'dark' ? '☾' : '☀'}</button>
  {:else}
    <button class="search" onclick={() => (app.paletteOpen = true)}>
      <span>Search or jump…</span>
      <span class="mono kbd">⌘K</span>
    </button>
    <button class="ghost">Filter</button>
    <button class="primary" onclick={openTaskDialog}>New task</button>
  {/if}
</div>

{#if mobile}
  <div class="mprogress">
    <div class="track">
      <div style:width="{pct(counts.done)}%" style:background="var(--done)"></div>
      <div style:width="{pct(counts.ready)}%" style:background="var(--ready)"></div>
      <div style:width="{pct(counts.blocked)}%" style:background="var(--blocked)"></div>
      <div style:width="{pct(counts.deferred)}%" style:background="var(--deferred)"></div>
    </div>
    <span class="mono ratio">{counts.pct}%</span>
  </div>
{/if}

<style>
  .bar {
    height: 54px;
    flex: none;
    border-bottom: 1px solid var(--border);
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 0 18px;
  }
  .bar.mobile {
    height: 50px;
    gap: 10px;
    padding: 0 14px;
    border-bottom: 0;
  }
  .logo {
    width: 23px;
    height: 23px;
    border-radius: 6px;
    background: var(--accent);
    color: var(--accent-fg);
    display: grid;
    place-items: center;
    font-size: 10px;
    font-weight: 600;
    flex: none;
  }
  .title {
    display: flex;
    align-items: baseline;
    gap: 9px;
    min-width: 0;
  }
  .name {
    font-weight: 600;
    font-size: 13.5px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .id {
    font-size: 11px;
    color: var(--fg3);
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
  .ratio {
    font-size: 11px;
    color: var(--fg2);
  }
  .mprogress {
    flex: none;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 14px 10px;
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
    width: 30px;
    height: 30px;
    border-radius: 7px;
    border: 0;
    background: transparent;
    color: var(--fg2);
    font-size: 15px;
    cursor: pointer;
    font-family: inherit;
    flex: none;
  }
  .search {
    display: flex;
    align-items: center;
    gap: 8px;
    background: var(--panel2);
    border: 1px solid var(--border);
    border-radius: 7px;
    padding: 5px 10px;
    width: 200px;
    color: var(--fg3);
    font-size: 12px;
    font-family: inherit;
    cursor: pointer;
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
  .primary {
    border: 0;
    background: var(--accent);
    color: var(--accent-fg);
    font-weight: 500;
  }
</style>
