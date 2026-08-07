<script lang="ts">
  import { app, addDependency, removeDependency } from '../lib/state.svelte';
  import { buildIndex, wouldCycle } from '../lib/derive';
  import { STATUS_VAR } from '../lib/types';
  import type { FlowNode } from '../lib/types';

  interface Props {
    node: FlowNode;
  }
  let { node }: Props = $props();

  const index = $derived(buildIndex(app.nodes, app.deps));
  const blockers = $derived(index.blockers.get(node.id) ?? []);
  const blocks = $derived(index.blocks.get(node.id) ?? []);

  let query = $state('');
  let open = $state(false);

  /**
   * The picker is the complete surface: the graph only shows expanded packages,
   * so a cross-package edge to a collapsed box is undraggable by construction —
   * and that is exactly the edge you most often need to add.
   */
  const candidates = $derived.by(() => {
    const q = query.trim().toLowerCase();
    if (!q) return [];
    return app.nodes
      .filter((n) => n.id !== node.id && n.type !== 'STEP')
      .filter((n) => n.title.toLowerCase().includes(q) || n.id.toLowerCase().includes(q))
      .filter((n) => !blockers.includes(n.id))
      .slice(0, 6)
      .map((n) => ({ node: n, cycles: wouldCycle(index, n.id, node.id) }));
  });

  const wpOf = (n: FlowNode) => (n.parentId ? index.byId.get(n.parentId)?.title ?? '' : '');

  async function add(id: string) {
    query = '';
    open = false;
    await addDependency(id, node.id);
  }
</script>

<section>
  <span class="label">Dependencies</span>

  {#each blockers as id (id)}
    {@const o = index.byId.get(id)}
    <div class="dep">
      <span class="mono dir">blocked by</span>
      <span class="ddot" style:background={o ? STATUS_VAR[o.status] : 'var(--fg3)'}></span>
      <span class="mono did">{id}</span>
      <span class="dtitle">{o?.title ?? 'outside this project'}</span>
      <button class="x" onclick={() => removeDependency(id, node.id)} aria-label="Remove">✕</button>
    </div>
  {/each}

  {#each blocks as id (id)}
    {@const o = index.byId.get(id)}
    <div class="dep">
      <span class="mono dir">blocks</span>
      <span class="ddot" style:background={o ? STATUS_VAR[o.status] : 'var(--fg3)'}></span>
      <span class="mono did">{id}</span>
      <span class="dtitle">{o?.title ?? 'outside this project'}</span>
      <button class="x" onclick={() => removeDependency(node.id, id)} aria-label="Remove">✕</button>
    </div>
  {/each}

  <div class="adder" class:open>
    <span class="mono dir accent">blocked by</span>
    <input
      bind:value={query}
      placeholder="search a task or package…"
      onfocus={() => (open = true)}
      onblur={() => setTimeout(() => (open = false), 140)}
      onkeydown={(e) => {
        if (e.key === 'Enter' && candidates[0] && !candidates[0].cycles) add(candidates[0].node.id);
        if (e.key === 'Escape') {
          query = '';
          (e.target as HTMLInputElement).blur();
        }
      }} />
  </div>

  {#if open && candidates.length}
    <div class="results">
      {#each candidates as c (c.node.id)}
        <button class="hit" class:dead={c.cycles} disabled={c.cycles} onclick={() => add(c.node.id)}>
          <span class="ddot" style:background={STATUS_VAR[c.node.status]}></span>
          <span class="mono did">{c.node.id}</span>
          <span class="dtitle">{c.node.title}</span>
          {#if c.cycles}
            <span class="cyc">would cycle</span>
          {:else if wpOf(c.node)}
            <span class="wp">{wpOf(c.node)}</span>
          {/if}
        </button>
      {/each}
    </div>
  {/if}

  <span class="fine">↵ add · ✕ remove · works across packages and levels</span>
</section>

<style>
  section {
    display: flex;
    flex-direction: column;
    gap: 7px;
  }
  .dep {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 7px 9px;
    border-radius: 6px;
    background: var(--panel2);
    border: 1px solid var(--border2);
  }
  .dir {
    font-size: 9px;
    color: var(--fg3);
    width: 54px;
    flex: none;
  }
  .dir.accent {
    color: var(--accent);
  }
  .ddot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    flex: none;
  }
  .did {
    font-size: 9.5px;
    color: var(--fg3);
    flex: none;
  }
  .dtitle {
    flex: 1;
    font-size: 10.5px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .x {
    background: transparent;
    border: 0;
    color: var(--fg3);
    font-size: 11px;
    cursor: pointer;
    padding: 0 2px;
    flex: none;
  }
  .x:hover {
    color: var(--blocked);
  }
  .adder {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 9px;
    border-radius: 6px;
    background: var(--panel2);
    border: 1px solid var(--border);
  }
  .adder.open {
    border-color: var(--accent);
  }
  .adder input {
    flex: 1;
    min-width: 0;
    background: transparent;
    border: 0;
    outline: none;
    color: var(--fg);
    font-family: inherit;
    font-size: 11.5px;
  }
  .adder input::placeholder {
    color: var(--fg3);
  }
  .results {
    border-radius: 7px;
    background: var(--panel2);
    border: 1px solid var(--border);
    overflow: hidden;
  }
  .hit {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 8px 10px;
    border: 0;
    background: transparent;
    cursor: pointer;
    font-family: inherit;
    color: var(--fg);
    text-align: left;
  }
  .hit:hover:not(.dead) {
    background: var(--hover);
  }
  .hit.dead {
    opacity: 0.4;
    cursor: default;
  }
  .wp {
    font-size: 9.5px;
    color: var(--hue-booking);
    flex: none;
  }
  .cyc {
    font-size: 9.5px;
    color: var(--blocked);
    flex: none;
  }
  .fine {
    font-size: 10px;
    color: var(--fg3);
  }

  @media (max-width: 860px) {
    .dep,
    .adder,
    .hit {
      min-height: 44px;
    }
    .adder input {
      font-size: 16px;
    }
    .x {
      padding: 0 8px;
    }
  }
</style>
