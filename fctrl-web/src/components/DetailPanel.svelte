<script lang="ts">
  import { app, select, setStatus, undo, verify } from '../lib/state.svelte';
  import { buildIndex, ownerTask, stepRatio, stepsOf } from '../lib/derive';
  import { ALL_STATUSES, STATUS_VAR, stepGlyph, verifyGlyph, verifyText } from '../lib/types';

  const index = $derived(buildIndex(app.nodes, app.deps));
  const raw = $derived(app.nodes.find((n) => n.id === app.selectedId));
  const node = $derived(raw ? ownerTask(index, raw) : undefined);
  const wp = $derived(node?.parentId ? index.byId.get(node.parentId) : undefined);
  const steps = $derived(node ? stepsOf(app.nodes, node.id) : []);
  const ratio = $derived(node ? stepRatio(app.nodes, node.id) : { label: '–' });
  const v = $derived(verifyGlyph(node?.lastResult ?? 'none'));
  const blockers = $derived(node ? index.blockers.get(node.id) ?? [] : []);
  const blocks = $derived(node ? index.blocks.get(node.id) ?? [] : []);
</script>

{#if node}
  <aside class="panel">
    <header>
      <div class="crumb">
        <span class="mono">{app.projects.find((p) => p.id === app.projectId)?.name} / {wp?.title} / {node.id}</span>
        <button class="x" onclick={() => select(null)}>✕</button>
      </div>
      <div class="title">
        <span class="dot" style:background={STATUS_VAR[node.status]}></span>
        <h1>{node.title}</h1>
      </div>
      <div class="statusrow">
        {#each ALL_STATUSES as s}
          <button
            class="sbtn"
            class:on={node.status === s}
            style:--hue={STATUS_VAR[s]}
            onclick={() => setStatus(node.id, s)}>{s}</button>
        {/each}
      </div>
    </header>

    <div class="scroll">
      {#if node.description}
        <section>
          <span class="label">Description</span>
          <p>{node.description}</p>
        </section>
      {/if}

      <section>
        <span class="label">Condition</span>
        <div class="condrow">
          <div class="mono field">{node.condition || 'none set'}</div>
          <button class="verify" disabled={app.verifying || !node.condition} onclick={() => verify(node.id)}>
            {app.verifying ? 'Running…' : 'Verify'}
          </button>
        </div>
        <div
          class="result"
          style:border-color={node.lastResult === 'none' ? 'var(--border)' : v.color}
          style:background={node.lastResult === 'pass'
            ? 'var(--ready-bg)'
            : node.lastResult === 'fail'
              ? 'var(--blocked-bg)'
              : 'var(--panel2)'}>
          <span class="mono" style:color={v.color}>{v.glyph}</span>
          <span style:color={v.color}>{verifyText(node.lastResult)}</span>
          <span class="mono when">{node.lastRun || '—'}</span>
        </div>
      </section>

      {#if steps.length}
        <section>
          <div class="sechead">
            <span class="label">Steps</span>
            <span class="mono ratio">{ratio.label}</span>
          </div>
          {#each steps as s (s.id)}
            <div class="step">
              <span class="mono glyph" style:color={STATUS_VAR[s.status]}>{stepGlyph(s.status)}</span>
              <div class="stepbody">
                <span style:color={s.status === 'DONE' ? 'var(--fg3)' : 'var(--fg)'}>{s.title}</span>
                <span class="mono cond">{s.condition || 'no condition'}</span>
              </div>
              <span class="mono sstatus" style:color={STATUS_VAR[s.status]}>{s.status}</span>
            </div>
          {/each}
        </section>
      {/if}

      {#if blockers.length || blocks.length}
        <section>
          <span class="label">Dependencies</span>
          {#each blockers as id}
            {@const o = index.byId.get(id)}
            <button class="dep" onclick={() => o && select(o.id)}>
              <span class="mono dir">blocked by</span>
              <span class="dot small" style:background={o ? STATUS_VAR[o.status] : 'var(--fg3)'}></span>
              <span class="mono did">{id}</span>
              <span class="ellipsis dtitle">{o?.title ?? 'outside this project'}</span>
              {#if o && o.type === 'TASK' && o.parentId !== node.parentId}
                <span class="note">cross-pkg</span>
              {/if}
            </button>
          {/each}
          {#each blocks as id}
            {@const o = index.byId.get(id)}
            <button class="dep" onclick={() => o && select(o.id)}>
              <span class="mono dir">blocks</span>
              <span class="dot small" style:background={o ? STATUS_VAR[o.status] : 'var(--fg3)'}></span>
              <span class="mono did">{id}</span>
              <span class="ellipsis dtitle">{o?.title ?? 'outside this project'}</span>
              {#if o && o.type === 'TASK' && o.parentId !== node.parentId}
                <span class="note">cross-pkg</span>
              {/if}
            </button>
          {/each}
          <p class="fine">
            Cross-level edges roll up to one package edge in the graph, badged with how many real
            dependencies it stands for.
          </p>
        </section>
      {/if}
    </div>

    <footer>
      <span class="mono id">{node.id}</span>
      {#if app.flash}
        <span class="flash">{app.flash}</span>
      {/if}
      <span class="spacer"></span>
      <button class="undo" onclick={undo}>undo</button>
    </footer>
  </aside>
{/if}

<style>
  .panel {
    width: 372px;
    flex: none;
    border-left: 1px solid var(--border);
    background: var(--panel);
    display: flex;
    flex-direction: column;
    min-height: 0;
  }
  header {
    padding: 14px 16px 12px;
    border-bottom: 1px solid var(--border2);
    display: flex;
    flex-direction: column;
    gap: 9px;
  }
  .crumb {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 10px;
    color: var(--fg3);
  }
  .crumb .mono {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .x {
    background: transparent;
    border: 0;
    color: var(--fg3);
    cursor: pointer;
    font-size: 13px;
  }
  .x:hover {
    color: var(--fg);
  }
  .title {
    display: flex;
    align-items: flex-start;
    gap: 9px;
  }
  .title h1 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
    line-height: 1.3;
    text-wrap: pretty;
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-top: 6px;
    flex: none;
  }
  .dot.small {
    width: 6px;
    height: 6px;
    margin: 0;
  }
  .statusrow {
    display: flex;
    gap: 5px;
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
  .sbtn:hover {
    border-color: var(--hue);
    color: var(--hue);
  }
  .sbtn.on {
    border-color: var(--hue);
    color: var(--hue);
    background: color-mix(in oklab, var(--hue) 14%, transparent);
  }
  .scroll {
    flex: 1;
    overflow: auto;
    padding: 14px 16px 18px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  section {
    display: flex;
    flex-direction: column;
    gap: 7px;
  }
  p {
    margin: 0;
    font-size: 12.5px;
    line-height: 1.5;
    color: var(--fg2);
    text-wrap: pretty;
  }
  .condrow {
    display: flex;
    gap: 7px;
  }
  .field {
    flex: 1;
    padding: 7px 9px;
    border-radius: 7px;
    background: var(--panel2);
    border: 1px solid var(--border);
    font-size: 11px;
    color: var(--fg);
  }
  .verify {
    padding: 7px 12px;
    border-radius: 7px;
    border: 0;
    font-size: 12px;
    font-weight: 500;
    background: var(--accent);
    color: var(--accent-fg);
    cursor: pointer;
    font-family: inherit;
    white-space: nowrap;
  }
  .verify:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .result {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 7px 9px;
    border-radius: 7px;
    border: 1px solid;
    font-size: 11.5px;
  }
  .result .when {
    margin-left: auto;
    font-size: 10px;
    color: var(--fg3);
  }
  .sechead {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .ratio {
    font-size: 10px;
    color: var(--fg3);
  }
  .step {
    display: flex;
    align-items: flex-start;
    gap: 9px;
    padding: 7px 9px;
    border-radius: 7px;
    background: var(--panel2);
    border: 1px solid var(--border2);
  }
  .glyph {
    font-size: 11px;
    margin-top: 1px;
  }
  .stepbody {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 3px;
    min-width: 0;
    font-size: 12px;
  }
  .cond {
    font-size: 10px;
    color: var(--fg3);
  }
  .sstatus {
    font-size: 9.5px;
  }
  .dep {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 7px 9px;
    border-radius: 7px;
    background: var(--panel2);
    border: 1px solid var(--border2);
    cursor: pointer;
    font-family: inherit;
    color: var(--fg);
    text-align: left;
  }
  .dep:hover {
    border-color: var(--border);
  }
  .dir {
    font-size: 9.5px;
    color: var(--fg3);
    width: 58px;
  }
  .did {
    font-size: 10px;
    color: var(--fg3);
  }
  .dtitle {
    flex: 1;
    font-size: 11.5px;
  }
  .note {
    font-size: 9.5px;
    color: var(--fg3);
  }
  .fine {
    font-size: 11px;
    color: var(--fg3);
    line-height: 1.5;
  }
  footer {
    flex: none;
    border-top: 1px solid var(--border2);
    padding: 10px 16px;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 10px;
    color: var(--fg3);
  }
  .flash {
    font-size: 10.5px;
    color: var(--fg2);
  }
  .spacer {
    flex: 1;
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
