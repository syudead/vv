import type { ReactNode } from "react";

import { t } from "../i18n";
import Components from "./Components";
import Foundations from "./Foundations";
import OverlayComponents from "./OverlayComponents";
import Patterns from "./Patterns";

const sections: { id: string; title: () => string; body?: () => ReactNode }[] = [
  {
    id: "foundations",
    title: () => t.designSystem.foundations,
    body: () => <Foundations />,
  },
  {
    id: "components",
    title: () => t.designSystem.components,
    body: () => (
      <div className="grid gap-8">
        <Components />
        <OverlayComponents />
      </div>
    ),
  },
  {
    id: "patterns",
    title: () => t.designSystem.patterns,
    body: () => <Patterns />,
  },
];

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
          {section.body?.()}
        </section>
      ))}
    </main>
  );
}
