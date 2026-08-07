<script lang="ts">
  import {
    app,
    clearFilters,
    toggleVerFilter,
    toggleWpFilter,
    activeFilterCount
  } from '../lib/state.svelte';
  import { workPackages, tasksOf } from '../lib/derive';

  const wps = $derived(workPackages(app.nodes));
  const mobile = $derived(app.width < 860);
  const tasks = $derived(app.nodes.filter((n) => n.type === 'TASK'));

  const buckets = [
    { key: 'verified', label: 'Verified', glyph: '✓', hue: 'var(--ready)' },
    { key: 'failed', label: 'Agent reported failure', glyph: '✕', hue: 'var(--blocked)' },
    { key: 'stale', label: 'Report stale', glyph: '◷', hue: 'var(--deferred)' },
    { key: 'none', label: 'Never verified', glyph: '–', hue: 'var(--fg3)' }
  ];

  function bucketOf(v: (typeof tasks)[number]['verification']) {
    if (!v) return 'none';
    if (v.human === 'accepted' || v.agent === 'pass') return 'verified';
    if (v.agent === 'fail') return 'failed';
    if (v.agent === 'stale') return 'stale';
    return 'none';
  }
  const bucketCount = (key: string) => tasks.filter((t) => bucketOf(t.verification) === key).length;
</script>

<div class="scrim" onclick={() => (app.filterOpen = false)} role="presentation"></div>
<div class="pop" class:sheet={mobile}>
  <div class="group">
    <span class="label">Work package</span>
    {#each wps as w (w.id)}
      <label class="row">
        <input type="checkbox" checked={app.wpFilter.includes(w.id)} onchange={() => toggleWpFilter(w.id)} />
        <span class="hue" style:background="var(--hue-{['auth', 'booking', 'pay', 'obs', 'ui'][wps.indexOf(w) % 5]})"></span>
        <span class="name">{w.title}</span>
        <span class="mono count">{tasksOf(app.nodes, w.id).length}</span>
      </label>
    {/each}
  </div>

  <div class="sep"></div>

  <div class="group">
    <span class="label">Verification</span>
    {#each buckets as b (b.key)}
      <label class="row">
        <input type="checkbox" checked={app.verFilter.includes(b.key)} onchange={() => toggleVerFilter(b.key)} />
        <span class="mono glyph" style:color={b.hue}>{b.glyph}</span>
        <span class="name">{b.label}</span>
        <span class="mono count">{bucketCount(b.key)}</span>
      </label>
    {/each}
  </div>

  <div class="sep"></div>

  <div class="group">
    <span class="label">Show</span>
    <label class="row">
      <input type="checkbox" bind:checked={app.showArchived} />
      <span class="name wide">Archived work packages</span>
    </label>
    <label class="row">
      <input type="checkbox" bind:checked={app.showSteps} />
      <span class="name wide">Steps in the table</span>
    </label>
  </div>

  <div class="foot">
    <span class="fine">{activeFilterCount()} {activeFilterCount() === 1 ? 'filter' : 'filters'} active</span>
    <span class="spacer"></span>
    <button onclick={clearFilters}>Clear all</button>
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
    right: 18px;
    top: 50px;
    z-index: 31;
    width: 320px;
    border-radius: 11px;
    background: var(--panel);
    border: 1px solid var(--border);
    box-shadow: var(--shadow);
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 15px;
    max-height: 70vh;
    overflow: auto;
  }
  .group {
    display: flex;
    flex-direction: column;
    gap: 9px;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 10px;
    cursor: pointer;
  }
  input {
    accent-color: var(--accent);
    margin: 0;
  }
  .hue {
    width: 3px;
    height: 12px;
    border-radius: 2px;
    flex: none;
  }
  .glyph {
    width: 12px;
    font-size: 10px;
    flex: none;
  }
  .name {
    flex: 1;
    font-size: 12.5px;
    color: var(--fg2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .count {
    font-size: 10px;
    color: var(--fg3);
  }
  .sep {
    height: 1px;
    background: var(--border2);
  }
  .foot {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .fine {
    font-size: 11.5px;
    color: var(--fg3);
  }
  .spacer {
    flex: 1;
  }
  button {
    background: transparent;
    border: 0;
    color: var(--accent);
    font-family: inherit;
    font-size: 12px;
    cursor: pointer;
    padding: 0;
  }
  .pop.sheet {
    left: 0;
    right: 0;
    bottom: 0;
    top: auto;
    width: auto;
    max-height: 78vh;
    border-radius: 16px 16px 0 0;
    padding: 18px 16px calc(16px + env(safe-area-inset-bottom));
  }
  .pop.sheet .row {
    min-height: 44px;
  }
  .pop.sheet .name {
    font-size: 15px;
  }
  .pop.sheet input {
    width: 20px;
    height: 20px;
  }
  .pop.sheet button {
    min-height: 44px;
    font-size: 15px;
  }
</style>
