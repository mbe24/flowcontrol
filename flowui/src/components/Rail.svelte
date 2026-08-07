<script lang="ts">
  import { app, load, toggleTheme } from '../lib/state.svelte';
  import Logo from './Logo.svelte';

  const abbr = (name: string) =>
    name.split(/\s+/).slice(0, 2).map((w) => w[0]?.toUpperCase() ?? '').join('');

  const HUES = ['auth', 'booking', 'pay', 'obs', 'ui'];
  const visible = $derived(app.projects.filter((p) => !p.archived).slice(0, 5));
</script>

<div class="rail">
  <button class="logo" onclick={() => (app.projectMenuOpen = !app.projectMenuOpen)} title="Switch project">
    <Logo size={28} />
  </button>
  <div class="hr"></div>
  {#each visible as p (p.id)}
    <button
      class="proj mono"
      class:active={p.id === app.projectId}
      style:--hue="var(--hue-{HUES[app.projects.indexOf(p) % 5]})"
      title={p.name}
      onclick={() => load(p.id)}>{abbr(p.name)}</button>
  {/each}
  <button
    class="proj plus"
    title="All projects"
    onclick={() => (app.projectMenuOpen = !app.projectMenuOpen)}>+</button>
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
    width: 32px;
    height: 32px;
    border-radius: 8px;
    display: grid;
    place-items: center;
    color: var(--accent);
    background: transparent;
    border: 0;
    cursor: pointer;
    flex: none;
  }
  .logo:hover {
    background: var(--hover);
  }
  .hr {
    width: 26px;
    height: 1px;
    background: var(--border);
    margin: 3px 0;
    flex: none;
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
    flex: none;
  }
  .proj:hover {
    background: var(--hover);
    color: var(--fg);
  }
  .proj.active {
    background: var(--hover);
    border-color: var(--hue, var(--border));
    color: var(--fg);
  }
  .plus {
    font-size: 15px;
  }
  .spacer {
    flex: 1;
  }
</style>
