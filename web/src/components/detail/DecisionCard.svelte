<script lang="ts">
  import type { DecisionInfo } from "../../lib/api/types";
  import { answerKey, optionKey, type IssueActions } from "../../lib/issues/actions.svelte";
  import DecisionForm from "../issues/DecisionForm.svelte";
  import InlineText from "./InlineText.svelte";
  import NeedsCard from "./NeedsCard.svelte";

  interface Props {
    id: string;
    /** The issue title: the question when the description has none. */
    issueTitle: string;
    decision: DecisionInfo;
    actions: IssueActions;
    /** The tracker (or this issue) takes no writes. */
    readOnly: boolean;
    /** Hands a started write to the page, which keeps focus and shows the "done" note once it landed. */
    onwrite: (write: Promise<boolean>, kind: "answered") => void;
  }

  let { id, issueTitle, decision, actions, readOnly, onwrite }: Props = $props();

  const busy = $derived(actions.isBusy(id));
</script>

<NeedsCard tone="accent" label="Decision" sub={decision.waiting_on ? `Waiting on: ${decision.waiting_on}` : undefined}>
  {#snippet title()}{decision.label === "needs-design" ? "Needs your design review" : "Needs your decision"}{/snippet}
  <p class="q"><InlineText src={decision.question ?? issueTitle} /></p>
  <!-- The shared decision form (beads-c2o.19): the same component and wiring as the peek drawer. -->
  <DecisionForm
    issueId={id}
    {decision}
    {busy}
    disabled={readOnly}
    option={actions.getDraft(optionKey(id))}
    text={actions.getDraft(answerKey(id))}
    onoption={(k) => actions.setDraft(optionKey(id), k)}
    ontext={(v) => actions.setDraft(answerKey(id), v)}
    onanswer={(a) => onwrite(actions.answer(id, a), "answered")}
  />
</NeedsCard>

<style>
  .q {
    margin: var(--space-2) 0 0;
    font-size: var(--text-base);
    font-weight: 600;
  }
</style>
