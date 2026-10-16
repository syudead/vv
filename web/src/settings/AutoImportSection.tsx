import { t } from "../i18n";
import { PageSection } from "../ui/patterns/page-section";

import AutoImportState, { autoImportStateLine } from "./AutoImportState";
import AutoImportSwitch from "./AutoImportSwitch";
import { useAutoImportSettings } from "./useAutoImportSettings";

/**
 * AutoImportSection は 1 つの設定画面の「Auto-import」区画である。切り替え（AutoImportSwitch）と
 * 監視の状態（AutoImportState）を 1 つの区画に並べ、状態の 1 行を切り替えの行の説明に置く
 * （specs/042-folder-watch-import/ui-design.md「Settings: Auto-import section」）。
 *
 * 2 つは 1 つの読み込みを共有するので、切り替えの保存はすぐに状態の 1 行へ映る。読み直しの契機は
 * 両方が要るものを合わせる。開いたとき、メディアフォルダを変えたあと（`reloadToken`）、それに
 * 監視の状態を追う契機（ウィンドウへ戻ったとき、直近の取り込みが終わったとき、`starting` の
 * あいだ 2 秒ごと）である。
 */
export default function AutoImportSection({ reloadToken }: { reloadToken: number }) {
  const text = t.settings.autoImport;
  const source = useAutoImportSettings({ reloadToken, followWatch: true });
  const { settings } = source;

  return (
    <PageSection title={text.heading} description={text.description}>
      <AutoImportSwitch
        source={source}
        description={settings !== null ? autoImportStateLine(settings) : undefined}
      />
      <AutoImportState source={source} />
    </PageSection>
  );
}
