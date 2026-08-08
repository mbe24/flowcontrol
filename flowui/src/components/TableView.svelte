<script lang="ts">
  import { app, select, setStatus, toggleTask, toggleWp, passesAll } from '../lib/state.svelte';
  import { buildIndex, stepRatio, stepsOf, tasksOf, workPackages, wpCounts } from '../lib/derive';
  import { STATUS_VAR, stepGlyph, verifyBadge } from '../lib/types';
  import type { FlowNode, Status } from '../lib/types';
  import InlineCreateRow from './InlineCreateRow.svelte';
  import EmptyState from './EmptyState.svelte';

  const index = $derived(buildIndex(app.nodes, app.deps));
  const allWps = $derived(workPackages(app.nodes));
  const inFilter = (w: FlowNode) => !app.wpFilter.length || app.wpFilter.includes(w.id);
  const isFinished = (w: FlowNode) => w.state === 'DONE' || w.state === 'ARCHIVED';
  /** Live packages, then the disclosure row, then the finished ones below it. */
  const liveWps = $derived(allWps.filter((w) => !isFinished(w)).filter(inFilter));
  const doneWps = $derived(allWps.filter(isFinished).filter(inFilter));
  const wps = $derived(app.showArchived ? [...liveWps, ...doneWps] : liveWps);
  const hidden = $derived(allWps.filter((w) => w.state === 'DONE' || w.state === 'ARCHIVED').length);
  const mobile = $derived(app.width < 860);
  /** On a desktop row, chips aren't counts once past 5 — switch to a bar the
     same width as those 5 chips (5×5px + 4×2px gap = 33px). */
  const DOT_CAP = 5;
  const narrow = $derived(app.panelMode === 'expanded' && !!app.selectedId && !mobile);

  /**
   * Expanding a task adds rows until the body overflows, and the vertical
   * scrollbar that then appears steals width from the row grid but not from
   * the header — so every flexible column shifts out from under its label.
   * Both grids get the same reservation via --sbw so columns never move.
   */
  const sbw = typeof document !== 'undefined' ? measureScrollbar() : 0;
  function measureScrollbar() {
    const el = document.createElement('div');
    el.style.cssText = 'width:50px;height:50px;overflow:scroll;visibility:hidden;position:absolute;top:-9999px';
    document.body.appendChild(el);
    const w = el.offsetWidth - el.clientWidth;
    el.remove();
    return w;
  }

  const visibleTasks = (wpId: string) => tasksOf(app.nodes, wpId).filter(passesAll);
  const anyVisible = $derived(wps.some((w) => visibleTasks(w.id).length > 0));
  /** Nothing exists at all vs. everything is filtered away — different screens. */
  const emptyProject = $derived(allWps.length === 0);
  const filteredOut = $derived(!emptyProject && !anyVisible && wps.length > 0);

  const pct = (n: number, total: number) => (total ? (n / total) * 100 : 0);

  function openMenu(e: MouseEvent, id: string) {
    e.preventDefault();
    e.stopPropagation();
    app.menuAt = { x: Math.min(e.clientX, window.innerWidth - 250), y: e.clientY };
    app.nodeMenuFor = id;
  }

  /** Tab / Shift-Tab on a selected row — the outliner idiom. */
  function onRowKey(e: KeyboardEvent, node: FlowNode) {
    if (e.key === 'Tab') {
      e.preventDefault();
      if (e.shiftKey && node.type === 'STEP') {
        app.dialog = { kind: 'move', nodeId: node.id, to: 'TASK' };
      } else if (!e.shiftKey && node.type === 'TASK') {
        app.dialog = { kind: 'move', nodeId: node.id, to: 'STEP' };
      }
    } else if (e.key === 'Delete' || e.key === 'Backspace') {
      e.preventDefault();
      app.dialog = { kind: 'delete', nodeId: node.id };
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      select(node.id);
    }
  }

  // swipe a card to set status, mobile only
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
    const next: Status = dx > 0 ? 'DONE' : t.status === 'BLOCKED' ? 'READY' : 'BLOCKED';
    setStatus(t.id, next);
  }
</script>

