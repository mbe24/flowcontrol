<script lang="ts">
  import { app, select, toggleTask, toggleWp, passesFilter } from '../lib/state.svelte';
  import { buildIndex, stepRatio, stepsOf, tasksOf, workPackages, wpCounts } from '../lib/derive';
  import { STATUS_VAR, stepGlyph, verifyGlyph } from '../lib/types';

  const index = $derived(buildIndex(app.nodes, app.deps));
  const wps = $derived(
    workPackages(app.nodes).filter((w) => app.showArchived || (w.state !== 'DONE' && w.state !== 'ARCHIVED'))
  );
  const hidden = $derived(
    workPackages(app.nodes).filter((w) => w.state === 'DONE' || w.state === 'ARCHIVED').length
  );

  const pct = (n: number, total: number) => (total ? (n / total) * 100 : 0);
</script>

<div class="head">
  <span></span><span>Node</span><span>Blocked by</span><span>Condition</span><span>Steps</span>
  <span class="right">Status</span>
</div>

<div class="scroll">
  {#each wps as wp (wp.id)}
    {@const counts = wpCounts(app.nodes, wp.id)}
    <div class="wp" onclick={() => toggleWp(wp.id)} role="button" tabindex="0">
      <span class="caret">{app.expandedWp[wp.id] ? '▾' : '▸'}</span>
      <div class="wpname">
        <span class="mono tag">WP</span>
        <span class="ellipsis strong">{wp.title}</span>
        <span
          class="mono state"
          style:color={wp.state === 'ACTIVE' ? 'var(--ready)' : 'var(--fg2)'}
          style:background={wp.state === 'ACTIVE' ? 'var(--ready-bg)' : 'var(--chip)'}>{wp.state}</span>
      </div>
      <div class="wpbar">
        <div class="track">
          <div style:width="{pct(counts.done, counts.total)}%" style:background="var(--done)"></div>
          <div style:width="{pct(counts.ready, counts.total)}%" style:background="var(--ready)"></div>
          <div style:width="{pct(counts.blocked, counts.total)}%" style:background="var(--blocked)"></div>
          <div style:width="{pct(counts.deferred, counts.total)}%" style:background="var(--deferred)"></div>
        </div>
        <span class="mono ratio">{counts.done}/{counts.total}</span>
        <span class="mono pct">{counts.pct}%</span>
      </div>
    </div>

    {#if app.expandedWp[wp.id]}
      {#each tasksOf(app.nodes, wp.id).filter((t) => passesFilter(t.status)) as t (t.id)}
        {@const ratio = stepRatio(app.nodes, t.id)}
        {@const v = verifyGlyph(t.lastResult)}
        {@const blockers = index.blockers.get(t.id) ?? []}
        <div
          class="row"
          class:selected={app.selectedId === t.id}
          onclick={() => select(t.id)}
          role="button"
          tabindex="0">
          <span
            class="caret sub"
            onclick={(e) => {
              e.stopPropagation();
              toggleTask(t.id);
            }}
            role="button"
            tabindex="-1">{stepsOf(app.nodes, t.id).length ? (app.expandedTask[t.id] ? '▾' : '▸') : ''}</span>
          <div class="node">
            <span class="dot" style:background={STATUS_VAR[t.status]}></span>
            <span class="mono nid">{t.id}</span>
            <span class="ellipsis" style:color={t.status === 'DONE' ? 'var(--fg3)' : 'var(--fg)'}>{t.title}</span>
          </div>
          <div class="blockers">
            {#each blockers as b}
              {@const bn = index.byId.get(b)}
              <span class="mono bchip">
                <span class="tinydot" style:background={bn ? STATUS_VAR[bn.status] : 'var(--fg3)'}></span>{b}
              </span>
            {/each}
          </div>
          <div class="cond">
            <span class="mono vglyph" style:color={v.color}>{v.glyph}</span>
            <span class="mono ellipsis">{t.condition || '—'}</span>
          </div>
          <div class="steps">
            <div class="dots">
              {#each stepsOf(app.nodes, t.id) as s (s.id)}
                <span class="sdot" style:background={s.status === 'DONE' ? 'var(--ready)' : 'var(--border)'}></span>
              {/each}
            </div>
            <span class="mono ratio">{ratio.label}</span>
          </div>
          <span class="mono status" style:color={STATUS_VAR[t.status]}>{t.status}</span>
        </div>

        {#if app.expandedTask[t.id]}
          {#each stepsOf(app.nodes, t.id) as s (s.id)}
            <div class="steprow">
              <span></span>
              <div class="stepname">
                <span class="mono sglyph" style:color={STATUS_VAR[s.status]}>{stepGlyph(s.status)}</span>
                <span class="ellipsis" style:color={s.status === 'DONE' ? 'var(--fg3)' : 'var(--fg2)'}>{s.title}</span>
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
    <button class="archived" onclick={() => (app.showArchived = !app.showArchived)}>
      <span class="caret">{app.showArchived ? '▾' : '▸'}</span>
      {hidden} completed work {hidden === 1 ? 'package' : 'packages'}
      <span class="mono done">100%</span>
    </button>
  {/if}
</div>

<style>
  .head,
  .wp,
  .row,
  .steprow {
    display: grid;
    grid-template-columns: 22px 1fr 210px 190px 74px 88px;
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
  .wp {
    height: 44px;
    background: var(--panel);
    border-bottom: 1px solid var(--border2);
    cursor: pointer;
    position: sticky;
    top: 0;
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
  .wpname {
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
  .ratio {
    font-size: 11px;
    color: var(--fg2);
  }
  .pct {
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
  .steprow:hover {
    background: var(--hover);
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
  .archived:hover {
    color: var(--fg2);
  }
  .done {
    color: var(--done);
    font-size: 10.5px;
  }
</style>
