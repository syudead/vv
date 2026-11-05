import type { AssistantProposal, AssistantProposalStep } from "../api/assistant";
import type { CardProposal } from "../assistant/proposal";

const stepStatus: Record<AssistantProposal["status"], AssistantProposalStep["status"]> = {
  proposed: "pending",
  approved: "pending",
  discarded: "pending",
  running: "running",
  done: "done",
  partial: "partial",
  stale: "stale",
  failed: "failed",
};

/**
 * "Create tag ライブ録画, then attach it to three videos": the second step uses the tag the first
 * creates. Each step's status follows the proposal's unless it is given.
 */
export function createAndAttachProposal(
  status: AssistantProposal["status"] = "proposed",
  steps: Partial<AssistantProposalStep>[] = [],
): AssistantProposal {
  const counts = { total: 0, applied: 0, failed: 0 };
  return {
    id: 1,
    status,
    request: "ライブ録画タグを作って付けて",
    steps: [
      {
        index: 0,
        operation: "createTag",
        irreversible: false,
        targets: { kind: "", count: 0, labels: [] },
        change: [{ argument: "name", value: "ライブ録画" }],
        created: ["ライブ録画"],
        createdBy: [],
        status: stepStatus[status],
        counts: { ...counts, total: 1 },
        ...steps[0],
      },
      {
        index: 1,
        operation: "updateVideoTags.add",
        irreversible: false,
        targets: {
          kind: "videos",
          count: 3,
          labels: [
            { kind: "video", id: 1, name: "a" },
            { kind: "video", id: 2, name: "b" },
            { kind: "video", id: 3, name: "c" },
          ],
        },
        change: [{ argument: "tagNames", labels: [{ kind: "tag", name: "ライブ録画" }] }],
        created: [],
        createdBy: [{ name: "ライブ録画", step: 0 }],
        status: stepStatus[status],
        counts: { ...counts, total: 3 },
        ...steps[1],
      },
    ],
  };
}

/** A proposal of one step, written as the one-operation card shows it. */
export function oneStepProposal(view: CardProposal): AssistantProposal {
  const {
    id,
    status,
    request,
    turnId,
    activityId,
    startedAt,
    finishedAt,
    error,
    ...step
  } = view;
  return {
    id,
    status,
    request,
    ...(turnId === undefined ? {} : { turnId }),
    ...(activityId === undefined ? {} : { activityId }),
    ...(startedAt === undefined ? {} : { startedAt }),
    ...(finishedAt === undefined ? {} : { finishedAt }),
    steps: [
      {
        ...step,
        index: 0,
        status: stepStatus[status],
        createdBy: [],
        ...(error === undefined ? {} : { error }),
      },
    ],
  };
}
