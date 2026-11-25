import { ArrowLeft, Ban, Check, Ellipsis, Film, Pencil, Star } from "lucide-react";
import { type MouseEvent, useState } from "react";

import { t } from "@/i18n";
import { KindToggleGroup } from "@/ui/KindToggleGroup";
import { PageSection } from "@/ui/patterns/page-section";
import { RecordEditorFigure, RecordEditorPage } from "@/ui/patterns/record-editor-page";
import { Shelf } from "@/ui/patterns/shelf";
import { Button } from "@/ui/shadcn/button";
import { Combobox } from "@/ui/shadcn/combobox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/ui/shadcn/dropdown-menu";
import { Input } from "@/ui/shadcn/input";
import { Separator } from "@/ui/shadcn/separator";
import TentativeMark from "@/ui/TentativeMark";
import { VideoThumbnail } from "@/ui/VideoThumbnail";

import { sampleKinds } from "./record-list-example";

// registry:block record-editor-page-example. Real pages use router links; see web/registry/rules/patterns.md, Record editor page.

const stay = (event: MouseEvent) => event.preventDefault();

export function RecordEditorPageExample() {
  const r = t.designSystem.pattern.record;
  const p = t.designSystem.pattern;
  const [kind, setKind] = useState("3");
  const [target, setTarget] = useState("");
  const name = r.tags.travel;
  const synonyms = r.synonyms.travel;
  const titles = Object.values(p.videoTitles).slice(0, 6);
  return (
    <RecordEditorPage
      back={
        <Button asChild variant="ghost" size="sm">
          <a href="#record-editor-example" onClick={stay}>
            <ArrowLeft aria-hidden="true" />
            {r.back}
          </a>
        </Button>
      }
      title={
        <span className="inline-flex items-center gap-2">
          {name}
          <TentativeMark size="row" />
        </span>
      }
      figures={<RecordEditorFigure value={42} unit={r.videosUnit(42)} />}
      actions={
        <>
          <Button size="sm" shape="pill">
            <Check aria-hidden="true" />
            {r.confirm}
          </Button>
          <Button variant="secondary" size="sm" shape="pill">
            <Pencil aria-hidden="true" />
            {r.rename}
          </Button>
          <Separator orientation="vertical" className="h-6" />
          <Button variant="ghost-destructive" size="sm" shape="pill">
            <Ban aria-hidden="true" />
            {r.reject}
          </Button>
        </>
      }
      main={
        <PageSection title={r.names}>
          <div className="flex items-center gap-2 bg-primary-soft">
            <Star
              aria-hidden="true"
              className="size-4 shrink-0 fill-primary text-primary"
            />
            <span className="min-w-0 flex-1 font-semibold wrap-anywhere">{name}</span>
            <span className="shrink-0 text-xs text-primary">{r.mainName}</span>
          </div>
          {synonyms.map((synonym) => (
            <div key={synonym} className="flex items-center gap-2">
              <span className="min-w-0 flex-1 wrap-anywhere">{synonym}</span>
              <Button
                variant="outline"
                size="sm"
                shape="pill"
                aria-label={r.makeMain(synonym)}
              >
                <Star aria-hidden="true" />
                {r.main}
              </Button>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={r.nameActions(synonym)}
                  >
                    <Ellipsis aria-hidden="true" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem>{r.moveName}</DropdownMenuItem>
                  <DropdownMenuItem>{r.detachName}</DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem variant="destructive">
                    {r.removeName}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          ))}
          <div>
            <Input aria-label={r.addName} placeholder={r.addName} className="h-8" />
          </div>
        </PageSection>
      }
      side={
        <>
          <PageSection title={r.kind}>
            <div>
              <KindToggleGroup
                label={r.kind}
                layout="wrap"
                value={kind}
                onValueChange={setKind}
                options={[
                  ...sampleKinds().map((entry) => ({
                    value: String(entry.id),
                    label: entry.name,
                    kind: entry.id,
                  })),
                  { value: "none", label: r.none },
                ]}
              />
            </div>
          </PageSection>
          <PageSection title={r.mergeTitle}>
            <div className="flex flex-col gap-3">
              <p className="text-muted-foreground">{r.mergeDescription}</p>
              <Combobox
                aria-label={r.tagToKeep}
                options={[
                  { value: "trains", label: r.tags.trains },
                  { value: "coast", label: r.tags.coast },
                ]}
                value={target}
                onValueChange={setTarget}
                placeholder={r.chooseTag}
                emptyText={r.chooseTag}
              />
              <Button size="sm" className="self-start" disabled={target === ""}>
                {r.mergeAction}
              </Button>
            </div>
          </PageSection>
        </>
      }
    >
      <Shelf
        title={r.videos}
        layout="row"
        actions={
          <Button asChild variant="ghost" size="sm">
            <a href="#record-editor-example" onClick={stay}>
              {r.openInLibrary}
            </a>
          </Button>
        }
      >
        {titles.map((title) => (
          <a
            key={title}
            href="#record-editor-example"
            onClick={stay}
            title={title}
            className="flex min-w-0 flex-col gap-2"
          >
            <VideoThumbnail>
              <span className="flex size-full items-center justify-center text-muted-foreground">
                <Film aria-hidden="true" className="size-6" />
              </span>
            </VideoThumbnail>
            <span className="truncate text-sm font-medium">{title}</span>
          </a>
        ))}
      </Shelf>
    </RecordEditorPage>
  );
}
