import { SquareDashed } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";

import { t } from "../i18n";
import { Button } from "../ui/shadcn/button";
import NewVirtualVideoDialog from "./NewVirtualVideoDialog";

const words = t.library.newVirtualVideo;

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
        <NewVirtualVideoDialog
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
