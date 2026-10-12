/**
 * UPLOAD_MAX_BYTES はサムネイルにアップロードできる画像の上限（64 MiB）で、サーバーの
 * `image` の上限と同じである（specs/046-thumbnail-image-upload/contracts/screen-api.md）。
 */
export const UPLOAD_MAX_BYTES = 64 * 1024 * 1024;

/** UPLOAD_TYPES はファイルピッカーに渡す、送れる画像の種類である。 */
export const UPLOAD_TYPES = "image/png,image/jpeg,image/webp";

const uploadTypes = new Set(UPLOAD_TYPES.split(","));

/**
 * uploadRefusal は選んだファイルを送る前に断る理由を返す。種類が PNG・JPEG・WebP でなければ
 * `type`、64 MiB を超えれば `size`、送れるなら `null` である。サーバーの検査が最終で、ここを
 * 通っても読めない画像や画素数の多すぎる画像はサーバーの理由で失敗する
 * （specs/046-thumbnail-image-upload/ui-design.md「Video page: upload entry」）。
 */
export function uploadRefusal(file: Pick<File, "type" | "size">): "type" | "size" | null {
  if (!uploadTypes.has(file.type)) return "type";
  if (file.size > UPLOAD_MAX_BYTES) return "size";
  return null;
}
