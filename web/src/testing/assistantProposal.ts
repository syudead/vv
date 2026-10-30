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
