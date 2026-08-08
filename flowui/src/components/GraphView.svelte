<script lang="ts">
  import {
    app,
    addDependency,
    removeDependency,
    select,
    setZoom,
    toggleWp,
    zoomBy,
    ZOOM_MAX,
    ZOOM_MIN
  } from '../lib/state.svelte';
  import { buildIndex, layoutGraph, wouldCycle } from '../lib/derive';
  import { STATUS_VAR } from '../lib/types';

  const mobile = $derived(app.width < 860);
  const index = $derived(buildIndex(app.nodes, app.deps));
  const expandedWps = $derived(
    new Set(app.nodes.filter((n) => n.type === 'WORK_PACKAGE' && app.expandedWp[n.id]).map((n) => n.id))
  );
  const graph = $derived(layoutGraph(app.nodes, app.deps, expandedWps));

  let canvas: HTMLDivElement | undefined = $state();
  /** Node the drag is currently over, for target highlighting. */
  let hoverId = $state('');

  const stroke = (kind: string) =>
    kind === 'satisfied' ? 'var(--done)' : kind === 'rollup' ? 'var(--deferred)' : 'var(--blocked)';

  // ── zoom ──────────────────────────────────────────────────────────────────

  /** Ctrl/⌘ + wheel zooms about the pointer; plain wheel scrolls the canvas. */
  function onWheel(e: WheelEvent) {
    if (!(e.ctrlKey || e.metaKey) || !canvas) return;
    e.preventDefault();
    const before = app.zoom;
    const next = Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, before * (e.deltaY < 0 ? 1.12 : 1 / 1.12)));
    if (next === before) return;
    // Keep the point under the cursor fixed.
    const r = canvas.getBoundingClientRect();
    const cx = e.clientX - r.left + canvas.scrollLeft;
    const cy = e.clientY - r.top + canvas.scrollTop;
    setZoom(next);
    const k = next / before;
    queueMicrotask(() => {
      if (!canvas) return;
      canvas.scrollLeft = cx * k - (e.clientX - r.left);
      canvas.scrollTop = cy * k - (e.clientY - r.top);
    });
  }

  function fit() {
    if (!canvas) return;
    const pad = 60;
    const k = Math.min(
      (canvas.clientWidth - pad) / graph.width,
      (canvas.clientHeight - pad) / graph.height
    );
    setZoom(k);
    queueMicrotask(() => {
      if (canvas) canvas.scrollLeft = canvas.scrollTop = 0;
    });
  }

  // ── edge dragging ─────────────────────────────────────────────────────────

  /** Canvas-space point from a pointer event, undoing the zoom transform. */
  function toCanvas(e: PointerEvent) {
    const r = canvas!.getBoundingClientRect();
    return {
      x: (e.clientX - r.left + canvas!.scrollLeft) / app.zoom,
      y: (e.clientY - r.top + canvas!.scrollTop) / app.zoom
    };
  }

  function startEdge(e: PointerEvent, fromId: string) {
    if (!app.editMode) return;
    e.stopPropagation();
    e.preventDefault();
    const p = toCanvas(e);
    app.pendingEdge = { fromId, x: p.x, y: p.y };
    (e.target as HTMLElement).setPointerCapture(e.pointerId);
  }

  function moveEdge(e: PointerEvent) {
    if (!app.pendingEdge) return;
    const p = toCanvas(e);
    app.pendingEdge = { ...app.pendingEdge, x: p.x, y: p.y };
    const hit = graph.nodes.find(
      (n) => p.x >= n.x && p.x <= n.x + n.w && p.y >= n.y && p.y <= n.y + n.h
    );
    hoverId = hit && legal(hit.node.id) ? hit.node.id : '';
  }

  async function endEdge() {
    const pending = app.pendingEdge;
    const target = hoverId;
    app.pendingEdge = null;
    hoverId = '';
    if (pending && target && target !== pending.fromId) {
      await addDependency(pending.fromId, target);
    }
  }

  /** A target is live only if the edge is new and would not close a cycle. */
  function legal(targetId: string): boolean {
    const from = app.pendingEdge?.fromId;
    if (!from || from === targetId) return false;
    if ((index.blockers.get(targetId) ?? []).includes(from)) return false;
    return !wouldCycle(index, from, targetId);
  }

  const anchorOf = (id: string) => graph.nodes.find((n) => n.node.id === id);
</script>

<div class="toolbar">
  <span class="note">
    {graph.clusters.length} expanded · {graph.boxes.length} collapsed{mobile ? '' : ' · rollup edges on'}
  </span>
  <div class="spacer"></div>
  <div class="zoom">
    <button onclick={() => zoomBy(1 / 1.25)} disabled={app.zoom <= ZOOM_MIN} aria-label="Zoom out">−</button>
    <button class="pct mono" onclick={() => setZoom(1)} title="Reset to 100%">
      {Math.round(app.zoom * 100)}%
    </button>
    <button onclick={() => zoomBy(1.25)} disabled={app.zoom >= ZOOM_MAX} aria-label="Zoom in">+</button>
  </div>
  <button class="fit" onclick={fit}>Fit</button>
  <button class="toggle" class:on={app.editMode} onclick={() => (app.editMode = !app.editMode)}>
    <span class="dot"></span>{app.editMode ? 'Editing edges' : 'Edit mode'}
  </button>
