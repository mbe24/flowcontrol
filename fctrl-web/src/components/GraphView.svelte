<script lang="ts">
  import { app, select, toggleWp } from '../lib/state.svelte';
  import { layoutGraph } from '../lib/derive';
  import { STATUS_VAR } from '../lib/types';

  const expandedWps = $derived(
    new Set(
      app.nodes
        .filter((n) => n.type === 'WORK_PACKAGE' && app.expandedWp[n.id])
        .map((n) => n.id),
    ),
  );
  const graph = $derived(layoutGraph(app.nodes, app.deps, expandedWps));

  const stroke = (kind: string) =>
    kind === 'satisfied'
      ? 'var(--done)'
      : kind === 'rollup'
        ? 'var(--deferred)'
        : 'var(--blocked)';
</script>

<div class="toolbar">
  <span class="note">
    {graph.clusters.length} expanded · {graph.boxes.length} collapsed · rollup edges
    on
  </span>
  <div class="spacer"></div>
  <span class="note">Zoom</span>
  <div class="zoom">
    <button>−</button><span class="mono">100%</span><button>+</button>
  </div>
  <button
    class="toggle"
    class:on={app.editMode}
    onclick={() => (app.editMode = !app.editMode)}
  >
    <span class="dot"></span>{app.editMode ? 'Editing edges' : 'Edit mode'}
  </button>
</div>

