import {
  CircleAlert,
  CircleCheck,
  CircleDashed,
  CircleOff,
  Clock,
  LoaderCircle,
  Minus,
  RotateCw,
  Undo2,
  type LucideIcon,
} from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";

import type {
  ActivityDetail,
  ActivityEntry,
  ActivityFaceLink,
  ActivityPipelineRef,
  ActivityTag,
  ActivityUndoRequest,
} from "../api/activity";
import { formatDateTime, t } from "../i18n";
import { cn } from "../lib/cn";
import { PageSection } from "../ui/patterns/page-section";
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/shadcn/tooltip";
import {
  FaceLinkFace,
  MarkIcon,
  OperationButton,
  ResultText,
  StatusIcon,
} from "./ActivityCard";
import {
  memberResult,
  memberTitle,
  type StageLine,
  stageLines,
  type StageView,
} from "./activityDetail";
import { entryOperations, faceLinkName, subjectText } from "./activityEntry";

/** One operation runs at a time on the page; every Retry and Undo waits for its refetch. */
export interface DetailOperations {
  pending: string | null;
  run: (key: string, send: () => Promise<void>) => void;
  retry: (entry: ActivityEntry) => Promise<void>;
  undo: (entry: ActivityEntry, part?: ActivityUndoRequest) => Promise<void>;
}

interface IconOperationProps {
  ops: DetailOperations;
  opKey: string;
  label: string;
  icon: LucideIcon;
  send: () => Promise<void>;
}

