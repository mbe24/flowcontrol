<script lang="ts">
  import { app, createNode } from '../lib/state.svelte';
  import { tasksOf, workPackages } from '../lib/derive';
  import type { NodeType } from '../lib/types';

  interface Props {
    nodeType: NodeType;
    parentId: string | null;
    initialTitle: string;
  }
  let { nodeType, parentId, initialTitle }: Props = $props();

  // These deliberately seed state from the props once (a dialog opens fresh).
  // svelte-ignore state_referenced_locally
  let kind = $state<NodeType>(nodeType);
  // svelte-ignore state_referenced_locally
  let parent = $state<string | null>(parentId);
  // svelte-ignore state_referenced_locally
  let title = $state(initialTitle);
  let description = $state('');
  let condition = $state('');
  // svelte-ignore state_referenced_locally
  let another = $state(initialTitle.length > 0);
  let busy = $state(false);
  let titleEl: HTMLInputElement | undefined = $state();

  // ── searchable parent combobox ─────────────────────────────────────────────
  let parentQuery = $state('');
  let parentOpen = $state(false);
  let parentIdx = $state(-1);
  let parentEl: HTMLDivElement | undefined = $state();

  $effect(() => {
    titleEl?.focus();
  });

  /** The parent list is filtered by kind — never a flat list of every node. */
  const parents = $derived.by(() => {
    if (kind === 'WORK_PACKAGE') return [];
    if (kind === 'TASK') return workPackages(app.nodes);
    return app.nodes.filter((n) => n.type === 'TASK');
  });

  const filteredParents = $derived(
    parentQuery.trim()
      ? parents.filter((p) => `${p.title} ${p.id}`.toLowerCase().includes(parentQuery.trim().toLowerCase()))
      : parents
  );
  const parentTitle = $derived(parent ? parents.find((p) => p.id === parent)?.title ?? '' : '');

  // Keep the parent legal when the kind changes.
  $effect(() => {
    if (kind === 'WORK_PACKAGE') {
      parent = null;
    } else if (!parents.some((p) => p.id === parent)) {
      parent = parents[0]?.id ?? null;
    }
  });

  const valid = $derived(title.trim().length > 0 && (kind === 'WORK_PACKAGE' || !!parent));

  async function submit() {
    if (!valid || busy) return;
    busy = true;
    try {
      await createNode({
        projectId: app.projectId,
        parentId: parent,
        type: kind,
        title: title.trim(),
        description: description.trim() ? description.trim().split(/\n{2,}/) : [],
        condition: condition.trim()
      });
      if (another) {
        title = '';
        description = '';
        condition = '';
        titleEl?.focus();
      } else {
        app.dialog = null;
      }
    } finally {
      busy = false;
    }
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey || (e.target as HTMLElement).tagName === 'INPUT')) {
      e.preventDefault();
      submit();
    }
  }

  const kinds: { v: NodeType; label: string }[] = [
    { v: 'WORK_PACKAGE', label: 'Work package' },
    { v: 'TASK', label: 'Task' },
    { v: 'STEP', label: 'Step' }
  ];
</script>

