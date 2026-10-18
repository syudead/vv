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
import HomeSection from "./HomeSection";
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

const sectionComponents: Record<SettingsSectionId, ComponentType> = {
  "scan-status": ScanStatusSection,
  "auto-import-state": AutoImportStateSection,
  "media-folders": MediaFoldersSection,
  "auto-import-switch": AutoImportSwitchSection,
  "version-judging": VersionJudgingSection,
  home: HomeSection,
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
 * Lock controls only when the connection is known to be unsaved; connection-load failure shows
 * Retry without locking.
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

const pageViews: Partial<
  Record<SettingsPageEntry["slug"], ComponentType<PageViewProps>>
> = { "auto-tagging": AutoTaggingPageView };

/**
 * Mount only the current page, keyed by route, so navigation discards unsaved drafts and isolates
 * requests. Reset scroll on navigation; CSS chooses the index versus default page at sm.
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
