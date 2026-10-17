import { ArrowLeft } from "lucide-react";
import type { ComponentType, ReactNode } from "react";
import { Link, Navigate, useLocation } from "react-router";

import { t } from "../i18n";
import { useDocumentTitle } from "../videoList/pageHooks";
import { useScrollTopOnChange } from "../videoList/useScrollTopOnChange";
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
import { ollamaLockNotice } from "./OllamaLockNotice";
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
import { useOllamaConnection } from "./useOllamaConnection";
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

interface PageViewProps {
  page: SettingsPageEntry;
  view: "list" | "page";
}

/** PageSections は対応表のページの節を上から順に描く。 */
function PageSections({ page }: { page: SettingsPageEntry }) {
  return page.sections.map((section) => {
    const Section = sectionComponents[section];
    return <Section key={section} />;
  });
}

function SettingsPageView({
  page,
  view,
  notice,
  children,
}: PageViewProps & { notice?: ReactNode; children: ReactNode }) {
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
      notice={notice}
    >
      {children}
    </SettingsPageLayout>
  );
}

function SectionsPageView(props: PageViewProps) {
  return (
    <SettingsPageView {...props}>
      <PageSections page={props.page} />
    </SettingsPageView>
  );
}

/**
 * AutoTaggingPageView は「Auto-tagging」のページである。開いたときに Ollama の接続先を読み、
 * 読み終えるまでは節の読み込み中の形を出し、保存されていなければ notice に理由を出して節の操作を
 * 押せなくする。読めなければ読めなかった旨と `Retry` を出し、操作は押せたままにする（specs/058-settings-pages/research.md R-5）。
 */
function AutoTaggingPageView(props: PageViewProps) {
  const { connection, retry } = useOllamaConnection();
  return (
    <SettingsPageView {...props} notice={ollamaLockNotice(connection, retry)}>
      <AutoTaggingSection
        locked={connection.status === "unsaved"}
        pending={connection.status === "loading"}
      />
    </SettingsPageView>
  );
}

/** pageViews は対応表の節を並べるだけでないページの描き方である。ほかは SectionsPageView。 */
const pageViews: Partial<
  Record<SettingsPageEntry["slug"], ComponentType<PageViewProps>>
> = { "auto-tagging": AutoTaggingPageView };

/**
 * SettingsPage は設定の 8 つのページと一覧である（specs/058-settings-pages/research.md R-1〜R-3、
 * ui-design.md「Page list and skeleton」）。`/settings/<slug>` はそのページを、`/settings` は
 * sm から「Scan」のページを、sm 未満で一覧だけを出す（どちらを見せるかは CSS が決める）。
 * 知らないスラッグは `/settings/scan` へ、対応表が知るアンカーの `/settings#<anchor>` はその
 * ページのアドレスへ置き換える。
 *
 * 今のページの節だけを組み込むので、ページを移ると前のページの未保存の入力は失われ、各ページは
 * 自分の読み込みだけをする。ページごとに key を変え、移った先のページを初めから組む。
 * 一覧の列は貼り付いたまま見えているので、長いページの途中から別のページへ移ったときは、移った先を
 * 先頭から見せる（題と最初の操作が見えるように）。
 */
export default function SettingsPage() {
  const location = useLocation();
  const route = settingsRoute(location.pathname, location.hash);
  useScrollTopOnChange(route.kind === "replace" ? undefined : route.page.slug);
  if (route.kind === "replace") return <Navigate to={route.to} replace />;
  const View = pageViews[route.page.slug] ?? SectionsPageView;
  return (
    <View
      key={route.page.slug}
      page={route.page}
      view={route.kind === "list" ? "list" : "page"}
    />
  );
}
