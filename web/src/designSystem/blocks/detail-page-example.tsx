import {
  ArrowLeft,
  CalendarPlus,
  Clock,
  HardDrive,
  Monitor,
  Play,
  X,
} from "lucide-react";

import { t } from "@/i18n";
import { DetailPage } from "@/ui/patterns/detail-page";
import { Badge } from "@/ui/shadcn/badge";
import { Button } from "@/ui/shadcn/button";

// 詳細ページの型の見本（registry:block detail-page-example）。見出しの帯に戻る操作・場所・
// 閉じる操作、主領域にメディアの場所と題名・タグ・情報の行、横の情報欄に関連する項目の並びを
// 置く。写した画面はメディアの場所を自分の中身（プレーヤーなど）に差し替える
// （web/registry/rules/patterns.md の Detail page）。

export function DetailPageExample() {
  const p = t.designSystem.pattern;
  const facts = [
    { id: "duration", icon: Clock, label: p.duration, value: "49:10" },
    { id: "resolution", icon: Monitor, label: p.resolution, value: "1920 × 1080" },
    { id: "size", icon: HardDrive, label: p.fileSize, value: "2.4 GB" },
    { id: "added", icon: CalendarPlus, label: p.added, value: "2026-10-02" },
  ];
  const related = [
    p.videoTitles.harbour,
    p.videoTitles.eikando,
    p.videoTitles.train,
    p.videoTitles.lighthouse,
  ];
  return (
    <DetailPage
      header={
        <>
          <Button variant="ghost" size="sm">
            <ArrowLeft aria-hidden="true" />
            {p.back}
          </Button>
          <span className="min-w-0 flex-1 truncate text-sm font-medium">{p.library}</span>
          <Button variant="ghost" size="icon" aria-label={t.common.close}>
            <X aria-hidden="true" />
          </Button>
        </>
      }
      media={
        <div className="flex aspect-video w-full items-center justify-center bg-navbar lg:rounded-lg">
          <Button size="lg" aria-label={p.play}>
            <Play aria-hidden="true" />
          </Button>
        </div>
      }
      aside={
        <section className="flex flex-col gap-3">
          <h2 className="text-sm font-semibold">{p.related}</h2>
          <ul className="flex flex-col gap-3">
            {related.map((title) => (
              <li key={title} className="flex gap-3">
                <span className="aspect-video w-related-thumb shrink-0 rounded-md bg-card" />
                <span className="line-clamp-2 min-w-0 text-sm font-medium">{title}</span>
              </li>
            ))}
          </ul>
        </section>
      }
    >
      <div className="flex flex-col gap-2">
        <h1 className="text-xl font-semibold">{p.videoTitle}</h1>
        <div className="flex flex-wrap gap-1.5">
          {[p.tagNames.travel, p.tagNames.kyoto, p.tagNames.autumn].map((name) => (
            <Badge key={name} variant="secondary">
              {name}
            </Badge>
          ))}
        </div>
      </div>
      <ul className="flex flex-wrap items-center gap-x-5 gap-y-2 border-t border-border pt-3 text-sm text-muted-foreground tabular-nums">
        {facts.map(({ id, icon: Icon, label, value }) => (
          <li key={id} title={label} className="flex items-center gap-1.5">
            <Icon className="size-4 shrink-0" aria-hidden="true" />
            <span className="sr-only">{label} </span>
            {value}
          </li>
        ))}
      </ul>
    </DetailPage>
  );
}
