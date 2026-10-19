import { Link } from "react-router";

import type { ActivityListItem, ActivityMemberSummary } from "../api/activity";
import { t } from "../i18n";
import { CardListItem, CardListMember } from "../ui/patterns/card-list";
import { Badge } from "../ui/shadcn/badge";
import { Actions, ResultWithProgress, StatusIcon, Subject } from "./ActivityCard";
import { memberResult, memberTitle } from "./activityDetail";
import { cardTone, entryTime, type OpenAction, subjectText } from "./activityEntry";
import {
  currentMembers,
  pipelineProgress,
  pipelineSummary,
  startsExpanded,
} from "./activityPipeline";
import type { OperationTarget } from "./useActivityOperations";

interface MemberRowProps {
  member: ActivityMemberSummary;
  pipeline: ActivityListItem;
  now: Date;
  from: string;
  onRetry: (entry: OperationTarget) => Promise<void>;
  onUndo: (entry: OperationTarget, tagId?: number) => Promise<void>;
}

/** A member's kind and result; its tags and full message are on its detail page. */
function MemberRow({ member, pipeline, now, from, onRetry, onUndo }: MemberRowProps) {
  return (
    <CardListMember
      icon={<StatusIcon entry={member} small />}
      title={
        <Link
          to={`/activity/${String(member.id)}`}
          state={{ from }}
          className="hover:underline"
        >
          {memberTitle(member, pipeline)}
        </Link>
      }
      detail={
        <>
          {member.matched === true && (
            <Badge variant="soft">{t.activity.pipeline.matches}</Badge>
          )}
          <ResultWithProgress
            result={memberResult(member, now)}
            progress={member.status === "running" ? member.progress : undefined}
          />
        </>
      }
      actions={
        <Actions
          entry={member}
          actions={[]}
          from={from}
          onRetry={onRetry}
          onUndo={onUndo}
        />
      }
    />
  );
}

export interface ActivityPipelineCardProps {
  item: ActivityListItem;
  now: Date;
  /** Return URL for the details and the video opened from this card. */
  from: string;
  /** The video's `Open`; a pipeline is never retried or undone itself. */
  actions: OpenAction[];
  /** The owner's last choice for this card; without one the card follows its state. */
  chosen: boolean | undefined;
  onExpandedChange: (expanded: boolean) => void;
  onRetry: (entry: OperationTarget) => Promise<void>;
  onUndo: (entry: OperationTarget, tagId?: number) => Promise<void>;
}

/** One import run: the run's card, with its tasks as rows inside it. */
export function ActivityPipelineCard({
  item,
  now,
  from,
  actions,
  chosen,
  onExpandedChange,
  onRetry,
  onUndo,
}: ActivityPipelineCardProps) {
  const expanded = chosen ?? startsExpanded(item);
  const time = entryTime(item.createdAt, now);
  const subject = subjectText(item);
  return (
    <CardListItem
      tone={cardTone(item)}
      icon={<StatusIcon entry={item} />}
      title={<Subject entry={item} from={from} />}
      badge={<Badge variant="secondary">{t.activity.kind.pipeline}</Badge>}
      detail={
        <ResultWithProgress
          result={pipelineSummary(item, now)}
          progress={pipelineProgress(item)}
        />
      }
      time={<span title={time.full}>{time.long}</span>}
      timeShort={<span title={time.full}>{time.short}</span>}
      actions={
        <Actions
          entry={item}
          actions={actions}
          from={from}
          onRetry={onRetry}
          onUndo={onUndo}
        />
      }
      expanded={expanded}
      onExpandedChange={onExpandedChange}
      toggleLabel={
        expanded
          ? {
              tooltip: t.activity.pipeline.hideTasks,
              name: t.activity.pipeline.hideTasksFor(subject),
            }
          : {
              tooltip: t.activity.pipeline.showTasks,
              name: t.activity.pipeline.showTasksFor(subject),
            }
      }
      members={currentMembers(item.members).map((member) => (
        <MemberRow
          key={member.id}
          member={member}
          pipeline={item}
          now={now}
          from={from}
          onRetry={onRetry}
          onUndo={onUndo}
        />
      ))}
    />
  );
}