<div class="canvas">
  <div class="grid"></div>
  <div
    class="inner"
    style:width="{graph.width}px"
    style:height="{graph.height}px"
  >
    <svg width={graph.width} height={graph.height}>
      {#each graph.edges as e (e.id)}
        <path
          d={e.d}
          fill="none"
          style:stroke={stroke(e.kind)}
          style:stroke-width={e.kind === 'rollup' ? '1.4px' : '1.6px'}
          style:stroke-dasharray={e.kind === 'rollup' ? '5 4' : 'none'}
          style:opacity={e.kind === 'satisfied' ? 0.6 : 0.9}
        />
        <circle
          cx={e.tx}
          cy={e.ty}
          r="3"
          style:fill={stroke(e.kind)}
          style:opacity={e.kind === 'satisfied' ? 0.6 : 0.9}
        />
      {/each}
    </svg>

    {#each graph.clusters as c (c.wp.id)}
      <div
        class="cluster"
        style:left="{c.x}px"
        style:top="{c.y}px"
        style:width="{c.w}px"
        style:height="{c.h}px"
        style:border-color={c.hue}
        style:background="color-mix(in oklab, {c.hue} 5%, transparent)"
      ></div>
      <button
        class="clusterlabel"
        style:left="{c.x + 12}px"
        style:top="{c.y - 24}px"
        onclick={() => toggleWp(c.wp.id)}
      >
        <span class="bar" style:background={c.hue}></span>
        <span class="name">{c.wp.title}</span>
        <span class="mono state">{c.wp.state}</span>
        <span class="mono ratio"
          >{c.counts.done}/{c.counts.total} · {c.counts.pct}%</span
        >
      </button>
    {/each}

    {#each graph.boxes as b (b.wp.id)}
      <button
        class="box"
        style:left="{b.x}px"
        style:top="{b.y}px"
        style:width="{b.w}px"
        style:--hue={b.hue}
        onclick={() => toggleWp(b.wp.id)}
      >
        <div class="boxhead">
          <span class="ellipsis">{b.wp.title}</span>
          <span class="mono pct">{b.counts.pct}%</span>
        </div>
        <div class="track">
          <div
            style:width="{(b.counts.done / Math.max(b.counts.total, 1)) * 100}%"
            style:background="var(--done)"
          ></div>
          <div
            style:width="{(b.counts.ready / Math.max(b.counts.total, 1)) *
              100}%"
            style:background="var(--ready)"
          ></div>
          <div
            style:width="{(b.counts.blocked / Math.max(b.counts.total, 1)) *
              100}%"
            style:background="var(--blocked)"
          ></div>
        </div>
        <span class="meta"
          >{b.counts.total} nodes · {b.wp.state?.toLowerCase()}</span
        >
      </button>
    {/each}

    {#each graph.nodes as n (n.node.id)}
      <button
        class="gnode"
        class:selected={app.selectedId === n.node.id}
        style:left="{n.x}px"
        style:top="{n.y}px"
        style:width="{n.w}px"
        style:height="{n.h}px"
        onclick={() => select(n.node.id)}
      >
        <div class="gmeta">
          <span class="dot" style:background={STATUS_VAR[n.node.status]}></span>
          <span class="mono id">{n.node.id}</span>
          <span class="mono steps">{n.steps}</span>
        </div>
        <div
          class="gtitle"
          style:color={n.node.status === 'DONE' ? 'var(--fg3)' : 'var(--fg)'}
        >
          {n.node.title}
        </div>
        {#if app.editMode}
          <span class="port left"></span>
          <span class="port right"></span>
        {/if}
      </button>
    {/each}

    {#each graph.edges.filter((e) => e.label) as e (e.id + '-badge')}
      <span
        class="badge mono"
        style:left="{e.mx - 60}px"
        style:top="{e.my - 9}px">{e.label}</span
      >
    {/each}
  </div>

  <div class="legend">
    <span class="label">Edges</span>
    <span><i style:border-color="var(--blocked)"></i>hard dependency</span>
    <span
      ><i class="dash" style:border-color="var(--deferred)"></i>rolled-up
      package edge</span
    >
    <span><i style:border-color="var(--done)"></i>satisfied</span>
  </div>
</div>

<style>
  .toolbar {
    flex: none;
    height: 40px;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 18px;
    border-bottom: 1px solid var(--border2);
  }
  .note {
    font-size: 11px;
    color: var(--fg3);
  }
  .spacer {
    flex: 1;
  }
  .zoom {
    display: flex;
    align-items: center;
    background: var(--panel2);
    border: 1px solid var(--border);
    border-radius: 6px;
    overflow: hidden;
    font-size: 11px;
  }
  .zoom button {
    border: 0;
    background: transparent;
    color: var(--fg2);
    padding: 3px 9px;
    cursor: pointer;
    font-family: inherit;
  }
  .zoom span {
    padding: 3px 9px;
    border-left: 1px solid var(--border);
    border-right: 1px solid var(--border);
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: 7px;
    padding: 4px 11px;
    border-radius: 6px;
    font-size: 12px;
    font-family: inherit;
    cursor: pointer;
    background: transparent;
    border: 1px solid var(--border);
    color: var(--fg2);
  }
  .toggle.on {
    background: var(--ready-bg);
    border-color: var(--ready);
    color: var(--ready);
  }
  .toggle .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: currentColor;
  }
  .canvas {
    flex: 1;
    position: relative;
    overflow: auto;
    background: var(--bg);
  }
  .grid {
    position: absolute;
    inset: 0;
    opacity: 0.5;
    background-image: radial-gradient(
      circle,
      var(--border) 1px,
      transparent 1px
    );
    background-size: 26px 26px;
    pointer-events: none;
  }
  .inner {
    position: relative;
    margin: 34px 0 24px 14px;
  }
  svg {
    position: absolute;
    left: 0;
    top: 0;
    overflow: visible;
  }
  .cluster {
    position: absolute;
    border: 1px solid;
    border-radius: 12px;
  }
  .clusterlabel {
    position: absolute;
    display: flex;
    align-items: center;
    gap: 8px;
    background: transparent;
    border: 0;
    color: var(--fg);
    font-family: inherit;
    cursor: pointer;
    padding: 0;
  }
  .clusterlabel .bar {
    width: 3px;
    height: 13px;
    border-radius: 2px;
  }
  .clusterlabel .name {
    font-size: 12px;
    font-weight: 600;
  }
  .clusterlabel .state {
    font-size: 10px;
    padding: 1px 5px;
    border-radius: 3px;
    background: var(--ready-bg);
    color: var(--ready);
  }
  .clusterlabel .ratio {
    font-size: 10px;
    color: var(--fg3);
  }
  .box {
    position: absolute;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-left: 2px solid var(--hue);
    border-radius: 9px;
    background: var(--panel);
    cursor: pointer;
    display: flex;
    flex-direction: column;
    gap: 7px;
    font-family: inherit;
    text-align: left;
    color: var(--fg);
  }
  .box:hover {
    border-color: var(--fg3);
  }
  .boxhead {
    display: flex;
    align-items: center;
    gap: 7px;
    font-size: 12px;
    font-weight: 600;
  }
  .boxhead .pct {
    margin-left: auto;
    font-size: 10px;
    color: var(--fg3);
    font-weight: 400;
  }
  .track {
    height: 5px;
    border-radius: 3px;
    overflow: hidden;
    display: flex;
    background: var(--track);
  }
  .meta {
    font-size: 10.5px;
    color: var(--fg3);
  }
  .gnode {
    position: absolute;
    border-radius: 8px;
    background: var(--panel);
    border: 1px solid var(--border);
    padding: 7px 9px;
    display: flex;
    flex-direction: column;
    gap: 4px;
    cursor: pointer;
    font-family: inherit;
    text-align: left;
  }
  .gnode:hover {
    border-color: var(--fg3);
  }
  .gnode.selected {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px color-mix(in oklab, var(--accent) 18%, transparent);
  }
  .gmeta {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .gmeta .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
  }
  .gmeta .id {
    font-size: 9.5px;
    color: var(--fg3);
  }
  .gmeta .steps {
    margin-left: auto;
    font-size: 9px;
    color: var(--fg3);
  }
  .gtitle {
    font-size: 11px;
    line-height: 1.25;
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .port {
    position: absolute;
    top: 19px;
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: var(--accent);
    border: 2px solid var(--bg);
    animation: pulse 1.8s ease-in-out infinite;
  }
  .port.left {
    left: -5px;
  }
  .port.right {
    right: -5px;
  }
  .badge {
    position: absolute;
    display: flex;
    align-items: center;
    padding: 2px 8px;
    border-radius: 12px;
    background: var(--panel);
    border: 1px dashed var(--deferred);
    font-size: 9.5px;
    color: var(--fg2);
    white-space: nowrap;
  }
  .legend {
    position: sticky;
    float: right;
    right: 16px;
    bottom: 16px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 11px 13px;
    border-radius: 9px;
    background: var(--panel);
    border: 1px solid var(--border);
    font-size: 10.5px;
    color: var(--fg2);
    width: max-content;
    margin: 0 16px 16px auto;
  }
  .legend span {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .legend i {
    width: 22px;
    border-top: 1.6px solid;
  }
  .legend i.dash {
    border-top-style: dashed;
  }
</style>
