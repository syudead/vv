import { Sparkles } from "lucide-react";
import { useState } from "react";

import { autoTagVideo } from "../api/client";
import { errorText, t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import { useToast } from "../ui/Toast";

/**
 * AutoTagButton は動画ページで、この動画を自動タグ付けの判定に回す操作である
 * （docs/design-docs/auto-tagging.md）。判定は裏で進み、タグが付くと動画の知らせで
 * ページが読み直すので、ここでは積んだことだけを知らせる。
 */
export default function AutoTagButton({ videoId }: { videoId: number }) {
  const text = t.player.autoTag;
  const toast = useToast();
  const [pending, setPending] = useState(false);

  const run = async () => {
    if (pending) return;
    setPending(true);
    try {
      const queued = await autoTagVideo(videoId);
      toast(queued ? text.queued : text.alreadyRunning);
    } catch (failure) {
      toast(text.failed(errorText(failure)));
    } finally {
      setPending(false);
    }
  };

  return (
    <div>
      <Button
        size="sm"
        variant="ghost"
        onClick={() => void run()}
        aria-disabled={pending || undefined}
      >
        <Sparkles aria-hidden="true" />
        {text.run}
      </Button>
    </div>
  );
}
