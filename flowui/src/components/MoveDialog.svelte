<script lang="ts">
  import { app, addDependency, moveNode } from '../lib/state.svelte';
  import { buildIndex, stepsOf, tasksOf, workPackages } from '../lib/derive';
  import type { NodeType } from '../lib/types';
  import SearchPick from './SearchPick.svelte';

  interface Props {
    nodeId: string;
    to: NodeType;
  }
  let { nodeId, to }: Props = $props();

  const index = $derived(buildIndex(app.nodes, app.deps));
  const node = $derived(app.nodes.find((n) => n.id === nodeId));
  const promoting = $derived(!!node && node.type === 'STEP' && to === 'TASK');
  const demoting = $derived(!!node && node.type === 'TASK' && to === 'STEP');
  const moving = $derived(!promoting && !demoting);

  /** Where it can land. Promote → the grandparent package; demote → sibling tasks. */
  const targets = $derived.by(() => {
    if (!node) return [];
    if (promoting) return workPackages(app.nodes);
    if (demoting) {
      const wp = node.parentId;
      return app.nodes.filter((n) => n.type === 'TASK' && n.parentId === wp && n.id !== node.id);
    }
    return workPackages(app.nodes).filter((w) => w.id !== node.parentId);
  });

  let target = $state('');
  $effect(() => {
    if (!targets.some((t) => t.id === target)) target = targets[0]?.id ?? '';
  });

  /** Promoting a step out of a task: the parent isn't done until this is. */
  let blockParent = $state(true);
  let busy = $state(false);

  const parentTask = $derived(promoting && node?.parentId ? index.byId.get(node.parentId) : undefined);
  const droppedSteps = $derived(demoting && node ? stepsOf(app.nodes, node.id).length : 0);
  const keptEdges = $derived(
    node ? app.deps.filter((d) => d.blockerId === node.id || d.blockedId === node.id).length : 0
  );
  const history = $derived(node ? app.activity.filter((a) => a.nodeId === node.id).length : 0);
  const targetBlocksIt = $derived(
    !!node && demoting && (index.blockers.get(node.id) ?? []).includes(target)
  );

  async function submit() {
    if (!node || !target || busy) return;
    busy = true;
    try {
      const oldParent = node.parentId;
      await moveNode(node.id, target, to);
      if (promoting && blockParent && oldParent) await addDependency(node.id, oldParent);
    } finally {
      busy = false;
    }
  }

  const heading = $derived(promoting ? 'Promote to task?' : demoting ? 'Demote to step?' : 'Move to package?');
  const accent = $derived(demoting ? 'var(--hue-ui)' : 'var(--accent)');
</script>

