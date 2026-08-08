<script lang="ts">
  import { app, select, passesAll } from '../lib/state.svelte';
  import EmptyState from './EmptyState.svelte';
  import { buildIndex, hueOf, stepsOf } from '../lib/derive';
  import { ALL_STATUSES, STATUS_VAR, verifyBadge } from '../lib/types';

  const index = $derived(buildIndex(app.nodes, app.deps));
  const allTasks = $derived(app.nodes.filter((n) => n.type === 'TASK'));
  const tasks = $derived(allTasks.filter(passesAll));
  const emptyProject = $derived(app.nodes.filter((n) => n.type === 'WORK_PACKAGE').length === 0);
  const filteredOut = $derived(!emptyProject && allTasks.length > 0 && tasks.length === 0);
  const mobile = $derived(app.width < 860);
  /** On mobile the lanes stack behind a swipeable tab strip — one at a time. */
  const shown = $derived(mobile ? [ALL_STATUSES[app.laneIndex]] : ALL_STATUSES);
  const wpName = (parentId: string | null) => (parentId ? index.byId.get(parentId)?.title ?? '' : '');
  const countOf = (s: string) => tasks.filter((t) => t.status === s).length;

  let startX = 0;
  function down(e: PointerEvent) {
    startX = e.clientX;
  }
  function up(e: PointerEvent) {
    if (!mobile) return;
    const dx = e.clientX - startX;
    if (dx < -60) app.laneIndex = Math.min(app.laneIndex + 1, ALL_STATUSES.length - 1);
    if (dx > 60) app.laneIndex = Math.max(app.laneIndex - 1, 0);
  }
</script>

{#if emptyProject}
  <EmptyState kind="project" />
{:else if filteredOut}
  <EmptyState kind="filtered" />
{:else}
{#if mobile}
  <div class="strip">
    {#each ALL_STATUSES as s, i}
      <button
        class="tab"
        class:on={i === app.laneIndex}
        style:--hue={STATUS_VAR[s]}
        onclick={() => (app.laneIndex = i)}>
        <span class="tdot"></span>{s[0] + s.slice(1).toLowerCase()}
        <span class="mono tcount">{countOf(s)}</span>
      </button>
    {/each}
  </div>
{/if}

<div class="lanes" class:single={mobile} onpointerdown={down} onpointerup={up} role="presentation">
  {#each shown as status (status)}
    {@const cards = tasks.filter((t) => t.status === status)}
    <div class="lane">
      {#if !mobile}
        <div class="lanehead" style:--hue={STATUS_VAR[status]}>
          <span class="dot"></span>
          <span class="name">{status}</span>
          <span class="mono count">{cards.length}</span>
        </div>
      {/if}
      <div class="cards">
        {#each cards as t (t.id)}
          {@const hue = hueOf(app.nodes, t.parentId ?? '')}
          {@const steps = stepsOf(app.nodes, t.id)}
          {@const badge = verifyBadge(t.verification)}
          <button
            class="card"
            class:selected={app.selectedId === t.id}
            style:--hue={hue}
            oncontextmenu={(e) => {
              e.preventDefault();
              app.menuAt = { x: Math.min(e.clientX, window.innerWidth - 250), y: e.clientY };
              app.nodeMenuFor = t.id;
            }}
            onclick={() => select(t.id)}>
            <div class="meta">
              <span class="mono id">{t.id}</span>
              <span class="wp">{wpName(t.parentId)}</span>
              <span class="mono ver" style:color={badge.color}>{badge.glyph}</span>
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

{#if mobile}
  <div class="pager">
    {#each ALL_STATUSES as _, i}
      <span class="pdot" class:on={i === app.laneIndex}></span>
    {/each}
  </div>
{/if}
{/if}

<style>
  .strip {
    flex: none;
    display: flex;
    gap: 6px;
    padding: 9px 10px;
    overflow-x: auto;
    border-bottom: 1px solid var(--border2);
  }
  .tab {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: none;
    padding: 6px 11px;
    border-radius: 15px;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--fg2);
    font-family: inherit;
    font-size: 11.5px;
    cursor: pointer;
  }
  .tab.on {
    border-color: var(--hue);
    color: var(--hue);
    background: color-mix(in oklab, var(--hue) 12%, transparent);
  }
  .tdot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: var(--hue);
  }
  .tcount {
    font-size: 9.5px;
    opacity: 0.7;
  }
  .lanes {
    flex: 1;
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 1px;
    background: var(--border2);
    min-height: 0;
  }
  .lanes.single {
    grid-template-columns: 1fr;
    background: transparent;
  }
  .lane {
    background: var(--bg);
    display: flex;
    flex-direction: column;
    min-height: 0;
    min-width: 0;
  }
  .lanehead {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 12px 14px 10px;
    border-bottom: 1px solid var(--border2);
    flex: none;
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
  /* Each lane scrolls independently — the whole board never scrolls as one. */
  .cards {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
    padding: 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    min-height: 0;
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
    flex: none;
  }
  .card:hover {
    border-color: var(--fg3);
    border-left-color: var(--hue);
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
    flex: none;
  }
  .wp {
    font-size: 10px;
    color: var(--hue);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .ver {
    margin-left: auto;
    font-size: 9.5px;
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
  .pager {
    flex: none;
    display: flex;
    justify-content: center;
    gap: 6px;
    padding: 9px 0 12px;
  }
  .pdot {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: var(--border);
  }
  .pdot.on {
    background: var(--accent);
  }
  @media (max-width: 860px) {
    .tab {
      min-height: 44px;
    }
    .card {
      min-height: 64px;
    }
  }
</style>
