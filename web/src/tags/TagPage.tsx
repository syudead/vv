import {
  ArrowLeft,
  Ban,
  Check,
  CircleAlert,
  Pencil,
  Tag as TagIcon,
  Trash2,
} from "lucide-react";
import { type ReactNode, useState } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router";

import { RequestFailed } from "../api/client";
import { refreshTagKinds } from "../api/tagKinds";
import {
  batchTags,
  confirmTag,
  deleteTag,
  getTag,
  rejectTag,
  renameTag,
  type Tag,
} from "../api/tags";
import { errorText, formatNumber, t, type UiText } from "../i18n";
import { isRemoteId } from "../lib/remoteId";
import { KindToggleGroup } from "../ui/KindToggleGroup";
import { EmptyState } from "../ui/patterns/empty-state";
import { ErrorState } from "../ui/patterns/error-state";
import { LoadingState } from "../ui/patterns/loading-state";
import { PageSection } from "../ui/patterns/page-section";
import { RecordEditorFigure, RecordEditorPage } from "../ui/patterns/record-editor-page";
import { Alert, AlertTitle } from "../ui/shadcn/alert";
import { Button } from "../ui/shadcn/button";
import { Separator } from "../ui/shadcn/separator";
import { Skeleton } from "../ui/shadcn/skeleton";
import { Spinner } from "../ui/shadcn/spinner";
import type { TagFieldError } from "../ui/nameField";
import RowNameInput from "../ui/RowNameInput";
import TentativeMark from "../ui/TentativeMark";
import { useToast } from "../ui/Toast";
import DeleteTagDialog from "./DeleteTagDialog";
import { MoveSynonymDialog } from "./MoveSynonymDialog";
import RejectTagDialog from "./RejectTagDialog";
import { tagPagePath } from "./TagCard";
import { TagMergeSection } from "./TagMergeSection";
import { TagNamesSection } from "./TagNamesSection";
import { tagFieldError } from "./tagNameField";
import { TagVideosShelf } from "./TagVideosShelf";
import { useTagKinds } from "./useTagKinds";
import { type TagPageNotice, type TagRecord, useTagRecord } from "./useTagRecord";

const text = t.tags.page;

