<script lang="ts">
  import { updateNode } from '../lib/state.svelte';

  interface Props {
    nodeId: string;
    field: 'title' | 'condition' | 'note';
    value: string;
    /** Rendered when not editing. */
    placeholder?: string;
    multiline?: boolean;
    klass?: string;
    /** Open in edit mode immediately, with the text selected — used by Rename.
        The parent should clear it (via onStarted) so editing is exit-able. */
    autoEdit?: boolean;
    onStarted?: () => void;
  }
  let {
    nodeId,
    field,
    value,
    placeholder = 'empty',
    multiline = false,
    klass = '',
    autoEdit = false,
    onStarted
  }: Props = $props();

  let editing = $state(false);
  let draft = $state('');
  let el: HTMLInputElement | HTMLTextAreaElement | undefined = $state();

  function start(selectAll = false) {
    draft = value;
    editing = true;
    queueMicrotask(() => {
      el?.focus();
      if (selectAll) el?.select();
    });
  }

  // Rename arrives with autoEdit set: one keystroke should replace the title.
  // entering editing makes `!editing` false so it fires once; the parent's
  // onStarted clears autoEdit so saving doesn't reopen it.
  $effect(() => {
    if (autoEdit && !editing) {
      start(true);
      onStarted?.();
    }
  });

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
  {#if !klass.includes('inline')}
    <span class="mono hint">{multiline ? '⌘↵' : '↵'} save · esc revert</span>
  {/if}
{:else}
  <button class="display {klass}" class:empty={!value} onclick={() => start()} title="Click to edit">
    {value || placeholder}
  </button>
{/if}

<style>
  /* Inside a step row the contracted field sits in a flex line, so it must not
     claim the full width or push the fold caret off the end. */
  .display.inline {
    display: inline-block;
    width: auto;
    max-width: 100%;
    margin: 0;
    padding: 2px 6px;
  }
  /* While editing, the field fills the whole row (`.field` is already block +
     width:100% + box-sizing), so a step title/condition/note isn't a sliver. */
  .field.inline {
    margin: 0;
    padding: 2px 6px;
    min-width: 180px;
    width: 100%;
  }
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
  /* Muted when it's just a placeholder (no value yet). */
  .display.empty {
    color: var(--fg3);
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
  @media (max-width: 860px) {
    .field {
      font-size: 16px;
      min-height: 44px;
    }
    .display {
      min-height: 32px;
    }
  }
  .hint {
    display: block;
    margin-top: 7px;
    font-size: 10px;
    color: var(--fg3);
  }
</style>