function IconOperation({ ops, opKey, label, icon, send }: IconOperationProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <OperationButton
          label={label}
          icon={icon}
          pending={ops.pending === opKey}
          disabled={ops.pending !== null}
          onClick={() => ops.run(opKey, send)}
        />
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

interface DetailRowProps {
  icon: ReactNode;
  title: ReactNode;
  children?: ReactNode;
  actions?: ReactNode;
}

/** A section row: the icon on the first line, then the title and its lines, actions at the end. */
function DetailRow({ icon, title, children, actions }: DetailRowProps) {
  return (
    <div className="flex items-start gap-3">
      <span className="flex h-5 shrink-0 items-center">{icon}</span>
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="min-w-0 wrap-anywhere">{title}</div>
        {children}
      </div>
      {actions !== undefined && (
        <div className="flex shrink-0 items-center gap-1">{actions}</div>
      )}
    </div>
  );
}

function Muted({ children }: { children: ReactNode }) {
  return <span className="text-xs text-muted-foreground">{children}</span>;
}

interface LinkProps {
  /** Location state for detail links: the list the back link returns to. */
  listFrom: string;
}

function EntryLink({
  id,
  listFrom,
  children,
}: LinkProps & { id: number; children: ReactNode }) {
  return (
    <Link
      to={`/activity/${String(id)}`}
      state={{ from: listFrom }}
      className="font-medium hover:underline"
    >
      {children}
    </Link>
  );
}

function taskActions(member: ActivityEntry, ops: DetailOperations): ReactNode {
  if (member.replaced === true) return undefined;
  const operations = entryOperations(member);
  if (!operations.retry && !operations.undo) return undefined;
  return (
    <>
      {operations.retry && (
        <IconOperation
          ops={ops}
          opKey={`retry-${String(member.id)}`}
          label={t.activity.action.retry}
          icon={RotateCw}
          send={() => ops.retry(member)}
        />
      )}
      {operations.undo && (
        <IconOperation
          ops={ops}
          opKey={`undo-${String(member.id)}`}
          label={t.activity.action.undo}
          icon={Undo2}
          send={() => ops.undo(member)}
        />
      )}
    </>
  );
}

interface SectionProps extends LinkProps {
  entry: ActivityDetail;
  ops: DetailOperations;
  now: Date;
}

/** A pipeline's members in join order, replaced ones included above their retry. */
export function TasksSection({ entry, ops, now, listFrom }: SectionProps) {
  const members = entry.members ?? [];
  if (entry.kind !== "pipeline" || members.length === 0) return null;
  return (
    <PageSection title={t.activity.detail.section.tasks}>
      {members.map((member) => (
        <DetailRow
          key={member.id}
          icon={<StatusIcon entry={member} small />}
          title={
            <EntryLink id={member.id} listFrom={listFrom}>
              {memberTitle(member, entry)}
            </EntryLink>
          }
          actions={taskActions(member, ops)}
        >
          <ResultText result={memberResult(member, now)} wrap />
          <Muted>{formatDateTime(member.createdAt)}</Muted>
        </DetailRow>
      ))}
    </PageSection>
  );
}

const stageIcon: Record<StageView, { icon: LucideIcon; className: string }> = {
  waiting: { icon: Clock, className: "text-muted-foreground" },
  processing: { icon: LoaderCircle, className: "animate-spin text-primary" },
  done: { icon: CircleCheck, className: "text-success" },
  failed: { icon: CircleAlert, className: "text-destructive" },
  gone: { icon: CircleOff, className: "text-muted-foreground" },
  notRun: { icon: CircleDashed, className: "text-muted-foreground" },
  absent: { icon: Minus, className: "text-muted-foreground" },
};

function StageRow({ line }: { line: StageLine }) {
  const { icon: Icon, className } = stageIcon[line.view];
  const absent = line.view === "absent";
  return (
    <DetailRow
      icon={<Icon aria-hidden="true" className={cn("size-4", className)} />}
      title={
        <span className={cn(absent && "text-muted-foreground")}>
          {t.activity.stage[line.stage]}
        </span>
      }
    >
      <span
        className={cn(
          line.view === "failed" ? "text-destructive" : "text-muted-foreground",
        )}
      >
        {t.activity.detail.stageState[line.view]}
      </span>
      {line.message !== undefined && (
        <p className="whitespace-pre-line wrap-anywhere">{line.message}</p>
      )}
      {line.updatedAt !== undefined && <Muted>{formatDateTime(line.updatedAt)}</Muted>}
    </DetailRow>
  );
}

/** An import's stages in ingest order, or the line saying they were not recorded. */
export function StagesSection({ entry }: { entry: ActivityDetail }) {
  const lines = stageLines(entry);
  if (lines === null) return null;
  return (
    <PageSection title={t.activity.detail.section.stages}>
      {lines === "notRecorded" ? (
        <p className="text-muted-foreground">{t.activity.detail.stagesNotRecorded}</p>
      ) : (
        lines.map((line) => <StageRow key={line.stage} line={line} />)
      )}
    </PageSection>
  );
}

function TagRow({
  entry,
  tag,
  undoable,
  ops,
}: {
  entry: ActivityEntry;
  tag: ActivityTag;
  undoable: boolean;
  ops: DetailOperations;
}) {
  return (
    <DetailRow
      icon={<span className="size-4" />}
      title={
        <span className={cn(tag.undone && "text-muted-foreground line-through")}>
          {tag.name}
        </span>
      }
      actions={
        undoable ? (
          <IconOperation
            ops={ops}
            opKey={`tag-${String(tag.id)}`}
            label={t.activity.detail.undoTag(tag.name)}
            icon={Undo2}
            send={() => ops.undo(entry, { tagId: tag.id })}
          />
        ) : undefined
      }
    >
      <span className="text-muted-foreground">
        {tag.suggested && <>{t.activity.result.suggested} </>}
        <span className="text-foreground tabular-nums">
          {t.activity.percent(tag.probability)}
        </span>
      </span>
    </DetailRow>
  );
}

/** An auto-tagging run's tags with their probabilities, each undone on its own. */
export function TagsSection({
  entry,
  ops,
}: {
  entry: ActivityDetail;
  ops: DetailOperations;
}) {
  const tags = entry.tags ?? [];
  if (entry.kind !== "auto_tag" || tags.length === 0) return null;
  const undoable = new Set(entryOperations(entry).undoTags.map((tag) => tag.id));
  return (
    <PageSection title={t.activity.detail.section.tags}>
      {tags.map((tag) => (
        <TagRow
          key={tag.id}
          entry={entry}
          tag={tag}
          undoable={undoable.has(tag.id)}
          ops={ops}
        />
      ))}
    </PageSection>
  );
}

function FaceLinkTitle({ link, here }: { link: ActivityFaceLink; here: string }) {
  const name = faceLinkName(link);
  const quiet = link.person?.provisional !== false;
  return (
    <span>
      {link.undone ? (
        <span className="text-muted-foreground line-through">{name}</span>
      ) : link.person === undefined ? (
        <span className="text-muted-foreground">{name}</span>
      ) : (
        <Link
          to={`/people/${String(link.person.id)}`}
          state={{ from: here }}
          className={cn("hover:underline", quiet && "text-muted-foreground")}
        >
          {name}
        </Link>
      )}
      {link.createdPerson && (
        <span className="text-muted-foreground"> {t.activity.detail.newPerson}</span>
      )}
    </span>
  );
}

/** A face analysis's links, as the tags of an auto-tagging run: each undone on its own. */
export function PeopleSection({
  entry,
  ops,
  here,
}: {
  entry: ActivityDetail;
  ops: DetailOperations;
  /** Return URL for the person pages opened from a row. */
  here: string;
}) {
  const links = entry.faceLinks ?? [];
  if (entry.kind !== "face_analysis" || links.length === 0) return null;
  const undoable = new Set(
    entryOperations(entry).undoLinks.map((link) => link.appearanceId),
  );
  return (
    <PageSection title={t.activity.detail.section.people}>
      {links.map((link) => (
        <DetailRow
          key={link.appearanceId}
          icon={<FaceLinkFace link={link} />}
          title={<FaceLinkTitle link={link} here={here} />}
          actions={
            undoable.has(link.appearanceId) ? (
              <IconOperation
                ops={ops}
                opKey={`link-${String(link.appearanceId)}`}
                label={t.activity.detail.undoLink(faceLinkName(link))}
                icon={Undo2}
                send={() => ops.undo(entry, { appearanceId: link.appearanceId })}
              />
            ) : undefined
          }
        />
      ))}
    </PageSection>
  );
}

const pipelineMark: Record<
  ActivityPipelineRef["status"],
  "running" | "attention" | "done"
> = { running: "running", attention: "attention", done: "done" };

/** The pipelines a member belongs to, newest first, each linking to its run. */
export function PartOfSection({
  entry,
  listFrom,
}: { entry: ActivityDetail } & LinkProps) {
  const pipelines = entry.pipelines ?? [];
  if (pipelines.length === 0) return null;
  return (
    <PageSection title={t.activity.detail.section.partOf}>
      {pipelines.map((pipeline) => (
        <DetailRow
          key={pipeline.id}
          icon={<MarkIcon mark={pipelineMark[pipeline.status]} small />}
          title={
            <EntryLink id={pipeline.id} listFrom={listFrom}>
              {subjectText(pipeline)}
            </EntryLink>
          }
        >
          <Muted>
            {t.activity.kind.pipeline} {t.activity.separator}{" "}
            {formatDateTime(pipeline.createdAt)}
          </Muted>
        </DetailRow>
      ))}
    </PageSection>
  );
}
