<script lang="ts">
  import type { Snippet } from "svelte";

  interface Props {
    label: string;
    /** Stable key of the field (data-field). The inline editors (beads-c2o.14) swap one field's value snippet for an editor. */
    name: string;
    children: Snippet;
  }

  let { label, name, children }: Props = $props();
</script>

<div class="field" data-field={name}>
  <div class="fl">{label}</div>
  <div class="fv">{@render children()}</div>
</div>

<style>
  .field {
    min-width: 0;
  }
  .fl {
    font-size: var(--text-sm);
    font-weight: 500;
    color: var(--text-muted);
    margin-bottom: 2px;
  }
  .fv {
    min-height: 22px;
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px;
    overflow-wrap: anywhere;
  }
</style>
