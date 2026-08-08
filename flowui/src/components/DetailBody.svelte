<script lang="ts">
  import {
    app,
    closeDetail,
    setStatus,
    setVerdict,
    submitComment,
    toggleStep,
    togglePanelMode,
    toggleVerified,
    undo,
    updateNode,
    select,
    setWpState
  } from '../lib/state.svelte';
  import { buildIndex, stepRatio, stepsOf, tasksOf, wpCounts } from '../lib/derive';
  import { takeFocusTarget } from '../lib/state.svelte';
  import { ACTIVITY_VAR, ALL_STATUSES, STATUS_VAR, stepGlyph, verifyBadge } from '../lib/types';
  import type { FlowNode, WPState } from '../lib/types';
  import EditableField from './EditableField.svelte';
  import InlineCreateRow from './InlineCreateRow.svelte';
  import DependencyPicker from './DependencyPicker.svelte';

  interface Props {
    /** 'peek' | 'expanded' on desktop, 'sheet' when inside the mobile sheet. */
    mode: 'peek' | 'expanded' | 'sheet';
    node: FlowNode;
  }
  let { mode, node }: Props = $props();

  const index = $derived(buildIndex(app.nodes, app.deps));
  const wp = $derived(node.parentId ? index.byId.get(node.parentId) : undefined);
  const steps = $derived(stepsOf(app.nodes, node.id));
  const ratio = $derived(stepRatio(app.nodes, node.id));
  const badge = $derived(verifyBadge(node.verification));
  const blockers = $derived(index.blockers.get(node.id) ?? []);
  const blocks = $derived(index.blocks.get(node.id) ?? []);
  const entries = $derived(app.activity.filter((a) => a.nodeId === node.id));
  const wide = $derived(mode === 'expanded');
  // Build the path in code so Svelte never trims the whitespace around "/".
  const crumb = $derived.by(() => {
    const parts: string[] = [];
    if (wide) {
      const p = app.projects.find((x) => x.id === app.projectId);
      if (p) parts.push(p.name);
    }
    if (wp) parts.push(wp.title);
    parts.push(node.id);
    return parts.join(' / ');
  });
  /** In peek the description truncates; expanded and full-screen show it all. */
  const paras = $derived(mode === 'peek' ? node.description.slice(0, 1) : node.description);
  const truncated = $derived(mode === 'peek' && node.description.length > 1);

  /**
   * A work package is not a task. It has no condition, no verification and no
   * steps — its statuses are PLANNED…ARCHIVED and its progress is the rollup
   * of its tasks. Rendering task chrome here was the bug.
   */
  const isWp = $derived(node.type === 'WORK_PACKAGE');
  const WP_STATES: WPState[] = ['PLANNED', 'ACTIVE', 'DONE', 'ARCHIVED'];
  const wpShort = (s: WPState) => (s === 'PLANNED' ? 'PLAN' : s === 'ARCHIVED' ? 'ARCH' : s);
  const counts = $derived(isWp ? wpCounts(app.nodes, node.id) : null);
  const wpTasks = $derived(isWp ? tasksOf(app.nodes, node.id) : []);
  const pct = (n: number) => (counts && counts.total ? (n / counts.total) * 100 : 0);

  // The menu says why it opened us; reveal that section once, then forget it.
  let focusDeps = $state(false);
  let focusTitle = $state(false);
  /** Which step + field the menu asked us to open in edit mode. */
  let focusStep = $state<{ id: string; field: 'title' | 'condition' | 'note' } | null>(null);
  let depsEl: HTMLElement | undefined = $state();
  let stepsEl: HTMLElement | undefined = $state();

  $effect(() => {
    const t = takeFocusTarget();
    if (!t) return;
    if (t.section === 'title') focusTitle = true;
    if (t.section === 'deps') {
      focusDeps = true;
      queueMicrotask(() => {
        depsEl?.scrollIntoView?.({ block: 'nearest' });
        focusDeps = false;
      });
    }
    if (t.section === 'steps') {
      if (t.nodeId) {
        focusStep = { id: t.nodeId, field: t.field ?? 'title' };
        app.expandedStep[t.nodeId] = true;
      }
      queueMicrotask(() => stepsEl?.scrollIntoView?.({ block: 'nearest' }));
    }
  });

  function clearStepFocus() {
    focusStep = null;
  }

  let editingDesc = $state(false);
  let descDraft = $state('');
  let descEl: HTMLTextAreaElement | undefined = $state();

  function startDesc() {
    descDraft = node.description.join('\n\n');
    editingDesc = true;
    queueMicrotask(() => descEl?.focus());
  }
  async function saveDesc() {
    const next = descDraft.trim();
    editingDesc = false;
    if (next === node.description.join('\n\n')) return;
    await updateNode(node.id, { description: next ? next.split(/\n{2,}/).map((x) => x.trim()) : [] });
  }
