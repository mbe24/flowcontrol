<script lang="ts">
  import { app, createTask, closeTaskDialog } from '../lib/state.svelte';
</script>

{#if app.taskDialog}
  <div
    class="backdrop"
    role="button"
    tabindex="0"
    onclick={(e) => {
      if (e.target === e.currentTarget) closeTaskDialog();
    }}
    onkeydown={(e) => {
      if (e.key === 'Escape' || e.key === 'Enter') closeTaskDialog();
    }}
  >
    <div class="sheet">
      <div class="title">New task</div>
      <input
        bind:value={app.taskTitle}
        type="text"
        placeholder="Task title"
        onkeydown={(e) => {
          if (e.key === 'Enter') createTask();
          if (e.key === 'Escape') closeTaskDialog();
        }}
      />
      <div class="row">
        <button class="ghost" onclick={closeTaskDialog}>Cancel</button>
        <button class="primary" onclick={createTask}>Create</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: absolute;
    inset: 0;
    background: color-mix(in srgb, var(--bg) 70%, transparent);
    display: grid;
    place-items: center;
    z-index: 40;
  }
  .sheet {
    width: 320px;
    max-width: 90vw;
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
    box-shadow: 0 12px 40px rgba(0, 0, 0, 0.35);
  }
  .title {
    font-size: 14px;
    font-weight: 600;
  }
  input {
    background: var(--panel2);
    border: 1px solid var(--border);
    border-radius: 8px;
    color: var(--fg);
    padding: 8px 10px;
    font: inherit;
  }
  input:focus {
    outline: none;
    border-color: var(--accent);
  }
  .row {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
