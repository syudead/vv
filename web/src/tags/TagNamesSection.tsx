import { ArrowRightLeft, Scissors, Star, Trash2 } from "lucide-react";

import {
  detachTagSynonym,
  promoteTagSynonym,
  removeTagSynonym,
  type Tag,
} from "../api/tags";
import { t } from "../i18n";
import { PageSection } from "../ui/patterns/page-section";
import { Button } from "../ui/shadcn/button";
import { DropdownMenuItem, DropdownMenuSeparator } from "../ui/shadcn/dropdown-menu";
import { Spinner } from "../ui/shadcn/spinner";
import { RowMoreMenu } from "../ui/RowMoreMenu";
import { useToast } from "../ui/Toast";
import { AddTagNameRow } from "./AddTagNameRow";
import type { TagRecord } from "./useTagRecord";

const text = t.tags.page;

/** The main name first, then each synonym with `Main` and its menu, then the field that adds one. */
export function TagNamesSection({
  tag,
  record,
  onMove,
}: {
  tag: Tag;
  record: TagRecord;
  /** Opens the dialog that moves the synonym to another tag. */
  onMove: (name: string) => void;
}) {
  return (
    <PageSection title={text.names}>
      <div className="flex items-center gap-2 bg-primary-soft">
        <Star aria-hidden="true" className="size-4 shrink-0 fill-primary text-primary" />
        <span className="min-w-0 flex-1 font-semibold wrap-anywhere">{tag.name}</span>
        <span className="shrink-0 text-xs text-primary">{text.mainName}</span>
      </div>
      {tag.synonyms.map((name) => (
        <SynonymRow key={name} tag={tag} name={name} record={record} onMove={onMove} />
      ))}
      <AddTagNameRow tag={tag} record={record} />
    </PageSection>
  );
}

function SynonymRow({
  tag,
  name,
  record,
  onMove,
}: {
  tag: Tag;
  name: string;
  record: TagRecord;
  onMove: (name: string) => void;
}) {
  const toast = useToast();
  const { busy, run, setTag, setNotice } = record;
  const blocked = busy !== null || undefined;
  const promoteKey = `promote:${name}`;
  const menuKey = `menu:${name}`;

  function promote() {
    void run({
      key: promoteKey,
      name,
      write: () => promoteTagSynonym(tag.id, name),
      done: (updated) => {
        setTag(updated);
        toast(text.promoted(name));
      },
    });
  }

  function detach() {
    void run({
      key: menuKey,
      name,
      write: () => detachTagSynonym(tag.id, name),
      done: ({ source, created }) => {
        setTag(source);
        setNotice({ kind: "detached", name: created.name, id: created.id });
      },
    });
  }

  function remove() {
    void run({
      key: menuKey,
      name,
      write: () => removeTagSynonym(tag.id, name),
      done: () => {
        setTag((current) =>
          current === undefined
            ? current
            : { ...current, synonyms: current.synonyms.filter((item) => item !== name) },
        );
        toast(text.removed(name));
      },
    });
  }

  return (
    <div data-synonym={name} className="flex items-center gap-2">
      <span className="min-w-0 flex-1 wrap-anywhere">{name}</span>
      <Button
        variant="outline"
        size="sm"
        shape="pill"
        aria-label={text.makeMain(name)}
        aria-disabled={blocked}
        onClick={promote}
      >
        {busy === promoteKey ? (
          <Spinner aria-hidden="true" />
        ) : (
          <Star aria-hidden="true" />
        )}
        {text.main}
      </Button>
      {busy === menuKey ? (
        <Spinner aria-hidden="true" className="mx-2" />
      ) : (
        <RowMoreMenu
          buttonRef={null}
          label={text.nameActions(name)}
          disabled={busy !== null}
        >
          <DropdownMenuItem onSelect={() => onMove(name)}>
            <ArrowRightLeft aria-hidden="true" />
            {text.moveName}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={detach}>
            <Scissors aria-hidden="true" />
            {text.detachName}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" onSelect={remove}>
            <Trash2 aria-hidden="true" />
            {text.removeName}
          </DropdownMenuItem>
        </RowMoreMenu>
      )}
    </div>
  );
}
