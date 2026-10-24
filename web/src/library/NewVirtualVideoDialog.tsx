import { useNavigate } from "react-router";

import { createVirtualVideo } from "../api/virtualVideos";
import { t } from "../i18n";
import NameDialog from "./NameDialog";

/** Mount while open; a created video opens on its page with `backTo` as its return target. */
export default function NewVirtualVideoDialog({
  backTo,
  onClose,
}: {
  backTo: string;
  onClose: () => void;
}) {
  const navigate = useNavigate();
  const words = t.library.newVirtualVideo;
  return (
    <NameDialog
      words={words}
      description={words.description}
      nameLabel={words.field}
      nameRule={words.rule}
      validate={(text) => (text.trim() === "" ? words.rule : null)}
      send={(title) => createVirtualVideo(title)}
      onClose={onClose}
      onDone={(video) => {
        onClose();
        void navigate(`/videos/${String(video.id)}`, { state: { from: backTo } });
      }}
    />
  );
}
