<script lang="ts">
  import { app, toggleFilter } from '../lib/state.svelte';
  import { projectCounts, workPackages } from '../lib/derive';
  import { ALL_STATUSES, STATUS_VAR } from '../lib/types';

  const counts = $derived(projectCounts(app.nodes));
  const wps = $derived(workPackages(app.nodes));
  const active = $derived(wps.filter((w) => w.state === 'ACTIVE').length);
  const doneWps = $derived(wps.filter((w) => w.state === 'DONE' || w.state === 'ARCHIVED').length);

  const countFor = (s: string) =>
    s === 'READY' ? counts.ready : s === 'BLOCKED' ? counts.blocked : s === 'DEFERRED' ? counts.deferred : counts.done;
</script>

<div class="bar">
  {#each ALL_STATUSES as s}
    <button
      class="chip"
      class:on={app.statusFilter.includes(s)}
      style:--hue={STATUS_VAR[s]}
      onclick={() => toggleFilter(s)}>
      <span class="dot"></span>
      {s[0] + s.slice(1).toLowerCase()} {countFor(s)}
    </button>
  {/each}
  <div class="spacer"></div>
  <span class="note">
    {active} of {wps.length} work packages active · {doneWps} done, auto-collapsed
  </span>
</div>

<style>
  .bar {
    height: 38px;
    flex: none;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 18px;
    border-bottom: 1px solid var(--border2);
  }
  .chip {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 3px 9px;
    border-radius: 14px;
    font-size: 11px;
    font-family: inherit;
    cursor: pointer;
    background: transparent;
    border: 1px solid var(--border);
    color: var(--hue);
  }
  .chip:hover {
    border-color: var(--hue);
  }
  .chip.on {
    border-color: var(--hue);
    background: color-mix(in oklab, var(--hue) 14%, transparent);
  }
  .dot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: var(--hue);
  }
  .spacer {
    flex: 1;
  }
  .note {
    font-size: 11px;
    color: var(--fg3);
  }
</style>
