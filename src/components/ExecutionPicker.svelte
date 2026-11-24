<script lang="ts">
  import { tick } from "svelte";
  import { Search, ChevronDown, Check, Clock3 } from "@lucide/svelte";
  import type { Execution } from "../lib/types";
  import { short } from "../lib/format";
  import Status from "./Status.svelte";
  let {
    executions,
    selected,
    onselect,
  }: {
    executions: Execution[];
    selected: string;
    onselect: (id: string) => void;
  } = $props();
  let open = $state(false),
    search = $state("");
  let root: HTMLDivElement;
  let searchInput = $state<HTMLInputElement>();
  const current = $derived(
    executions.find((execution) => execution.id === selected),
  );
  const visible = $derived(
    [...executions]
      .sort(
        (a, b) =>
          Date.parse(b.created_at) - Date.parse(a.created_at) ||
          b.id.localeCompare(a.id),
      )
      .filter((execution) =>
        `${execution.workflow} ${execution.brief ?? ""} ${execution.project} ${execution.context_id} ${execution.id}`
          .toLowerCase()
          .includes(search.toLowerCase()),
      ),
  );
  $effect(() => {
    if (!open) return;
    let cancelled = false;
    // Focus after the input mounts, before a later animation frame can steal
    // focus back from a result selected with the keyboard.
    void tick().then(() => {
      if (!cancelled) searchInput?.focus();
    });
    return () => {
      cancelled = true;
    };
  });
  function close(event: PointerEvent) {
    if (open && !root.contains(event.target as Node)) open = false;
  }
  function keyboard(event: KeyboardEvent) {
    if (!open) return;
    if (event.key === "Escape") {
      open = false;
      root
        .querySelector<HTMLButtonElement>(".execution-picker-trigger")
        ?.focus();
    }
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
    const items = [
      ...root.querySelectorAll<HTMLButtonElement>(".execution-option"),
    ];
    if (!items.length) return;
    event.preventDefault();
    const index = items.indexOf(document.activeElement as HTMLButtonElement);
    const target =
      index < 0
        ? event.key === "ArrowDown"
          ? 0
          : items.length - 1
        : (index + (event.key === "ArrowDown" ? 1 : -1) + items.length) %
          items.length;
    items[target].focus();
  }
</script>

<svelte:window onpointerdown={close} onkeydown={keyboard} />
<div class="execution-picker" bind:this={root}>
  <button
    class="execution-picker-trigger"
    aria-label="Select execution"
    aria-expanded={open}
    aria-controls="execution-picker-options"
    onclick={() => (open = !open)}
  >
    <div>
      <small>EXECUTION</small><span
        >{current?.workflow || "Select execution"}{#if selected}<code
            >{short(selected)}</code
          >{/if}</span
      >
    </div>
    <ChevronDown size={15} />
  </button>
  {#if open}<div class="execution-picker-popover" id="execution-picker-options">
      <label class="search"
        ><Search size={15} /><input
          bind:this={searchInput}
          aria-label="Search executions"
          placeholder="Search name, context, ID or project…"
          bind:value={search}
        /></label
      >
      <div class="picker-context">
        <small><Clock3 size={12} />Newest first</small>
      </div>
      <div class="execution-options">
        {#each visible as execution (execution.id)}<button
            class="execution-option"
            class:chosen={selected === execution.id}
            onclick={() => {
              onselect(execution.id);
              open = false;
              search = "";
            }}
          >
            <div>
              <strong>{execution.workflow}</strong><code
                >{short(execution.id)}</code
              ><Status
                value={execution.state}
              />{#if selected === execution.id}<Check size={14} />{/if}
            </div>
            <p>{execution.brief || execution.project}</p>
            <footer>
              <span>{execution.context_id}</span><time
                datetime={execution.created_at}
                >{new Date(execution.created_at).toLocaleString()}</time
              >
            </footer>
          </button>{:else}<p class="muted picker-empty">
            No matching executions
          </p>{/each}
      </div>
    </div>{/if}
</div>
