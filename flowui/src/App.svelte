<script lang="ts">
  import { onMount } from 'svelte';
  import { app, boot, closeOverlays } from './lib/state.svelte';
  import Rail from './components/Rail.svelte';
  import TopBar from './components/TopBar.svelte';
  import FilterBar from './components/FilterBar.svelte';
  import TableView from './components/TableView.svelte';
  import LanesView from './components/LanesView.svelte';
  import GraphView from './components/GraphView.svelte';
  import DetailPanel from './components/DetailPanel.svelte';
  import DetailSheet from './components/DetailSheet.svelte';
  import OverrideDialog from './components/OverrideDialog.svelte';
  import CreateNodeDialog from './components/CreateNodeDialog.svelte';
  import DeleteDialog from './components/DeleteDialog.svelte';
  import MoveDialog from './components/MoveDialog.svelte';
  import ProjectDialog from './components/ProjectDialog.svelte';
  import ProjectSwitcher from './components/ProjectSwitcher.svelte';
  import FilterPopover from './components/FilterPopover.svelte';
  import NodeMenu from './components/NodeMenu.svelte';
  import Palette from './components/Palette.svelte';

  onMount(boot);

  const mobile = $derived(app.width < 860);
  const menuNode = $derived(app.nodes.find((n) => n.id === app.nodeMenuFor));

  function onKeydown(e: KeyboardEvent) {
    const typing =
      e.target instanceof HTMLInputElement ||
      e.target instanceof HTMLTextAreaElement ||
      e.target instanceof HTMLSelectElement;

    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      app.paletteOpen = !app.paletteOpen;
      app.paletteQuery = '';
      return;
    }
    if (e.key === 'Escape') {
      if (app.dialog) app.dialog = null;
      else if (app.confirmOverride) app.confirmOverride = null;
      else if (app.nodeMenuFor) app.nodeMenuFor = null;
      else if (app.filterOpen) app.filterOpen = false;
      else if (app.projectMenuOpen) app.projectMenuOpen = false;
      else if (app.paletteOpen) app.paletteOpen = false;
      else if (app.sheet === 'full') app.sheet = 'peek';
      return;
    }
    if (typing || app.dialog) return;

    if (e.key === '1') app.view = 'table';
    if (e.key === '2') app.view = 'lanes';
    if (e.key === '3') app.view = 'graph';
    if (e.key === 'f' && !e.metaKey && !e.ctrlKey) {
      e.preventDefault();
      app.filterOpen = !app.filterOpen;
    }
    if (e.key === 'n' && app.selectedId) {
      e.preventDefault();
      const sel = app.nodes.find((n) => n.id === app.selectedId);
      if (sel) app.dialog = { kind: 'create', nodeType: 'STEP', parentId: sel.id, title: '' };
    }
    if (e.key === 'F2' && app.selectedId) {
      e.preventDefault();
      app.renameId = app.selectedId;
    }
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

  {#if app.projectMenuOpen}
    <ProjectSwitcher />
  {/if}
  {#if app.filterOpen}
    <FilterPopover />
  {/if}
  {#if app.nodeMenuFor && menuNode}
    <NodeMenu node={menuNode} x={app.menuAt.x} y={app.menuAt.y} onclose={() => (app.nodeMenuFor = null)} />
  {/if}
  {#if app.paletteOpen}
    <Palette />
  {/if}

  {#if app.dialog?.kind === 'create'}
    <CreateNodeDialog
      nodeType={app.dialog.nodeType}
      parentId={app.dialog.parentId}
      initialTitle={app.dialog.title} />
  {:else if app.dialog?.kind === 'delete'}
    <DeleteDialog nodeId={app.dialog.nodeId} />
  {:else if app.dialog?.kind === 'move'}
    <MoveDialog nodeId={app.dialog.nodeId} to={app.dialog.to} />
  {:else if app.dialog?.kind === 'newProject'}
    <ProjectDialog />
  {:else if app.dialog?.kind === 'editProject'}
    <ProjectDialog projectId={app.dialog.projectId} />
  {/if}

  <OverrideDialog />
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
