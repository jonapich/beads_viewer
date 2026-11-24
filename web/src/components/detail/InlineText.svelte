<script lang="ts">
  import { parseInline } from "../../lib/markdown";
  import Inlines from "./Inlines.svelte";

  /** One run of inline Markdown (code, **strong**, links): a sign-off line, a close reason, a decision question. */
  let { src }: { src: string } = $props();

  const nodes = $derived(parseInline(src));
</script>

<Inlines {nodes} />
