<script lang="ts">
  import type { DisplayStatus } from "../../lib/issues/model";
  import { GLYPH, STATUS_NAME } from "../../lib/issues/present";

  let { status, dim = false }: { status: DisplayStatus; dim?: boolean } = $props();
</script>

<span class="sg" class:dim data-status={status} title={STATUS_NAME[status]}>{GLYPH[status]}</span>

<style>
  .sg {
    order: 3;
    width: 16px;
    flex: none;
    text-align: center;
    font-size: 13px;
    line-height: 1;
  }
  .sg.dim {
    opacity: 0.7;
  }
  .sg[data-status="open"] {
    color: var(--text-muted);
  }
  .sg[data-status="in_progress"] {
    color: var(--accent);
  }
  .sg[data-status="blocked"] {
    color: var(--danger);
  }
  .sg[data-status="closed"] {
    color: var(--success);
  }
  .sg[data-status="deferred"] {
    color: var(--text-faint);
  }
</style>
