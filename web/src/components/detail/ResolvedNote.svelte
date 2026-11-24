<script lang="ts">
  import NeedsCard from "./NeedsCard.svelte";

  interface Props {
    kind: "approved" | "disputed" | "answered";
    /** The follow-up bead a dispute filed. */
    followUp?: string;
  }

  let { kind, followUp }: Props = $props();
</script>

<!-- The one-line note that stands where the card was, for the rest of this visit. View state, not API data. -->
<NeedsCard tone={kind === "disputed" ? "danger" : "success"} label="Done" compact bell={false}>
  {#snippet title()}
    {#if kind === "approved"}✓ Approved just now
    {:else if kind === "answered"}✓ Answered just now
    {:else}Disputed just now{#if followUp}{" · follow-up "}<a href="/issues/{encodeURIComponent(followUp)}">{followUp}</a>{/if}{/if}
  {/snippet}
</NeedsCard>
