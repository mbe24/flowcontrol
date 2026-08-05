<script lang="ts">
  import { app } from '../lib/state.svelte';
  import { projectCounts } from '../lib/derive';
  import type { ViewName } from '../lib/state.svelte';

  const views: ViewName[] = ['table', 'lanes', 'graph'];
  const counts = $derived(projectCounts(app.nodes));
  const projectName = $derived(
    app.projects.find((p) => p.id === app.projectId)?.name ?? '—',
  );
  const pct = (n: number) => (counts.total ? (n / counts.total) * 100 : 0);
</script>

<div class="bar">
  <div class="title">
    <span class="name">{projectName}</span>
    <span class="mono id">{app.projectId}</span>
  </div>

  <div class="tabs">
    {#each views as v}
      <button
        class="tab"
        class:on={app.view === v}
        onclick={() => (app.view = v)}
      >
        {v[0].toUpperCase() + v.slice(1)}
      </button>
    {/each}
  </div>

  <div class="progress">
    <div class="track">
      <div
        style:width="{pct(counts.done)}%"
        style:background="var(--done)"
      ></div>
      <div
        style:width="{pct(counts.ready)}%"
        style:background="var(--ready)"
      ></div>
      <div
        style:width="{pct(counts.blocked)}%"
        style:background="var(--blocked)"
      ></div>
      <div
        style:width="{pct(counts.deferred)}%"
        style:background="var(--deferred)"
      ></div>
    </div>
    <span class="mono ratio">{counts.pct}% · {counts.done}/{counts.total}</span>
  </div>

  <div class="spacer"></div>

  <button class="search" onclick={() => (app.paletteOpen = true)}>
    <span>Search or jump…</span>
    <span class="mono kbd">⌘K</span>
  </button>
  <button class="ghost">Filter</button>
  <button class="primary">New task</button>
</div>

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
  .title {
    display: flex;
    align-items: baseline;
    gap: 9px;
  }
  .name {
    font-weight: 600;
    font-size: 14px;
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
  }
  .tab {
    padding: 4px 13px;
    border-radius: 5px;
    border: 0;
    background: transparent;
    color: var(--fg2);
    font-size: 12px;
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
  .spacer {
    flex: 1;
  }
  .search {
    display: flex;
    align-items: center;
    gap: 8px;
    background: var(--panel2);
    border: 1px solid var(--border);
    border-radius: 7px;
    padding: 5px 10px;
    width: 210px;
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
    padding: 5px 12px;
    border-radius: 7px;
    font-size: 12px;
    font-family: inherit;
    cursor: pointer;
  }
  .ghost {
    border: 1px solid var(--border);
    background: transparent;
    color: var(--fg2);
  }
  .ghost:hover {
    border-color: var(--fg3);
    color: var(--fg);
  }
  .primary {
    border: 0;
    background: var(--accent);
    color: var(--accent-fg);
    font-weight: 500;
  }
</style>
