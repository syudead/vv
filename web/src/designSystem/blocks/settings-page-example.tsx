import { ArrowLeft } from "lucide-react";
import { type MouseEvent, type ReactNode, useState } from "react";

import { t } from "@/i18n";
import { FactList } from "@/ui/patterns/fact-list";
import { FormRow } from "@/ui/patterns/form-row";
import { PageHeader } from "@/ui/patterns/page-header";
import { PageSection } from "@/ui/patterns/page-section";
import { SettingsNav, SettingsNavItem, SettingsPage } from "@/ui/patterns/settings-page";
import { Button } from "@/ui/shadcn/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/ui/shadcn/select";
import { Switch } from "@/ui/shadcn/switch";

// 設定ページの型の見本（registry:block settings-page-example）。見本のデータで埋めた
// 動く組み合わせで、写した画面は文言と値を自分のものに差し替える
// （web/registry/rules/patterns.md の Settings page）。ページの一覧（nav）を持つ設定の
// 3 ページを見せる。見本はページの切り替えを状態で持つが、写した画面は一覧の項目と戻るリンクを
// ルーターのリンクにし、一覧の住所を view="list"、各ページの住所を view="page" にする。

const pageIds = ["library", "playback", "server"] as const;
type PageId = (typeof pageIds)[number];

function LibraryRows() {
  const p = t.designSystem.pattern;
  const [showHidden, setShowHidden] = useState(false);
  const [sort, setSort] = useState("added");
  return (
    <PageSection title={p.videosSection} description={p.videosSectionDescription}>
      <FormRow
        label={p.showHidden}
        description={p.showHiddenDescription}
        htmlFor="settings-example-hidden"
      >
        <Switch
          id="settings-example-hidden"
          checked={showHidden}
          onCheckedChange={setShowHidden}
        />
      </FormRow>
      <FormRow
        label={p.defaultSort}
        description={p.defaultSortDescription}
        htmlFor="settings-example-sort"
      >
        <Select value={sort} onValueChange={setSort}>
          <SelectTrigger id="settings-example-sort" size="sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="added">{p.sortAdded}</SelectItem>
            <SelectItem value="title">{p.sortTitle}</SelectItem>
            <SelectItem value="duration">{p.sortDuration}</SelectItem>
          </SelectContent>
        </Select>
      </FormRow>
      <FormRow
        label={p.rescan}
        description={p.rescanDescription}
        htmlFor="settings-example-scan"
      >
        <Button id="settings-example-scan" variant="outline" size="sm">
          {p.scan}
        </Button>
      </FormRow>
    </PageSection>
  );
}

function PlaybackRows() {
  const p = t.designSystem.pattern;
  const [autoplay, setAutoplay] = useState(true);
  return (
    <PageSection title={p.playerSection} description={p.playerSectionDescription}>
      <FormRow
        label={p.autoplay}
        description={p.autoplayDescription}
        htmlFor="settings-example-autoplay"
      >
        <Switch
          id="settings-example-autoplay"
          checked={autoplay}
          onCheckedChange={setAutoplay}
        />
      </FormRow>
    </PageSection>
  );
}

function ServerRows() {
  const p = t.designSystem.pattern;
  return (
    <PageSection title={p.detailsSection} description={p.detailsSectionDescription}>
      <FactList
        facts={[
          { id: "version", term: p.version, value: "0.38.0" },
          { id: "data", term: p.dataFolder, value: "/srv/vvmdm/data" },
          { id: "port", term: p.port, value: "8080" },
        ]}
      />
    </PageSection>
  );
}

const pageRows: Record<PageId, ReactNode> = {
  library: <LibraryRows />,
  playback: <PlaybackRows />,
  server: <ServerRows />,
};

export function SettingsPageExample() {
  const p = t.designSystem.pattern;
  // null は一覧の住所。sm からは最初のページを出し、sm 未満は一覧だけを出す。
  const [open, setOpen] = useState<PageId | null>(null);
  const shown = open ?? pageIds[0];
  const go = (next: PageId | null) => (event: MouseEvent) => {
    event.preventDefault();
    setOpen(next);
  };
  return (
    <SettingsPage
      view={open === null ? "list" : "page"}
      nav={
        <SettingsNav label={p.settings}>
          {pageIds.map((id) => (
            <SettingsNavItem key={id} current={id === shown}>
              <a href={`#settings-example-${id}`} onClick={go(id)}>
                {p.settingsPages[id]}
              </a>
            </SettingsNavItem>
          ))}
        </SettingsNav>
      }
      header={
        <PageHeader
          title={p.settingsPages[shown]}
          leading={
            <Button asChild variant="ghost" size="sm">
              <a href="#settings-example" onClick={go(null)}>
                <ArrowLeft aria-hidden="true" />
                {p.settings}
              </a>
            </Button>
          }
        />
      }
    >
      {pageRows[shown]}
    </SettingsPage>
  );
}
