import { ArrowLeft, Play, Tag } from "lucide-react";

import { t } from "@/i18n";
import { DetailPage } from "@/ui/patterns/detail-page";
import { FactList } from "@/ui/patterns/fact-list";
import { PageHeader } from "@/ui/patterns/page-header";
import { PageSection } from "@/ui/patterns/page-section";
import { Badge } from "@/ui/shadcn/badge";
import { Button } from "@/ui/shadcn/button";

// 詳細ページの型の見本（registry:block detail-page-example）。主領域に動画の場所と説明の
// 節、横の情報欄に事実の一覧を置く。写した画面は動画の場所を自分の中身（プレーヤーなど）に
// 差し替える（web/registry/rules/patterns.md の Detail page）。

export function DetailPageExample() {
  const p = t.designSystem.pattern;
  return (
    <DetailPage
      header={
        <PageHeader
          leading={
            <Button variant="ghost" size="sm">
              <ArrowLeft aria-hidden="true" />
              {p.back}
            </Button>
          }
          title={p.videoTitle}
          description={p.videoSubtitle}
          actions={
            <Button variant="outline">
              <Tag aria-hidden="true" />
              {p.addTag}
            </Button>
          }
        />
      }
      aside={
        <>
          <PageSection title={p.information}>
            <FactList
              facts={[
                { id: "duration", term: p.duration, value: "49:10" },
                { id: "resolution", term: p.resolution, value: "1920 × 1080" },
                { id: "size", term: p.fileSize, value: "2.4 GB" },
                { id: "added", term: p.added, value: "2026-10-02" },
              ]}
            />
          </PageSection>
          <PageSection title={p.tags}>
            <div className="flex flex-wrap gap-2">
              {[p.tagNames.travel, p.tagNames.kyoto, p.tagNames.autumn].map((name) => (
                <Badge key={name} variant="secondary">
                  {name}
                </Badge>
              ))}
            </div>
          </PageSection>
        </>
      }
    >
      <div className="flex aspect-video items-center justify-center rounded-md bg-navbar">
        <Button size="lg" aria-label={p.play}>
          <Play aria-hidden="true" />
        </Button>
      </div>
      <PageSection title={p.description}>
        <p>{p.descriptionText}</p>
      </PageSection>
    </DetailPage>
  );
}
