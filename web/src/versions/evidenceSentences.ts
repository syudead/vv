import type { VersionJudgeEvidence } from "../api/client";
import { t, type UiText } from "../i18n";
import { formatDuration } from "../lib/format";

/** The frame rule's reasoning, then what the AI was asked and answered, as sentences an owner can follow. */
export function evidenceSentences(
  evidence: VersionJudgeEvidence,
  probability: number | undefined,
): UiText[] {
  const sentences = ruleSentences(evidence, probability);
  if (evidence.conflict === true) sentences.push(t.versions.judgement.conflict);
  return sentences;
}

function ruleSentences(
  evidence: VersionJudgeEvidence,
  probability: number | undefined,
): UiText[] {
  const j = t.versions.judgement;
  const { compared, differing } = evidence;
  switch (evidence.rule) {
    case "frames_match":
      return [j.matched(compared, differing)];
    case "incomparable":
      return [evidence.versionMismatch === true ? j.versionMismatch : j.tooFew(compared)];
    case "frames_differ":
      break;
  }
  const sentences: UiText[] = [
    evidence.longestRun === undefined
      ? j.scattered(differing, compared)
      : j.run(
          differing,
          compared,
          evidence.longestRun,
          formatDuration(evidence.runStartMs),
          formatDuration(evidence.runEndMs),
        ),
  ];
  const shown = evidence.frames?.length ?? 0;
  if (shown > 0) sentences.push(j.asked(shown));
  sentences.push(
    probability === undefined ? j.noAnswer : j.answer(t.activity.percent(probability)),
  );
  return sentences;
}
