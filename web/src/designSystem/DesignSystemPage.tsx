import { t } from "../i18n";

const sections = [
  { id: "foundations", title: () => t.designSystem.foundations },
  { id: "components", title: () => t.designSystem.components },
  { id: "patterns", title: () => t.designSystem.patterns },
] as const;

/**
 * DesignSystemPage は開発時だけの見本で、デザインシステムの段階ごとの確認に使う
 * （specs/038-design-system/research.md R-1、R-2）。各段階の実装が自分の節を埋める。
 */
export default function DesignSystemPage() {
  return (
    <main data-showcase="design-system" className="mx-auto grid max-w-6xl gap-8 p-6">
      <h1 className="text-xl font-semibold">{t.designSystem.title}</h1>
      {sections.map((section) => (
        <section key={section.id} id={section.id} className="grid gap-4">
          <h2 className="text-lg font-semibold">{section.title()}</h2>
        </section>
      ))}
    </main>
  );
}
