<script lang="ts">
  import { app, select } from '../lib/state.svelte';
  import { buildIndex, hueOf, stepsOf } from '../lib/derive';
  import { ALL_STATUSES, STATUS_VAR } from '../lib/types';

  const index = $derived(buildIndex(app.nodes, app.deps));
  const tasks = $derived(app.nodes.filter((n) => n.type === 'TASK'));
  const wpName = (parentId: string | null) => (parentId ? index.byId.get(parentId)?.title ?? '' : '');
</script>

<div class="lanes">
  {#each ALL_STATUSES as status}
    {@const cards = tasks.filter((t) => t.status === status)}
    <div class="lane">
      <div class="lanehead" style:--hue={STATUS_VAR[status]}>
        <span class="dot"></span>
        <span class="name">{status}</span>
        <span class="mono count">{cards.length}</span>
      </div>
      <div class="cards">
        {#each cards as t (t.id)}
          {@const hue = hueOf(app.nodes, t.parentId ?? '')}
          {@const steps = stepsOf(app.nodes, t.id)}
          <button
            class="card"
            class:selected={app.selectedId === t.id}
            style:--hue={hue}
            onclick={() => select(t.id)}>
            <div class="meta">
              <span class="mono id">{t.id}</span>
              <span class="wp ellipsis">{wpName(t.parentId)}</span>
            </div>
            <div class="title" style:color={status === 'DONE' ? 'var(--fg2)' : 'var(--fg)'}>{t.title}</div>
            <div class="foot">
              {#each index.blockers.get(t.id) ?? [] as b}
                {@const bn = index.byId.get(b)}
                <span class="mono bchip">
                  <span class="tinydot" style:background={bn ? STATUS_VAR[bn.status] : 'var(--fg3)'}></span>{b}
                </span>
              {/each}
              <span class="spacer"></span>
              <span class="dots">
                {#each steps as s (s.id)}
                  <span class="sdot" style:background={s.status === 'DONE' ? 'var(--ready)' : 'var(--border)'}></span>
                {/each}
              </span>
            </div>
          </button>
        {/each}
        {#if cards.length === 0}
          <div class="empty">nothing here</div>
        {/if}
      </div>
    </div>
  {/each}
</div>

<style>
  .lanes {
    flex: 1;
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 1px;
    background: var(--border2);
    min-height: 0;
  }
  .lane {
    background: var(--bg);
    display: flex;
    flex-direction: column;
    min-height: 0;
  }
  .lanehead {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 12px 14px 10px;
    border-bottom: 1px solid var(--border2);
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--hue);
  }
  .name {
    font-size: 11px;
    letter-spacing: 0.06em;
    font-weight: 600;
    color: var(--hue);
  }
  .count {
    font-size: 11px;
    color: var(--fg3);
  }
  .cards {
    flex: 1;
    overflow: auto;
    padding: 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .card {
    border: 1px solid var(--border);
    border-left: 2px solid var(--hue);
    border-radius: 7px;
    background: var(--panel2);
    padding: 10px 11px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    cursor: pointer;
    text-align: left;
    font-family: inherit;
  }
  .card:hover {
    border-color: var(--fg3);
  }
  .card.selected {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px color-mix(in oklab, var(--accent) 18%, transparent);
  }
  .meta {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }
  .id {
    font-size: 10px;
    color: var(--fg3);
  }
  .wp {
    font-size: 10px;
    color: var(--hue);
  }
  .title {
    font-size: 12.5px;
    line-height: 1.35;
    text-wrap: pretty;
  }
  .foot {
    display: flex;
    align-items: center;
    gap: 7px;
    flex-wrap: wrap;
  }
  .bchip {
    display: flex;
    align-items: center;
    gap: 5px;
    font-size: 9.5px;
    padding: 2px 6px;
    border-radius: 4px;
    background: var(--chip);
    border: 1px solid var(--border);
    color: var(--fg2);
  }
  .tinydot {
    width: 4px;
    height: 4px;
    border-radius: 50%;
  }
  .spacer {
    flex: 1;
  }
  .dots {
    display: flex;
    gap: 2px;
  }
  .sdot {
    width: 5px;
    height: 5px;
    border-radius: 1px;
  }
  .empty {
    font-size: 11px;
    color: var(--fg3);
    padding: 2px;
  }
</style>
