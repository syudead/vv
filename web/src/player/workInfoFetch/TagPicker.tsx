import { useId } from "react";

import type { Video } from "../../api/client";
import type { WorkInfoFetch } from "../../api/workInfoPlugins";
import { t } from "../../i18n";
import { Checkbox } from "../../ui/shadcn/checkbox";
import { Label } from "../../ui/shadcn/label";
import { selectableTags, videoTagNames } from "./fetchRules";
import { Chip } from "./TagChips";

/** TagPick は比較の段のタグの選択と、それを変える手段である。 */
export interface TagPick {
  /** 選んだタグの名前。 */
  selected: readonly string[];
  /** 反映を送っている間か結果が期限切れで、選択を変えられない。 */
  locked: boolean;
  onToggle: (name: string, on: boolean) => void;
}

/**
 * TagPicker は取得した側のタグの並びで、動画にまだ無いタグは Checkbox とそのラベルのチップ、
 * 動画にあるタグは選べない `On the video` のチップである。畳まずに全部を出し、最後に
 * `{n} of {m} selected` を添える（specs/056-plugin-tooling/ui-design.md「Compare: tags row」）。
 */
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
  // 同じ名前が結果に 2 回あっても、チェックボックスは 1 つにする。
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
