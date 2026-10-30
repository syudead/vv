import type { AssistantLabel, AssistantProgressItem } from "../api/assistant";

/** A run of plain lines, or of list lines, of the model's answer. */
export type AnswerBlock =
  { kind: "text"; text: string } | { kind: "list"; ordered: boolean; items: string[] };

const bulletLine = /^\s*- (.*)$/;
const numberedLine = /^\s*\d+\. (.*)$/;

/**
 * Splits the answer into its plain text and its `- ` and `1. ` lists, with `**` dropped. Nothing
 * else is read as Markdown, since the text is the model's.
 */
export function answerBlocks(answer: string): AnswerBlock[] {
  const blocks: AnswerBlock[] = [];
  for (const line of answer.replaceAll("**", "").split("\n")) {
    const bullet = bulletLine.exec(line)?.[1];
    const numbered = numberedLine.exec(line)?.[1];
    const item = bullet ?? numbered;
    const last = blocks.at(-1);
    if (item !== undefined) {
      const ordered = bullet === undefined;
      if (last?.kind === "list" && last.ordered === ordered) last.items.push(item);
      else blocks.push({ kind: "list", ordered, items: [item] });
    } else if (last?.kind === "text") {
      last.text = `${last.text}\n${line}`;
    } else {
      blocks.push({ kind: "text", text: line });
    }
  }
  return blocks.flatMap((block): AnswerBlock[] => {
    if (block.kind === "list") return [block];
    const text = block.text.replace(/^\n+|\n+$/g, "");
    return text.trim() === "" ? [] : [{ kind: "text", text }];
  });
}

type Script = "other" | "word" | "hiragana" | "katakana" | "han";

function scriptOf(char: string): Script {
  if (char === "ー" || /\p{Script=Katakana}/u.test(char)) return "katakana";
  if (/\p{Script=Hiragana}/u.test(char)) return "hiragana";
  if (/\p{Script=Han}/u.test(char) || char === "〆") return "han";
  if (/[\p{L}\p{N}]/u.test(char)) return "word";
  return "other";
}

function fold(text: string): string[] {
  return Array.from(text.normalize("NFKC").toLowerCase());
}

function sameScript(a: string | undefined, b: string | undefined): boolean {
  if (a === undefined || b === undefined) return false;
  const script = scriptOf(a);
  return script !== "other" && script === scriptOf(b);
}

/**
 * Where the name first occurs in the answer as its own token, or -1: no letter of the same script
 * touches it, so 「ライブ」 is not found in 「ライブラリ」, as the server's grounding check reads it.
 */
export function mentionAt(answer: readonly string[], name: string): number {
  const target = fold(name);
  if (target.length === 0) return -1;
  for (let start = 0; start + target.length <= answer.length; start += 1) {
    const end = start + target.length;
    if (!target.every((char, offset) => answer[start + offset] === char)) continue;
    if (sameScript(answer[start - 1], answer[start])) continue;
    if (sameScript(answer[end], answer[end - 1])) continue;
    return start;
  }
  return -1;
}

/** At most this many entities follow an answer. */
export const maxMentioned = 20;

export type MentionedLabel = AssistantLabel & { id: number };

/** The labels with an id from the turn's tool calls that the answer names, in the answer's order. */
export function mentionedLabels(
  answer: string,
  progress: readonly AssistantProgressItem[],
): MentionedLabel[] {
  const folded = fold(answer);
  const found = new Map<string, { label: MentionedLabel; at: number }>();
  for (const item of progress) {
    for (const label of item.labels) {
      if (label.id === undefined) continue;
      const key = `${label.kind}:${String(label.id)}`;
      if (found.has(key)) continue;
      const at = mentionAt(folded, label.name);
      if (at >= 0) found.set(key, { label: { ...label, id: label.id }, at });
    }
  }
  return [...found.values()]
    .sort((a, b) => a.at - b.at)
    .slice(0, maxMentioned)
    .map((entry) => entry.label);
}