</div>

<div class="canvas" bind:this={canvas} onwheel={onWheel}>
  <div class="grid" style:background-size="{26 * app.zoom}px {26 * app.zoom}px"></div>
  <div
    class="inner"
    style:width="{graph.width * app.zoom}px"
    style:height="{graph.height * app.zoom}px">
    <div
      class="scaled"
      style:width="{graph.width}px"
      style:height="{graph.height}px"
      style:transform="scale({app.zoom})"
      onpointermove={moveEdge}
      onpointerup={endEdge}
      role="presentation">
      <svg width={graph.width} height={graph.height}>
        {#each graph.edges as e (e.id)}
          <path
            d={e.d}
            fill="none"
            style:stroke={stroke(e.kind)}
            style:stroke-width={e.kind === 'rollup' ? '1.4px' : '1.6px'}
            style:stroke-dasharray={e.kind === 'rollup' ? '5 4' : 'none'}
            style:opacity={e.kind === 'satisfied' ? 0.6 : 0.9} />
          <circle cx={e.tx} cy={e.ty} r="3" style:fill={stroke(e.kind)} style:opacity={e.kind === 'satisfied' ? 0.6 : 0.9} />
        {/each}
        {#if app.pendingEdge}
          {@const a = anchorOf(app.pendingEdge.fromId)}
          {#if a}
            <path
              d="M{a.x + a.w},{a.y + a.h / 2} C{a.x + a.w + 50},{a.y + a.h / 2} {app.pendingEdge.x - 50},{app.pendingEdge.y} {app.pendingEdge.x},{app.pendingEdge.y}"
              fill="none"
              stroke="var(--accent)"
              stroke-width="1.8"
              stroke-dasharray="5 4" />
            <circle cx={app.pendingEdge.x} cy={app.pendingEdge.y} r="4" fill="var(--accent)" />
          {/if}
        {/if}
      </svg>

      {#each graph.clusters as c (c.wp.id)}
        <div
          class="cluster"
          style:left="{c.x}px"
          style:top="{c.y}px"
          style:width="{c.w}px"
          style:height="{c.h}px"
          style:border-color={c.hue}
          style:background="color-mix(in oklab, {c.hue} 5%, transparent)">
        </div>
        <button class="clusterlabel" style:left="{c.x + 12}px" style:top="{c.y - 24}px" onclick={() => toggleWp(c.wp.id)}>
          <span class="bar" style:background={c.hue}></span>
          <span class="name">{c.wp.title}</span>
          <span class="mono state">{c.wp.state}</span>
          <span class="mono ratio">{c.counts.done}/{c.counts.total} · {c.counts.pct}%</span>
        </button>
      {/each}

      {#each graph.boxes as b (b.wp.id)}
        <button class="box" style:left="{b.x}px" style:top="{b.y}px" style:width="{b.w}px" style:--hue={b.hue} onclick={() => toggleWp(b.wp.id)}>
          <div class="boxhead">
            <span class="ellipsis">{b.wp.title}</span>
            <span class="mono pct">{b.counts.pct}%</span>
          </div>
          <div class="track">
            <div style:width="{(b.counts.done / Math.max(b.counts.total, 1)) * 100}%" style:background="var(--done)"></div>
            <div style:width="{(b.counts.ready / Math.max(b.counts.total, 1)) * 100}%" style:background="var(--ready)"></div>
            <div style:width="{(b.counts.blocked / Math.max(b.counts.total, 1)) * 100}%" style:background="var(--blocked)"></div>
          </div>
          <span class="meta">{b.counts.total} nodes · {b.wp.state?.toLowerCase()}</span>
        </button>
      {/each}

      {#each graph.nodes as n (n.node.id)}
        {@const dragging = !!app.pendingEdge}
        {@const ok = dragging && legal(n.node.id)}
        <button
          class="gnode"
          class:selected={app.selectedId === n.node.id}
          class:target={hoverId === n.node.id}
          class:dead={dragging && !ok && app.pendingEdge?.fromId !== n.node.id}
          style:left="{n.x}px"
          style:top="{n.y}px"
          style:width="{n.w}px"
          style:height="{n.h}px"
          onclick={() => !dragging && select(n.node.id)}>
          <div class="gmeta">
            <span class="dot" style:background={STATUS_VAR[n.node.status]}></span>
            <span class="mono id">{n.node.id}</span>
            <span class="mono steps">{n.steps}</span>
          </div>
          <div class="gtitle" style:color={n.node.status === 'DONE' ? 'var(--fg3)' : 'var(--fg)'}>{n.node.title}</div>
          {#if app.editMode}
            <span
              class="port left"
              onpointerdown={(e) => startEdge(e, n.node.id)}
              role="button"
              tabindex="-1"
              aria-label="Drag to connect"></span>
            <span
              class="port right"
              onpointerdown={(e) => startEdge(e, n.node.id)}
              role="button"
              tabindex="-1"
              aria-label="Drag to connect"></span>
          {/if}
        </button>
      {/each}

      {#each graph.edges.filter((e) => e.label) as e (e.id + '-badge')}
        <span class="badge mono" style:left="{e.mx - 60}px" style:top="{e.my - 9}px">{e.label}</span>
      {/each}

      {#if app.editMode}
        {#each graph.edges.filter((e) => e.kind !== 'rollup') as e (e.id + '-cut')}
          <button
            class="cut"
            style:left="{e.mx - 9}px"
            style:top="{e.my - 9}px"
            title="Remove {e.from} → {e.to}"
            onclick={() => removeDependency(e.from, e.to)}>✕</button>
        {/each}
      {/if}
    </div>
  </div>

  <div class="legend" class:hide={mobile}>
    <span class="label">Edges</span>
    <span><i style:border-color="var(--blocked)"></i>hard dependency</span>
    <span><i class="dash" style:border-color="var(--deferred)"></i>rolled-up package edge</span>
    <span><i style:border-color="var(--done)"></i>satisfied</span>
    <span class="tip">⌘ + scroll to zoom</span>
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
    padding: 4px 10px;
    cursor: pointer;
    font-family: inherit;
  }
  .zoom button:hover:not(:disabled) {
    color: var(--fg);
    background: var(--hover);
  }
  .zoom button:disabled {
    opacity: 0.4;
    cursor: default;
  }
  .pct {
    border-left: 1px solid var(--border);
    border-right: 1px solid var(--border);
    min-width: 46px;
  }
  .fit,
  .toggle {
    padding: 4px 11px;
    border-radius: 6px;
    font-size: 12px;
    font-family: inherit;
    cursor: pointer;
    background: transparent;
    border: 1px solid var(--border);
    color: var(--fg2);
  }
  .fit:hover {
    color: var(--fg);
    border-color: var(--fg3);
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: 7px;
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
    touch-action: pan-x pan-y;
  }
  .grid {
    position: absolute;
    inset: 0;
    opacity: 0.5;
    background-image: radial-gradient(circle, var(--border) 1px, transparent 1px);
    pointer-events: none;
  }
  .inner {
    position: relative;
    margin: 34px 0 24px 14px;
  }
  /* One transform on the wrapper — every child keeps its layout coordinates,
     so hit-testing only has to divide by the zoom. */
  .scaled {
    position: absolute;
    left: 0;
    top: 0;
    transform-origin: 0 0;
  }
  svg {
    position: absolute;
    left: 0;
    top: 0;
    overflow: visible;
    pointer-events: none;
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
    border-left-color: var(--hue);
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
  .ellipsis {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
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
    transition: opacity 0.1s;
  }
  .gnode:hover {
    border-color: var(--fg3);
  }
  .gnode.selected {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px color-mix(in oklab, var(--accent) 18%, transparent);
  }
  .gnode.target {
    border-color: var(--accent);
    box-shadow: 0 0 0 4px color-mix(in oklab, var(--accent) 26%, transparent);
  }
  /* Illegal targets are never a live drop — dimmed during the drag. */
  .gnode.dead {
    opacity: 0.35;
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
    line-clamp: 2;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .port {
    position: absolute;
    top: 19px;
    width: 11px;
    height: 11px;
    border-radius: 50%;
    background: var(--accent);
    border: 2px solid var(--bg);
    cursor: crosshair;
    touch-action: none;
    animation: pulse 1.8s ease-in-out infinite;
  }
  .port:hover {
    transform: scale(1.25);
    animation: none;
  }
  .port.left {
    left: -6px;
  }
  .port.right {
    right: -6px;
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
  .cut {
    position: absolute;
    width: 18px;
    height: 18px;
    border-radius: 50%;
    border: 1px solid var(--blocked);
    background: var(--panel);
    color: var(--blocked);
    font-size: 9px;
    line-height: 1;
    cursor: pointer;
    display: grid;
    place-items: center;
    padding: 0;
  }
  .cut:hover {
    background: var(--blocked);
    color: var(--panel);
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
  .legend .tip {
    color: var(--fg3);
    border-top: 1px solid var(--border2);
    padding-top: 6px;
    margin-top: 2px;
  }
  .legend.hide {
    display: none;
  }
  @media (max-width: 860px) {
    .toolbar {
      padding: 0 12px;
      gap: 7px;
    }
    .zoom button {
      min-width: 36px;
      min-height: 36px;
    }
    .fit,
    .toggle {
      min-height: 36px;
    }
    .port {
      width: 16px;
      height: 16px;
      top: 16px;
    }
    .port.left {
      left: -9px;
    }
    .port.right {
      right: -9px;
    }
    .cut {
      width: 26px;
      height: 26px;
      font-size: 11px;
    }
  }
</style>
