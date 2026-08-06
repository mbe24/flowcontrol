<script lang="ts">
  import { app, closeDetail } from '../lib/state.svelte';
  import { buildIndex, ownerTask, stepsOf, stepRatio } from '../lib/derive';
  import { STATUS_VAR } from '../lib/types';
  import DetailBody from './DetailBody.svelte';

  const index = $derived(buildIndex(app.nodes, app.deps));
  const raw = $derived(app.nodes.find((n) => n.id === app.selectedId));
  const node = $derived(raw ? ownerTask(index, raw) : undefined);
  const steps = $derived(node ? stepsOf(app.nodes, node.id) : []);
  const ratio = $derived(node ? stepRatio(app.nodes, node.id) : { label: '–' });
  const blockers = $derived(node ? index.blockers.get(node.id) ?? [] : []);
  const blocks = $derived(node ? index.blocks.get(node.id) ?? [] : []);
  const full = $derived(app.sheet === 'full');

  let dragY = $state(0);

  function onDown(e: PointerEvent) {
    dragY = e.clientY;
    (e.target as HTMLElement).setPointerCapture(e.pointerId);
  }
  function onMove(e: PointerEvent) {
    if (!dragY) return;
    const dy = dragY - e.clientY;
    if (dy > 60) app.sheet = 'full';
    if (dy < -60) {
      if (app.sheet === 'full') app.sheet = 'peek';
      else closeDetail();
    }
  }
  function onUp(e: PointerEvent) {
    dragY = 0;
    (e.target as HTMLElement).releasePointerCapture(e.pointerId);
  }
</script>

{#if node && app.sheet !== 'closed'}
  <!-- Tap-behind only exists while the sheet is at peek; at full the list is covered. -->
  {#if !full}
    <button class="scrim" onclick={closeDetail} aria-label="Dismiss"></button>
  {/if}

  <div class="sheet" class:full>
    <div
      class="handle"
      onpointerdown={onDown}
      onpointermove={onMove}
      onpointerup={onUp}
      role="button"
      tabindex="0"
      aria-label="Drag to expand">
      <span></span>
    </div>

    {#if full}
      <div class="fullbody">
        <DetailBody mode="sheet" {node} />
      </div>
    {:else}
      <div class="peek">
        <div class="ptitle">
          <span class="dot" style:background={STATUS_VAR[node.status]}></span>
          <span class="h">{node.title}</span>
          <button class="x" onclick={closeDetail} aria-label="Close">✕</button>
        </div>
        <div class="prow">
          <span class="mono pill" style:color={STATUS_VAR[node.status]} style:background="color-mix(in oklab, {STATUS_VAR[node.status]} 14%, transparent)">{node.status}</span>
          {#if steps.length}
            <span class="mono sratio">{ratio.label} steps</span>
            <span class="dots">
              {#each steps as s (s.id)}
                <span class="sdot" style:background={s.status === 'DONE' ? 'var(--ready)' : 'var(--border)'}></span>
              {/each}
            </span>
          {/if}
        </div>
        {#if blockers.length || blocks.length}
          <div class="chips">
            {#each blockers as id}
              {@const o = index.byId.get(id)}
              <span class="mono chip">
                <span class="cdot" style:background={o ? STATUS_VAR[o.status] : 'var(--fg3)'}></span>blocked by {id}
              </span>
            {/each}
            {#each blocks as id}
              {@const o = index.byId.get(id)}
              <span class="mono chip">
                <span class="cdot" style:background={o ? STATUS_VAR[o.status] : 'var(--fg3)'}></span>blocks {id}
              </span>
            {/each}
          </div>
        {/if}
        <button class="promote" onclick={() => (app.sheet = 'full')}>
          <span class="arrow">↑</span> Swipe up for description, steps and status
        </button>
      </div>
    {/if}
  </div>
{/if}

<style>
  .scrim {
    position: absolute;
    inset: 0 0 300px 0;
    background: rgba(0, 0, 0, 0.4);
    border: 0;
    padding: 0;
    z-index: 25;
    cursor: default;
  }
  .sheet {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    height: 300px;
    background: var(--panel);
    border-top: 1px solid var(--border);
    border-radius: 16px 16px 0 0;
    box-shadow: 0 -18px 44px rgba(0, 0, 0, 0.5);
    display: flex;
    flex-direction: column;
    z-index: 26;
    transition: height 0.22s cubic-bezier(0.32, 0.72, 0, 1);
  }
  .sheet.full {
    height: 100%;
    border-radius: 0;
  }
  .handle {
    display: grid;
    place-items: center;
    padding: 9px 0 5px;
    flex: none;
    cursor: grab;
    touch-action: none;
    background: transparent;
    border: 0;
  }
  .handle span {
    width: 34px;
    height: 4px;
    border-radius: 2px;
    background: var(--border);
  }
  .fullbody {
    flex: 1;
    min-height: 0;
  }
  .peek {
    padding: 4px 16px 14px;
    display: flex;
    flex-direction: column;
    gap: 11px;
  }
  .ptitle {
    display: flex;
    align-items: flex-start;
    gap: 10px;
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-top: 6px;
    flex: none;
  }
  .h {
    flex: 1;
    font-size: 16px;
    font-weight: 600;
    line-height: 1.3;
    text-wrap: pretty;
  }
  .x {
    background: transparent;
    border: 0;
    color: var(--fg3);
    font-size: 14px;
    cursor: pointer;
    padding: 2px 0 0;
  }
  .prow {
    display: flex;
    align-items: center;
    gap: 9px;
  }
  .pill {
    font-size: 10px;
    padding: 3px 9px;
    border-radius: 12px;
  }
  .sratio {
    font-size: 10px;
    color: var(--fg2);
  }
  .dots {
    display: flex;
    gap: 2px;
  }
  .sdot {
    width: 6px;
    height: 6px;
    border-radius: 1px;
  }
  .chips {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .chip {
    display: flex;
    align-items: center;
    gap: 5px;
    font-size: 10px;
    padding: 4px 8px;
    border-radius: 5px;
    background: var(--chip);
    border: 1px solid var(--border);
    color: var(--fg2);
  }
  .cdot {
    width: 4px;
    height: 4px;
    border-radius: 50%;
  }
  .promote {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 9px 0 2px;
    margin-top: 1px;
    border: 0;
    border-top: 1px solid var(--border2);
    background: transparent;
    color: var(--fg3);
    font-size: 11px;
    font-family: inherit;
    cursor: pointer;
  }
  .arrow {
    font-size: 12px;
  }
</style>
