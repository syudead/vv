import { ArrowLeft } from "lucide-react";
import type { ComponentType } from "react";
import { Link, Navigate, useLocation } from "react-router";

import { t } from "../i18n";
import { useDocumentTitle } from "../videoList/pageHooks";
import { PageHeader } from "../ui/patterns/page-header";
import {
  SettingsNav,
  SettingsNavItem,
  SettingsPage as SettingsPageLayout,
} from "../ui/patterns/settings-page";
import { Button } from "../ui/shadcn/button";
import APITokensSection from "./APITokensSection";
import { AutoImportStateSection, AutoImportSwitchSection } from "./AutoImportSection";
import AutoTaggingSection from "./AutoTaggingSection";
import ComfyUISection from "./ComfyUISection";
import MediaFoldersSection from "./MediaFoldersSection";
import NetworkSection from "./NetworkSection";
import OllamaSection from "./OllamaSection";
import ScanStatusSection from "./ScanStatusSection";
import {
  settingsPagePath,
  settingsPages,
  settingsRoute,
  type SettingsPageEntry,
  type SettingsSectionId,
} from "./settingsPages";
import TranscodingSection from "./TranscodingSection";
import VersionJudgingSection from "./VersionJudgingSection";
import WorkInfoPluginsSection from "./WorkInfoPluginsSection";

/** sectionComponents は対応表（settingsPages）の節を描く部品である。 */
const sectionComponents: Record<SettingsSectionId, ComponentType> = {
  "scan-status": ScanStatusSection,
  "auto-import-state": AutoImportStateSection,
  "media-folders": MediaFoldersSection,
  "auto-import-switch": AutoImportSwitchSection,
  "version-judging": VersionJudgingSection,
  "auto-tagging": AutoTaggingSection,
  ollama: OllamaSection,
  comfyui: ComfyUISection,
  "work-info-plugins": WorkInfoPluginsSection,
  transcoding: TranscodingSection,
  network: NetworkSection,
  "api-tokens": APITokensSection,
};

function PageList({ current }: { current: SettingsPageEntry }) {
  return (
    <SettingsNav label={t.settings.title}>
      {settingsPages.map((page) => (
        <SettingsNavItem key={page.slug} current={page.slug === current.slug}>
          <Link to={settingsPagePath(page.slug)}>{t.settings.pages[page.slug]}</Link>
        </SettingsNavItem>
      ))}
    </SettingsNav>
  );
}

function SettingsPageView({
  page,
  view,
}: {
  page: SettingsPageEntry;
  view: "list" | "page";
}) {
  const title = t.settings.pages[page.slug];
  useDocumentTitle(title);
  return (
    <SettingsPageLayout
      view={view}
      nav={<PageList current={page} />}
      header={
        <PageHeader
          title={title}
          leading={
            <Button asChild variant="ghost" size="sm">
              <Link to="/settings">
                <ArrowLeft aria-hidden="true" />
                {t.settings.title}
              </Link>
            </Button>
          }
        />
      }
    >
      {page.sections.map((section) => {
        const Section = sectionComponents[section];
        return <Section key={section} />;
      })}
    </SettingsPageLayout>
  );
}

/**
 * SettingsPage は設定の 8 つのページと一覧である（specs/058-settings-pages/research.md R-1〜R-3、
 * ui-design.md「Page list and skeleton」）。`/settings/<slug>` はそのページを、`/settings` は
 * sm から「Scan」のページを、sm 未満で一覧だけを出す（どちらを見せるかは CSS が決める）。
 * 知らないスラッグは `/settings/scan` へ、対応表が知るアンカーの `/settings#<anchor>` はその
 * ページのアドレスへ置き換える。
 *
 * 今のページの節だけを組み込むので、ページを移ると前のページの未保存の入力は失われ、各ページは
 * 自分の読み込みだけをする。ページごとに key を変え、移った先のページを初めから組む。
 */
export default function SettingsPage() {
  const location = useLocation();
  const route = settingsRoute(location.pathname, location.hash);
  if (route.kind === "replace") return <Navigate to={route.to} replace />;
  return (
    <SettingsPageView
      key={route.page.slug}
      page={route.page}
      view={route.kind === "list" ? "list" : "page"}
    />
  );
}
