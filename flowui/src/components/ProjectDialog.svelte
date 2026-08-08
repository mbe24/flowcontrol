<script lang="ts">
  import { app, createProject, updateProject } from '../lib/state.svelte';

  interface Props {
    /** Set to edit an existing project instead of creating one. */
    projectId?: string;
  }
  let { projectId }: Props = $props();

  const existing = $derived(projectId ? app.projects.find((p) => p.id === projectId) : undefined);

  // Seed the form once from the project; `existing` is a prop/derived snapshot.
  // svelte-ignore state_referenced_locally
  let name = $state(existing?.name ?? '');
  // svelte-ignore state_referenced_locally
  let description = $state(existing?.description ?? '');
  /** A brand-new project has no work package, so the table has nowhere to
      show a ghost row. Seeding one keeps the inline path working. */
  let seed = $state(true);
  let busy = $state(false);
  let el: HTMLInputElement | undefined = $state();

  $effect(() => {
    el?.focus();
  });

  const valid = $derived(name.trim().length > 0);

  async function submit() {
    if (!valid || busy) return;
    busy = true;
    try {
      if (existing) await updateProject(existing.id, { name: name.trim(), description: description.trim() });
      else await createProject(name.trim(), description.trim(), seed);
    } finally {
      busy = false;
    }
  }
</script>

<div class="scrim" onclick={() => (app.dialog = null)} role="presentation">
  <div
    class="dialog"
    onclick={(e) => e.stopPropagation()}
    onkeydown={(e) => {
      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey || (e.target as HTMLElement).tagName === 'INPUT')) submit();
    }}
    role="dialog"
    tabindex="-1">
    <span class="h">{existing ? 'Edit project' : 'New project'}</span>

    <div class="field">
      <span class="label">Name</span>
      <input bind:this={el} bind:value={name} placeholder="Fleet Dashboard" />
    </div>

    <div class="field">
      <div class="labelrow"><span class="label">Description</span><span class="opt">optional</span></div>
      <textarea bind:value={description} rows="2" placeholder="What is this project for?"></textarea>
    </div>

    {#if !existing}
      <div class="field">
        <span class="label">Start with</span>
        <div class="seg">
          <button class="segbtn" class:on={!seed} onclick={() => (seed = false)}>Empty</button>
          <button class="segbtn" class:on={seed} onclick={() => (seed = true)}>One work package</button>
        </div>
      </div>
    {/if}

    {#if existing}
      <span class="fine">
        Project mutations are not in <span class="mono">FlowService</span> yet — this writes to the
        local store only.
      </span>
    {/if}

    <div class="actions">
      <span class="spacer"></span>
      <button class="ghost" onclick={() => (app.dialog = null)}>Cancel</button>
      <button class="primary" disabled={!valid || busy} onclick={submit}>
        {existing ? 'Save' : 'Create'} ↵
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
    z-index: 42;
    padding: 24px;
  }
  .dialog {
    width: 460px;
    max-width: 100%;
    border-radius: 13px;
    background: var(--panel);
    border: 1px solid var(--border);
    box-shadow: var(--shadow);
    padding: 22px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .h {
    font-size: 17px;
    font-weight: 600;
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
  .seg {
    display: flex;
    gap: 8px;
  }
  .segbtn {
    flex: 1;
    padding: 10px 0;
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
  .fine {
    font-size: 11.5px;
    line-height: 1.45;
    color: var(--fg3);
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
      border-radius: 16px 16px 0 0;
      padding: 20px 16px calc(16px + env(safe-area-inset-bottom));
    }
    input,
    textarea {
      min-height: 44px;
      font-size: 16px;
    }
    .segbtn,
    .ghost,
    .primary {
      min-height: 44px;
      flex: 1;
    }
  }
</style>
