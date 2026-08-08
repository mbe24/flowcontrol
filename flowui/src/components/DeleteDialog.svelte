<script lang="ts">
  import { app, deleteNode } from '../lib/state.svelte';
  import { buildIndex, descendantsOf } from '../lib/derive';

  interface Props {
    nodeId: string;
  }
  let { nodeId }: Props = $props();

  const index = $derived(buildIndex(app.nodes, app.deps));
  const node = $derived(app.nodes.find((n) => n.id === nodeId));
  const kids = $derived(node ? descendantsOf(app.nodes, nodeId) : []);
  const doomed = $derived(new Set([nodeId, ...kids.map((k) => k.id)]));
  const edges = $derived(
    app.deps.filter((d) => doomed.has(d.blockerId) || doomed.has(d.blockedId))
  );
  /** The surprising part: what stops being blocked when this goes away. */
  const unblocked = $derived(
    app.deps
      .filter((d) => doomed.has(d.blockerId) && !doomed.has(d.blockedId))
      .map((d) => index.byId.get(d.blockedId))
      .filter((n): n is NonNullable<typeof n> => !!n && n.status === 'BLOCKED')
  );
  const history = $derived(app.activity.filter((a) => doomed.has(a.nodeId)).length);
  const total = $derived(doomed.size);
  let busy = $state(false);
</script>

{#if node}
  <div class="scrim" onclick={() => (app.dialog = null)} role="presentation">
    <div class="dialog" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.key === 'Escape' && (app.dialog = null)} role="alertdialog" tabindex="-1">
      <div class="head">
        <span class="mark">!</span>
        <span class="h">Delete {node.id}?</span>
      </div>
      <p><span class="strong">{node.title}</span>{kids.length ? ' and everything under it.' : '.'}</p>

      <div class="tally">
        {#if kids.length}
          <div class="line">
            <span class="mono n">{kids.length}</span>
            <span class="what">{kids.length === 1 ? 'child node' : 'child nodes'}</span>
            <span class="note warn">deleted</span>
          </div>
        {/if}
        {#if edges.length}
          <div class="line">
            <span class="mono n">{edges.length}</span>
            <span class="what">dependency {edges.length === 1 ? 'edge' : 'edges'}</span>
            <span class="note">removed</span>
          </div>
        {/if}
        <div class="line">
          <span class="mono n ok">{history}</span>
          <span class="what">activity entries</span>
          <span class="note">kept — the log is append-only</span>
        </div>
      </div>

      {#if unblocked.length}
        <div class="cascade">
          <span class="ic">⊘</span>
          <span>
            {unblocked.map((n) => n.id).join(' and ')}
            {unblocked.length === 1 ? 'is' : 'are'} blocked by this. Deleting it will unblock
            {unblocked.length === 1 ? 'it' : 'them'} immediately.
          </span>
        </div>
      {/if}

      <div class="actions">
        <span class="fine">Undoable for 30s</span>
        <span class="spacer"></span>
        <button class="ghost" onclick={() => (app.dialog = null)}>Cancel</button>
        <button
          class="danger"
          disabled={busy}
          onclick={async () => {
            busy = true;
            await deleteNode(nodeId);
          }}>Delete {total} {total === 1 ? 'node' : 'nodes'}</button>
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
    width: 520px;
    max-width: 100%;
    border-radius: 13px;
    background: var(--panel);
    border: 1px solid color-mix(in oklab, var(--blocked) 45%, var(--border));
    box-shadow: var(--shadow);
    padding: 22px;
    display: flex;
    flex-direction: column;
    gap: 15px;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 11px;
  }
  .mark {
    width: 22px;
    height: 22px;
    border-radius: 5px;
    background: var(--blocked-bg);
    border: 1px solid var(--blocked);
    color: var(--blocked);
    display: grid;
    place-items: center;
    font-size: 11px;
  }
  .h {
    font-size: 17px;
    font-weight: 600;
  }
  p {
    margin: 0;
    font-size: 13.5px;
    line-height: 1.5;
    color: var(--fg2);
    text-wrap: pretty;
  }
  .strong {
    color: var(--fg);
  }
  .tally {
    display: flex;
    flex-direction: column;
    gap: 9px;
    padding: 14px 16px;
    border-radius: 9px;
    background: var(--panel2);
    border: 1px solid var(--border);
  }
  .line {
    display: flex;
    align-items: center;
    gap: 11px;
  }
  .n {
    width: 26px;
    font-size: 12px;
    color: var(--fg2);
  }
  .n.ok {
    color: var(--ready);
  }
  .what {
    flex: 1;
    font-size: 13px;
    color: var(--fg2);
  }
  .note {
    font-size: 11.5px;
    color: var(--fg3);
  }
  .note.warn {
    color: var(--blocked);
  }
  .cascade {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 12px 14px;
    border-radius: 8px;
    background: var(--blocked-bg);
    border: 1px solid color-mix(in oklab, var(--blocked) 45%, transparent);
    font-size: 12.5px;
    line-height: 1.5;
    color: var(--blocked);
    text-wrap: pretty;
  }
  .ic {
    flex: none;
    font-size: 12px;
  }
  .actions {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .fine {
    font-size: 11.5px;
    color: var(--fg3);
  }
  .spacer {
    flex: 1;
  }
  .ghost,
  .danger {
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
  .danger {
    border: 0;
    background: var(--blocked);
    color: #0e0f12;
    font-weight: 500;
  }
  .danger:disabled {
    opacity: 0.5;
  }

  @media (max-width: 860px) {
    .scrim {
      place-items: end center;
      padding: 0;
    }
    .dialog {
      width: 100%;
      border-radius: 16px 16px 0 0;
      padding: 20px 16px calc(16px + env(safe-area-inset-bottom));
    }
    .ghost,
    .danger {
      min-height: 44px;
      flex: 1;
    }
    .fine {
      width: 100%;
    }
    .actions {
      flex-wrap: wrap;
    }
  }
</style>
