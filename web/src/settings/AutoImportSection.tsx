import { t } from "../i18n";
import { PageSection } from "../ui/patterns/page-section";

import AutoImportState from "./AutoImportState";
import AutoImportSwitch from "./AutoImportSwitch";
import { PageLinkText } from "./PageLink";
import { useAutoImportSettings } from "./useAutoImportSettings";

/**
 * Refetch on opening, window focus and import completion; poll every two seconds while starting.
 * Reopening refreshes folders changed on another page.
 */
export function AutoImportStateSection() {
  const text = t.settings.autoImport;
  const source = useAutoImportSettings({ followWatch: true });
  return (
    <PageSection title={text.heading} description={text.description}>
      <AutoImportState source={source} />
    </PageSection>
  );
}

/**
 * Refetch on opening and window focus without following watcher state; stable copy avoids layout
 * shifts on toggle.
 */
export function AutoImportSwitchSection() {
  const text = t.settings.autoImport;
  const source = useAutoImportSettings();
  return (
    <PageSection title={text.heading} description={text.description}>
      <AutoImportSwitch
        source={source}
        description={<PageLinkText sentence={text.switchDescription} page="scan" />}
      />
    </PageSection>
  );
}
