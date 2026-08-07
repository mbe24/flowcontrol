<script lang="ts">
  import { app, clearFilters, activeFilterCount } from '../lib/state.svelte';
  import { ALL_STATUSES, STATUS_VAR } from '../lib/types';

  interface Props {
    /** 'project' = nothing exists yet · 'filtered' = everything is hidden. */
    kind: 'project' | 'filtered';
  }
  let { kind }: Props = $props();
</script>

<div class="empty">
  {#if kind === 'project'}
    <div class="mark">
      <span class="edge a"></span>
      <span class="edge b"></span>
      <span class="node fill"></span>
      <span class="node ring t"></span>
      <span class="node ring b2"></span>
    </div>
    <span class="h">Nothing here yet</span>
    <p>Work packages group tasks. Start with one.</p>
    <button
      class="primary"
      onclick={() => (app.dialog = { kind: 'create', nodeType: 'WORK_PACKAGE', parentId: null, title: '' })}>
      New work package
    </button>
  {:else}
    <span class="glyph">⊘</span>
    <span class="h">No tasks match</span>
    <div class="chips">
      {#each app.statusFilter as s}
        <span class="chip" style:--hue={STATUS_VAR[s]}>{s} ✕</span>
      {/each}
      {#each app.wpFilter as id}
        <span class="chip">{app.nodes.find((n) => n.id === id)?.title ?? id} ✕</span>
      {/each}
      {#each app.verFilter as v}
        <span class="chip">{v} ✕</span>
      {/each}
    </div>
    <!-- Never offer "create" here: the new node would vanish behind the filter. -->
    <button class="link" onclick={clearFilters}>Clear all filters</button>
  {/if}
</div>

<style>
  .empty {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 14px;
    padding: 40px 26px;
    min-height: 0;
  }
  .mark {
    position: relative;
    width: 52px;
    height: 52px;
    opacity: 0.55;
  }
  .edge {
    position: absolute;
    left: 18.1px;
    width: 17.9px;
    height: 2.4px;
    background: var(--accent);
    transform-origin: left center;
  }
  .edge.a {
    top: 22.2px;
    transform: rotate(-26.57deg);
  }
  .edge.b {
    top: 27.3px;
    transform: rotate(26.57deg);
  }
  .node {
    position: absolute;
    border-radius: 50%;
    box-sizing: border-box;
  }
  .fill {
    left: 6px;
    top: 20px;
    width: 13px;
    height: 13px;
    background: var(--accent);
  }
  .ring {
    left: 33px;
    width: 13px;
    height: 13px;
    border: 2.2px solid var(--accent);
  }
  .ring.t {
    top: 7px;
  }
  .ring.b2 {
    top: 33px;
  }
  .glyph {
    font-size: 24px;
    color: var(--fg3);
  }
  .h {
    font-size: 15px;
    font-weight: 600;
  }
  p {
    margin: 0;
    font-size: 12.5px;
    line-height: 1.5;
    color: var(--fg2);
    text-align: center;
    max-width: 280px;
    text-wrap: pretty;
  }
  .chips {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
    justify-content: center;
  }
  .chip {
    font-family: 'IBM Plex Mono', monospace;
    font-size: 10px;
    padding: 4px 8px;
    border-radius: 12px;
    border: 1px solid var(--hue, var(--border));
    color: var(--hue, var(--fg2));
  }
  .primary {
    padding: 8px 15px;
    border-radius: 8px;
    border: 0;
    background: var(--accent);
    color: var(--accent-fg);
    font-family: inherit;
    font-size: 12.5px;
    font-weight: 500;
    cursor: pointer;
  }
  .link {
    background: transparent;
    border: 0;
    color: var(--accent);
    font-family: inherit;
    font-size: 12.5px;
    cursor: pointer;
  }
</style>
