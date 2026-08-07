<script lang="ts">
  import { app, createNode, select, setStatus } from '../lib/state.svelte';
  import { buildIndex } from '../lib/derive';
  import { ALL_STATUSES, stepGlyph } from '../lib/types';
  import type { Status } from '../lib/types';

  const index = $derived(buildIndex(app.nodes, app.deps));

  type Hit = { kind: 'task' | 'step' | 'cmd'; id: string; title: string; hint: string; status?: Status; run: () => void | Promise<void> };

  const results = $derived.by((): Hit[] => {
    const q = app.paletteQuery.trim().toLowerCase();
    if (!q) return [];
    const hits: Hit[] = [];

    // "new task …" / "new step …" / "new package …" / "new project …".
    // Search still comes first: create rows only appear for a "new" prefix.
    const verb = q.match(/^new\s+(task|step|package|work package|project)\s*(.*)$/);
    if (verb) {
      const what = verb[1];
      const title = app.paletteQuery.trim().slice(app.paletteQuery.trim().toLowerCase().indexOf(verb[2] || '~')).trim();
      const sel = app.nodes.find((n) => n.id === app.selectedId);
      if (what === 'project') {
        hits.push({
          kind: 'cmd', id: '', title: 'Create project…', hint: 'opens the dialog',
          run: () => { app.paletteOpen = false; app.dialog = { kind: 'newProject' }; }
        });
      } else {
        const type = what === 'step' ? 'STEP' : what === 'task' ? 'TASK' : 'WORK_PACKAGE';
        const parent =
          type === 'WORK_PACKAGE' ? null
          : type === 'STEP' ? (sel?.type === 'STEP' ? sel.parentId : sel?.id ?? null)
          : (sel?.type === 'WORK_PACKAGE' ? sel.id : sel?.parentId ?? null);
        const parentName = parent ? index.byId.get(parent)?.title ?? parent : '';
        if (title) {
          hits.push({
            kind: 'cmd', id: '',
            title: `Create ${what} “${title}”`,
            hint: parent ? `under ${parent}` : 'in this project',
            run: async () => {
              app.paletteOpen = false;
              if (type !== 'WORK_PACKAGE' && !parent) return;
              await createNode({ projectId: app.projectId, parentId: parent, type, title }, true);
            }
          });
        }
        hits.push({
          kind: 'cmd', id: '',
          title: title ? `Create ${what}, pick a different parent…` : `Create a ${what}…`,
          hint: parentName ? `now: ${parentName}` : 'full form',
          run: () => {
            app.paletteOpen = false;
            app.dialog = { kind: 'create', nodeType: type, parentId: parent, title };
          }
        });
      }
    }
    for (const n of app.nodes) {
      if (n.type === 'WORK_PACKAGE') continue;
      if (!n.title.toLowerCase().includes(q)) continue;
      const parent = n.parentId ? index.byId.get(n.parentId) : undefined;
      hits.push({
        kind: n.type === 'STEP' ? 'step' : 'task',
        id: n.id,
        title: n.title,
        hint: parent?.title ?? '',
        status: n.status,
        run: () => {
          select(n.type === 'STEP' ? n.parentId ?? n.id : n.id);
          app.paletteOpen = false;
        }
      });
      if (hits.length >= 8) break;
    }
    for (const s of ALL_STATUSES) {
      const label = `set status: ${s}`;
      if (label.toLowerCase().includes(q) && app.selectedId) {
        hits.push({
          kind: 'cmd',
          id: '',
          title: label,
          hint: 'command',
          run: () => {
            if (app.selectedId) setStatus(app.selectedId, s);
            app.paletteOpen = false;
          }
        });
      }
    }
    return hits;
  });

  let cursor = $state(0);

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      cursor = Math.min(cursor + 1, Math.max(results.length - 1, 0));
      e.preventDefault();
    }
    if (e.key === 'ArrowUp') {
      cursor = Math.max(cursor - 1, 0);
      e.preventDefault();
    }
    if (e.key === 'Enter') results[cursor]?.run();
  }

  const glyph = (h: Hit) =>
    h.kind === 'cmd' ? '⌘' : h.kind === 'step' ? stepGlyph(h.status ?? 'READY') : '●';
</script>

<div class="scrim" onclick={() => (app.paletteOpen = false)} role="presentation">
  <div class="palette" onclick={(e) => e.stopPropagation()} role="dialog" tabindex="-1">
    <div class="input">
      <span class="mono caret">›</span>
      <!-- svelte-ignore a11y_autofocus -->
      <input
        class="mono"
        bind:value={app.paletteQuery}
        onkeydown={onKeydown}
        placeholder="task, step or command"
        autofocus />
      <span class="mono count">{results.length} results</span>
    </div>
    <div class="list">
      {#each results as h, i (h.kind + h.id + h.title)}
        <button class="hit" class:on={i === cursor} onmouseenter={() => (cursor = i)} onclick={h.run}>
          <span class="mono kind">{glyph(h)}</span>
          <span class="mono id">{h.id}</span>
          <span class="ellipsis title">{h.title}</span>
          <span class="hint">{h.hint}</span>
        </button>
      {/each}
      {#if results.length === 0}
        <div class="empty">Search tasks, steps and commands — try “rotate”.</div>
      {/if}
    </div>
    <div class="foot mono">
      <span>↵ open</span><span>⌘↵ set status</span><span>⇥ filter by package</span>
      <span class="spacer"></span><span>esc close</span>
    </div>
  </div>
</div>

<style>
  .scrim {
    position: absolute;
    inset: 0;
    background: rgba(0, 0, 0, 0.45);
    display: flex;
    justify-content: center;
    align-items: flex-start;
    padding-top: 120px;
    z-index: 20;
  }
  .palette {
    width: 620px;
    border-radius: 12px;
    background: var(--panel);
    border: 1px solid var(--border);
    box-shadow: var(--shadow);
    overflow: hidden;
  }
  .input {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px 16px;
    border-bottom: 1px solid var(--border2);
  }
  .caret {
    color: var(--accent);
    font-size: 13px;
  }
  input {
    flex: 1;
    background: transparent;
    border: 0;
    outline: none;
    color: var(--fg);
    font-size: 13.5px;
  }
  .count {
    font-size: 11px;
    color: var(--fg3);
  }
  .list {
    padding: 6px;
    max-height: 320px;
    overflow: auto;
  }
  .hit {
    display: flex;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 9px 11px;
    border-radius: 7px;
    border: 0;
    background: transparent;
    cursor: pointer;
    font-family: inherit;
    color: var(--fg);
    text-align: left;
  }
  .hit.on {
    background: var(--hover);
  }
  .kind {
    font-size: 10px;
    color: var(--fg3);
    width: 14px;
  }
  .id {
    font-size: 10.5px;
    color: var(--fg3);
    width: 62px;
  }
  .title {
    flex: 1;
    font-size: 12.5px;
  }
  .hint {
    font-size: 10.5px;
    color: var(--fg3);
  }
  .empty {
    padding: 12px;
    font-size: 12px;
    color: var(--fg3);
  }
  .foot {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 9px 14px;
    border-top: 1px solid var(--border2);
    font-size: 10.5px;
    color: var(--fg3);
  }
  .spacer {
    flex: 1;
  }
</style>
