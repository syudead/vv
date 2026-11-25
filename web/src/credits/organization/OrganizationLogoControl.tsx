import { Camera, ImageUp, Trash2 } from "lucide-react";
import { useState } from "react";

import {
  isCreditGone,
  type Organization,
  organizationLogoSrc,
  removeOrganizationLogo,
  uploadOrganizationLogo,
} from "../../api/credits";
import { updateFavorites } from "../../api/favorites";
import { errorText, t, type UiText } from "../../i18n";
import { UPLOAD_TYPES } from "../../player/thumbnailUpload/uploadRules";
import FavoriteToggle from "../../ui/FavoriteToggle";
import { ImageUploadDialog } from "../../ui/ImageUploadDialog";
import { OrganizationLogo, OrganizationLogoMark } from "../../ui/OrganizationLogo";
import { Button } from "../../ui/shadcn/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../../ui/shadcn/dropdown-menu";
import { Spinner } from "../../ui/shadcn/spinner";
import { imageRefusal, RemoveImageDialog, withImageOf } from "../CreditImageParts";

const text = t.credits.organizationPage.logo;
const personText = t.credits.personPage;
const details = t.credits.details;

/**
 * The logo with its heart, upload and removal. An uploaded image is saved whole at once; a failed
 * change keeps the previous logo and says why under the frame. A missing organization calls onGone.
 */
export default function OrganizationLogoControl({
  organization,
  onChanged,
  onGone,
}: {
  organization: Organization;
  onChanged: (next: Organization | ((current: Organization) => Organization)) => void;
  onGone: () => void;
}) {
  const [uploading, setUploading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [failure, setFailure] = useState<UiText | null>(null);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const src = organizationLogoSrc(organization);

  async function save(file: File) {
    setFailure(null);
    setSaving(true);
    try {
      onChanged(withImageOf(await uploadOrganizationLogo(organization.id, file)));
    } catch (error) {
      if (isCreditGone(error)) onGone();
      else setFailure(details.logoFailed(errorText(error)));
    } finally {
      setSaving(false);
    }
  }

  function upload(file: File): UiText | null {
    const refusal = imageRefusal(file);
    if (refusal !== null) return refusal;
    setUploading(false);
    void save(file);
    return null;
  }

  async function toggleFavorite() {
    const favorite = !organization.favorite;
    await updateFavorites([], [], favorite, { organizationIds: [organization.id] });
    onChanged((current) => ({ ...current, favorite }));
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="group relative">
        <OrganizationLogo
          shape="profile"
          src={src}
          alt={details.logoAlt(organization.name)}
          addLabel={details.logoAdd}
          onAdd={saving ? undefined : () => setUploading(true)}
        >
          <OrganizationLogoMark corner="top-end">
            <FavoriteToggle
              favorite={organization.favorite}
              label={
                organization.favorite ? personText.favoriteOn : personText.favoriteOff
              }
              onToggle={toggleFavorite}
              variant="card"
            />
          </OrganizationLogoMark>
          <OrganizationLogoMark corner="bottom-end">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={text.change}
                  className="bg-overlay"
                  disabled={saving}
                >
                  {saving ? (
                    <Spinner aria-hidden="true" />
                  ) : (
                    <Camera aria-hidden="true" />
                  )}
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => setUploading(true)}>
                  <ImageUp aria-hidden="true" />
                  {personText.face.upload}
                </DropdownMenuItem>
                {src !== undefined && (
                  <DropdownMenuItem
                    variant="destructive"
                    onSelect={() => setConfirmRemove(true)}
                  >
                    <Trash2 aria-hidden="true" />
                    {details.logoRemove}
                  </DropdownMenuItem>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          </OrganizationLogoMark>
        </OrganizationLogo>
      </div>
      {failure !== null && <p className="text-sm text-destructive">{failure}</p>}
      <ImageUploadDialog
        open={uploading}
        onOpenChange={setUploading}
        title={details.logoUploadTitle}
        description={text.uploadHint}
        accept={UPLOAD_TYPES}
        onFile={upload}
      />
      {confirmRemove && (
        <RemoveImageDialog
          title={text.removeTitle(organization.name)}
          onRemove={async () => {
            setFailure(null);
            onChanged(withImageOf(await removeOrganizationLogo(organization.id)));
          }}
          onClose={() => setConfirmRemove(false)}
          onGone={onGone}
        />
      )}
    </div>
  );
}
