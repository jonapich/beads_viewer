/** The part of a decision option the comment needs. */
export interface OptionLike {
  key: string;
  text: string;
}

/**
 * The comment text an answer posts: "Decision: (a) <option>" for a picked option,
 * "Decision: <text>" for free text, and the option followed by " — <text>" when
 * both are given. A display-only mirror of agentdesk.DecisionComment for the
 * preview line: the server composes the real comment from {option, text}.
 */
export function decisionComment(opt: OptionLike | null | undefined, free: string): string {
  const text = free.trim();
  if (opt && text !== "") return `Decision: (${opt.key}) ${opt.text} — ${text}`;
  if (opt) return `Decision: (${opt.key}) ${opt.text}`;
  return `Decision: ${text}`;
}
