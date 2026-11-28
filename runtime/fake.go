package runtime

// このファイルは、テストのにせもの（plugins/go/spec/go.hozo の Requirement.Fakes）が使う型。基底 go が、テストのパッケージに出す、にせものの
// 型のフィールドになる。testing は import しない。

// Fake は、値を返すメソッド一つのにせもの: 呼ばれた引数を覚え、言われた値か、失敗を返す。A は、覚える引数、R は、返す値の型。
// 零値のまま、使える。同時に呼ばれることは、守らない。
type Fake[A, R any] struct {
	// Calls は、呼ばれた引数（呼ばれた順）。
	Calls []A

	set   bool
	value R
	err   error
	at    map[int]fakeResult[R] // 何回目の呼び出しか（1 から）→ その回だけの結果
}

// fakeResult は、呼び出し一回の結果。
type fakeResult[R any] struct {
	value R
	err   error
}

// Returns は、これからの呼び出しが、v を返すようにする（本物は、呼ばない）。
func (f *Fake[A, R]) Returns(v R) { f.set, f.value, f.err = true, v, nil }

// Throws は、これからの呼び出しが、R の零値と、err を返すようにする（本物は、呼ばない）。
func (f *Fake[A, R]) Throws(err error) {
	var zero R
	f.set, f.value, f.err = true, zero, err
}

// ReturnsAt は、n 回目（1 から数える）の呼び出しだけが、v を返すようにする（その回は、本物を呼ばない）。ほかの回は、変えない。
func (f *Fake[A, R]) ReturnsAt(n int, v R) { f.setAt(n, fakeResult[R]{value: v}) }

// ThrowsAt は、n 回目（1 から数える）の呼び出しだけが、R の零値と、err を返すようにする（その回は、本物を呼ばない）。
func (f *Fake[A, R]) ThrowsAt(n int, err error) { f.setAt(n, fakeResult[R]{err: err}) }

func (f *Fake[A, R]) setAt(n int, r fakeResult[R]) {
	if f.at == nil {
		f.at = map[int]fakeResult[R]{}
	}
	f.at[n] = r
}

// Call は、生成したコードが呼ぶ: args を Calls に足し、その回に ReturnsAt か ThrowsAt が言われていれば、そのとおりに返す。なければ、
// Returns か Throws が言われていれば、そのとおりに返す。どれも言われていなくて、impl が nil でなければ、impl（本物）を呼ぶ。nil なら、
// R の零値を返す（失敗は、ない）。
func (f *Fake[A, R]) Call(args A, impl func() (R, error)) (R, error) {
	f.Calls = append(f.Calls, args)
	if r, ok := f.at[len(f.Calls)]; ok {
		return r.value, r.err
	}
	if f.set || impl == nil {
		return f.value, f.err
	}
	return impl()
}

// FakeVoid は、値を返さないメソッド一つのにせもの: 呼ばれた引数を覚え、言われれば、失敗を返す。A は、覚える引数。
// 零値のまま、使える。同時に呼ばれることは、守らない。
type FakeVoid[A any] struct {
	// Calls は、呼ばれた引数（呼ばれた順）。
	Calls []A

	set bool
	err error
	at  map[int]error // 何回目の呼び出しか（1 から）→ その回だけの結果（nil は、正常に終わる）
}

// Returns は、これからの呼び出しが、何もせずに、正常に終わるようにする（本物は、呼ばない）。
func (f *FakeVoid[A]) Returns() { f.set, f.err = true, nil }

// Throws は、これからの呼び出しが、err を返すようにする（本物は、呼ばない）。
func (f *FakeVoid[A]) Throws(err error) { f.set, f.err = true, err }

// ReturnsAt は、n 回目（1 から数える）の呼び出しだけが、何もせずに、正常に終わるようにする（その回は、本物を呼ばない）。
func (f *FakeVoid[A]) ReturnsAt(n int) { f.setAt(n, nil) }

// ThrowsAt は、n 回目（1 から数える）の呼び出しだけが、err を返すようにする（その回は、本物を呼ばない）。
func (f *FakeVoid[A]) ThrowsAt(n int, err error) { f.setAt(n, err) }

func (f *FakeVoid[A]) setAt(n int, err error) {
	if f.at == nil {
		f.at = map[int]error{}
	}
	f.at[n] = err
}

// Call は、生成したコードが呼ぶ: args を Calls に足し、その回に ReturnsAt か ThrowsAt が言われていれば、そのとおりに返す。なければ、
// Returns か Throws が言われていれば、そのとおりに返す。どれも言われていなくて、impl が nil でなければ、impl（本物）を呼ぶ。nil なら、
// nil を返す。
func (f *FakeVoid[A]) Call(args A, impl func() error) error {
	f.Calls = append(f.Calls, args)
	if err, ok := f.at[len(f.Calls)]; ok {
		return err
	}
	if f.set || impl == nil {
		return f.err
	}
	return impl()
}
