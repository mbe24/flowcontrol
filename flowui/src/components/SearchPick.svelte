<script lang="ts">
  interface Item {
    id: string;
    title: string;
    /** Prefix the id (mono) e.g. for sibling tasks. */
    showId?: boolean;
  }

  let {
    items,
    value,
    onchange,
    flag = '',
    placeholder = ''
  }: {
    items: Item[];
    value: string | null;
    onchange: (id: string) => void;
    flag?: string;
    placeholder?: string;
  } = $props();

  let query = $state('');
  let open = $state(false);
  let idx = $state(-1);

  const filtered = $derived(
    query.trim()
      ? items.filter((i) => `${i.title} ${i.id}`.toLowerCase().includes(query.trim().toLowerCase()))
      : items
  );
  const current = $derived(value ? items.find((i) => i.id === value) : undefined);

  function pick(id: string) {
    query = '';
    open = false;
    onchange(id);
  }

  function key(e: KeyboardEvent) {
    e.stopPropagation();
    if (e.key === 'Escape') open = false;
    else if (e.key === 'Enter' && filtered.length) {
      e.preventDefault();
      pick(filtered[Math.max(idx, 0)].id);
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      open = true;
      idx = idx + 1 < filtered.length ? idx + 1 : 0;
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      idx = idx - 1 >= 0 ? idx - 1 : filtered.length - 1;
    } else {
      idx = -1;
    }
  }
</script>

<div class="picker">
  <input
    class="mono"
    bind:value={query}
    placeholder={current ? current.title : placeholder}
    onfocus={() => (open = true)}
    onblur={() => window.setTimeout(() => (open = false), 120)}
    onkeydown={key}
  />
  {#if flag}<span class="mono flag">{flag}</span>{/if}

  {#if open}
    <div class="options">
      {#if filtered.length === 0}
        <div class="empty">No matches</div>
      {:else}
        {#each filtered as item, i (item.id)}
          <button
            class="opt"
            class:active={idx === i}
            onmouseenter={() => (idx = i)}
            onmousedown={(e) => {
              e.preventDefault();
              pick(item.id);
            }}>
            {#if item.showId}<span class="mono">{item.id} ·&#160;</span>{/if}
            <span class="title">{item.title}</span>
          </button>
        {/each}
      {/if}
    </div>
  {/if}
</div>

<style>
  .picker {
    position: relative;
    display: flex;
    align-items: center;
    gap: 9px;
  }
  input {
    flex: 1;
    min-width: 0;
    box-sizing: border-box;
    padding: 10px 11px;
    border-radius: 8px;
    background: var(--panel2);
    border: 1px solid var(--border);
    color: var(--fg);
    font-family: inherit;
    font-size: 13px;
    outline: none;
  }
  input:focus {
    border-color: var(--accent);
  }
  .flag {
    font-size: 10px;
    color: var(--accent);
    flex: none;
    white-space: nowrap;
  }
  .options {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    right: 0;
    z-index: 50;
    max-height: 220px;
    overflow-y: auto;
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: var(--shadow);
    padding: 4px;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .opt {
    display: flex;
    align-items: center;
    min-height: 30px;
    padding: 6px 10px;
    border-radius: 6px;
    border: 0;
    background: transparent;
    color: var(--fg2);
    cursor: pointer;
    font-family: inherit;
    font-size: 12.5px;
    line-height: 1.2;
    white-space: nowrap;
    overflow: hidden;
    text-align: left;
  }
  .opt:hover,
  .opt.active {
    background: var(--hover);
    color: var(--fg);
  }
  .opt .mono {
    flex: 0 0 auto;
    line-height: 1;
    font-size: 11.5px;
    color: var(--fg3);
  }
  .opt .title {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .empty {
    padding: 8px 10px;
    color: var(--fg3);
    font-size: 12px;
  }

  @media (max-width: 860px) {
    input {
      min-height: 44px;
      font-size: 16px;
    }
  }
</style>
