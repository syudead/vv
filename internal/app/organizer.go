package app

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// ドラッグの操作と取り消しの台帳（specs/050-drag-organize/research.md R-1・R-2）。操作は保存先が
// 1 つの取引で行い、変えた行の前後の像をバイト列で返す。Organizer はその像を乱数のトークンで
// メモリに持ち、取り消しで保存先へ返す。像の中身は読まない。

const (
	// undoLifetime は取り消しのトークンの有効な長さである（R-2）。
	undoLifetime = 10 * time.Minute
	// undoCapacity は同時に持つトークンの数の上限である。超えたら古いものから忘れる。
	undoCapacity = 100
	// undoTokenBytes はトークンの乱数のバイト数である。
	undoTokenBytes = 16
)

// OrganizeStore はドラッグの操作の保存先である。internal/store の *OrganizeStore がこれを満たす。
// 操作は取り消しの像を返し、Undo はその像の前の状態へ戻す。
type OrganizeStore interface {
	// Reorder はグループ groupID の項目 itemIDs を、その順のまままとめて項目 afterItemID の
	// すぐ後ろへ移す（nil なら先頭）。グループや項目が無ければ domain.ErrGroupTargetNotFound。
	Reorder(ctx context.Context, groupID int64, itemIDs []int64, afterItemID *int64) ([]byte, error)
	// Undo は像 image の前の状態へ戻す。行が変わっていれば domain.ErrUndoConflict で、何も書かない。
	Undo(ctx context.Context, image []byte) error
}

// OrganizerOptions は NewOrganizer の入力である。
type OrganizerOptions struct {
	Store OrganizeStore
	// Now は今の時刻の元である。nil なら time.Now を使う。
	Now func() time.Time
	// Random はトークンの乱数の元である。nil なら crypto/rand.Reader を使う。
	Random io.Reader
}

// Organizer はドラッグの操作を保存先へ渡し、その取り消しの像をトークンで持つ（R-2）。トークンは
// 1 回だけ使え、10 分で切れ、同時に持つのは 100 件までである。
type Organizer struct {
	store  OrganizeStore
	now    func() time.Time
	random io.Reader

	mu sync.Mutex
	// entries は持っているトークンで、作った順に並ぶ。
	entries []undoEntry
}

// undoEntry はトークン 1 つと、その操作の像である。
type undoEntry struct {
	token   string
	image   []byte
	created time.Time
}

// NewOrganizer は Organizer を作る。
func NewOrganizer(opts OrganizerOptions) *Organizer {
	o := &Organizer{store: opts.Store, now: opts.Now, random: opts.Random}
	if o.now == nil {
		o.now = time.Now
	}
	if o.random == nil {
		o.random = rand.Reader
	}
	return o
}

// Reorder はグループの項目をまとめて並べ替え、取り消しのトークンを返す。
func (o *Organizer) Reorder(ctx context.Context, groupID int64, itemIDs []int64, afterItemID *int64) (domain.OrganizeResult, error) {
	image, err := o.store.Reorder(ctx, groupID, itemIDs, afterItemID)
	if err != nil {
		return domain.OrganizeResult{}, err
	}
	token, err := o.keep(image)
	if err != nil {
		return domain.OrganizeResult{}, err
	}
	return domain.OrganizeResult{UndoToken: token}, nil
}

// Undo はトークン token の操作の前の状態へ戻す。トークンが無い、使い済み、または 10 分より古ければ
// domain.ErrUndoExpired。トークンは結果にかかわらずこの 1 回で使い済みになる。行が変わっていれば
// 保存先の domain.ErrUndoConflict をそのまま返す。
func (o *Organizer) Undo(ctx context.Context, token string) error {
	image, ok := o.take(token)
	if !ok {
		return domain.ErrUndoExpired
	}
	return o.store.Undo(ctx, image)
}

// keep は像 image を新しいトークンで持ち、そのトークンを返す。切れたものを忘れ、上限を超えれば
// いちばん古いものを忘れる。
func (o *Organizer) keep(image []byte) (string, error) {
	raw := make([]byte, undoTokenBytes)
	if _, err := io.ReadFull(o.random, raw); err != nil {
		return "", fmt.Errorf("cannot create an undo token: %w", err)
	}
	token := sessionTokenEncoding.EncodeToString(raw)
	o.mu.Lock()
	defer o.mu.Unlock()
	o.forgetExpired()
	o.entries = append(o.entries, undoEntry{token: token, image: image, created: o.now()})
	if extra := len(o.entries) - undoCapacity; extra > 0 {
		o.entries = slices.Delete(o.entries, 0, extra)
	}
	return token, nil
}

// take はトークン token の像を取り出して忘れる。無いか切れていれば false。
func (o *Organizer) take(token string) ([]byte, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.forgetExpired()
	i := slices.IndexFunc(o.entries, func(e undoEntry) bool { return e.token == token })
	if i < 0 {
		return nil, false
	}
	image := o.entries[i].image
	o.entries = slices.Delete(o.entries, i, i+1)
	return image, true
}

// forgetExpired は 10 分より古いトークンを忘れる。呼び出し側が mu を持つ。
func (o *Organizer) forgetExpired() {
	now := o.now()
	o.entries = slices.DeleteFunc(o.entries, func(e undoEntry) bool { return !now.Before(e.created.Add(undoLifetime)) })
}
