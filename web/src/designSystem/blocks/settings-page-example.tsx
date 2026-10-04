import { useState } from "react";

import { t } from "@/i18n";
import { FactList } from "@/ui/patterns/fact-list";
import { FormRow } from "@/ui/patterns/form-row";
import { PageHeader } from "@/ui/patterns/page-header";
import { PageSection } from "@/ui/patterns/page-section";
import { SettingsPage } from "@/ui/patterns/settings-page";
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
// （web/registry/rules/patterns.md の Settings page）。

export function SettingsPageExample() {
  const p = t.designSystem.pattern;
  const [showHidden, setShowHidden] = useState(false);
  const [autoplay, setAutoplay] = useState(true);
  const [sort, setSort] = useState("added");
  return (
    <SettingsPage
      header={<PageHeader title={p.settings} description={p.settingsDescription} />}
    >
      <PageSection title={p.librarySection} description={p.librarySectionDescription}>
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
      <PageSection title={p.serverSection} description={p.serverSectionDescription}>
        <FactList
          facts={[
            { id: "version", term: p.version, value: "0.38.0" },
            { id: "data", term: p.dataFolder, value: "/srv/vvmdm/data" },
            { id: "port", term: p.port, value: "8080" },
          ]}
        />
      </PageSection>
    </SettingsPage>
  );
}
