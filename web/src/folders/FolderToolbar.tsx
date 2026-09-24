import { ChevronDown, SlidersHorizontal } from "lucide-react";

import type { VideoSort } from "../api/client";
import { sortKindOf } from "../library/listCriteria";
import { ZoomSlider } from "../library/LibraryToolbar";
import { sortOptions } from "../library/SortControls";

// ランダムは「並べ直す」と seed が要るので、フォルダ画面に検索を足す単位で
// ライブラリの部品へ揃えるまで出さない。
const folderSortOptions = sortOptions.filter((option) => option.value !== "random");
import { cn } from "../lib/cn";
import type { Zoom } from "../preferences/viewPreferences";
import Button from "../ui/Button";
import {
  MenuContent,
  MenuLabel,
  MenuRadioGroup,
  MenuRadioItem,
  MenuRoot,
  MenuTrigger,
} from "../ui/Menu";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "../ui/Popover";
import Tooltip from "../ui/Tooltip";

export interface FolderToolbarProps {
  /** 動画を出さない最上位では並び順を出さない。 */
  sort?: VideoSort;
  onSortChange: (value: VideoSort) => void;
  zoom: Zoom;
  onZoomChange: (value: Zoom) => void;
}

/**
 * FolderToolbar はフォルダ画面のトップバー内の操作で、並び順と表示倍率だけを
 * 持つ。部品と幅の境界はライブラリのツールバーと同じにし、検索欄が無いので
 * 右寄せにする（ui-design.md）。
 */
export default function FolderToolbar({
  sort,
  onSortChange,
  zoom,
  onZoomChange,
}: FolderToolbarProps) {
  // 種類だけを選ばせる。向きの切り替えと「並べ直す」は、フォルダ画面の検索を
  // 足すときにライブラリの部品（library/SortControls）へ揃える。
  const activeSort = sort === undefined ? undefined : sortKindOf(sort);

  return (
    <div className="flex min-w-0 flex-1 items-center justify-end gap-1.5">
      {sort !== undefined && (
        <div className="hidden md:block">
          <MenuRoot>
            <MenuTrigger asChild>
              <Button
                variant="secondary"
                aria-label={`並び順: ${activeSort?.label ?? "並び順"}`}
              >
                {activeSort?.label ?? "並び順"}
                <ChevronDown className="-mr-1 text-fg-muted" />
              </Button>
            </MenuTrigger>
            <MenuContent align="end">
              <MenuLabel>並び順</MenuLabel>
              <MenuRadioGroup
                value={activeSort?.initial ?? sort}
                onValueChange={(value) => onSortChange(value as VideoSort)}
              >
                {folderSortOptions.map((option) => (
                  <MenuRadioItem key={option.kind} value={option.value}>
                    <option.icon />
                    {option.label}
                  </MenuRadioItem>
                ))}
              </MenuRadioGroup>
            </MenuContent>
          </MenuRoot>
        </div>
      )}

      <Tooltip content="カードの大きさ">
        <ZoomSlider
          zoom={zoom}
          onZoomChange={onZoomChange}
          className="hidden w-24 xl:flex"
        />
      </Tooltip>

      <PopoverRoot>
        <PopoverTrigger asChild>
          <Button
            variant="secondary"
            aria-label={sort === undefined ? "表示" : "表示と並び順"}
            className="px-2.5 xl:hidden"
          >
            <SlidersHorizontal />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="end" className="w-64">
          <div className="space-y-4">
            {sort !== undefined && (
              <fieldset className="md:hidden">
                <legend className="mb-2 text-xs font-semibold text-fg-muted uppercase">
                  並び順
                </legend>
                <div className="grid grid-cols-2 gap-1">
                  {folderSortOptions.map((option) => (
                    <label
                      key={option.kind}
                      className={cn(
                        "flex h-8 cursor-pointer items-center justify-center gap-2 rounded-md text-sm transition-colors",
                        activeSort?.kind === option.kind
                          ? "bg-accent text-accent-fg"
                          : "text-fg hover:bg-hover-wash",
                      )}
                    >
                      <input
                        type="radio"
                        name="folder-compact-sort"
                        value={option.value}
                        checked={activeSort?.kind === option.kind}
                        onChange={() => onSortChange(option.value)}
                        className="sr-only"
                      />
                      <option.icon className="size-4" />
                      {option.label}
                    </label>
                  ))}
                </div>
              </fieldset>
            )}
            <fieldset>
              <legend className="mb-1 text-xs font-semibold text-fg-muted uppercase">
                カードの大きさ
              </legend>
              <ZoomSlider zoom={zoom} onZoomChange={onZoomChange} className="w-full" />
            </fieldset>
          </div>
        </PopoverContent>
      </PopoverRoot>
    </div>
  );
}
