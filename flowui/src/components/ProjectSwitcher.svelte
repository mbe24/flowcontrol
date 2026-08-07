<script lang="ts">
  import { app, archiveProject, load } from '../lib/state.svelte';

  const active = $derived(app.projects.filter((p) => !p.archived));
  const archived = $derived(app.projects.filter((p) => p.archived));
  let query = $state('');

  const shown = $derived(
    (app.showArchived ? app.projects : active).filter((p) =>
      p.name.toLowerCase().includes(query.trim().toLowerCase())
    )
  );

  const abbr = (name: string) =>
    name.split(/\s+/).slice(0, 2).map((w) => w[0]?.toUpperCase() ?? '').join('');

  const HUES = ['auth', 'booking', 'pay', 'obs', 'ui'];
  const hue = (id: string) => `var(--hue-${HUES[app.projects.findIndex((p) => p.id === id) % 5]})`;

  const nodeCount = (id: string) =>
    id === app.projectId ? app.nodes.filter((n) => n.type !== 'WORK_PACKAGE').length : null;
</script>

<div class="scrim" onclick={() => (app.projectMenuOpen = false)} role="presentation"></div>
<div class="pop">
  <div class="search">
    <input bind:value={query} placeholder="Search projects…" />
  </div>
  <div class="list">
    {#each shown as p (p.id)}
      {@const count = nodeCount(p.id)}
      <div class="row" class:on={p.id === app.projectId}>
        <button class="pick" onclick={() => load(p.id)}>
          <span class="tile" style:background={hue(p.id)}>{abbr(p.name)}</span>
          <span class="body">
            <span class="name" class:dim={p.archived}>{p.name}</span>
            <span class="mono meta">
              {count !== null ? `${count} nodes` : p.description || '—'}{p.archived ? ' · archived' : ''}
            </span>
          </span>
        </button>
        <button
          class="more"
          title={p.archived ? 'Restore' : 'Archive'}
          onclick={() => archiveProject(p.id, !p.archived)}>{p.archived ? '↺' : '⌷'}</button>
        <button
          class="more"
          title="Rename"
          onclick={() => {
            app.projectMenuOpen = false;
            app.dialog = { kind: 'editProject', projectId: p.id };
          }}>✎</button>
      </div>
    {/each}
    {#if shown.length === 0}
      <div class="empty">No projects match “{query}”.</div>
    {/if}
  </div>
  <div class="foot">
    {#if archived.length && !app.showArchived}
      <button class="link" onclick={() => (app.showArchived = true)}>
        Show {archived.length} archived
      </button>
    {/if}
    <span class="spacer"></span>
    <button
      class="new"
      onclick={() => {
        app.projectMenuOpen = false;
        app.dialog = { kind: 'newProject' };
      }}>
      <span class="plus">+</span>New project
    </button>
  </div>
</div>

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 30;
  }
  .pop {
    position: absolute;
    left: 58px;
    top: 14px;
    z-index: 31;
    width: 312px;
    border-radius: 11px;
    background: var(--panel);
    border: 1px solid var(--border);
    box-shadow: var(--shadow);
    overflow: hidden;
  }
  .search {
    padding: 11px 13px;
    border-bottom: 1px solid var(--border2);
  }
  .search input {
    width: 100%;
    background: transparent;
    border: 0;
    outline: none;
    color: var(--fg);
    font-family: inherit;
    font-size: 12.5px;
  }
  .search input::placeholder {
    color: var(--fg3);
  }
  .list {
    padding: 6px;
    max-height: 320px;
    overflow: auto;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 2px;
    border-radius: 7px;
    padding-right: 5px;
  }
  .row:hover,
  .row.on {
    background: var(--hover);
  }
  .pick {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 11px;
    min-width: 0;
    padding: 9px 11px;
    border: 0;
    background: transparent;
    cursor: pointer;
    font-family: inherit;
    text-align: left;
  }
  .tile {
    width: 24px;
    height: 24px;
    border-radius: 6px;
    color: #0e0f12;
    display: grid;
    place-items: center;
    font-family: 'IBM Plex Mono', monospace;
    font-size: 9.5px;
    font-weight: 600;
    flex: none;
  }
  .body {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .name {
    font-size: 13px;
    color: var(--fg);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .name.dim {
    color: var(--fg3);
  }
  .meta {
    font-size: 9.5px;
    color: var(--fg3);
  }
  .more {
    width: 22px;
    height: 22px;
    border: 0;
    border-radius: 5px;
    background: transparent;
    color: var(--fg3);
    font-size: 11px;
    cursor: pointer;
    flex: none;
  }
  .more:hover {
    background: var(--panel2);
    color: var(--fg);
  }
  .empty {
    padding: 12px;
    font-size: 12px;
    color: var(--fg3);
  }
  .foot {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 10px;
    border-top: 1px solid var(--border2);
  }
  .spacer {
    flex: 1;
  }
  .link,
  .new {
    background: transparent;
    border: 0;
    color: var(--accent);
    font-family: inherit;
    font-size: 12.5px;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 6px;
    border-radius: 6px;
  }
  .link {
    color: var(--fg3);
  }
  .link:hover {
    color: var(--fg2);
  }
  .new:hover {
    background: var(--hover);
  }
  .plus {
    width: 20px;
    height: 20px;
    border-radius: 5px;
    border: 1px dashed var(--border);
    display: grid;
    place-items: center;
    font-size: 12px;
  }

  @media (max-width: 860px) {
    .pop {
      left: 8px;
      right: 8px;
      top: 8px;
      width: auto;
    }
    .pick,
    .new,
    .link {
      min-height: 44px;
    }
    .more {
      width: 34px;
      height: 34px;
    }
    .search input {
      font-size: 16px;
    }
  }
</style>
