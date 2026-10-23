import { SquareDashed } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";

import { createVirtualVideo } from "../api/virtualVideos";
import { t, type UiText } from "../i18n";
import { Button } from "../ui/shadcn/button";
import NameDialog from "./NameDialog";

const words = t.library.newVirtualVideo;

function validateTitle(text: string): UiText | null {
  return text.trim() === "" ? words.rule : null;
}

/** Owner-only header action; a created video opens on its page with the list as its return target. */
export default function NewVirtualVideoButton({ backTo }: { backTo: string }) {
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
        <SquareDashed aria-hidden="true" />
        {words.action}
      </Button>
      {open && (
        <NameDialog
          words={words}
          description={words.description}
          nameLabel={words.field}
          nameRule={words.rule}
          validate={validateTitle}
          send={(title) => createVirtualVideo(title)}
          onClose={() => setOpen(false)}
          onDone={(video) => {
            setOpen(false);
            void navigate(`/videos/${String(video.id)}`, { state: { from: backTo } });
          }}
        />
      )}
    </>
  );
}
