<script lang="ts">
  import {
    app,
    select,
    setStatus,
    toggleTask,
    toggleWp,
    passesFilter,
  } from '../lib/state.svelte';
  import {
    buildIndex,
    stepRatio,
    stepsOf,
    tasksOf,
    workPackages,
    wpCounts,
  } from '../lib/derive';
  import { STATUS_VAR, stepGlyph, verifyBadge } from '../lib/types';
  import type { FlowNode, Status } from '../lib/types';

  const index = $derived(buildIndex(app.nodes, app.deps));
  const wps = $derived(
    workPackages(app.nodes).filter(
      (w) => app.showArchived || (w.state !== 'DONE' && w.state !== 'ARCHIVED'),
    ),
  );
  const hidden = $derived(
    workPackages(app.nodes).filter(
      (w) => w.state === 'DONE' || w.state === 'ARCHIVED',
    ).length,
  );
  const mobile = $derived(app.width < 860);
  const narrow = $derived(
    app.panelMode === 'expanded' && !!app.selectedId && !mobile,
  );

  const pct = (n: number, total: number) => (total ? (n / total) * 100 : 0);

  /** Swipe a row left/right to set status without opening anything. */
  let swipeId = $state('');
  let swipeX = $state(0);
  let startX = 0;

  function down(e: PointerEvent, id: string) {
    if (!mobile) return;
    startX = e.clientX;
    swipeId = id;
  }
  function move(e: PointerEvent) {
    if (!swipeId) return;
    swipeX = e.clientX - startX;
  }
  function up(t: FlowNode) {
    const dx = swipeX;
    swipeId = '';
    swipeX = 0;
    if (Math.abs(dx) < 70) return;
    const next: Status =
      dx > 0 ? 'DONE' : t.status === 'BLOCKED' ? 'READY' : 'BLOCKED';
    setStatus(t.id, next);
  }
</script>

