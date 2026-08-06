<script lang="ts">
  import { app, setPanelMode } from '../lib/state.svelte';
  import { buildIndex, ownerTask } from '../lib/derive';
  import DetailBody from './DetailBody.svelte';

  const index = $derived(buildIndex(app.nodes, app.deps));
  const raw = $derived(app.nodes.find((n) => n.id === app.selectedId));
  const node = $derived(raw ? ownerTask(index, raw) : undefined);
  const wide = $derived(app.panelMode === 'expanded');

  /** Drag the left edge to switch modes — a real resize needs a stored width. */
  let dragStart = $state(0);

  function onDown(e: PointerEvent) {
    dragStart = e.clientX;
    app.dragging = true;
    (e.target as HTMLElement).setPointerCapture(e.pointerId);
  }
  function onMove(e: PointerEvent) {
    if (!app.dragging) return;
    const dx = dragStart - e.clientX;
    if (dx > 90) setPanelMode('expanded');
    if (dx < -90) setPanelMode('peek');
  }
  function onUp(e: PointerEvent) {
    app.dragging = false;
    (e.target as HTMLElement).releasePointerCapture(e.pointerId);
  }
</script>

{#if node}
  <div
    class="grip"
    class:dragging={app.dragging}
    onpointerdown={onDown}
    onpointermove={onMove}
    onpointerup={onUp}
    role="separator"
    aria-orientation="vertical"
    aria-label="Resize panel">
    <span class="bar"></span>
  </div>
  <aside class="panel" class:wide>
    <DetailBody mode={wide ? 'expanded' : 'peek'} {node} />
  </aside>
{/if}

<style>
  .grip {
    width: 5px;
    flex: none;
    background: var(--panel2);
    border-left: 1px solid var(--border);
    display: grid;
    place-items: center;
    cursor: col-resize;
    touch-action: none;
  }
  .grip:hover .bar,
  .grip.dragging .bar {
    background: var(--accent);
  }
  .bar {
    width: 2px;
    height: 26px;
    border-radius: 1px;
    background: var(--border);
  }
  .panel {
    width: 372px;
    flex: none;
    border-left: 1px solid var(--border);
    min-height: 0;
  }
  .panel.wide {
    width: min(52%, 760px);
  }
</style>
