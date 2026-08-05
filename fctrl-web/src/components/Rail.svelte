<script lang="ts">
  import { app, load, toggleTheme } from '../lib/state.svelte';
  import logo from '../assets/logo-mark-dark.svg';

  const abbr = (name: string) =>
    name
      .split(/\s+/)
      .slice(0, 2)
      .map((w) => w[0]?.toUpperCase() ?? '')
      .join('');
</script>

<div class="rail">
  <img class="logo" src={logo} alt="FlowControl" />
  <div class="hr"></div>
  {#each app.projects as p (p.id)}
    <button
      class="proj mono"
      class:active={p.id === app.projectId}
      title={p.name}
      onclick={() => load(p.id)}>{abbr(p.name)}</button
    >
  {/each}
  <button class="proj plus" title="New project">+</button>
  <div class="spacer"></div>
  <button class="proj" title="Toggle theme" onclick={toggleTheme}>
    {app.theme === 'dark' ? '☾' : '☀'}
  </button>
</div>

<style>
  .rail {
    width: 52px;
    flex: none;
    background: var(--panel);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 14px 0;
    gap: 9px;
  }
  .logo {
    width: 26px;
    height: 26px;
    display: block;
  }
  .hr {
    width: 26px;
    height: 1px;
    background: var(--border);
    margin: 3px 0;
  }
  .proj {
    width: 30px;
    height: 30px;
    border-radius: 8px;
    display: grid;
    place-items: center;
    font-size: 11px;
    background: transparent;
    border: 1px solid transparent;
    color: var(--fg3);
    cursor: pointer;
    font-family: 'IBM Plex Mono', monospace;
  }
  .proj:hover {
    background: var(--hover);
    color: var(--fg);
  }
  .proj.active {
    background: var(--hover);
    border-color: var(--border);
    color: var(--fg);
  }
  .plus {
    font-size: 15px;
  }
  .spacer {
    flex: 1;
  }
</style>