</script>

<div class="detail" class:wide>
  <header class:big={wide}>
    <div class="crumb">
      <span class="mono path">{crumb}</span>
      {#if mode !== 'sheet'}
        <button
          class="icon"
          class:on={wide}
          title={wide ? 'Collapse panel' : 'Expand panel'}
          onclick={togglePanelMode}>{wide ? '⤡' : '⤢'}</button>
      {/if}
      <button
        class="icon"
        title="Actions"
        onclick={(e) => {
          const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
          app.menuAt = { x: Math.min(r.left - 190, window.innerWidth - 250), y: r.bottom + 6 };
          app.nodeMenuFor = node.id;
        }}>⋯</button>
      <button class="icon plain" onclick={closeDetail} aria-label="Close">✕</button>
    </div>

    <div class="title" class:big={wide}>
      {#if isWp}
        <span class="hue"></span>
      {:else}
        <span class="dot" style:background={STATUS_VAR[node.status]}></span>
      {/if}
      <h1>
        <EditableField
          nodeId={node.id}
          field="title"
          value={node.title}
          placeholder="Untitled"
          autoEdit={focusTitle}
          onStarted={() => (focusTitle = false)} />
      </h1>
      {#if isWp}<span class="mono kindtag">WP</span>{/if}
    </div>

    <div class="statusrow" class:inline={wide}>
      {#if isWp}
        {#each WP_STATES as st}
          <button
            class="sbtn"
            class:on={node.state === st}
            class:tall={mode === 'sheet'}
            style:--hue={st === 'ACTIVE' ? 'var(--ready)' : st === 'DONE' ? 'var(--done)' : 'var(--deferred)'}
            onclick={() => setWpState(node.id, st)}>{mode === 'sheet' ? wpShort(st) : st}</button>
        {/each}
      {:else}
        {#each ALL_STATUSES as s}
          <button
            class="sbtn"
            class:on={node.status === s}
            class:tall={mode === 'sheet'}
            style:--hue={STATUS_VAR[s]}
            onclick={() => setStatus(node.id, s)}>{mode === 'sheet' && s === 'DEFERRED' ? 'DEFER' : s}</button>
        {/each}
      {/if}
      {#if wide}
        <span class="spacer"></span>
        <span class="mono meta">
          {#if isWp}{wpTasks.length} tasks · {blockers.length + blocks.length} deps
          {:else}{steps.length} steps · {blockers.length + blocks.length} deps{/if}
        </span>
      {/if}
    </div>
  </header>

  <div class="cols" class:two={wide}>
    <div class="col scroll">
      <section>
        <span class="label">Description</span>
        {#if editingDesc}
          <textarea
            bind:this={descEl}
            bind:value={descDraft}
            rows={wide ? 8 : 5}
            onblur={saveDesc}
            onkeydown={(e) => {
              if (e.key === 'Escape') { e.stopPropagation(); editingDesc = false; }
              if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) saveDesc();
            }}></textarea>
          <span class="mono fine">⌘↵ save · esc revert · blank line starts a paragraph</span>
        {:else if paras.length}
          <button class="proseedit" onclick={startDesc}>
            {#each paras as p}
              <p class:lead={wide}>{p}</p>
            {/each}
          </button>
          {#if truncated}
            <button class="more" onclick={togglePanelMode}>more</button>
          {/if}
        {:else}
          <button class="proseedit empty" onclick={startDesc}>Add a description…</button>
        {/if}
      </section>

      {#if isWp && counts}
        <section>
          <span class="label">Progress</span>
          <div class="track">
            <div style:width="{pct(counts.done)}%" style:background="var(--done)"></div>
            <div style:width="{pct(counts.ready)}%" style:background="var(--ready)"></div>
            <div style:width="{pct(counts.blocked)}%" style:background="var(--blocked)"></div>
            <div style:width="{pct(counts.deferred)}%" style:background="var(--deferred)"></div>
          </div>
          <span class="mono fine">
            {counts.done}/{counts.total} · {counts.pct}% · {counts.ready} ready · {counts.blocked} blocked
          </span>
        </section>
      {:else}
      <section>
          <span class="label">Condition</span>
          <div class="mono field">
            <EditableField nodeId={node.id} field="condition" value={node.condition ?? ''} placeholder="none set" />
          </div>
          <div class="result" style:border-color={badge.color} style:background={badge.bg}>
            <button
              class="check"
              class:ticked={badge.accepted}
              style:--hue={badge.color}
              onclick={() => toggleVerified(node.id)}
              aria-label="Mark verified">{badge.accepted ? '✓' : badge.glyph}</button>
            <div class="rbody">
              <span style:color={badge.color}>{badge.label}</span>
              {#if badge.detail}<span class="mono when">{badge.detail}</span>{/if}
            </div>
            {#if badge.accepted}
              <button class="clear" onclick={() => setVerdict(node.id, 'none')}>Clear</button>
            {/if}
          </div>
          <span class="fine">fctrl never runs conditions. The agent reports; you accept.</span>
        </section>
      {/if}

      {#if !wide}
        {@render stepList()}
        {@render depList()}
        {@render activityList()}
      {/if}
    </div>

    {#if wide}
      <div class="col scroll right">
        {@render stepList()}
        {@render depList()}
        {@render activityList()}
      </div>
    {/if}
  </div>

  {#if mode !== 'sheet'}
    <footer>
      <span class="mono id">{node.id}</span>
      {#if app.flash}<span class="flash">{app.flash}</span>{/if}
      <span class="spacer"></span>
      <button class="undo" onclick={undo}>undo</button>
    </footer>
  {/if}
</div>

{#snippet stepList()}
  {#if isWp}
    <section bind:this={stepsEl}>
      <div class="sechead">
        <span class="label">Tasks</span>
        <span class="mono ratio">{wpTasks.length}</span>
      </div>
      {#each wpTasks as t (t.id)}
        {@const r = stepRatio(app.nodes, t.id)}
        <button class="wptask" onclick={() => select(t.id)}>
          <span class="tdot" style:background={STATUS_VAR[t.status]}></span>
          <span class="mono tid">{t.id}</span>
          <span class="ttitle">{t.title}</span>
          <span class="mono tsteps">{r.label}</span>
        </button>
      {/each}
      <InlineCreateRow
        projectId={app.projectId}
        parentId={node.id}
        type="TASK"
        variant={wpTasks.length === 0 ? 'prominent' : 'ghost'}
        label={wpTasks.length === 0 ? 'Add the first task…' : 'Add task…'}
        onEscalate={(title) => (app.dialog = { kind: 'create', nodeType: 'TASK', parentId: node.id, title })} />
    </section>
  {:else if steps.length}
    <section>
      <div class="sechead" bind:this={stepsEl}>
        <span class="label">Steps</span>
        <span class="mono ratio">{ratio.label}</span>
      </div>
      {#each steps as s (s.id)}
        {@const open = wide ? app.expandedStep[s.id] !== false : app.expandedStep[s.id]}
        <div class="step" class:open>
          <div class="steprow">
            <button
              class="glyphbtn mono"
              style:color={STATUS_VAR[s.status]}
              title="Toggle notes"
              aria-label="Toggle step"
              onclick={() => toggleStep(s.id)}>{stepGlyph(s.status)}</button>
            <span class="stitle" style:color={s.status === 'DONE' ? 'var(--fg3)' : 'var(--fg)'}>
              <EditableField
                nodeId={s.id}
                field="title"
                value={s.title}
                placeholder="Untitled step"
                klass="inline"
                autoEdit={focusStep?.id === s.id && focusStep.field === 'title'}
                onStarted={clearStepFocus} />
            </span>
            <button
              class="caret"
              onclick={() => toggleStep(s.id)}
              aria-label="Toggle notes">{open ? '⌃' : '⌄'}</button>
          </div>
          {#if open}
            <div class="snbody">
              <EditableField
                nodeId={s.id}
                field="note"
                value={s.note ?? ''}
                placeholder="Add a description"
                multiline
                klass="inline"
                autoEdit={focusStep?.id === s.id && focusStep.field === 'note'}
                onStarted={clearStepFocus} />
              <span class="mono scond">
                <EditableField
                  nodeId={s.id}
                  field="condition"
                  value={s.condition ?? ''}
                  placeholder="no condition"
                  klass="inline"
                  autoEdit={focusStep?.id === s.id && focusStep.field === 'condition'}
                  onStarted={clearStepFocus} />
              </span>
            </div>
          {/if}
        </div>
      {/each}
      <InlineCreateRow
        projectId={app.projectId}
        parentId={node.id}
        type="STEP"
        label="Add a step…"
        onEscalate={(title) => (app.dialog = { kind: 'create', nodeType: 'STEP', parentId: node.id, title })} />
    </section>
  {:else}
    <section>
      <span class="label">Steps</span>
      <InlineCreateRow
        projectId={app.projectId}
        parentId={node.id}
        type="STEP"
        variant="prominent"
        label="Break this into steps…"
        onEscalate={(title) => (app.dialog = { kind: 'create', nodeType: 'STEP', parentId: node.id, title })} />
    </section>
  {/if}
{/snippet}

{#snippet depList()}
  <div bind:this={depsEl}>
    <DependencyPicker {node} autoFocus={focusDeps} />
  </div>
{/snippet}

{#snippet activityList()}
  <section>
    <span class="label">Activity</span>
    {#each entries as a (a.id)}
      <div class="act">
        <div class="spine">
          <span class="adot" style:background={ACTIVITY_VAR[a.kind]}></span>
          <span class="line"></span>
        </div>
        <div class="abody">
          <div class="byline">
            <span class="who">{a.author}</span>
            <span class="mono awhen">{a.when}</span>
          </div>
          <span class="what">{a.text}</span>
        </div>
      </div>
    {/each}
    {#if entries.length === 0}
      <span class="fine">No activity yet.</span>
    {/if}
    <div class="composer">
      <input
        bind:value={app.draftComment}
        placeholder="Leave a note…"
        onkeydown={(e) => {
          if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submitComment(node.id);
        }} />
      <span class="mono kbd">⌘↵</span>
    </div>
  </section>
{/snippet}

<style>
  .detail {
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
    background: var(--panel);
  }
  header {
    flex: none;
    padding: 13px 15px 12px;
    border-bottom: 1px solid var(--border2);
    display: flex;
    flex-direction: column;
    gap: 9px;
  }
  header.big {
    padding: 15px 20px 14px;
    gap: 11px;
  }
  .crumb {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .path {
    flex: 1;
    font-size: 9.5px;
    color: var(--fg3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .icon {
    display: grid;
    place-items: center;
    width: 21px;
    height: 21px;
    border-radius: 5px;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--fg2);
    font-size: 10px;
    cursor: pointer;
    font-family: inherit;
  }
  .icon:hover {
    color: var(--fg);
    border-color: var(--fg3);
  }
  .icon.on {
    border-color: var(--accent);
    color: var(--accent);
  }
  .icon.plain {
    border-color: transparent;
    font-size: 12px;
    color: var(--fg3);
  }
  .title {
    display: flex;
    align-items: flex-start;
    gap: 9px;
  }
  .title h1 {
    flex: 1;
    min-width: 0;
    margin: 0;
    font-size: 15px;
    font-weight: 600;
    line-height: 1.3;
    text-wrap: pretty;
  }
  .title.big h1 {
    font-size: 22px;
    letter-spacing: -0.01em;
    line-height: 1.25;
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-top: 6px;
    flex: none;
  }
  .title.big .dot {
    width: 9px;
    height: 9px;
    margin-top: 9px;
  }
  .statusrow {
    display: flex;
    gap: 5px;
  }
  .statusrow.inline {
    align-items: center;
  }
  .sbtn {
    flex: 1;
    padding: 5px 0;
    border-radius: 6px;
    font-family: 'IBM Plex Mono', monospace;
    font-size: 10px;
    letter-spacing: 0.03em;
    cursor: pointer;
    background: transparent;
    border: 1px solid var(--border);
    color: var(--fg3);
  }
  .statusrow.inline .sbtn {
    flex: none;
    padding: 5px 13px;
  }
  .sbtn.tall {
    padding: 10px 0;
    font-size: 10.5px;
    color: var(--fg2);
  }
  .sbtn:hover {
    border-color: var(--hue);
    color: var(--hue);
  }
  .sbtn.on {
    border-color: var(--hue);
    color: var(--hue);
    background: color-mix(in oklab, var(--hue) 14%, transparent);
  }
  .meta {
    font-size: 9.5px;
    color: var(--fg3);
  }
  .cols {
    flex: 1;
    display: flex;
    min-height: 0;
  }
  .cols.two {
    display: grid;
    grid-template-columns: 1fr 320px;
  }
  .col {
    flex: 1;
    min-width: 0;
    padding: 14px 15px 18px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .cols.two .col {
    padding: 16px 20px;
  }
  .right {
    border-left: 1px solid var(--border2);
    padding-left: 18px;
  }
  .scroll {
    overflow: auto;
  }
  section {
    display: flex;
    flex-direction: column;
    gap: 7px;
  }
  p {
    margin: 0;
    font-size: 12.5px;
    line-height: 1.55;
    color: var(--fg2);
    text-wrap: pretty;
  }
  p.lead {
    font-size: 13.5px;
    line-height: 1.62;
    color: var(--fg);
    opacity: 0.85;
  }
  .hue {
    width: 3px;
    height: 20px;
    border-radius: 2px;
    background: var(--accent);
    margin-top: 3px;
    flex: none;
  }
  .kindtag {
    font-size: 10px;
    color: var(--fg3);
    border: 1px solid var(--border);
    border-radius: 3px;
    padding: 1px 5px;
    flex: none;
    margin-top: 4px;
  }
  .track {
    height: 8px;
    border-radius: 4px;
    overflow: hidden;
    display: flex;
    background: var(--track);
  }
  .wptask {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 8px 10px;
    border-radius: 7px;
    background: var(--panel2);
    border: 1px solid var(--border2);
    cursor: pointer;
    font-family: inherit;
    color: var(--fg);
    text-align: left;
  }
  .wptask:hover {
    border-color: var(--border);
  }
  .tdot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex: none;
  }
  .tid {
    font-size: 9.5px;
    color: var(--fg3);
    flex: none;
  }
  .ttitle {
    flex: 1;
    font-size: 11.5px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .tsteps {
    font-size: 9.5px;
    color: var(--fg3);
    flex: none;
  }
  .proseedit {
    display: block;
    width: 100%;
    text-align: left;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 7px;
    padding: 7px 9px;
    margin: -7px -9px;
    font: inherit;
    color: inherit;
    cursor: text;
  }
  .proseedit:hover {
    background: var(--panel2);
    border-color: var(--border);
  }
  .proseedit.empty {
    font-size: 12.5px;
    color: var(--fg3);
  }
  .proseedit p + p {
    margin-top: 9px;
  }
  textarea {
    width: 100%;
    box-sizing: border-box;
    background: var(--panel2);
    border: 1px solid var(--accent);
    border-radius: 7px;
    padding: 9px 10px;
    color: var(--fg);
    font-family: inherit;
    font-size: 12.5px;
    line-height: 1.55;
    outline: none;
    resize: vertical;
  }
  .more {
    align-self: flex-start;
    background: transparent;
    border: 0;
    color: var(--accent);
    font-size: 12px;
    padding: 0;
    cursor: pointer;
    font-family: inherit;
  }
  .field {
    padding: 8px 10px;
    border-radius: 7px;
    background: var(--panel2);
    border: 1px solid var(--border);
    font-size: 11px;
    color: var(--fg);
  }
  .result {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 8px 10px;
    border-radius: 7px;
    border: 1px solid;
    font-size: 11.5px;
  }
  .check {
    width: 22px;
    height: 22px;
    flex: none;
    background: transparent;
    border: 0;
    padding: 0;
    color: var(--hue);
    font-size: 17px;
    line-height: 1;
    display: grid;
    place-items: center;
    cursor: pointer;
    font-family: inherit;
  }
  .check:hover {
    color: var(--fg);
  }
  .check.ticked {
    color: var(--hue);
    font-weight: 700;
  }
  .rbody {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .when {
    font-size: 9.5px;
    color: var(--fg3);
  }
  .clear {
    margin-left: auto;
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 5px;
    color: var(--fg2);
    font-size: 10.5px;
    padding: 3px 9px;
    cursor: pointer;
    font-family: inherit;
  }
  .fine {
    font-size: 10.5px;
    color: var(--fg3);
    line-height: 1.5;
  }
  .sechead {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .ratio {
    font-size: 9.5px;
    color: var(--fg3);
  }
  .step {
    border-radius: 7px;
    background: var(--panel2);
    border: 1px solid var(--border2);
    overflow: hidden;
  }
  .step.open {
    border-color: var(--border);
  }
  .steprow {
    display: flex;
    align-items: center;
    gap: 9px;
    width: 100%;
    padding: 7px 9px;
    background: transparent;
    font-family: inherit;
    text-align: left;
  }
  .glyphbtn {
    flex: none;
    width: 14px;
    padding: 0;
    border: 0;
    background: transparent;
    font-size: 10px;
    cursor: pointer;
  }
  .stitle {
    flex: 1;
    min-width: 0;
    font-size: 11.5px;
  }
  .caret {
    flex: none;
    border: 0;
    background: transparent;
    color: var(--fg3);
    font-size: 9px;
    cursor: pointer;
    padding: 2px 4px;
  }
  /* The expanded editing area: step body (note) then its condition. */
  .snbody {
    padding: 2px 10px 8px 21px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .scond {
    display: block;
    font-size: 9.5px;
    color: var(--fg3);
  }
  .act {
    display: flex;
    gap: 9px;
  }
  .spine {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
    padding-top: 3px;
  }
  .adot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex: none;
  }
  .line {
    width: 1px;
    flex: 1;
    background: var(--border);
  }
  .abody {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding-bottom: 9px;
    min-width: 0;
  }
  .byline {
    display: flex;
    align-items: baseline;
    gap: 7px;
  }
  .who {
    font-size: 10.5px;
    color: var(--fg);
    opacity: 0.8;
  }
  .awhen {
    font-size: 9px;
    color: var(--fg3);
  }
  .what {
    font-size: 10.5px;
    line-height: 1.45;
    color: var(--fg2);
    text-wrap: pretty;
  }
  .composer {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 7px 10px;
    border-radius: 7px;
    background: var(--panel2);
    border: 1px solid var(--border);
  }
  .composer input {
    flex: 1;
    background: transparent;
    border: 0;
    outline: none;
    color: var(--fg);
    font-family: inherit;
    font-size: 11px;
  }
  .kbd {
    font-size: 9px;
    color: var(--fg3);
  }
  footer {
    flex: none;
    border-top: 1px solid var(--border2);
    padding: 9px 15px;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 9.5px;
    color: var(--fg3);
  }
  .flash {
    font-size: 10px;
    color: var(--fg2);
  }
  .spacer {
    flex: 1;
  }
  @media (max-width: 860px) {
    .sbtn.tall {
      min-height: 44px;
    }
    .wptask,
    .steprow {
      min-height: 44px;
    }
    .glyphbtn,
    .caret {
      min-width: 32px;
      min-height: 32px;
    }
    .track {
      height: 10px;
    }
    .icon {
      width: 32px;
      height: 32px;
    }
    textarea {
      font-size: 16px;
    }
  }
  .undo {
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 5px;
    color: var(--fg2);
    font-size: 10px;
    padding: 2px 7px;
    cursor: pointer;
    font-family: inherit;
  }
</style>
