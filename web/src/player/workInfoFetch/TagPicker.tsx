import { useId } from "react";

import type { Video } from "../../api/client";
import type { WorkInfoFetch } from "../../api/workInfoPlugins";
import { t } from "../../i18n";
import { Checkbox } from "../../ui/shadcn/checkbox";
import { Label } from "../../ui/shadcn/label";
import { selectableTags, videoTagNames } from "./fetchRules";
import { Chip } from "./TagChips";

export interface TagPick {
  selected: readonly string[];
  locked: boolean;
  onToggle: (name: string, on: boolean) => void;
}

export default function TagPicker({
  video,
  result,
  pick,
}: {
  video: Video;
  result: WorkInfoFetch;
  pick: TagPick;
}) {
  const id = useId();
  const onVideo = videoTagNames(video);
  const selectable = selectableTags(video, result);
  const selected = selectable.filter((name) => pick.selected.includes(name)).length;
  // Duplicate result names share one checkbox.
  const names = [...new Set(result.tags)];
  return (
    <div className="flex w-full min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
      {names.map((name, index) => {
        if (onVideo.has(name))
          return <Chip key={name} item={{ name, tentative: true, mark: "onVideo" }} />;
        const checkId = `${id}-${String(index)}`;
        return (
          <span key={name} className="inline-flex max-w-full min-w-0 items-center gap-2">
            <Checkbox
              id={checkId}
              checked={pick.selected.includes(name)}
              disabled={pick.locked}
              onCheckedChange={(checked) => pick.onToggle(name, checked === true)}
            />
            <Label htmlFor={checkId} className="min-w-0 font-normal">
              <Chip item={{ name, tentative: true }} />
            </Label>
          </span>
        );
      })}
      <span className="text-xs text-muted-foreground">
        {t.player.workInfoFetch.tagsSelected(selected, selectable.length)}
      </span>
    </div>
  );
}