<div class="scrim" onclick={() => (app.dialog = null)} onkeydown={(e) => e.key === 'Escape' && (app.dialog = null)} role="presentation">
  <div class="dialog" onclick={(e) => e.stopPropagation()} onkeydown={onKey} role="dialog" tabindex="-1">
    <div class="head">
      <span class="h">New node</span>
      <span class="spacer"></span>
      <button class="x" onclick={() => (app.dialog = null)} aria-label="Close">✕</button>
    </div>

    <div class="field">
      <span class="label">Kind</span>
      <div class="seg">
        {#each kinds as k}
          <button class="segbtn" class:on={kind === k.v} onclick={() => (kind = k.v)}>{k.label}</button>
        {/each}
      </div>
    </div>

    {#if kind !== 'WORK_PACKAGE'}
      <div class="field">
        <span class="label">Parent</span>
        <div class="picker" bind:this={parentEl}>
          <input
            class="mono"
            bind:value={parentQuery}
            placeholder={parent ? parentTitle : kind === 'TASK' ? 'Search package…' : 'Search task…'}
            onfocus={() => (parentOpen = true)}
            onblur={() => {
              // delay so a click on an option registers before the list closes
              setTimeout(() => (parentOpen = false), 120);
            }}
            onkeydown={(e) => {
              e.stopPropagation();
              if (e.key === 'Escape') {
                parentOpen = false;
              } else if (e.key === 'Enter' && filteredParents.length) {
                e.preventDefault();
                const pick = filteredParents[Math.max(parentIdx, 0)];
                parent = pick.id;
                parentQuery = '';
                parentOpen = false;
              } else if (e.key === 'ArrowDown') {
                e.preventDefault();
                parentOpen = true;
                parentIdx = parentIdx + 1 < filteredParents.length ? parentIdx + 1 : 0;
              } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                parentIdx = parentIdx - 1 >= 0 ? parentIdx - 1 : filteredParents.length - 1;
              } else {
                parentIdx = -1;
              }
            }}
          />
          {#if parentOpen}
            <div class="options">
              {#if filteredParents.length === 0}
                <div class="empty">No matching {kind === 'TASK' ? 'packages' : 'tasks'}</div>
              {:else}
                {#each filteredParents as p, i (p.id)}
                  <button
                    class="opt"
                    class:active={parentIdx === i}
                    onmouseenter={() => (parentIdx = i)}
                    onclick={() => {
                      parent = p.id;
                      parentQuery = '';
                      parentOpen = false;
                    }}>
                    {#if p.type === 'TASK'}<span class="mono">{p.id} ·&#160;</span>{/if}
                    <span class="title">{p.title}</span>
                  </button>
                {/each}
              {/if}
            </div>
          {/if}
        </div>
      </div>
    {/if}

    <div class="field">
      <span class="label">Title</span>
      <input bind:this={titleEl} bind:value={title} placeholder="What needs doing?" />
    </div>

    <div class="field">
      <div class="labelrow"><span class="label">Description</span><span class="opt">optional · markdown</span></div>
      <textarea bind:value={description} rows="3" placeholder="What does done look like?"></textarea>
    </div>

    <div class="field">
      <div class="labelrow">
        <span class="label">Condition</span>
        <span class="opt">optional · the agent verifies this, fctrl never runs it</span>
      </div>
      <input class="mono" bind:value={condition} placeholder="pnpm test:auth --grep rotate" />
    </div>

    <div class="actions">
      <label class="another">
        <input type="checkbox" bind:checked={another} />
        Create another
      </label>
      <span class="spacer"></span>
      <button class="ghost" onclick={() => (app.dialog = null)}>Cancel</button>
      <button class="primary" disabled={!valid || busy} onclick={submit}>
        Create {kind === 'WORK_PACKAGE' ? 'package' : kind.toLowerCase()} ↵
      </button>
    </div>
  </div>
</div>

<style>
  .scrim {
    position: absolute;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: grid;
    place-items: center;
    z-index: 40;
    padding: 24px;
  }
  .dialog {
    width: 560px;
    max-width: 100%;
    max-height: 100%;
    overflow: auto;
    border-radius: 13px;
    background: var(--panel);
    border: 1px solid var(--border);
    box-shadow: var(--shadow);
    padding: 22px;
    display: flex;
    flex-direction: column;
    gap: 17px;
  }
  .head {
    display: flex;
    align-items: center;
  }
  .h {
    font-size: 17px;
    font-weight: 600;
  }
  .x {
    background: transparent;
    border: 0;
    color: var(--fg3);
    font-size: 14px;
    cursor: pointer;
  }
  .spacer {
    flex: 1;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .labelrow {
    display: flex;
    align-items: center;
    gap: 9px;
  }
  .opt {
    font-size: 11px;
    color: var(--fg3);
  }
  .seg {
    display: flex;
    gap: 7px;
  }
  .segbtn {
    flex: 1;
    padding: 9px 0;
    border-radius: 8px;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--fg2);
    font-family: inherit;
    font-size: 12.5px;
    cursor: pointer;
  }
  .segbtn.on {
    border-color: var(--accent);
    background: color-mix(in oklab, var(--accent) 12%, transparent);
    color: var(--accent);
    font-weight: 500;
  }
  .picker {
    position: relative;
  }
  .options {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    right: 0;
    z-index: 50;
    max-height: 220px;
    overflow-y: auto;
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: var(--shadow);
    padding: 4px;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .opt {
    display: flex;
    align-items: center;
    min-height: 30px;
    padding: 6px 10px;
    border-radius: 6px;
    border: 0;
    background: transparent;
    color: var(--fg2);
    cursor: pointer;
    font-family: inherit;
    font-size: 12.5px;
    line-height: 1.2;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .opt:hover,
  .opt.active {
    background: var(--hover);
    color: var(--fg);
  }
  .opt .mono {
    flex: 0 0 auto;
    line-height: 1;
    font-size: 11.5px;
    color: var(--fg3);
  }
  .opt .title {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .empty {
    padding: 8px 10px;
    color: var(--fg3);
    font-size: 12px;
  }
  input,
  textarea {
    width: 100%;
    box-sizing: border-box;
    padding: 10px 11px;
    border-radius: 8px;
    background: var(--panel2);
    border: 1px solid var(--border);
    color: var(--fg);
    font-family: inherit;
    font-size: 13px;
    outline: none;
    resize: vertical;
  }
  input:focus,
  textarea:focus {
    border-color: var(--accent);
  }
  input::placeholder,
  textarea::placeholder {
    color: var(--fg3);
  }
  .actions {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .another {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12.5px;
    color: var(--fg2);
    cursor: pointer;
  }
  .another input {
    width: auto;
    accent-color: var(--accent);
  }
  .ghost,
  .primary {
    padding: 9px 15px;
    border-radius: 8px;
    font-size: 13px;
    font-family: inherit;
    cursor: pointer;
    white-space: nowrap;
  }
  .ghost {
    border: 1px solid var(--border);
    background: transparent;
    color: var(--fg2);
  }
  .primary {
    border: 0;
    background: var(--accent);
    color: var(--accent-fg);
    font-weight: 500;
  }
  .primary:disabled {
    opacity: 0.45;
    cursor: default;
  }

  @media (max-width: 860px) {
    .scrim {
      place-items: end center;
      padding: 0;
    }
    .dialog {
      width: 100%;
      max-height: 92%;
      border-radius: 16px 16px 0 0;
      padding: 20px 16px calc(16px + env(safe-area-inset-bottom));
    }
    .seg {
      flex-wrap: wrap;
    }
    .segbtn {
      min-height: 44px;
      flex: 1 1 30%;
    }
    input,
    textarea {
      min-height: 44px;
      font-size: 16px; /* iOS zooms below 16 */
    }
    .actions {
      flex-wrap: wrap;
    }
    .ghost,
    .primary {
      min-height: 44px;
      flex: 1;
    }
  }
</style>