{#if mobile}
  <div class="cards">
    {#each wps as wp (wp.id)}
      {@const counts = wpCounts(app.nodes, wp.id)}
      <div class="cardwp">
        <button class="wphead" onclick={() => toggleWp(wp.id)}>
          <span class="caret">{app.expandedWp[wp.id] ? '▾' : '▸'}</span>
          <span class="wpname">{wp.title}</span>
          <span class="mono wppct">{counts.pct}%</span>
        </button>
        <div class="track thin">
          <div
            style:width="{pct(counts.done, counts.total)}%"
            style:background="var(--done)"
          ></div>
          <div
            style:width="{pct(counts.ready, counts.total)}%"
            style:background="var(--ready)"
          ></div>
          <div
            style:width="{pct(counts.blocked, counts.total)}%"
            style:background="var(--blocked)"
          ></div>
          <div
            style:width="{pct(counts.deferred, counts.total)}%"
            style:background="var(--deferred)"
          ></div>
        </div>
      </div>
      {#if app.expandedWp[wp.id]}
        {#each tasksOf(app.nodes, wp.id).filter( (t) => passesFilter(t.status) ) as t (t.id)}
          {@const ratio = stepRatio(app.nodes, t.id)}
          {@const badge = verifyBadge(t.verification)}
          <button
            class="card"
            class:sel={app.selectedId === t.id}
            style:transform={swipeId === t.id
              ? `translateX(${swipeX}px)`
              : 'none'}
            onpointerdown={(e) => down(e, t.id)}
            onpointermove={move}
            onpointerup={() => up(t)}
            onpointercancel={() => (swipeId = '')}
            onclick={() => select(t.id)}
          >
            <div class="cmeta">
              <span class="cdot" style:background={STATUS_VAR[t.status]}></span>
              <span class="mono cid">{t.id}</span>
              <span class="mono cver" style:color={badge.color}
                >{badge.glyph}</span
              >
              <span class="mono csteps">{ratio.label}</span>
            </div>
            <span
              class="ctitle"
              style:color={t.status === 'DONE' ? 'var(--fg2)' : 'var(--fg)'}
              >{t.title}</span
            >
            {#if (index.blockers.get(t.id) ?? []).length}
              <div class="cchips">
                {#each index.blockers.get(t.id) ?? [] as b}
                  {@const bn = index.byId.get(b)}
                  <span class="mono cchip">
                    <span
                      class="tinydot"
                      style:background={bn
                        ? STATUS_VAR[bn.status]
                        : 'var(--fg3)'}
                    ></span>{b}
                  </span>
                {/each}
              </div>
            {/if}
          </button>
        {/each}
      {/if}
    {/each}
    <p class="hint">Swipe a card right to mark DONE, left to toggle BLOCKED.</p>
  </div>
{:else if narrow}
  <!-- Expanded panel: the list becomes a rail of dot + id + title, grouped by package. -->
  <div class="rail">
    {#each wps as wp (wp.id)}
      <div class="railhead">
        <span
          class="hue"
          style:background={STATUS_VAR[
            wp.state === 'ACTIVE' ? 'READY' : 'DEFERRED'
          ]}
        ></span>
        <span class="rname">{wp.title}</span>
        <span class="mono rpct">{wpCounts(app.nodes, wp.id).pct}%</span>
      </div>
      {#each tasksOf(app.nodes, wp.id).filter( (t) => passesFilter(t.status) ) as t (t.id)}
        <button
          class="railrow"
          class:sel={app.selectedId === t.id}
          onclick={() => select(t.id)}
        >
          <span class="rdot" style:background={STATUS_VAR[t.status]}></span>
          <span class="mono rid">{t.id}</span>
          <span class="rtitle">{t.title}</span>
        </button>
      {/each}
    {/each}
  </div>
{:else}
  <div class="head">
    <span></span><span>Node</span><span>Blocked by</span><span>Condition</span
    ><span>Steps</span>
    <span class="right">Status</span>
  </div>

  <div class="scroll">
    {#each wps as wp (wp.id)}
      {@const counts = wpCounts(app.nodes, wp.id)}
      <div
        class="wp"
        role="button"
        tabindex="0"
        onclick={() => toggleWp(wp.id)}
        onkeydown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            toggleWp(wp.id);
          }
        }}
      >
        <span class="caret">{app.expandedWp[wp.id] ? '▾' : '▸'}</span>
        <div class="wpname2">
          <span class="mono tag">WP</span>
          <span class="ellipsis strong">{wp.title}</span>
          <span
            class="mono state"
            style:color={wp.state === 'ACTIVE' ? 'var(--ready)' : 'var(--fg2)'}
            style:background={wp.state === 'ACTIVE'
              ? 'var(--ready-bg)'
              : 'var(--chip)'}>{wp.state}</span
          >
        </div>
        <div class="wpbar">
          <div class="track">
            <div
              style:width="{pct(counts.done, counts.total)}%"
              style:background="var(--done)"
            ></div>
            <div
              style:width="{pct(counts.ready, counts.total)}%"
              style:background="var(--ready)"
            ></div>
            <div
              style:width="{pct(counts.blocked, counts.total)}%"
              style:background="var(--blocked)"
            ></div>
            <div
              style:width="{pct(counts.deferred, counts.total)}%"
              style:background="var(--deferred)"
            ></div>
          </div>
          <span class="mono ratio">{counts.done}/{counts.total}</span>
          <span class="mono pctcell">{counts.pct}%</span>
        </div>
      </div>

      {#if app.expandedWp[wp.id]}
        {#each tasksOf(app.nodes, wp.id).filter( (t) => passesFilter(t.status) ) as t (t.id)}
          {@const ratio = stepRatio(app.nodes, t.id)}
          {@const badge = verifyBadge(t.verification)}
          {@const blockers = index.blockers.get(t.id) ?? []}
          <div
            class="row"
            class:selected={app.selectedId === t.id}
            role="button"
            tabindex="0"
            onclick={() => select(t.id)}
            onkeydown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                select(t.id);
              }
            }}
          >
            <span
              class="caret sub"
              role="button"
              tabindex="-1"
              onclick={(e) => {
                e.stopPropagation();
                toggleTask(t.id);
              }}
              onkeydown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  toggleTask(t.id);
                }
              }}
              >{stepsOf(app.nodes, t.id).length
                ? app.expandedTask[t.id]
                  ? '▾'
                  : '▸'
                : ''}</span
            >
            <div class="node">
              <span class="dot" style:background={STATUS_VAR[t.status]}></span>
              <span class="mono nid">{t.id}</span>
              <span
                class="ellipsis"
                style:color={t.status === 'DONE' ? 'var(--fg3)' : 'var(--fg)'}
                >{t.title}</span
              >
            </div>
            <div class="blockers">
              {#each blockers as b}
                {@const bn = index.byId.get(b)}
                <span class="mono bchip">
                  <span
                    class="tinydot"
                    style:background={bn ? STATUS_VAR[bn.status] : 'var(--fg3)'}
                  ></span>{b}
                </span>
              {/each}
            </div>
            <div class="cond" title={badge.label}>
              <span class="mono vglyph" style:color={badge.color}
                >{badge.glyph}</span
              >
              <span class="mono ellipsis">{t.condition || '—'}</span>
            </div>
            <div class="steps">
              <div class="dots">
                {#each stepsOf(app.nodes, t.id) as s (s.id)}
                  <span
                    class="sdot"
                    style:background={s.status === 'DONE'
                      ? 'var(--ready)'
                      : 'var(--border)'}
                  ></span>
                {/each}
              </div>
              <span class="mono ratio">{ratio.label}</span>
            </div>
            <span class="mono status" style:color={STATUS_VAR[t.status]}
              >{t.status}</span
            >
          </div>

          {#if app.expandedTask[t.id]}
            {#each stepsOf(app.nodes, t.id) as s (s.id)}
              <div class="steprow">
                <span></span>
                <div class="stepname">
                  <span class="mono sglyph" style:color={STATUS_VAR[s.status]}
                    >{stepGlyph(s.status)}</span
                  >
                  <span
                    class="ellipsis"
                    style:color={s.status === 'DONE'
                      ? 'var(--fg3)'
                      : 'var(--fg2)'}>{s.title}</span
                  >
                </div>
                <span></span>
                <span class="mono scond ellipsis">{s.condition || ''}</span>
                <span></span>
                <span class="mono sstatus">{s.status}</span>
              </div>
            {/each}
          {/if}
        {/each}
      {/if}
    {/each}

    {#if hidden > 0}
      <button
        class="archived"
        onclick={() => (app.showArchived = !app.showArchived)}
      >
        <span class="caret">{app.showArchived ? '▾' : '▸'}</span>
        {hidden} completed work {hidden === 1 ? 'package' : 'packages'}
        <span class="mono done">100%</span>
      </button>
    {/if}
  </div>
{/if}

<style>
  .head,
  .wp,
  .row,
  .steprow {
    display: grid;
    grid-template-columns: 22px 1fr 190px 200px 74px 84px;
    align-items: center;
    padding: 0 18px;
  }
  .head {
    height: 30px;
    flex: none;
    border-bottom: 1px solid var(--border2);
    font-size: 10.5px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--fg3);
  }
  .right {
    text-align: right;
  }
  .scroll {
    flex: 1;
    overflow: auto;
  }
  .ellipsis {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .wp {
    height: 44px;
    background: var(--panel);
    border-bottom: 1px solid var(--border2);
    cursor: pointer;
    position: sticky;
    top: 0;
    z-index: 1;
  }
  .wp:hover {
    background: var(--hover);
  }
  .caret {
    color: var(--fg2);
    font-size: 10px;
  }
  .caret.sub {
    color: var(--fg3);
    padding-left: 8px;
  }
  .wpname2 {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
  }
  .tag {
    font-size: 10px;
    color: var(--fg3);
    border: 1px solid var(--border);
    border-radius: 3px;
    padding: 1px 4px;
  }
  .strong {
    font-weight: 600;
    font-size: 13px;
  }
  .state {
    font-size: 10px;
    padding: 1px 6px;
    border-radius: 3px;
  }
  .wpbar {
    grid-column: 3 / span 4;
    display: flex;
    align-items: center;
    gap: 12px;
    padding-right: 14px;
  }
  .track {
    flex: 1;
    height: 6px;
    border-radius: 3px;
    overflow: hidden;
    display: flex;
    background: var(--track);
  }
  .track.thin {
    height: 5px;
  }
  .ratio {
    font-size: 11px;
    color: var(--fg2);
  }
  .pctcell {
    font-size: 11px;
    color: var(--fg3);
    width: 34px;
    text-align: right;
  }
  .row {
    height: 38px;
    border-bottom: 1px solid var(--border2);
    cursor: pointer;
  }
  .row:hover {
    background: var(--hover);
  }
  .row.selected {
    background: var(--hover);
    box-shadow: inset 2px 0 0 var(--accent);
  }
  .node {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
    padding-left: 8px;
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex: none;
  }
  .nid {
    font-size: 11px;
    color: var(--fg3);
    flex: none;
  }
  .blockers {
    display: flex;
    gap: 5px;
    overflow: hidden;
  }
  .bchip {
    display: flex;
    align-items: center;
    gap: 5px;
    font-size: 10px;
    padding: 2px 6px;
    border-radius: 4px;
    background: var(--chip);
    border: 1px solid var(--border);
    color: var(--fg2);
    white-space: nowrap;
  }
  .tinydot {
    width: 4px;
    height: 4px;
    border-radius: 50%;
  }
  .cond {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    padding-right: 12px;
    font-size: 10.5px;
    color: var(--fg2);
  }
  .vglyph {
    font-size: 10px;
    flex: none;
  }
  .steps {
    display: flex;
    align-items: center;
    gap: 6px;
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
  .status {
    text-align: right;
    font-size: 10px;
    letter-spacing: 0.04em;
  }
  .steprow {
    height: 31px;
    border-bottom: 1px solid var(--border2);
    background: var(--panel2);
  }
  .stepname {
    display: flex;
    align-items: center;
    gap: 10px;
    padding-left: 34px;
    min-width: 0;
    font-size: 12px;
  }
  .sglyph {
    font-size: 11px;
    flex: none;
  }
  .scond {
    font-size: 10px;
    color: var(--fg3);
    padding-right: 12px;
  }
  .sstatus {
    text-align: right;
    font-size: 10px;
    color: var(--fg3);
  }
  .archived {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 0 18px;
    height: 38px;
    color: var(--fg3);
    font-size: 12px;
    font-family: inherit;
    background: transparent;
    border: 0;
    border-bottom: 1px solid var(--border2);
    cursor: pointer;
    text-align: left;
  }
  .done {
    color: var(--done);
    font-size: 10.5px;
  }

  /* rail (expanded panel) */
  .rail {
    flex: 1;
    overflow: auto;
    min-width: 0;
  }
  .railhead {
    display: flex;
    align-items: center;
    gap: 7px;
    padding: 0 12px;
    height: 30px;
    background: var(--panel);
    border-bottom: 1px solid var(--border2);
    position: sticky;
    top: 0;
  }
  .hue {
    width: 3px;
    height: 11px;
    border-radius: 2px;
  }
  .rname {
    font-size: 10.5px;
    font-weight: 600;
    color: var(--fg2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .rpct {
    margin-left: auto;
    font-size: 9px;
    color: var(--fg3);
  }
  .railrow {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 0 12px;
    height: 32px;
    border: 0;
    border-bottom: 1px solid var(--border2);
    background: transparent;
    cursor: pointer;
    font-family: inherit;
    text-align: left;
  }
  .railrow:hover {
    background: var(--hover);
  }
  .railrow.sel {
    background: var(--hover);
    box-shadow: inset 2px 0 0 var(--accent);
  }
  .rdot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex: none;
  }
  .rid {
    font-size: 9.5px;
    color: var(--fg3);
    flex: none;
  }
  .rtitle {
    flex: 1;
    font-size: 11px;
    color: var(--fg2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* mobile cards */
  .cards {
    flex: 1;
    overflow: auto;
    padding: 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .cardwp {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 4px 2px 2px;
  }
  .wphead {
    display: flex;
    align-items: center;
    gap: 9px;
    background: transparent;
    border: 0;
    padding: 0;
    cursor: pointer;
    font-family: inherit;
    color: var(--fg);
  }
  .wpname {
    font-size: 12.5px;
    font-weight: 600;
  }
  .wppct {
    margin-left: auto;
    font-size: 10px;
    color: var(--fg3);
  }
  .card {
    border: 1px solid var(--border);
    border-radius: 9px;
    background: var(--panel2);
    padding: 11px 12px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    cursor: pointer;
    text-align: left;
    font-family: inherit;
    touch-action: pan-y;
    transition: transform 0.12s ease-out;
  }
  .card.sel {
    border-color: var(--accent);
  }
  .cmeta {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .cdot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
  }
  .cid {
    font-size: 9.5px;
    color: var(--fg3);
  }
  .cver {
    font-size: 9.5px;
  }
  .csteps {
    margin-left: auto;
    font-size: 9px;
    color: var(--fg3);
  }
  .ctitle {
    font-size: 13px;
    line-height: 1.35;
    text-wrap: pretty;
  }
  .cchips {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .cchip {
    display: flex;
    align-items: center;
    gap: 5px;
    font-size: 9.5px;
    padding: 3px 7px;
    border-radius: 4px;
    background: var(--chip);
    border: 1px solid var(--border);
    color: var(--fg2);
  }
  .hint {
    margin: 4px 2px 14px;
    font-size: 10.5px;
    color: var(--fg3);
    text-align: center;
  }
</style>
