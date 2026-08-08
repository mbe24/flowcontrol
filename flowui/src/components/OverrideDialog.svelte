<script lang="ts">
  import { app, confirmOverride } from '../lib/state.svelte';

  const node = $derived(app.nodes.find((n) => n.id === app.confirmOverride));
</script>

{#if node}
  <div class="scrim" role="presentation" onclick={() => (app.confirmOverride = null)}>
    <div class="dialog" role="alertdialog" tabindex="-1" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.key === 'Escape' && (app.confirmOverride = null)}>
      <div class="head">
        <span class="mark">✕</span>
        <span class="h">The agent reported a failure</span>
      </div>
      <p>
        {node.verification?.agentName || 'The agent'} ran
        <span class="mono">{node.condition}</span>
        {node.verification?.agentWhen} and it failed. Marking this verified records your
        acceptance over that result.
      </p>
      <div class="actions">
        <button class="ghost" onclick={() => (app.confirmOverride = null)}>Cancel</button>
        <button class="primary" onclick={confirmOverride}>Accept anyway</button>
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
    z-index: 40;
    padding: 20px;
  }
  .dialog {
    width: 340px;
    max-width: 100%;
    border-radius: 11px;
    background: var(--panel);
    border: 1px solid var(--border);
    box-shadow: var(--shadow);
    padding: 17px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 9px;
  }
  .mark {
    width: 18px;
    height: 18px;
    border-radius: 4px;
    background: var(--blocked-bg);
    border: 1px solid var(--blocked);
    color: var(--blocked);
    display: grid;
    place-items: center;
    font-size: 10px;
  }
  .h {
    font-size: 13.5px;
    font-weight: 600;
  }
  p {
    margin: 0;
    font-size: 12px;
    line-height: 1.55;
    color: var(--fg2);
    text-wrap: pretty;
  }
  .mono {
    font-family: 'IBM Plex Mono', monospace;
    font-size: 11px;
    color: var(--fg);
  }
  .actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    padding-top: 1px;
  }
  button {
    padding: 7px 13px;
    border-radius: 7px;
    font-size: 12px;
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
    background: var(--accent);
    color: var(--accent-fg);
    font-weight: 500;
  }
</style>
