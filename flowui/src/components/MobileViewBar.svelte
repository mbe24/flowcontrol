<script lang="ts">
  import { app } from '../lib/state.svelte';
  import type { ViewName } from '../lib/state.svelte';

  /**
   * The three views, moved out of the header on mobile.
   *
   * The header can't hold them: project name, search, filter and theme already
   * fill 390px, which is why the tabs ended up behind `{#if !mobile}` and the
   * lanes and graph views became unreachable on a phone.
   *
   * A bottom bar is the right home for them — always visible, thumb-reachable,
   * and it costs the header nothing. It sits under the detail sheet by
   * z-index, so opening a task covers it rather than fighting it.
   */
  const views: { id: ViewName; label: string; glyph: string }[] = [
    { id: 'table', label: 'Table', glyph: '▤' },
    { id: 'lanes', label: 'Lanes', glyph: '▥' },
    { id: 'graph', label: 'Graph', glyph: '⁘' }
  ];
</script>

<nav class="bar" aria-label="Views">
  {#each views as v (v.id)}
    <button
      class="tab"
      class:on={app.view === v.id}
      aria-current={app.view === v.id ? 'page' : undefined}
      onclick={() => (app.view = v.id)}>
      <span class="glyph">{v.glyph}</span>
      <span class="label">{v.label}</span>
    </button>
  {/each}
</nav>

<style>
  .bar {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 20;
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    background: var(--panel);
    border-top: 1px solid var(--border);
    padding-bottom: env(safe-area-inset-bottom);
  }
  .tab {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 3px;
    min-height: 52px;
    border: 0;
    background: transparent;
    color: var(--fg3);
    font-family: inherit;
    cursor: pointer;
    /* The active marker is a top rule rather than a fill, so the bar stays
       quiet against the content scrolling behind it. */
    box-shadow: inset 0 2px 0 transparent;
  }
  .tab.on {
    color: var(--accent);
    box-shadow: inset 0 2px 0 var(--accent);
  }
  .glyph {
    font-size: 15px;
    line-height: 1;
  }
  .label {
    font-size: 11px;
    letter-spacing: 0.02em;
  }
</style>