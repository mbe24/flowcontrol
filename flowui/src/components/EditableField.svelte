<script lang="ts">
  import { updateNode } from '../lib/state.svelte';

  interface Props {
    nodeId: string;
    field: 'title' | 'condition';
    value: string;
    /** Rendered when not editing. */
    placeholder?: string;
    multiline?: boolean;
    klass?: string;
  }
  let { nodeId, field, value, placeholder = 'empty', multiline = false, klass = '' }: Props = $props();

  let editing = $state(false);
  let draft = $state('');
  let el: HTMLInputElement | HTMLTextAreaElement | undefined = $state();

  function start() {
    draft = value;
    editing = true;
    queueMicrotask(() => el?.focus());
  }

  async function save() {
    const next = draft.trim();
    editing = false;
    if (next === value) return;
    await updateNode(nodeId, { [field]: next });
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.stopPropagation();
      editing = false;
    } else if (e.key === 'Enter' && (!multiline || e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      save();
    }
  }
</script>

{#if editing}
  {#if multiline}
    <textarea
      bind:this={el}
      bind:value={draft}
      class="field {klass}"
      onblur={save}
      onkeydown={onKey}
      rows="3"></textarea>
  {:else}
    <input bind:this={el} bind:value={draft} class="field {klass}" onblur={save} onkeydown={onKey} />
  {/if}
  <span class="mono hint">{multiline ? '⌘↵' : '↵'} save · esc revert</span>
{:else}
  <button class="display {klass}" onclick={start} title="Click to edit">
    {value || placeholder}
  </button>
{/if}

<style>
  .display {
    display: block;
    width: 100%;
    text-align: left;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    padding: 5px 8px;
    margin: -5px -8px;
    color: inherit;
    font: inherit;
    cursor: text;
  }
  .display:hover {
    background: var(--panel2);
    border-color: var(--border);
  }
  .field {
    display: block;
    width: 100%;
    box-sizing: border-box;
    background: var(--panel2);
    border: 1px solid var(--accent);
    border-radius: 6px;
    padding: 5px 8px;
    margin: -5px -8px 0;
    color: var(--fg);
    font: inherit;
    outline: none;
    resize: vertical;
  }
  .hint {
    display: block;
    margin-top: 7px;
    font-size: 10px;
    color: var(--fg3);
  }
</style>
