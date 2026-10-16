package domain

// 作品情報プラグインが返すカバー画像である。プラグインは workInfo.cover に画像の URL を返し、vv が
// プラグインの要求と同じアドレスの規則で 1 回だけ取り、取得の結果と一緒にメモリに持つ。反映で選べば、
// 手元の画像のアップロードと同じ確かめと適用で動画の代表サムネイルにする
// （docs/design-docs/work-info-plugins.md のカバー画像の節）。

const (
	// WorkInfoCoverMaxBytes はカバー画像の本文の上限である。プラグインの 1 つの応答の本文の上限と同じである。
	WorkInfoCoverMaxBytes = WorkInfoPluginHTTPMaxBodyBytes
	// WorkInfoFetchCoverBudget は、メモリに持つ取得の結果のカバー画像が合わせて占めてよいバイト数である。
	// 越えれば古い結果から捨てる。
	WorkInfoFetchCoverBudget = 8 * WorkInfoCoverMaxBytes
	// WorkInfoFieldCover はカバー画像で、動画の代表サムネイルにする。
	WorkInfoFieldCover WorkInfoField = "cover"
)

// WorkInfoCover は取得したカバー画像である。Data は PNG・JPEG・静止画の WebP の本文で、ContentType は
// その形式の MIME タイプ、Width と Height は見出しの画素数である。
type WorkInfoCover struct {
	Data        []byte
	ContentType string
	Width       int
	Height      int
}

// workInfoCoverURL はプラグインが返したカバー画像の URL を、http か https の、WorkInfoFetchURLMaxRunes 文字
// 以下の絶対 URL のときだけ返す。外れれば空で、カバー画像は無いものとする。
func workInfoCoverURL(raw string) string {
	if CheckWorkInfoFetchURL(raw) != nil {
		return ""
	}
	return raw
}
