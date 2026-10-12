package domain

import (
	"errors"
	"time"
)

// サムネイルの編集のセッションの値と誤りである（specs/045-comfyui-thumbnails/data-model.md の
// Edit session、contracts/screen-api.md）。セッションは internal/app の ThumbnailEdits がメモリに
// 持ち、画面の API へはこの値で渡す。

const (
	// ThumbnailEditIdleLimit は、読まれないセッションを終えるまでの時間である（research.md R-4）。
	ThumbnailEditIdleLimit = 5 * time.Minute
	// ThumbnailEditRunLimit は、送った実行が終わらないとき timed_out で失敗させるまでの時間である
	// （research.md R-11）。
	ThumbnailEditRunLimit = 10 * time.Minute
	// ThumbnailEditMaxInstruction は指示の文の長さ（文字数）の上限である。
	ThumbnailEditMaxInstruction = 4000
	// ThumbnailEditMaxReferenceBytes は参考画像の大きさの上限である。
	ThumbnailEditMaxReferenceBytes = 20 << 20
	// ThumbnailFrameMaxSide はコマの長辺の上限（px）である（research.md R-8）。
	ThumbnailFrameMaxSide = 2048
)

var (
	// ErrThumbnailEditNotFound は、セッションが無いか終わったことを表す。API では
	// not_found（404）/ thumbnail_edit_not_found になる。
	ErrThumbnailEditNotFound = errors.New("the thumbnail edit has ended")
	// ErrThumbnailEditNotReady は、ComfyUI の接続先が保存されていないことを表す。API では
	// conflict（409）/ thumbnail_edit_not_ready になる。
	ErrThumbnailEditNotReady = errors.New("no ComfyUI connection is saved")
	// ErrThumbnailEditMaskInvalid は、マスクがコマの大きさの PNG でないことを表す。何も上げない。
	// API では invalid_request（400）/ thumbnail_edit_mask_invalid になる。
	ErrThumbnailEditMaskInvalid = errors.New("the mask is not a PNG of the frame's size")
	// ErrThumbnailEditReferenceInvalid は、参考画像が PNG・JPEG・WebP として読めないか大きすぎる
	// ことを表す。何も上げない。API では invalid_request（400）/ thumbnail_edit_reference_invalid になる。
	ErrThumbnailEditReferenceInvalid = errors.New("the reference image cannot be read")
	// ErrThumbnailEditNoResult は、セッションが succeeded でなく適用する画像が無いことを表す。
	// API では conflict（409）/ thumbnail_edit_no_result になる。
	ErrThumbnailEditNoResult = errors.New("the thumbnail edit has no result")
)

// ThumbnailFrame は編集するコマである。PNG はその画像、Width と Height は画素の大きさで、
// マスクはこの大きさで描く。
type ThumbnailFrame struct {
	PNG    []byte
	Width  int
	Height int
}

// ThumbnailEditRunInput は 1 回の実行に持ち主が渡す入力である。Mask と Reference は nil なら無い。
type ThumbnailEditRunInput struct {
	Instruction string
	Mask        []byte
	Reference   []byte
}

// ThumbnailEdit は画面へ返す編集のセッションの今の姿である（contracts/screen-api.md の
// ThumbnailEdit）。Elapsed は実行を送ってからの時間で、ready では nil である。終わった実行では
// 終わった時点で止まる。Progress は提供元が報告したときだけ、Error は failed のときだけ持つ。
// HasResult は succeeded で適用する画像があることである。
type ThumbnailEdit struct {
	ID          string
	VideoID     int64
	State       ThumbnailEditState
	FrameWidth  int
	FrameHeight int
	Elapsed     *time.Duration
	Progress    *ImageEditProgress
	Error       *ImageEditError
	HasResult   bool
}