/** The tag list the page was opened from, with its criteria; the plain list otherwise. */
export function tagListTarget(state: unknown): string {
  const from: unknown = (state as { from?: unknown } | null)?.from;
  if (typeof from !== "string" || !/^\/tags(?:[?#]|$)/.test(from)) return "/tags";
  return from;
}

function BackLink({ to }: { to: string }) {
  return (
    <Button asChild variant="ghost" size="sm">
      <Link to={to}>
        <ArrowLeft aria-hidden="true" />
        {text.back}
      </Link>
    </Button>
  );
}

function NoticeAlert({ notice }: { notice: TagPageNotice }) {
  if (notice.kind === "detached") {
    return (
      <Alert>
        <Check aria-hidden="true" />
        <AlertTitle className="flex flex-wrap items-center gap-x-2">
          {text.detached(notice.name)}
          <Link
            to={tagPagePath(notice.id)}
            className="text-primary underline-offset-4 hover:underline"
          >
            {text.openDetached(notice.name)}
          </Link>
        </AlertTitle>
      </Alert>
    );
  }
  return (
    <Alert variant={notice.kind === "error" ? "destructive" : "warning"}>
      <CircleAlert aria-hidden="true" />
      <AlertTitle>{notice.message}</AlertTitle>
    </Alert>
  );
}

function ActionButton({
  record,
  busyKey,
  icon,
  label,
  variant,
  onClick,
}: {
  record: TagRecord;
  busyKey: string;
  icon: ReactNode;
  label: UiText;
  variant: "default" | "secondary" | "ghost-destructive";
  onClick: () => void;
}) {
  return (
    <Button
      variant={variant}
      size="sm"
      shape="pill"
      aria-disabled={record.busy !== null || undefined}
      onClick={() => {
        if (record.busy === null) onClick();
      }}
    >
      {record.busy === busyKey ? <Spinner aria-hidden="true" /> : icon}
      {label}
    </Button>
  );
}

type Dialog = "reject" | "delete" | null;

/** `/tags/:id`: one tag's names, kind and merge, each acting at once. */
export default function TagPage() {
  const params = useParams();
  const id = Number(params.id);
  const record = useTagRecord(Number.isSafeInteger(id) && id > 0 ? id : 0);
  const { tag, gone, loadError } = record;
  const location = useLocation();
  const listFrom = tagListTarget(location.state);
  const back = <BackLink to={listFrom} />;

  if (gone) {
    return (
      <RecordEditorPage
        back={back}
        title={text.gone}
        main={null}
        status={
          <EmptyState
            icon={<TagIcon aria-hidden="true" />}
            title={text.gone}
            action={
              <Button asChild size="sm">
                <Link to={listFrom}>{text.backToTags}</Link>
              </Button>
            }
          />
        }
      />
    );
  }
  if (tag === undefined) {
    return loadError !== null ? (
      <RecordEditorPage
        back={back}
        title={text.loadFailed}
        main={null}
        status={
          <ErrorState
            title={text.loadFailed}
            retryLabel={t.common.retry}
            onRetry={record.load}
          />
        }
      />
    ) : (
      <RecordEditorPage
        back={back}
        title={text.loading}
        titleField={<Skeleton className="h-6 w-1/3" />}
        main={<LoadingState label={text.loading} layout="table" count={4} />}
        side={<LoadingState label={text.loading} layout="table" count={2} />}
      />
    );
  }
  return <LoadedTagPage tag={tag} record={record} back={back} listFrom={listFrom} />;
}

function LoadedTagPage({
  tag,
  record,
  back,
  listFrom,
}: {
  tag: Tag;
  record: TagRecord;
  back: ReactNode;
  listFrom: string;
}) {
  const toast = useToast();
  const navigate = useNavigate();
  const [renaming, setRenaming] = useState(false);
  const [renameError, setRenameError] = useState<TagFieldError | null>(null);
  const [moving, setMoving] = useState<string | null>(null);
  const [dialog, setDialog] = useState<Dialog>(null);
  const [dialogError, setDialogError] = useState<UiText | null>(null);
  const { busy, run, setTag } = record;

  function leave(message: UiText) {
    toast(message);
    void navigate(listFrom);
  }

  function confirm() {
    void run({
      key: "confirm",
      write: () => confirmTag(tag.id),
      done: (updated) => {
        setTag(updated);
        toast(t.tags.confirmed(updated.name));
      },
    });
  }

  function rename(name: string) {
    void run({
      key: "rename",
      write: () => renameTag(tag.id, name),
      done: (updated) => {
        setTag(updated);
        setRenaming(false);
      },
      failed: (failure) => {
        if (!(failure instanceof RequestFailed)) return false;
        if (failure.code !== "tag_name_taken" && failure.code !== "invalid_request") {
          return false;
        }
        setRenameError(tagFieldError(failure, { submitted: name, ownTagName: tag.name }));
        return true;
      },
    });
  }

  function remove(kind: "reject" | "delete") {
    let handled = false;
    void run({
      key: kind,
      write: () => (kind === "reject" ? rejectTag(tag.id) : deleteTag(tag.id)),
      done: () =>
        leave(kind === "reject" ? t.tags.rejected(tag.name) : t.tags.deleted(tag.name)),
      failed: (failure) => {
        handled = true;
        setDialogError(errorText(failure));
        return true;
      },
    }).then((removed) => {
      if (!removed && !handled) setDialog(null);
    });
  }

  function setKind(value: string) {
    void run({
      key: "kind",
      write: async () => {
        await (value === "none"
          ? batchTags("clearKind", [tag.id])
          : batchTags({ action: "setKind", kindId: Number(value) }, [tag.id]));
        return getTag(tag.id);
      },
      done: setTag,
      failed: (failure) => {
        if (failure instanceof RequestFailed && failure.reason === "tag_kind_not_found") {
          refreshTagKinds().catch(() => undefined);
        }
        return false;
      },
    });
  }

  const actions = (
    <>
      {tag.tentative && (
        <ActionButton
          record={record}
          busyKey="confirm"
          icon={<Check aria-hidden="true" />}
          label={text.confirm}
          variant="default"
          onClick={confirm}
        />
      )}
      <ActionButton
        record={record}
        busyKey="rename"
        icon={<Pencil aria-hidden="true" />}
        label={text.rename}
        variant="secondary"
        onClick={() => {
          setRenameError(null);
          setRenaming(true);
        }}
      />
      <Separator orientation="vertical" className="h-6" />
      <ActionButton
        record={record}
        busyKey={tag.tentative ? "reject" : "delete"}
        icon={tag.tentative ? <Ban aria-hidden="true" /> : <Trash2 aria-hidden="true" />}
        label={tag.tentative ? text.reject : text.delete}
        variant="ghost-destructive"
        onClick={() => {
          setDialogError(null);
          setDialog(tag.tentative ? "reject" : "delete");
        }}
      />
    </>
  );

  return (
    <RecordEditorPage
      back={back}
      title={
        <span className="inline-flex flex-wrap items-center gap-2">
          {tag.name}
          {tag.tentative && <TentativeMark size="row" />}
        </span>
      }
      titleField={
        renaming && (
          <RowNameInput
            idPrefix="tag-rename"
            rowId={tag.id}
            name={tag.name}
            label={t.tags.row.renameLabel(tag.name)}
            pending={busy === "rename"}
            error={renameError}
            onSubmit={rename}
            onCancel={() => setRenaming(false)}
            onDraftChange={() => setRenameError(null)}
          />
        )
      }
      figures={
        <RecordEditorFigure
          value={formatNumber(tag.videoCount)}
          unit={text.videosUnit(tag.videoCount)}
        />
      }
      actions={actions}
      notice={record.notice !== null && <NoticeAlert notice={record.notice} />}
      main={<TagNamesSection tag={tag} record={record} onMove={setMoving} />}
      side={
        <>
          <TagKindSection tag={tag} busy={busy} onChange={setKind} />
          <TagMergeSection
            tag={tag}
            record={record}
            onMerged={(target) => leave(t.tags.merged(tag.name, target.name))}
          />
        </>
      }
    >
      <TagVideosShelf tagId={tag.id} videoCount={tag.videoCount} />
      {moving !== null && (
        <MoveSynonymDialog
          tag={tag}
          name={moving}
          record={record}
          onClose={() => setMoving(null)}
        />
      )}
      {dialog === "reject" && (
        <RejectTagDialog
          tag={tag}
          pending={busy === "reject"}
          error={dialogError}
          onClose={() => setDialog(null)}
          onReject={() => remove("reject")}
        />
      )}
      {dialog === "delete" && (
        <DeleteTagDialog
          tag={tag}
          pending={busy === "delete"}
          error={dialogError}
          onClose={() => setDialog(null)}
          onDelete={() => remove("delete")}
        />
      )}
    </RecordEditorPage>
  );
}

function TagKindSection({
  tag,
  busy,
  onChange,
}: {
  tag: Tag;
  busy: string | null;
  onChange: (value: string) => void;
}) {
  const { kinds } = useTagKinds();
  // The shared list also holds the kinds only a paired VVMDM has, which this node's tag cannot take.
  const settable = (kinds ?? []).filter((kind) => !isRemoteId(kind.id));
  return (
    <PageSection title={text.kind}>
      <div className="flex items-center gap-2">
        <KindToggleGroup
          label={text.kind}
          layout="wrap"
          value={
            tag.kind === null || tag.kind === undefined ? "none" : String(tag.kind.id)
          }
          onValueChange={onChange}
          disabled={busy !== null}
          options={[
            ...settable.map((kind) => ({
              value: String(kind.id),
              label: kind.name,
              kind: kind.id,
            })),
            { value: "none", label: text.none },
          ]}
        />
        {busy === "kind" && <Spinner aria-hidden="true" />}
      </div>
    </PageSection>
  );
}