{#if emptyProject}
  <EmptyState kind="project" />
{:else if filteredOut}
  <EmptyState kind="filtered" />
{:else if mobile}
  <div class="cards">
    {#each liveWps as wp (wp.id)}
      {@const counts = wpCounts(app.nodes, wp.id)}
      {@const tasks = visibleTasks(wp.id)}
      <div class="cardwp">
        <button class="wphead" onclick={() => toggleWp(wp.id)}>
          <span class="caret">{app.expandedWp[wp.id] ? '▾' : '▸'}</span>
          <span class="wpname">{wp.title}</span>
          <span class="mono wppct">{counts.pct}%</span>
        </button>
        <div class="track thin">
          <div style:width="{pct(counts.done, counts.total)}%" style:background="var(--done)"></div>
          <div style:width="{pct(counts.ready, counts.total)}%" style:background="var(--ready)"></div>
          <div style:width="{pct(counts.blocked, counts.total)}%" style:background="var(--blocked)"></div>
          <div style:width="{pct(counts.deferred, counts.total)}%" style:background="var(--deferred)"></div>
        </div>
      </div>
      {#if app.expandedWp[wp.id]}
        {#each tasks as t (t.id)}
          {@const ratio = stepRatio(app.nodes, t.id)}
          {@const badge = verifyBadge(t.verification)}
          <button
            class="card"
            class:sel={app.selectedId === t.id}
            style:transform={swipeId === t.id ? `translateX(${swipeX}px)` : 'none'}
            onpointerdown={(e) => down(e, t.id)}
            onpointermove={move}
            onpointerup={() => up(t)}
            onpointercancel={() => (swipeId = '')}
            oncontextmenu={(e) => openMenu(e, t.id)}
            onclick={() => select(t.id)}>
            <div class="cmeta">
              <span class="cdot" style:background={STATUS_VAR[t.status]}></span>
              <span class="mono cid">{t.id}</span>
              <span class="mono cver" style:color={badge.color}>{badge.glyph}</span>
              <span class="mono csteps">{ratio.label}</span>
            </div>
            <span class="ctitle" style:color={t.status === 'DONE' ? 'var(--fg2)' : 'var(--fg)'}>{t.title}</span>
            {#if (index.blockers.get(t.id) ?? []).length}
              <div class="cchips">
                {#each index.blockers.get(t.id) ?? [] as b}
                  {@const bn = index.byId.get(b)}
                  <span class="mono cchip">
                    <span class="tinydot" style:background={bn ? STATUS_VAR[bn.status] : 'var(--fg3)'}></span>{b}
                  </span>
                {/each}
              </div>
            {/if}
          </button>
        {/each}
        {#if tasks.length === 0}
          <span class="nonetask">No tasks in this package</span>
        {/if}
        <InlineCreateRow
          projectId={app.projectId}
          parentId={wp.id}
          type="TASK"
          variant={tasks.length === 0 ? 'prominent' : 'ghost'}
          label={tasks.length === 0 ? 'Add the first task…' : `Add task to ${wp.title}`}
          onEscalate={(title) => (app.dialog = { kind: 'create', nodeType: 'TASK', parentId: wp.id, title })} />
      {/if}
    {/each}
    {#if hidden > 0}
      <button class="marchived" onclick={() => (app.showArchived = !app.showArchived)}>
        <span class="caret">{app.showArchived ? '▾' : '▸'}</span>
        {hidden} completed work {hidden === 1 ? 'package' : 'packages'}
      </button>
      {#if app.showArchived}
        {#each doneWps as wp (wp.id)}
          {@const counts = wpCounts(app.nodes, wp.id)}
          <div class="cardwp">
            <button class="wphead" onclick={() => toggleWp(wp.id)}>
              <span class="caret">{app.expandedWp[wp.id] ? '▾' : '▸'}</span>
              <span class="wpname dim">{wp.title}</span>
              <span class="mono wppct">{counts.pct}%</span>
            </button>
          </div>
          {#if app.expandedWp[wp.id]}
            {#each visibleTasks(wp.id) as t (t.id)}
              <button class="card" onclick={() => select(t.id)}>
                <div class="cmeta">
                  <span class="cdot" style:background={STATUS_VAR[t.status]}></span>
                  <span class="mono cid">{t.id}</span>
                </div>
                <span class="ctitle" style:color="var(--fg2)">{t.title}</span>
              </button>
            {/each}
          {/if}
        {/each}
      {/if}
    {/if}
    <button
      class="mnewwp"
      onclick={() => (app.dialog = { kind: 'create', nodeType: 'WORK_PACKAGE', parentId: null, title: '' })}>
      <span class="plus">+</span>New work package
    </button>
    <p class="hint">Swipe a card right to mark DONE, left to toggle BLOCKED.</p>
  </div>
{:else if narrow}
  <!-- Expanded panel: the list becomes a rail of dot + id + title, grouped by package. -->
  <div class="rail">
    {#each wps as wp (wp.id)}
      <div class="railhead">
        <span class="hue" style:background={STATUS_VAR[wp.state === 'ACTIVE' ? 'READY' : 'DEFERRED']}></span>
        <span class="rname">{wp.title}</span>
        <span class="mono rpct">{wpCounts(app.nodes, wp.id).pct}%</span>
      </div>
      {#each visibleTasks(wp.id) as t (t.id)}
        <button
          class="railrow"
          class:sel={app.selectedId === t.id}
          oncontextmenu={(e) => openMenu(e, t.id)}
          onclick={() => select(t.id)}>
          <span class="rdot" style:background={STATUS_VAR[t.status]}></span>
          <span class="mono rid">{t.id}</span>
          <span class="rtitle">{t.title}</span>
        </button>
      {/each}
    {/each}
  </div>
{:else}
  <div class="tbl" style="--sbw: {sbw}px">
    <div class="head">
      <span></span><span>Node</span><span>Blocked by</span><span>Condition</span><span>Steps</span>
      <span class="right">Status</span>
    </div>

    <div class="scroll">
      {@render wpGroup(liveWps)}

      {#if hidden > 0}
        <button class="archived" onclick={() => (app.showArchived = !app.showArchived)}>
          <span class="caret">{app.showArchived ? '▾' : '▸'}</span>
          {hidden} completed work {hidden === 1 ? 'package' : 'packages'}
          <span class="mono done">100%</span>
        </button>
        {#if app.showArchived}
          {@render wpGroup(doneWps)}
        {/if}
      {/if}

      <button
        class="newwp"
        onclick={() => (app.dialog = { kind: 'create', nodeType: 'WORK_PACKAGE', parentId: null, title: '' })}>
        <span class="plus">+</span>New work package
      </button>
    </div>
  </div>
{/if}

{#snippet wpGroup(group: FlowNode[])}
  {#each group as wp (wp.id)}
      {@const counts = wpCounts(app.nodes, wp.id)}
      {@const tasks = visibleTasks(wp.id)}
      <div
        class="wp"
        onclick={() => toggleWp(wp.id)}
        oncontextmenu={(e) => openMenu(e, wp.id)}
        role="button"
        tabindex="0"
        onkeydown={(e) => e.key === 'Enter' && toggleWp(wp.id)}>
        <span class="caret">{app.expandedWp[wp.id] ? '▾' : '▸'}</span>
        <div class="wpname2">
          <span class="mono tag">WP</span>
          <span class="ellipsis strong">{wp.title}</span>
          <span
            class="mono state"
            style:color={wp.state === 'ACTIVE' ? 'var(--ready)' : 'var(--fg2)'}
            style:background={wp.state === 'ACTIVE' ? 'var(--ready-bg)' : 'var(--chip)'}>{wp.state}</span>
        </div>
        <div class="wpcounts">
          {#if counts.ready}<span class="wpc"><span class="cdot2" style:background="var(--ready)"></span>{counts.ready} ready</span>{/if}
          {#if counts.blocked}<span class="wpc"><span class="cdot2" style:background="var(--blocked)"></span>{counts.blocked} blocked</span>{/if}
          {#if counts.deferred}<span class="wpc"><span class="cdot2" style:background="var(--deferred)"></span>{counts.deferred} deferred</span>{/if}
        </div>
        <div class="wpbar">
          <div class="track">
            <div style:width="{pct(counts.done, counts.total)}%" style:background="var(--done)"></div>
            <div style:width="{pct(counts.ready, counts.total)}%" style:background="var(--ready)"></div>
            <div style:width="{pct(counts.blocked, counts.total)}%" style:background="var(--blocked)"></div>
            <div style:width="{pct(counts.deferred, counts.total)}%" style:background="var(--deferred)"></div>
          </div>
        </div>
        <span class="mono wpratio">{counts.done}/{counts.total}</span>
        <span class="mono wppctcell">{counts.pct}%</span>
      </div>

      {#if app.expandedWp[wp.id]}
        {#each tasks as t (t.id)}
          {@const ratio = stepRatio(app.nodes, t.id)}
          {@const badge = verifyBadge(t.verification)}
          {@const blockers = index.blockers.get(t.id) ?? []}
          {@const steps = stepsOf(app.nodes, t.id)}
          <div
            class="row"
            class:selected={app.selectedId === t.id}
            onclick={() => select(t.id)}
            oncontextmenu={(e) => openMenu(e, t.id)}
            onkeydown={(e) => onRowKey(e, t)}
            role="button"
            tabindex="0">
            <span
              class="caret sub"
              onclick={(e) => {
                e.stopPropagation();
                toggleTask(t.id);
              }}
              onkeydown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.stopPropagation();
                  toggleTask(t.id);
                }
              }}
              role="button"
              tabindex="-1">{steps.length ? (app.expandedTask[t.id] ? '▾' : '▸') : ''}</span>
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
            <div class="cond" title={badge.label}>
              <span class="mono vglyph" style:color={badge.color}>{badge.glyph}</span>
              <span class="mono ellipsis">{t.condition || '—'}</span>
            </div>
            <div class="steps">
              {#if steps.length === 0}
                <span class="mono nosteps">–</span>
              {:else}
                <span class="mono ratio">{ratio.label}</span>
                {#if steps.length <= DOT_CAP}
                  <div class="dots">
                    {#each steps as s (s.id)}
                      <span class="sdot" style:background={s.status === 'DONE' ? 'var(--ready)' : 'var(--border)'}></span>
                    {/each}
                  </div>
                {:else}
                  <div class="minibar" title="{ratio.label} steps done">
                    <div style:width="{(ratio.done / steps.length) * 100}%"></div>
                  </div>
                {/if}
              {/if}
            </div>
            <div class="tail">
              <span class="mono status" style:color={STATUS_VAR[t.status]}>{t.status}</span>
              <button class="dots3" onclick={(e) => openMenu(e, t.id)} aria-label="Actions">⋯</button>
            </div>
          </div>

          {#if app.expandedTask[t.id] || app.showSteps}
            {#each steps as s (s.id)}
              <div
                class="steprow"
                onclick={() => select(s.id)}
                oncontextmenu={(e) => openMenu(e, s.id)}
                onkeydown={(e) => onRowKey(e, s)}
                role="button"
                tabindex="0">
                <span></span>
                <div class="stepname">
                  <span class="mono sglyph" style:color={STATUS_VAR[s.status]}>{stepGlyph(s.status)}</span>
                  <span class="ellipsis" style:color={s.status === 'DONE' ? 'var(--fg3)' : 'var(--fg2)'}>{s.title}</span>
                </div>
                <span></span>
                <span class="mono scond ellipsis">{s.condition || ''}</span>
                <span></span>
                <div class="tail">
                  <span class="mono sstatus">{s.status}</span>
                  <button class="dots3" onclick={(e) => openMenu(e, s.id)} aria-label="Actions">⋯</button>
                </div>
              </div>
            {/each}
            <div class="stepadd">
              <InlineCreateRow
                projectId={app.projectId}
                parentId={t.id}
                type="STEP"
                label="Add a step…"
                onEscalate={(title) => (app.dialog = { kind: 'create', nodeType: 'STEP', parentId: t.id, title })} />
            </div>
          {/if}
        {/each}

        {#if tasks.length === 0}
          <div class="wpempty">
            <span class="nonetask">No tasks in this package</span>
          </div>
        {/if}
        <InlineCreateRow
          projectId={app.projectId}
          parentId={wp.id}
          type="TASK"
          variant={tasks.length === 0 ? 'prominent' : 'ghost'}
          label={tasks.length === 0 ? 'Add the first task…' : `Add task to ${wp.title}`}
          onEscalate={(title) => (app.dialog = { kind: 'create', nodeType: 'TASK', parentId: wp.id, title })} />
      {/if}

  {/each}
{/snippet}

<style>
  .tbl,
  .head,
  .wp,
  .row,
  .steprow {
    display: grid;
    grid-template-columns:
      22px
      minmax(260px, 1.6fr)
      minmax(104px, 0.5fr)
      minmax(200px, 1fr)
      82px
      104px;
    align-items: center;
    padding: 0 18px;
  }
  .tbl {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: stretch;
    min-height: 0;
    padding: 0;
  }
  .head {
    height: 30px;
    flex: none;
    border-bottom: 1px solid var(--border2);
    font-size: 10.5px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--fg3);
    /* Keep the head the same width as the body even when the body's scrollbar
       appears — otherwise the columns shift out from under their labels. */
    padding-right: calc(18px + var(--sbw, 0px));
  }
  .right {
    text-align: right;
    /* Align the "Status" label with the task/step status text: each row's
       trailing ⋯ button (20px) + its 4px gap means the label has to keep the
       same 24px room on the right as the status text does. */
    padding-right: 24px;
  }
  .scroll {
    flex: 1;
    overflow: auto;
    /* Reserve the gutter even with no overflow, so the head's matching
       padding-right stays accurate and columns don't shift either way. */
    scrollbar-gutter: stable;
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
  /* Column 3: the space that used to be empty carries a status breakdown. */
  .wpcounts {
    display: flex;
    align-items: center;
    gap: 12px;
    overflow: hidden;
    padding-right: 12px;
  }
  .wpc {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 10.5px;
    color: var(--fg2);
    white-space: nowrap;
  }
  .cdot2 {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    flex: none;
  }
  /* Column 4: same cell as CONDITION, so every package bar starts and ends on
     the same x — that is what makes them comparable at a glance. */
  .wpbar {
    display: flex;
    align-items: center;
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
  .wpratio {
    font-size: 11px;
    color: var(--fg2);
  }
  .ratio {
    font-size: 11px;
    color: var(--fg2);
    /* Reserve a fixed counter width so the chips/bar after it always start at
       the same x, whatever the digit count ("3/5" and "12/34" line up the
       same). 6ch fits "99/99"-ish ratios in the 11px mono font. */
    min-width: 6ch;
    display: inline-block;
  }
.wppctcell {
    font-size: 11px;
    color: var(--fg3);
    text-align: right;
    /* Same 24px reservation as .right and the rows' status text, so the WP %
       lines up with the task/step status above/below it. */
    padding-right: 24px;
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
  /* Over the cap (5 chips) a bar stands in for the dots, sized to the same
     width as 5 chips so the step counter never jumps in width. */
  .nosteps {
    font-size: 10px;
    color: var(--fg3);
    width: 33px;
  }
  .minibar {
    width: 33px;
    height: 5px;
    border-radius: 3px;
    background: var(--border);
    overflow: hidden;
    flex: none;
  }
  .minibar > div {
    height: 100%;
    background: var(--ready);
  }
  .tail {
    display: flex;
    align-items: center;
    gap: 4px;
    justify-content: flex-end;
  }
  .status {
    font-size: 10px;
    letter-spacing: 0.04em;
  }
  .dots3 {
    width: 20px;
    height: 20px;
    border: 0;
    border-radius: 5px;
    background: transparent;
    color: var(--fg3);
    font-size: 13px;
    line-height: 1;
    cursor: pointer;
    opacity: 0;
    flex: none;
  }
  .row:hover .dots3,
  .steprow:hover .dots3,
  .dots3:focus {
    opacity: 1;
  }
  .dots3:hover {
    background: var(--panel2);
    color: var(--fg);
  }
  .steprow {
    height: 31px;
    border-bottom: 1px solid var(--border2);
    background: var(--panel2);
    cursor: pointer;
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
    font-size: 10px;
    color: var(--fg3);
  }
  .stepadd {
    padding-left: 52px;
    background: var(--panel2);
  }
  .wpempty {
    padding: 14px 18px 4px;
  }
  .nonetask {
    font-size: 12px;
    color: var(--fg3);
  }
  .archived,
  .newwp {
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
  .archived:hover,
  .newwp:hover {
    color: var(--fg2);
    background: var(--hover);
  }
  .newwp {
    color: var(--accent);
    border-bottom: 0;
    height: 44px;
  }
  .plus {
    font-size: 14px;
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
    min-height: 44px;
    cursor: pointer;
    font-family: inherit;
    color: var(--fg);
  }
  .wpname {
    font-size: 13px;
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
  .marchived,
  .mnewwp {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    min-height: 44px;
    padding: 0 4px;
    background: transparent;
    border: 0;
    color: var(--fg3);
    font-family: inherit;
    font-size: 12.5px;
    cursor: pointer;
    text-align: left;
  }
  .mnewwp {
    color: var(--accent);
  }
  .wpname.dim {
    color: var(--fg3);
  }
  .hint {
    margin: 4px 2px 14px;
    font-size: 10.5px;
    color: var(--fg3);
    text-align: center;
  }
</style>
