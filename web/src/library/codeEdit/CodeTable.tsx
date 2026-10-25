import { RotateCcw, Scissors, TriangleAlert } from "lucide-react";
import { memo } from "react";

import { t } from "../../i18n";
import { cn } from "../../lib/cn";
import { DataTable } from "../../ui/patterns/data-table";
import { Badge } from "../../ui/shadcn/badge";
import { Button } from "../../ui/shadcn/button";
import { FieldError } from "../../ui/shadcn/field";
import { Input } from "../../ui/shadcn/input";
import {
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../../ui/shadcn/table";
import {
  VideoThumbnail,
  VideoThumbnailDuration,
  VideoThumbnailImage,
} from "../../ui/VideoThumbnail";
import { shownVersionsCount, VersionsMark } from "../../videoList/VersionsMark";
import { VirtualMark } from "../../videoList/VirtualMark";
import { WithTooltip } from "../SelectionAction";
import { codeLimit, type DerivedRow } from "./rows";

/** The ID of a row's code input, so a failed Apply can scroll to it and focus it. */
export function codeInputId(videoId: number): string {
  return `code-edit-${String(videoId)}`;
}

export interface CodeTableProps {
  rows: readonly DerivedRow[];
  /** What the owner typed, shown as typed so trimming does not fight the keyboard. */
  typed: ReadonlyMap<number, string>;
  disabled: boolean;
  /** The row a failed Apply named, with the reason shown under its input. */
  failed?: { videoId: number; reason: string };
  onType: (videoId: number, value: string) => void;
  onRestore: (videoId: number) => void;
}

/**
 * One row per video: the thumbnail, the title, the file name, the code input and the marks. Below
 * `md` the thumbnail, file name and state columns drop, and their text moves into the two cells left.
 */
export function CodeTable({
  rows,
  typed,
  disabled,
  failed,
  onType,
  onRestore,
}: CodeTableProps) {
  const c = t.library.codeEdit;
  return (
    // Below lg the dialog scrolls under the sticky band; from lg the panel scrolls by itself.
    <div className="max-lg:[&_thead]:top-navbar!">
      <DataTable label={c.codes} stickyHeaderTop={0}>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead className="hidden w-list-thumb-cell md:table-cell" />
            <TableHead className="md:w-1/4">{c.columns.video}</TableHead>
            <TableHead className="hidden w-3/8 md:table-cell">
              {c.columns.fileName}
            </TableHead>
            <TableHead className="w-3/8">{c.columns.code}</TableHead>
            <TableHead className="hidden md:table-cell">{c.columns.state}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row) => (
            <CodeRow
              key={row.row.video.id}
              row={row}
              typed={typed.get(row.row.video.id)}
              disabled={disabled}
              reason={failed?.videoId === row.row.video.id ? failed.reason : undefined}
              onType={onType}
              onRestore={onRestore}
            />
          ))}
        </TableBody>
      </DataTable>
    </div>
  );
}

interface CodeRowProps {
  row: DerivedRow;
  typed: string | undefined;
  disabled: boolean;
  reason: string | undefined;
  onType: (videoId: number, value: string) => void;
  onRestore: (videoId: number) => void;
}

/** Derivation builds new row objects on every keystroke, so compare their fields. */
function sameRow(a: CodeRowProps, b: CodeRowProps): boolean {
  return (
    a.row.row === b.row.row &&
    a.row.state === b.row.state &&
    a.row.value === b.row.value &&
    a.row.writes === b.row.writes &&
    a.row.cut === b.row.cut &&
    a.row.duplicate === b.row.duplicate &&
    a.typed === b.typed &&
    a.disabled === b.disabled &&
    a.reason === b.reason &&
    a.onType === b.onType &&
    a.onRestore === b.onRestore
  );
}

const CodeRow = memo(function CodeRow({
  row,
  typed,
  disabled,
  reason,
  onType,
  onRestore,
}: CodeRowProps) {
  const c = t.library.codeEdit;
  const { video } = row.row;
  const versions = shownVersionsCount(video);
  const id = codeInputId(video.id);
  const errorId = `${id}-error`;
  return (
    <TableRow className="hover:bg-transparent">
      <TableCell className="hidden w-list-thumb-cell md:table-cell">
        <VideoThumbnail className="w-list-thumb rounded-sm">
          {video.thumbnailUrl !== undefined && (
            <VideoThumbnailImage src={video.thumbnailUrl} />
          )}
          {video.virtual && (
            <VideoThumbnailDuration>
              <VirtualMark />
            </VideoThumbnailDuration>
          )}
        </VideoThumbnail>
      </TableCell>
      <TableCell className="whitespace-normal">
        <span className="line-clamp-2 text-sm font-medium break-all text-foreground">
          {video.title}
          {versions !== null && (
            <span className="ml-1 inline-flex align-middle text-muted-foreground">
              <VersionsMark count={versions} />
            </span>
          )}
        </span>
        <span className="md:hidden">
          <FileName fileTitle={video.fileTitle} />
        </span>
      </TableCell>
      <TableCell className="hidden whitespace-normal md:table-cell">
        <FileName fileTitle={video.fileTitle} />
      </TableCell>
      <TableCell className="whitespace-normal">
        <Input
          id={id}
          className={cn(
            "h-8",
            row.state === "suggested" ? "text-muted-foreground" : "text-foreground",
          )}
          aria-label={c.codeOf(video.title)}
          aria-invalid={reason !== undefined || undefined}
          aria-describedby={reason === undefined ? undefined : errorId}
          aria-disabled={disabled || undefined}
          value={typed ?? row.value}
          onChange={(event) => {
            if (!disabled) onType(video.id, event.target.value);
          }}
        />
        {reason !== undefined && (
          <FieldError id={errorId} className="mt-1">
            {reason}
          </FieldError>
        )}
        <div className="mt-1 md:hidden">
          <Marks row={row} disabled={disabled} onRestore={onRestore} />
        </div>
      </TableCell>
      <TableCell className="hidden md:table-cell">
        <Marks row={row} disabled={disabled} onRestore={onRestore} />
      </TableCell>
    </TableRow>
  );
}, sameRow);

function FileName({ fileTitle }: { fileTitle: string | undefined }) {
  return (
    <span className="line-clamp-2 text-xs break-all text-muted-foreground">
      {fileTitle ?? t.library.codeEdit.noFile}
    </span>
  );
}

/** The state mark says Apply writes the row; the warnings never stop it. */
function Marks({
  row,
  disabled,
  onRestore,
}: {
  row: DerivedRow;
  disabled: boolean;
  onRestore: (videoId: number) => void;
}) {
  const c = t.library.codeEdit;
  const edited = row.state === "edited";
  return (
    <span className="flex items-center gap-1 whitespace-nowrap">
      {row.writes && (
        <Badge variant={edited ? "soft" : "secondary"}>
          {edited ? c.changed : c.suggested}
        </Badge>
      )}
      {row.duplicate && (
        <Badge variant="warning">
          <TriangleAlert aria-hidden="true" />
          {c.duplicate}
        </Badge>
      )}
      {row.cut && (
        <Badge variant="warning">
          <Scissors aria-hidden="true" />
          {c.cut(codeLimit)}
        </Badge>
      )}
      {edited && (
        <WithTooltip label={c.restore}>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={c.restore}
            aria-disabled={disabled || undefined}
            onClick={() => {
              if (!disabled) onRestore(row.row.video.id);
            }}
          >
            <RotateCcw aria-hidden="true" />
          </Button>
        </WithTooltip>
      )}
    </span>
  );
}
