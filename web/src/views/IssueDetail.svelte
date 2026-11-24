<script lang="ts">
  import { onMount, untrack } from "svelte";
  import DetailHeader from "../components/detail/DetailHeader.svelte";
  import DetailSkeleton from "../components/detail/DetailSkeleton.svelte";
  import FieldRail from "../components/detail/FieldRail.svelte";
  import InlineText from "../components/detail/InlineText.svelte";
  import Markdown from "../components/detail/Markdown.svelte";
  import Section from "../components/detail/Section.svelte";
  import { designOpenByDefault, readOnlyNotice } from "../lib/issues/detail";
  import { DetailFeed } from "../lib/issues/detail-feed.svelte";
  import { route } from "../lib/router/router.svelte";

  const feed = new DetailFeed();

  // /issues/a to /issues/b keeps this component, so everything follows the id.
  const id = $derived(route.params["id"] ?? "");
  $effect(() => {
    const current = id;
    untrack(() => feed.setId(current));
  });
  onMount(() => feed.start());

  /** The feed can still hold the previous issue for a frame after the id changed. */
  const detail = $derived(feed.detail && feed.detail.id === id ? feed.detail : null);
  // The relative ages are computed once per refetch, not per render.
  const now = $derived.by(() => {
    void feed.detail;
    return Date.now();
  });
  const notice = $derived(readOnlyNotice(feed.meta, detail));

  // Which sections the user opened or closed, keyed by issue id and section. A refetch (every live
  // update) re-renders in place, so nothing is reset; another issue starts from the defaults.
  let toggled = $state<Record<string, boolean>>({});
  const isOpen = (name: string, fallback: boolean): boolean => toggled[`${id}:${name}`] ?? fallback;
  const setOpen = (name: string, open: boolean): void => {
    toggled[`${id}:${name}`] = open;
  };
</script>

<div class="page">
  {#if notice}
    <div class="banner" role="note">
      <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" aria-hidden="true"><circle cx="8" cy="8" r="6.2" /><path d="M8 7.2V11M8 5v.1" /></svg>
      <div>{notice}</div>
    </div>
  {/if}

  {#if detail}
    <!-- DOM order: header, rail, sections. The rail is rendered once; CSS moves it beside the sections on wide screens. -->
    <div class="grid">
      <div class="head"><DetailHeader {detail} {now} /></div>
      <div class="railcell"><FieldRail {detail} /></div>
      <div class="main">
        {#if detail.status === "closed" && detail.close_reason}
          <Section name="resolution" title="Resolution" bind:open={() => isOpen("resolution", true), (v) => setOpen("resolution", v)}>
            <p class="resolution"><InlineText src={detail.close_reason} /></p>
          </Section>
        {/if}

        <Section name="description" title="Description" bind:open={() => isOpen("description", true), (v) => setOpen("description", v)}>
          {#if detail.description.trim()}<Markdown src={detail.description} />{:else}<p class="empty">No description.</p>{/if}
        </Section>

        {#if detail.acceptance_criteria?.trim()}
          <Section name="acceptance" title="Acceptance" bind:open={() => isOpen("acceptance", true), (v) => setOpen("acceptance", v)}>
            <Markdown src={detail.acceptance_criteria} />
          </Section>
        {/if}

        {#if detail.design?.trim()}
          <Section
            name="design"
            title="Design"
            bind:open={() => isOpen("design", designOpenByDefault(detail.design)), (v) => setOpen("design", v)}
          >
            <Markdown src={detail.design} />
          </Section>
        {/if}

        {#if detail.notes?.trim()}
          <Section name="notes" title="Notes" bind:open={() => isOpen("notes", true), (v) => setOpen("notes", v)}>
            <Markdown src={detail.notes} />
          </Section>
        {/if}
      </div>
    </div>
  {:else if feed.status === "missing"}
    <div class="nf">
      <h2>No issue {id}</h2>
      <p class="muted">It may have been deleted, or the ID is wrong.</p>
      <p class="back"><a href="/issues">Back to issues</a></p>
    </div>
  {:else if feed.status === "error"}
    <div class="nf">
      <h2>Couldn't load {id}</h2>
      <p class="muted">{feed.error?.message ?? "Something went wrong."}</p>
      <p class="back"><button type="button" class="retry" onclick={() => feed.reload()}>Retry</button></p>
    </div>
  {:else}
    <DetailSkeleton />
  {/if}
</div>

<style>
  .banner {
    display: flex;
    gap: var(--space-2);
    align-items: flex-start;
    background: var(--bg-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: var(--space-2) var(--space-3);
    margin-bottom: var(--space-4);
    font-size: 13px;
  }
  .banner svg {
    flex: none;
    margin-top: 1px;
  }
  .grid {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
  }
  .head,
  .main {
    min-width: 0;
  }
  .main {
    margin-top: var(--space-4);
  }
  /* Wide: sections on the left, the field rail sticky on the right, the header across both. */
  @media (min-width: 1100px) {
    .grid {
      grid-template-columns: minmax(0, 760px) 280px;
      column-gap: var(--space-6);
    }
    .head {
      grid-column: 1 / -1;
      grid-row: 1;
    }
    .main {
      grid-column: 1;
      grid-row: 2;
    }
    .railcell {
      grid-column: 2;
      grid-row: 2;
      margin-top: var(--space-4);
      padding-top: var(--space-4);
      border-top: 1px solid var(--border);
    }
  }
  .resolution {
    margin: 0;
    font-size: var(--text-base);
    line-height: 1.55;
    max-width: 72ch;
    overflow-wrap: anywhere;
  }
  .empty {
    margin: 0;
    color: var(--text-faint);
    font-style: italic;
  }
  .muted {
    color: var(--text-muted);
  }
  .nf {
    max-width: 420px;
    margin: 48px auto;
    text-align: center;
  }
  .nf h2 {
    font-size: var(--text-lg);
    margin-bottom: var(--space-2);
    overflow-wrap: anywhere;
  }
  .nf p {
    margin: 0;
  }
  .nf .back {
    margin-top: var(--space-3);
  }
  .retry {
    height: 28px;
    padding: 0 var(--space-3);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--bg-raised);
    font-size: var(--text-sm);
    cursor: pointer;
  }
  .retry:hover {
    background: var(--bg-hover);
  }
</style>
