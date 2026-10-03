package desktop

import (
	"fmt"
	"time"
)

// 二重起動の判定の手順（specs/037-windows-app/research.md R-6、contracts/windows-app.md §2）。
// OS の呼び出しは instanceOS の裏にあり、Windows の実装は instance_windows.go にある。
// 手順だけをここに置くので、プロセスの割り込みの順をどの OS でも試験できる。

// InstanceState は AcquireInstance の結果である。
type InstanceState int

const (
	// InstanceAcquired はこのプロセスが唯一の VVMDM として起動してよいことを表す。
	InstanceAcquired InstanceState = iota
	// InstanceActivated は同じセッションの既存のウィンドウを前面に出したことを表す。
	// このプロセスは何も示さずに終わる。
	InstanceActivated
	// InstanceOtherSession は同じ利用者が別のセッションで起動中であることを表す。
	InstanceOtherSession
	// InstanceStillRunning は同じセッションの VVMDM が、待つ間にウィンドウを出さず
	// 終わりもしなかったことを表す。
	InstanceStillRunning
)

// instancePoll は待つ間に既存のウィンドウとミューテックスを確かめ直す間隔である。
const instancePoll = 200 * time.Millisecond

// instanceOS は二重起動の判定に使う OS の呼び出しである。
type instanceOS interface {
	// CreateMutex は名前付きミューテックスを作るか開く。既にあったら existed は真で、
	// owner を求めても持ち主にはならない。
	CreateMutex(name string, owner bool) (handle uintptr, existed bool, err error)
	// MutexExists は名前付きミューテックスが今あるかを返す。
	MutexExists(name string) bool
	// Close はハンドルを閉じる。
	Close(handle uintptr)
	// Wait はミューテックスを最大 d 待ち、取れたら真を返す。前の持ち主が放さずに
	// 終わったときも取れたとする。
	Wait(handle uintptr, d time.Duration) (acquired bool, err error)
	// ActivateWindow は同じセッションの VVMDM のウィンドウが表に出ていれば前面に
	// 出し、真を返す。
	ActivateWindow() bool
}

// acquireInstance は二重起動を判定し、取れたらプロセスの終わりまで持つハンドルを
// 返す。
//
// どのプロセスも local を先に作ってから global を作る。だから global があるなら、
// その持ち主は既に local を持っている。持ち主が同じセッションにいれば、自分の
// local を閉じたあとも local は残り、別のセッションにいれば残らない。持ち主が
// global と local の間で止まっていても、同じセッションを別のセッションと取り違え
// ない。
func acquireInstance(sys instanceOS, globalName, localName string, wait time.Duration) (InstanceState, []uintptr, error) {
	local, _, err := sys.CreateMutex(localName, false)
	if err != nil {
		return 0, nil, err
	}
	global, existed, err := sys.CreateMutex(globalName, true)
	if err != nil {
		sys.Close(local)
		return 0, nil, err
	}
	if !existed {
		return InstanceAcquired, []uintptr{local, global}, nil
	}

	sys.Close(local)
	if !sys.MutexExists(localName) {
		sys.Close(global)
		return InstanceOtherSession, nil, nil
	}

	// 待つ間も local を持つ。global が取れた時点で local を持っているように、
	// また、待つ間に来た同じセッションの 3 つ目が同じセッションと分かるように。
	local, _, err = sys.CreateMutex(localName, false)
	if err != nil {
		sys.Close(global)
		return 0, nil, err
	}
	release := func() {
		sys.Close(global)
		sys.Close(local)
	}
	deadline := time.Now().Add(wait)
	for {
		if sys.ActivateWindow() {
			release()
			return InstanceActivated, nil, nil
		}
		acquired, err := sys.Wait(global, instancePoll)
		if err != nil {
			release()
			return 0, nil, fmt.Errorf("cannot wait for the running VVMDM: %w", err)
		}
		if acquired {
			return InstanceAcquired, []uintptr{local, global}, nil
		}
		if time.Now().After(deadline) {
			release()
			return InstanceStillRunning, nil, nil
		}
	}
}
