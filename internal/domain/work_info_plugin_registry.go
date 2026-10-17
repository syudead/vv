package domain

import (
	"errors"
	"time"
	"unicode/utf8"
)

// 所有者が追加した作品情報プラグインの行と、その設定の値の規則である。保存は internal/store、判断は internal/app が行う。

// WorkInfoPluginSettingValueMaxRunes は設定の値 1 つの長さの上限（文字数）である。
const WorkInfoPluginSettingValueMaxRunes = 2000

var (
	// ErrWorkInfoPluginChanged は承認の要求のダイジェストが今のモジュールのダイジェストと違うことを表す。
	// 所有者が見た一覧のあとにプラグインが差し替えられた。
	ErrWorkInfoPluginChanged = errors.New("the work-info plugin was replaced after it was listed")
	// ErrWorkInfoPluginUnusable はプラグインに問題があり、承認も設定の値の書き込みもできないことを表す。
	ErrWorkInfoPluginUnusable = errors.New("the work-info plugin has a problem and cannot be used")
	// ErrWorkInfoPluginSettingInvalid は、宣言されていないキーか、上限を超える値を書こうとしたことを表す。
	ErrWorkInfoPluginSettingInvalid = errors.New("the work-info plugin setting is not declared or its value is too long")
)

// WorkInfoPluginUpload はアップロードされたモジュールを検査した結果である。Problem があれば
// Module は保存せず、Manifest の宣言（Operations・Settings）は読めなかったものとして扱う。
type WorkInfoPluginUpload struct {
	// FileName はアップロードされたファイルの名前である。
	FileName string
	// Module はモジュールのバイト列である。
	Module []byte
	// Digest は Module の SHA-256 の小文字の 16 進である。
	Digest string
	// Manifest は読めたマニフェストである。Name が空なら名前を読めなかった。
	Manifest WorkInfoPluginManifest
	// Problem はプラグインを承認できない理由である。空なら問題は無い。
	Problem WorkInfoPluginProblem
}

// WorkInfoPlugin は保存された作品情報プラグインの行である。モジュールのバイト列は持たない。
// Values は設定の値を含むので、internal/app の外へはそのまま渡さない（Entry を使う）。
type WorkInfoPlugin struct {
	ID       int64
	FileName string
	// Manifest は保存したマニフェストである。Name が空なら名前を読めなかった。問題のある行の
	// Operations と Settings は空である。
	Manifest       WorkInfoPluginManifest
	Digest         string
	Problem        WorkInfoPluginProblem
	ApprovedDigest string
	AddedAt        time.Time
	UpdatedAt      time.Time
	// Generation はアップロードによる差し替えと承認の取り消しのたびに増える世代である。取得の結果は
	// 取得した時の値を持ち、反映で変わっていれば古いとする。
	Generation int64
	// Values は所有者が入力した設定の値で、キーは宣言の key である。
	Values map[string]string
	// LastRunAt は残っている直近の実行が終わった時刻で、ゼロなら残っていない
	//
	LastRunAt time.Time
}

// State はプラグインの状態を返す。
func (p WorkInfoPlugin) State() WorkInfoPluginState {
	return WorkInfoPluginStateOf(p.Problem, p.Digest, p.ApprovedDigest)
}

// WorkInfoPluginSettingEntry は一覧に載せる設定の 1 つである。秘密の設定の値は持たず、
// 値があるかだけを Set で表す。
type WorkInfoPluginSettingEntry struct {
	WorkInfoPluginSetting
	// Set は値が保存されているかである。
	Set bool
	// Value は保存された値である。Secret のときは常に空である。
	Value string
	// StaleValue は保存された値が今の選択肢の外にあるときのその値で、それ以外は空である。実行は代わりに
	// 既定値を受け取る。
	StaleValue string
}

// WorkInfoPluginEntry は画面に見せるプラグインである。秘密の設定の値は含まない。
type WorkInfoPluginEntry struct {
	ID          int64
	Name        string
	FileName    string
	Version     string
	Description string
	Operations  []WorkInfoOperation
	Digest      string
	State       WorkInfoPluginState
	Problem     WorkInfoPluginProblem
	Settings    []WorkInfoPluginSettingEntry
	UpdatedAt   time.Time
	// LastRunAt は残っている直近の実行が終わった時刻で、ゼロなら残っていない。
	LastRunAt time.Time
}

// Entry は画面に見せる形を返す。設定は宣言の順に並べ、秘密の設定の値を落とす。
func (p WorkInfoPlugin) Entry() WorkInfoPluginEntry {
	settings := make([]WorkInfoPluginSettingEntry, 0, len(p.Manifest.Settings))
	for _, s := range p.Manifest.Settings {
		value, set := p.Values[s.Key]
		entry := WorkInfoPluginSettingEntry{WorkInfoPluginSetting: s, Set: set}
		if !s.Secret {
			entry.Value = value
		}
		if _, stale := EffectiveWorkInfoPluginValue(s, p.Values); stale {
			entry.StaleValue = value
		}
		settings = append(settings, entry)
	}
	operations := p.Manifest.Operations
	if operations == nil {
		operations = []WorkInfoOperation{}
	}
	return WorkInfoPluginEntry{
		ID:          p.ID,
		Name:        p.Manifest.Name,
		FileName:    p.FileName,
		Version:     p.Manifest.Version,
		Description: p.Manifest.Description,
		Operations:  operations,
		Digest:      p.Digest,
		State:       p.State(),
		Problem:     p.Problem,
		Settings:    settings,
		UpdatedAt:   p.UpdatedAt,
		LastRunAt:   p.LastRunAt,
	}
}

// CheckWorkInfoPluginValues は、設定の値の書き込み values が宣言 settings に対して書けるかを確かめる。
// キーはどれも宣言にあり、nil でない値は WorkInfoPluginSettingValueMaxRunes 文字以下の正しい UTF-8 で
// なければならない。nil の値はそのキーの値を消すことを表す。外れれば ErrWorkInfoPluginSettingInvalid を返す。
func CheckWorkInfoPluginValues(settings []WorkInfoPluginSetting, values map[string]*string) error {
	declared := make(map[string]bool, len(settings))
	for _, s := range settings {
		declared[s.Key] = true
	}
	for key, value := range values {
		if !declared[key] {
			return ErrWorkInfoPluginSettingInvalid
		}
		if value != nil && (!utf8.ValidString(*value) || utf8.RuneCountInString(*value) > WorkInfoPluginSettingValueMaxRunes) {
			return ErrWorkInfoPluginSettingInvalid
		}
	}
	return nil
}
