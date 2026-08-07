<script lang="ts">
  import { app } from '../lib/state.svelte';
  import { stepsOf, tasksOf, workPackages } from '../lib/derive';
  import type { FlowNode, NodeType } from '../lib/types';

  interface Props {
    node: FlowNode;
    /** Anchor position in viewport coordinates. */
    x: number;
    y: number;
    onclose: () => void;
  }
  let { node, x, y, onclose }: Props = $props();

  const mobile = $derived(app.width < 860);

  const isTask = $derived(node.type === 'TASK');
  const isStep = $derived(node.type === 'STEP');
  const isWp = $derived(node.type === 'WORK_PACKAGE');
  /** Demoting needs somewhere to land: another task in the same package. */
  const canDemote = $derived(
    isTask && node.parentId ? tasksOf(app.nodes, node.parentId).some((t) => t.id !== node.id) : false
  );

  function act(fn: () => void) {
    onclose();
    fn();
  }

  function create(type: NodeType, parentId: string) {
    app.dialog = { kind: 'create', nodeType: type, parentId, title: '' };
  }
</script>

<div class="scrim" onclick={onclose} oncontextmenu={(e) => { e.preventDefault(); onclose(); }} role="presentation"></div>
<div class="menu" class:sheet={mobile} style:left={mobile ? undefined : `${x}px`} style:top={mobile ? undefined : `${y}px`} role="menu">
  <button onclick={() => act(() => (app.selectedId = node.id))}>
    <span class="mono ic">✎</span>Rename<span class="mono k">F2</span>
  </button>

  {#if isWp}
    <button onclick={() => act(() => create('TASK', node.id))}>
      <span class="mono ic">+</span>Add task
    </button>
  {/if}
  {#if isTask}
    <button onclick={() => act(() => create('STEP', node.id))}>
      <span class="mono ic">+</span>Add step
    </button>
    <button onclick={() => act(() => (app.selectedId = node.id))}>
      <span class="mono ic">⇄</span>Add dependency
    </button>
  {/if}

  <div class="sep"></div>

  {#if isStep}
    <button onclick={() => act(() => (app.dialog = { kind: 'move', nodeId: node.id, to: 'TASK' }))}>
      <span class="mono ic">↑</span>Promote to task<span class="mono k">⇧⇥</span>
    </button>
  {/if}
  {#if isTask}
    <button
      disabled={!canDemote}
      title={canDemote ? '' : 'No other task in this package to demote into'}
      onclick={() => canDemote && act(() => (app.dialog = { kind: 'move', nodeId: node.id, to: 'STEP' }))}>
      <span class="mono ic">↓</span>Demote to step<span class="mono k">⇥</span>
    </button>
    <button onclick={() => act(() => (app.dialog = { kind: 'move', nodeId: node.id, to: 'TASK' }))}>
      <span class="mono ic">↗</span>Move to package…
    </button>
  {/if}

  <div class="sep"></div>

  <button class="danger" onclick={() => act(() => (app.dialog = { kind: 'delete', nodeId: node.id }))}>
    <span class="mono ic">✕</span>Delete<span class="mono k">⌫</span>
  </button>
</div>

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 44;
  }
  .menu {
    position: fixed;
    z-index: 45;
    min-width: 232px;
    border-radius: 10px;
    background: var(--panel);
    border: 1px solid var(--border);
    box-shadow: var(--shadow);
    padding: 6px;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  button {
    display: flex;
    align-items: center;
    gap: 11px;
    width: 100%;
    padding: 8px 11px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--fg);
    font-family: inherit;
    font-size: 13px;
    text-align: left;
    cursor: pointer;
  }
  button:hover:not(:disabled) {
    background: var(--hover);
  }
  button:disabled {
    color: var(--fg3);
    cursor: default;
  }
  .danger {
    color: var(--blocked);
  }
  .danger:hover {
    background: color-mix(in oklab, var(--blocked) 10%, transparent);
  }
  .ic {
    width: 14px;
    font-size: 11px;
    color: var(--fg3);
    flex: none;
  }
  .danger .ic {
    color: var(--blocked);
  }
  .k {
    margin-left: auto;
    font-size: 10px;
    color: var(--fg3);
  }
  .sep {
    height: 1px;
    background: var(--border2);
    margin: 4px 6px;
  }
  .menu.sheet {
    left: 0;
    right: 0;
    bottom: 0;
    top: auto;
    min-width: 0;
    border-radius: 16px 16px 0 0;
    padding: 8px 8px calc(8px + env(safe-area-inset-bottom));
  }
  .menu.sheet button {
    min-height: 48px;
    font-size: 15px;
  }
  @media (max-width: 860px) {
    .scrim {
      background: rgba(0, 0, 0, 0.4);
    }
  }
</style>
