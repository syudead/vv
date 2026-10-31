import { SendHorizontal, X } from "lucide-react";
import { useState } from "react";

import { t } from "@/i18n";
import { SidePanel } from "@/ui/patterns/side-panel";
import { Button } from "@/ui/shadcn/button";
import { Textarea } from "@/ui/shadcn/textarea";

// registry:block side-panel-example. Real panels keep their open state above the routes; see
// web/registry/rules/patterns.md, Side panel.

export function SidePanelExample() {
  const p = t.designSystem.pattern;
  const [open, setOpen] = useState(false);
  const close = () => setOpen(false);
  return (
    <div className="flex flex-wrap gap-2 p-4">
      <Button
        variant="outline"
        size="sm"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        {p.sidePanel.open}
      </Button>
      {open && (
        <SidePanel
          label={p.sidePanel.title}
          title={p.sidePanel.title}
          onClose={close}
          actions={
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t.common.close}
              onClick={close}
            >
              <X aria-hidden="true" />
            </Button>
          }
          footer={
            <div className="flex items-end gap-2">
              <Textarea
                aria-label={p.sidePanel.field}
                placeholder={p.sidePanel.field}
                className="max-h-assistant-composer min-h-8 resize-none py-1.5"
              />
              <Button size="icon-sm" aria-label={p.sidePanel.send}>
                <SendHorizontal aria-hidden="true" />
              </Button>
            </div>
          }
        >
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <p className="max-w-5/6 self-end rounded-lg bg-secondary px-3 py-2 text-sm text-secondary-foreground">
                {p.sidePanel.request}
              </p>
              <p className="text-sm">{p.sidePanel.reply}</p>
            </div>
          </div>
        </SidePanel>
      )}
    </div>
  );
}
