package app

import (
	"context"
	"crypto/rand"
	"errors"
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
	// undoMaxBytes は持っている像の合計のバイト数の上限である。像はグループの全項目を前後の 2 回
	// 持つので、大きなグループを続けて動かすと件数の上限だけではメモリを抑えられない。超えたら古い
	// ものから忘れる。いちばん新しい像はこの上限より大きくても持つ（その操作の取り消しを残すため）。
	undoMaxBytes = 64 << 20
	// undoTokenBytes はトークンの乱数のバイト数である。
	undoTokenBytes = 16
)

// OrganizeStore はドラッグの操作の保存先である。internal/store の *OrganizeStore がこれを満たす。
// 操作は取り消しの像を返し、Undo はその像の前の状態へ戻す。
type OrganizeStore interface {
	// Reorder はグループ groupID の項目 itemIDs を、その順のまままとめて項目 afterItemID の
	// すぐ後ろへ移す（nil なら先頭）。グループや項目が無ければ domain.ErrGroupTargetNotFound。
	Reorder(ctx context.Context, groupID int64, itemIDs []int64, afterItemID *int64) ([]byte, error)
	// Move はカードを move.To へ足し、move.From から外す。From・To・カードが無いかカードが From に
	// 無ければ domain.ErrGroupTargetNotFound、グループの入れ子が循環すれば domain.ErrGroupCycle。
	Move(ctx context.Context, move domain.OrganizeMove) ([]byte, error)
	// Bundle は動画 videoIDs を落とした先の動画 targetID と束ね、targetID を代表にする。動画が無ければ
	// domain.ErrGroupTargetNotFound、集まりを展開して多すぎれば domain.ErrTooManyVersions。
	Bundle(ctx context.Context, videoIDs []int64, targetID int64) ([]byte, error)
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
// 1 回だけ使え、10 分で切れ、同時に持つのは 100 件かつ像の合計 undoMaxBytes までである。
type Organizer struct {
	store  OrganizeStore
	now    func() time.Time
	random io.Reader

	mu sync.Mutex
	// entries は持っているトークンで、作った順に並ぶ。
	entries []undoEntry
	// bytes は entries の像の合計のバイト数である。
	bytes int
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
// トークンは保存先の操作より先に作る。乱数が読めなければ何も書かずに失敗し、並びが変わったのに
// トークンを返せない、ということが起きない。
func (o *Organizer) Reorder(ctx context.Context, groupID int64, itemIDs []int64, afterItemID *int64) (domain.OrganizeResult, error) {
	return o.run(func() ([]byte, error) { return o.store.Reorder(ctx, groupID, itemIDs, afterItemID) })
}

// Move はカードをグループへ入れるか、グループの間で動かすか、グループから外し、取り消しの
// トークンを返す。
func (o *Organizer) Move(ctx context.Context, move domain.OrganizeMove) (domain.OrganizeResult, error) {
	return o.run(func() ([]byte, error) { return o.store.Move(ctx, move) })
}

// Bundle は動画を落とした先の動画を代表にして束ね、取り消しのトークンを返す。
func (o *Organizer) Bundle(ctx context.Context, videoIDs []int64, targetID int64) (domain.OrganizeResult, error) {
	return o.run(func() ([]byte, error) { return o.store.Bundle(ctx, videoIDs, targetID) })
}

// run は保存先の操作 operation を行い、返した像をトークンで持つ。トークンは操作より先に作る
// （Reorder）。
func (o *Organizer) run(operation func() ([]byte, error)) (domain.OrganizeResult, error) {
	token, err := o.newToken()
	if err != nil {
		return domain.OrganizeResult{}, err
	}
	image, err := operation()
	if err != nil {
		return domain.OrganizeResult{}, err
	}
	o.keep(undoEntry{token: token, image: image, created: o.now()})
	return domain.OrganizeResult{UndoToken: token}, nil
}

// Undo はトークン token の操作の前の状態へ戻す。トークンが無い、使い済み、または 10 分より古ければ
// domain.ErrUndoExpired。取り消しが済むか、行が変わっていて保存先が domain.ErrUndoConflict を
// 返せば、トークンは使い済みになる。それ以外の誤り（取引の中断など）では何も書かれていないので、
// トークンを戻して同じトークンでやり直せるようにする。使っている間は持っていないので、同じ
// トークンの 2 つ目の取り消しは ErrUndoExpired である。
func (o *Organizer) Undo(ctx context.Context, token string) error {
	entry, ok := o.take(token)
	if !ok {
		return domain.ErrUndoExpired
	}
	err := o.store.Undo(ctx, entry.image)
	if err != nil && !errors.Is(err, domain.ErrUndoConflict) {
		o.keep(entry)
	}
	return err
}

// newToken は新しいトークンを作る。
func (o *Organizer) newToken() (string, error) {
	raw := make([]byte, undoTokenBytes)
	if _, err := io.ReadFull(o.random, raw); err != nil {
		return "", fmt.Errorf("cannot create an undo token: %w", err)
	}
	return sessionTokenEncoding.EncodeToString(raw), nil
}

// keep は entry を作った順の位置に持つ。切れたものを忘れ、件数かバイト数の上限を超えれば
// いちばん古いものから忘れる。いちばん新しいものは忘れない。
func (o *Organizer) keep(entry undoEntry) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.forgetExpired()
	i := slices.IndexFunc(o.entries, func(e undoEntry) bool { return e.created.After(entry.created) })
	if i < 0 {
		i = len(o.entries)
	}
	o.entries = slices.Insert(o.entries, i, entry)
	o.bytes += len(entry.image)
	for len(o.entries) > 1 && (len(o.entries) > undoCapacity || o.bytes > undoMaxBytes) {
		o.bytes -= len(o.entries[0].image)
		o.entries = slices.Delete(o.entries, 0, 1)
	}
}

// take はトークン token の項目を取り出して忘れる。無いか切れていれば false。
func (o *Organizer) take(token string) (undoEntry, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.forgetExpired()
	i := slices.IndexFunc(o.entries, func(e undoEntry) bool { return e.token == token })
	if i < 0 {
		return undoEntry{}, false
	}
	entry := o.entries[i]
	o.bytes -= len(entry.image)
	o.entries = slices.Delete(o.entries, i, i+1)
	return entry, true
}

// forgetExpired は 10 分より古いトークンを忘れる。呼び出し側が mu を持つ。
func (o *Organizer) forgetExpired() {
	now := o.now()
	o.entries = slices.DeleteFunc(o.entries, func(e undoEntry) bool {
		expired := !now.Before(e.created.Add(undoLifetime))
		if expired {
			o.bytes -= len(e.image)
		}
		return expired
	})
}
