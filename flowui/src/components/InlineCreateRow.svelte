<script lang="ts">
  import { createNode } from '../lib/state.svelte';
  import type { NodeType } from '../lib/types';

  interface Props {
    projectId: string;
    parentId: string;
    type: NodeType;
    /** 'ghost' dims until hover; 'prominent' is the empty-state variant. */
    variant?: 'ghost' | 'prominent';
    label: string;
    /** Tab escalates to the full dialog, carrying the typed title. */
    onEscalate?: (title: string) => void;
  }
  let { projectId, parentId, type, variant = 'ghost', label, onEscalate }: Props = $props();

  let value = $state('');
  let active = $state(false);
  let busy = $state(false);
  let el: HTMLInputElement | undefined = $state();

  async function commit() {
    const title = value.trim();
    if (!title || busy) return;
    busy = true;
    try {
      await createNode({ projectId, parentId, type, title });
      value = '';
      // Stay open — a run of five steps should be five lines of typing.
      el?.focus();
    } finally {
      busy = false;
    }
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Enter') {
      e.preventDefault();
      commit();
    } else if (e.key === 'Tab' && !e.shiftKey && onEscalate) {
      e.preventDefault();
      onEscalate(value.trim());
      value = '';
    } else if (e.key === 'Escape') {
      value = '';
      el?.blur();
    }
  }
</script>

<div
  class="row {variant}"
  class:active={active || value.length > 0}
  onclick={() => el?.focus()}
  role="presentation">
  <span class="plus">+</span>
  <input
    bind:this={el}
    bind:value
    placeholder={label}
    onfocus={() => (active = true)}
    onblur={() => (active = false)}
    onkeydown={onKey} />
  {#if value.length > 0}
    <span class="mono kbd">↵ add</span>
    {#if onEscalate}<span class="mono kbd">⇥ more</span>{/if}
  {/if}
</div>

<style>
  .row {
    display: flex;
    align-items: center;
    gap: 11px;
    padding: 0 14px;
    height: 36px;
    cursor: text;
    border-bottom: 1px solid var(--border2);
    opacity: 0.42;
    transition: opacity 0.12s ease-out, background 0.12s ease-out;
  }
  .row:hover,
  .row.active {
    opacity: 1;
    background: var(--panel2);
  }
  .row.prominent {
    opacity: 1;
    border: 1px solid var(--accent);
    border-radius: 8px;
    background: var(--panel2);
    height: 40px;
    width: 100%;
    box-sizing: border-box;
  }
  .plus {
    color: var(--accent);
    font-size: 14px;
    flex: none;
    line-height: 1;
  }
  input {
    flex: 1;
    min-width: 0;
    background: transparent;
    border: 0;
    outline: none;
    color: var(--fg);
    font-family: inherit;
    font-size: 12.5px;
  }
  input::placeholder {
    color: var(--fg3);
  }
  .kbd {
    flex: none;
    font-size: 10px;
    color: var(--fg3);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 2px 6px;
  }

  @media (max-width: 860px) {
    /* Never below the 44px touch target, and always visible — a 42%-opacity
       row is undiscoverable without hover. */
    .row {
      height: 44px;
      opacity: 1;
    }
    .row.prominent {
      height: 48px;
    }
    input {
      font-size: 16px;
    }
    .kbd {
      display: none;
    }
  }
</style>
