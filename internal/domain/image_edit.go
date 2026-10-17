package domain

import "fmt"

// 画像編集の 1 回の実行の値である。サムネイルの編集のセッション（internal/app）と、
// 提供元のアダプタ（internal/comfyui）の間で受け渡す
//

// ThumbnailEditState はサムネイルの編集のセッションの状態である。
// 提供元の状態はアダプタが queued から cancelled までのどれかに対応させる
type ThumbnailEditState string

const (
	// ThumbnailEditReady はコマを取り出し、まだ実行していない状態である。
	ThumbnailEditReady ThumbnailEditState = "ready"
	// ThumbnailEditQueued は提供元の待ち行列にある状態である。
	ThumbnailEditQueued ThumbnailEditState = "queued"
	// ThumbnailEditPreparing は提供元が実行の用意をしている状態である。Comfy Cloud だけが報告する。
	ThumbnailEditPreparing ThumbnailEditState = "preparing"
	// ThumbnailEditRunning は提供元が実行している状態である。
	ThumbnailEditRunning ThumbnailEditState = "running"
	// ThumbnailEditSucceeded は実行が終わり、出力の画像がある状態である。
	ThumbnailEditSucceeded ThumbnailEditState = "succeeded"
	// ThumbnailEditFailed は実行が失敗した状態で、ImageEditFailure の理由を 1 つ持つ。
	ThumbnailEditFailed ThumbnailEditState = "failed"
	// ThumbnailEditCancelled は実行が中止された状態である。
	ThumbnailEditCancelled ThumbnailEditState = "cancelled"
)

// Terminal は実行がこれ以上進まない状態かを返す。
func (s ThumbnailEditState) Terminal() bool {
	return s == ThumbnailEditSucceeded || s == ThumbnailEditFailed || s == ThumbnailEditCancelled
}

// ImageEditFailure は画像編集が失敗した理由である。
type ImageEditFailure string

const (
	// ImageEditUnreachable は接続できない、名前が引けない、または 5xx が返ったことを表す。
	ImageEditUnreachable ImageEditFailure = "unreachable"
	// ImageEditUnauthorized は Comfy Cloud が 401 か 403 を返したことを表す。
	ImageEditUnauthorized ImageEditFailure = "unauthorized"
	// ImageEditInsufficientCredits は Comfy Cloud が 402 を返したか、誤りの種類がクレジットを名指したことを表す。
	ImageEditInsufficientCredits ImageEditFailure = "insufficient_credits"
	// ImageEditMissingModels は送ったワークフローのノードの型かモデルファイルが提供元に無いことを表す。
	ImageEditMissingModels ImageEditFailure = "missing_models"
	// ImageEditWorkflowRejected は POST /prompt がワークフローを 400 で拒んだことを表す。
	ImageEditWorkflowRejected ImageEditFailure = "workflow_rejected"
	// ImageEditRunFailed は提供元が実行の失敗を報告したことを表す。
	ImageEditRunFailed ImageEditFailure = "run_failed"
	// ImageEditNoOutput は実行は成功したが vv:output に画像が無いことを表す。
	ImageEditNoOutput ImageEditFailure = "no_output"
	// ImageEditTimedOut は送ってから 10 分たっても終わらなかったことを表す。数えるのは編集のセッションである。
	ImageEditTimedOut ImageEditFailure = "timed_out"
)

// ImageEditError は画像編集の失敗である。Message は提供元が渡した文（ノードの誤り、実行の誤り、
// 足りない名前）か、vv が述べた原因で、空のこともある。提供元の応答の本文そのものは入れない。
type ImageEditError struct {
	Reason  ImageEditFailure
	Message string
}

// Error は理由と文を述べる。
func (e *ImageEditError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("the image edit failed (%s)", e.Reason)
	}
	return fmt.Sprintf("the image edit failed (%s): %s", e.Reason, e.Message)
}

// ImageEditInput は 1 回の実行に渡す入力である。
type ImageEditInput struct {
	// Frame は編集するコマの PNG である。
	Frame []byte
	// Mask は塗った範囲の PNG で、コマと同じ大きさ、塗った画素が不透明な白、残りが不透明な黒である。
	// nil なら塗っていない。アダプタはコマの大きさの真っ白なマスクを送る。
	Mask []byte
	// Reference は参考画像である。nil なら参考画像の無いワークフローを使う。
	Reference []byte
	// Instruction は持ち主の指示の文である。空でマスクを塗っていれば、アダプタが既定の指示を送る。
	Instruction string
}

// ImageEditJob は提供元に送った 1 回の実行である。状態の読み取り、中止、出力の取得に渡す。
// Connection は送ったときの接続先で、実行の途中で設定が変わってもこの値で問い合わせる。
// ID と Output は提供元の識別子（ComfyUI の prompt_id と、出力のノードの id）である。
type ImageEditJob struct {
	Connection ComfyUISettings
	ID         string
	Output     string
}

// ImageEditProgress は実行のステップの進み具合で、Max ステップのうち Value まで進んだことを表す。
// 提供元が報告したときだけあり、実行の状態は決めない。
type ImageEditProgress struct {
	Value int
	Max   int
}
