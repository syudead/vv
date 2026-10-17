import { t } from "../i18n";
import { PageSection } from "../ui/patterns/page-section";

import AutoImportState from "./AutoImportState";
import AutoImportSwitch from "./AutoImportSwitch";
import { PageLinkText } from "./PageLink";
import { useAutoImportSettings } from "./useAutoImportSettings";

/**
 * AutoImportStateSection は「Scan」のページの「Auto-import」節である。監視の状態とその問題を
 * 出し、監視の状態を追う契機（開いたとき、ウィンドウへ戻ったとき、直近の取り込みが終わったとき、
 * `starting` のあいだ 2 秒ごと）で読み直す。メディアフォルダは別のページで変わるので、
 * このページを開き直すと読み直す（specs/058-settings-pages/ui-design.md「Scan page: Auto-import
 * state」）。
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
 * AutoImportSwitchSection は「Library」のページの「Auto-import」節である。切り替えの 1 行で、
 * 説明は監視の状態のある「Scan」のページへのリンクである（どちらの状態でも同じ文なので、押しても
 * 行の高さが変わらない）。開いたときとウィンドウへ戻ったときに読み直す。監視の状態は追わない
 * （specs/058-settings-pages/ui-design.md「Library page」）。
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
