import { memo, type MouseEvent } from "react";
import { Link } from "react-router";

import type { Tag } from "../api/tags";
import { formatNumber, t } from "../i18n";
import { libraryFilterPath } from "../library/libraryPath";
import { TAG_PARAM } from "../library/tagCriteria";
import { isRemoteId } from "../lib/remoteId";
import { RecordCard } from "../ui/RecordCard";
import TentativeMark from "../ui/TentativeMark";
import { ItemRemoteMark } from "../videoList/RemoteItemMark";

export function tagPagePath(id: number): string {
  return `/tags/${String(id)}`;
}

function detailOf(tag: Tag): string {
  const kind = tag.kind?.name ?? t.tags.list.noKind;
  return tag.tentative ? t.tags.list.tentativeDetail(kind) : kind;
}

/** How a card takes part in the selection; the callbacks are shared by every card. */
export interface TagCardSelection {
  /** Something is selected: a press on the card selects it instead of opening it. */
  selectionMode: boolean;
  onSelectedChange: (id: number, selected: boolean) => void;
  onToggle: (id: number) => void;
}

/**
 * One tag as a record card. A tag only the paired VVMDM holds has no page here, so its card opens the
 * library filtered by it instead.
 */
function TagCard({
  tag,
  selected,
  selection,
  onOpen,
}: {
  tag: Tag;
  selected: boolean;
  selection: TagCardSelection;
  onOpen: (tag: Tag) => void;
}) {
  const remote = isRemoteId(tag.id);
  const { selectionMode, onSelectedChange, onToggle } = selection;
  function press(event: MouseEvent) {
    if (!selectionMode) {
      onOpen(tag);
      return;
    }
    event.preventDefault();
    onToggle(tag.id);
  }
  return (
    <RecordCard
      link={
        <Link
          to={remote ? libraryFilterPath(TAG_PARAM, tag.id) : tagPagePath(tag.id)}
          data-tag-id={tag.id}
          onClick={press}
        />
      }
      name={tag.name}
      nameMarks={
        <>
          {tag.tentative && <TentativeMark />}
          {remote && <ItemRemoteMark itemId={tag.id} />}
        </>
      }
      count={formatNumber(tag.videoCount)}
      countTitle={t.tags.row.videoCount(tag.videoCount)}
      detail={detailOf(tag)}
      secondary={tag.synonyms}
      kind={tag.kind?.id}
      tentative={tag.tentative}
      selection={{
        selected,
        selectionMode,
        label: t.tags.row.select(tag.name),
        onSelectedChange: (next) => onSelectedChange(tag.id, next),
      }}
    />
  );
}

export default memo(TagCard);
