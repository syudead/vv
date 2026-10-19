import { ArrowLeft, CircleAlert, FileQuestion, RotateCw, Undo2 } from "lucide-react";
import { type ReactNode, useCallback, useEffect, useState } from "react";
import { Link, useLocation, useParams } from "react-router";

import type { ActivityDetail } from "../api/activity";
import { formatDateTime, t } from "../i18n";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { type Fact, FactList } from "../ui/patterns/fact-list";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageSection } from "../ui/patterns/page-section";
import { RecordPage } from "../ui/patterns/record-page";
import { Alert, AlertAction, AlertDescription, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { Spinner } from "../ui/shadcn/spinner";
import { ResultText, StatusIcon } from "./ActivityCard";
import {
  deciderText,
  type DetailNotice,
  detailNotice,
  headerVideo,
  listTarget,
} from "./activityDetail";
import {
  type DetailOperations,
  PartOfSection,
  PeopleSection,
  StagesSection,
  TagsSection,
  TasksSection,
} from "./ActivityDetailSections";
import {
  entryOperations,
  entryResult,
  statusMark,
  subjectParts,
  subjectPerson,
  subjectText,
} from "./activityEntry";
import { useActivityDetail } from "./useActivityDetail";
import { useActivityOperations } from "./useActivityOperations";

function parseEntryId(raw: string | undefined): number | null {
  const id = Number(raw);
  return Number.isSafeInteger(id) && id > 0 ? id : null;
}

function useTitle(title: string) {
  useEffect(() => {
    const previous = document.title;
    document.title = title;
    return () => {
      document.title = previous;
    };
  }, [title]);
}

function OperationLabel({
  pending,
  icon,
  label,
}: {
  pending: boolean;
  icon: ReactNode;
  label: string;
}) {
  return (
    <>
      {pending ? <Spinner aria-hidden="true" /> : icon}
      {label}
    </>
  );
}

function Notice({
  notice,
  entry,
  ops,
}: {
  notice: DetailNotice;
  entry: ActivityDetail;
  ops: DetailOperations;
}) {
  const key = `retry-${String(entry.id)}`;
  return (
    <Alert variant={notice.tone}>
      <CircleAlert aria-hidden="true" />
      <AlertTitle>{notice.title.parts.map((part) => part.text).join(" ")}</AlertTitle>
      {notice.message !== undefined && (
        <AlertDescription>
          <p className="whitespace-pre-line wrap-anywhere">{notice.message}</p>
        </AlertDescription>
      )}
      {notice.retry && (
        <AlertAction>
          <Button
            variant="outline"
            size="sm"
            aria-busy={ops.pending === key || undefined}
            disabled={ops.pending !== null}
            onClick={() => ops.run(key, () => ops.retry(entry))}
          >
            <OperationLabel
              pending={ops.pending === key}
              icon={<RotateCw aria-hidden="true" />}
              label={t.activity.action.retry}
            />
          </Button>
        </AlertAction>
      )}
    </Alert>
  );
}

function Videos({ entry, here }: { entry: ActivityDetail; here: string }) {
  return (
    <span className="flex flex-col gap-1">
      {subjectParts(entry).map((part, index) =>
        part.videoId === undefined ? (
          <span key={index}>
            {part.label}{" "}
            <span className="text-muted-foreground">
              {t.activity.separator} {t.activity.result.notInLibrary}
            </span>
          </span>
        ) : (
          <Link
            key={index}
            to={`/videos/${String(part.videoId)}`}
            state={{ from: here }}
            className="hover:underline"
          >
            {part.label}
          </Link>
        ),
      )}
    </span>
  );
}

function PersonFact({ entry, here }: { entry: ActivityDetail; here: string }) {
  const [part] = subjectParts(entry);
  const label = part?.label ?? "";
  return part?.personId === undefined ? (
    <span>{label}</span>
  ) : (
    <Link
      to={`/people/${String(part.personId)}`}
      state={{ from: here }}
      className="hover:underline"
    >
      {label}
    </Link>
  );
}

/** The header's Open: the video of a video subject, or the person of a person subject. */
function headerOpen(entry: ActivityDetail, here: string): ReactNode {
  const video = headerVideo(entry);
  const person = subjectPerson(entry);
  const target =
    video !== undefined
      ? { to: `/videos/${String(video)}`, label: t.activity.action.open }
      : person !== undefined
        ? { to: `/people/${String(person)}`, label: t.activity.action.openPerson }
        : undefined;
  if (target === undefined) return undefined;
  return (
    <Button asChild variant="outline" size="sm">
      <Link to={target.to} state={{ from: here }}>
        {target.label}
      </Link>
    </Button>
  );
}

function ResultFact({ entry, ops }: { entry: ActivityDetail; ops: DetailOperations }) {
  const key = `undo-${String(entry.id)}`;
  return (
    <span className="flex flex-wrap items-center gap-2">
      <ResultText result={entryResult(entry, new Date())} wrap />
      {entryOperations(entry).undo && (
        <Button
          variant="outline"
          size="sm"
          aria-busy={ops.pending === key || undefined}
          disabled={ops.pending !== null}
          onClick={() => ops.run(key, () => ops.undo(entry))}
        >
          <OperationLabel
            pending={ops.pending === key}
            icon={<Undo2 aria-hidden="true" />}
            label={t.activity.action.undo}
          />
        </Button>
      )}
    </span>
  );
}

function summaryFacts(
  entry: ActivityDetail,
  ops: DetailOperations,
  here: string,
): Fact[] {
  const f = t.activity.detail.fact;
  const kind = entry.subject.kind;
  const facts: Fact[] = [
    {
      id: "status",
      term: f.status,
      value: (
        <span className="flex items-center gap-2">
          <StatusIcon entry={entry} small />
          {t.activity.statusIcon[statusMark(entry)]}
        </span>
      ),
    },
    { id: "process", term: f.process, value: t.activity.kind[entry.kind] },
  ];
  if (kind === "video" || kind === "pair")
    facts.push({
      id: "videos",
      term: kind === "pair" ? f.videos : f.video,
      value: <Videos entry={entry} here={here} />,
    });
  if (kind === "person")
    facts.push({
      id: "person",
      term: f.person,
      value: <PersonFact entry={entry} here={here} />,
    });
  facts.push(
    { id: "decider", term: f.decider, value: deciderText(entry) },
    { id: "started", term: f.started, value: formatDateTime(entry.createdAt) },
    { id: "updated", term: f.updated, value: formatDateTime(entry.updatedAt) },
  );
  if (entry.status === "done")
    facts.push({
      id: "result",
      term: f.result,
      value: <ResultFact entry={entry} ops={ops} />,
    });
  return facts;
}

function useDetailOperations(refresh: () => Promise<void>): DetailOperations {
  const operations = useActivityOperations(refresh);
  const [pending, setPending] = useState<string | null>(null);
  const run = useCallback((key: string, send: () => Promise<void>) => {
    setPending(key);
    void send().finally(() => setPending(null));
  }, []);
  return { pending, run, retry: operations.retry, undo: operations.undo };
}

function Loaded({
  entry,
  ops,
  back,
  listFrom,
  here,
}: {
  entry: ActivityDetail;
  ops: DetailOperations;
  back: ReactNode;
  listFrom: string;
  here: string;
}) {
  const now = new Date();
  const notice = detailNotice(entry, now);
  return (
    <RecordPage
      back={back}
      title={subjectText(entry)}
      description={t.activity.kind[entry.kind]}
      actions={headerOpen(entry, here)}
      notice={
        notice === null ? undefined : <Notice notice={notice} entry={entry} ops={ops} />
      }
    >
      <PageSection title={t.activity.detail.section.summary}>
        <FactList facts={summaryFacts(entry, ops, here)} />
      </PageSection>
      <TasksSection entry={entry} ops={ops} now={now} listFrom={listFrom} />
      <StagesSection entry={entry} />
      <TagsSection entry={entry} ops={ops} />
      <PeopleSection entry={entry} ops={ops} here={here} />
      <PartOfSection entry={entry} listFrom={listFrom} />
    </RecordPage>
  );
}

function EntryDetail({ rawId }: { rawId: string | undefined }) {
  const location = useLocation();
  const listFrom = listTarget(location.state);
  const here = `${location.pathname}${location.search}`;
  const { state, retry, refresh } = useActivityDetail(parseEntryId(rawId));
  const ops = useDetailOperations(refresh);
  useTitle(
    state.kind === "ready"
      ? t.activity.detail.documentTitle(subjectText(state.entry))
      : t.activity.documentTitle,
  );
  const back = (
    <Button asChild variant="ghost" size="sm">
      <Link to={listFrom}>
        <ArrowLeft aria-hidden="true" />
        {t.activity.title}
      </Link>
    </Button>
  );
  if (state.kind === "ready")
    return (
      <Loaded entry={state.entry} ops={ops} back={back} listFrom={listFrom} here={here} />
    );
  return (
    <RecordPage back={back} title={t.activity.title} titleHidden>
      {state.kind === "loading" && (
        <LoadingState label={t.activity.detail.loading} layout="grouped" />
      )}
      {state.kind === "notFound" && (
        <EmptyState
          icon={<FileQuestion aria-hidden="true" />}
          title={t.activity.detail.notFound}
          action={
            <Button asChild>
              <Link to="/activity">{t.activity.detail.backToList}</Link>
            </Button>
          }
        />
      )}
      {state.kind === "failed" && (
        <ErrorState
          title={t.activity.detail.loadFailed}
          retryLabel={t.common.retry}
          onRetry={retry}
        />
      )}
    </RecordPage>
  );
}

/** One activity entry of any kind; a new id starts a fresh page. */
export default function ActivityDetailPage() {
  const { id } = useParams();
  return <EntryDetail key={id} rawId={id} />;
}