{#if node}
  <div class="scrim" onclick={() => (app.dialog = null)} role="presentation">
    <div class="dialog" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.key === 'Escape' && (app.dialog = null)} role="dialog" tabindex="-1">
      <span class="h">{heading}</span>

      <div class="subject">
        <span class="dot" style:background="var(--{node.status === 'DONE' ? 'done' : node.status === 'READY' ? 'ready' : node.status === 'BLOCKED' ? 'blocked' : 'deferred'})"></span>
        <span class="mono sid">{node.id}</span>
        <span class="stitle">{node.title}</span>
      </div>

      <div class="field">
        <span class="label">
          {promoting ? 'Becomes a task in' : demoting ? 'Becomes a step of' : 'Move to'}
        </span>
        {#if targets.length}
          <SearchPick
            items={targets.map((t) => ({
              id: t.id,
              title: t.title,
              showId: t.type === 'TASK'
            }))}
            value={target}
            onchange={(id) => (target = id)}
            flag={targetBlocksIt ? 'blocks it' : ''}
            placeholder={demoting ? 'Search task…' : 'Search work package…'}
          />
        {:else}
          <span class="fine">Nowhere to move this — there is no other {demoting ? 'task in this package' : 'work package'}.</span>
        {/if}
      </div>

      {#if promoting && parentTask}
        <label class="check">
          <input type="checkbox" bind:checked={blockParent} />
          <span class="cbody">
            <span class="ctitle">Block {parentTask.id} until it's done</span>
            <span class="cnote">The parent isn't finished until this work is. Uncheck to extract it cleanly.</span>
          </span>
        </label>
      {/if}

      {#if demoting}
        <div class="tally">
          {#if droppedSteps}
            <div class="line">
              <span class="mono n warn">{droppedSteps}</span>
              <span class="what">{droppedSteps === 1 ? 'step' : 'steps'}</span>
              <span class="note warn">discarded</span>
            </div>
          {/if}
          <div class="line">
            <span class="mono n">{keptEdges}</span>
            <span class="what">dependency {keptEdges === 1 ? 'edge' : 'edges'}</span>
            <span class="note">kept</span>
          </div>
          <div class="line">
            <span class="mono n ok">{history}</span>
            <span class="what">activity entries</span>
            <span class="note">kept — append-only</span>
          </div>
        </div>
        <span class="fine">The title carries over and becomes the step.</span>
      {/if}

      <div class="actions">
        {#if demoting && droppedSteps}
          <span class="fine warn">Dropped steps can't be recovered.</span>
        {/if}
        <span class="spacer"></span>
        <button class="ghost" onclick={() => (app.dialog = null)}>Cancel</button>
        <button
          class="primary"
          style:background={accent}
          disabled={!target || busy}
          onclick={submit}>{promoting ? 'Promote' : demoting ? 'Demote' : 'Move'} ↵</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .scrim {
    position: absolute;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: grid;
    place-items: center;
    z-index: 42;
    padding: 24px;
  }
  .dialog {
    width: 500px;
    max-width: 100%;
    border-radius: 13px;
    background: var(--panel);
    border: 1px solid var(--border);
    box-shadow: var(--shadow);
    padding: 22px;
    display: flex;
    flex-direction: column;
    gap: 15px;
  }
  .h {
    font-size: 17px;
    font-weight: 600;
  }
  .subject {
    display: flex;
    align-items: center;
    gap: 11px;
    padding: 11px 13px;
    border-radius: 8px;
    background: var(--panel2);
    border: 1px solid var(--border);
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex: none;
  }
  .sid {
    font-size: 10.5px;
    color: var(--fg3);
  }
  .stitle {
    flex: 1;
    font-size: 13.5px;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .check {
    display: flex;
    align-items: flex-start;
    gap: 11px;
    padding: 13px 14px;
    border-radius: 8px;
    background: var(--panel2);
    border: 1px solid var(--border);
    cursor: pointer;
  }
  .check input {
    margin-top: 2px;
    accent-color: var(--accent);
  }
  .cbody {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .ctitle {
    font-size: 13px;
  }
  .cnote {
    font-size: 11.5px;
    line-height: 1.45;
    color: var(--fg3);
    text-wrap: pretty;
  }
  .tally {
    display: flex;
    flex-direction: column;
    gap: 9px;
    padding: 13px 14px;
    border-radius: 8px;
    background: var(--panel2);
    border: 1px solid var(--border);
  }
  .line {
    display: flex;
    align-items: center;
    gap: 11px;
  }
  .n {
    width: 22px;
    font-size: 12px;
    color: var(--fg2);
  }
  .n.warn {
    color: var(--hue-ui);
  }
  .n.ok {
    color: var(--ready);
  }
  .what {
    flex: 1;
    font-size: 12.5px;
    color: var(--fg2);
  }
  .note {
    font-size: 11px;
    color: var(--fg3);
  }
  .note.warn {
    color: var(--hue-ui);
  }
  .fine {
    font-size: 11.5px;
    line-height: 1.45;
    color: var(--fg3);
  }
  .fine.warn {
    color: var(--hue-ui);
  }
  .actions {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .spacer {
    flex: 1;
  }
  .ghost,
  .primary {
    padding: 9px 15px;
    border-radius: 8px;
    font-size: 13px;
    font-family: inherit;
    cursor: pointer;
  }
  .ghost {
    border: 1px solid var(--border);
    background: transparent;
    color: var(--fg2);
  }
  .primary {
    border: 0;
    color: #0e0f12;
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
      overflow: auto;
      border-radius: 16px 16px 0 0;
      padding: 20px 16px calc(16px + env(safe-area-inset-bottom));
    }
    .ghost,
    .primary {
      min-height: 44px;
      flex: 1;
    }
    .actions {
      flex-wrap: wrap;
    }
    .fine {
      width: 100%;
    }
  }
</style>
