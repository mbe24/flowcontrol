<script lang="ts">
  import { onMount } from 'svelte';
  import { app, boot } from './lib/state.svelte';
  import Rail from './components/Rail.svelte';
  import TopBar from './components/TopBar.svelte';
  import FilterBar from './components/FilterBar.svelte';
  import TableView from './components/TableView.svelte';
  import LanesView from './components/LanesView.svelte';
  import GraphView from './components/GraphView.svelte';
  import DetailPanel from './components/DetailPanel.svelte';
  import DetailSheet from './components/DetailSheet.svelte';
  import OverrideDialog from './components/OverrideDialog.svelte';
  import Palette from './components/Palette.svelte';
  import NewTaskDialog from './components/NewTaskDialog.svelte';

  onMount(boot);

  const mobile = $derived(app.width < 860);

  function onKeydown(e: KeyboardEvent) {
    const typing = e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement;
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      app.paletteOpen = !app.paletteOpen;
      app.paletteQuery = '';
      return;
    }
    if (e.key === 'Escape') {
      if (app.confirmOverride) app.confirmOverride = null;
      else if (app.paletteOpen) app.paletteOpen = false;
      else if (app.sheet === 'full') app.sheet = 'peek';
      return;
    }
    if (typing) return;
    if (e.key === '1') app.view = 'table';
    if (e.key === '2') app.view = 'lanes';
    if (e.key === '3') app.view = 'graph';
    /** Toggle the expanded detail panel. */
    if (e.key === '\\' && app.selectedId && !mobile) {
      app.panelMode = app.panelMode === 'peek' ? 'expanded' : 'peek';
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="shell" class:mobile>
  {#if !mobile}
    <Rail />
  {/if}
  <div class="main">
    <TopBar />
    {#if !mobile}
      <FilterBar />
    {/if}
    <div class="body">
      <div class="views">
        {#if app.error}
          <div class="msg blocked">{app.error}</div>
        {:else if app.loading}
          <div class="msg">loading project…</div>
        {:else if app.view === 'table'}
          <TableView />
        {:else if app.view === 'lanes'}
          <LanesView />
        {:else}
          <GraphView />
        {/if}
      </div>
      {#if app.selectedId && !mobile}
        <DetailPanel />
      {/if}
    </div>
  </div>
  {#if mobile}
    <DetailSheet />
  {/if}
  {#if app.paletteOpen}
    <Palette />
  {/if}
  <OverrideDialog />
  <NewTaskDialog />
</div>

<style>
  .shell {
    display: flex;
    height: 100%;
    position: relative;
    overflow: hidden;
  }
  .main {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
  }
  .body {
    flex: 1;
    display: flex;
    min-height: 0;
  }
  .views {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }
  .msg {
    padding: 22px 20px;
    color: var(--fg3);
    font-size: 12px;
  }
  .blocked {
    color: var(--blocked);
  }
</style>
