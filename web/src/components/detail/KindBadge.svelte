<script lang="ts">
  /** A comment kind (brief, advice, coder, ...) as a coloured badge, by [data-kind]: no style attributes under the CSP. */
  let { kind }: { kind: string } = $props();
</script>

<span class="kind" data-kind={kind}>{kind}</span>

<style>
  .kind {
    --c: var(--text-muted);
    display: inline-block;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    font-weight: 600;
    line-height: 16px;
    padding: 0 6px;
    border-radius: var(--radius-sm);
    color: var(--c);
    background: color-mix(in srgb, var(--c) 14%, transparent);
    border: 1px solid color-mix(in srgb, var(--c) 30%, transparent);
  }
  .kind[data-kind="user"],
  .kind[data-kind="answer"] {
    --c: var(--kind-user);
  }
  .kind[data-kind="brief"] {
    --c: var(--kind-brief);
  }
  .kind[data-kind="advice"] {
    --c: var(--kind-advice);
  }
  .kind[data-kind="coder"] {
    --c: var(--kind-coder);
  }
  .kind[data-kind="review"] {
    --c: var(--kind-review);
  }
  .kind[data-kind="verify"] {
    --c: var(--kind-verify);
  }
  .kind[data-kind="signoff"] {
    --c: var(--kind-signoff);
  }
</style>
