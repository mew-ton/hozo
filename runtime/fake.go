package runtime

// このファイルは、テストのにせもの（docs/plugin-go.md「テストのにせもの」）が使う型。基底 go が、テストのパッケージに出す、にせものの
// 型のフィールドになる。testing は import しない。

// Fake は、値を返すメソッド一つのにせもの: 呼ばれた引数を覚え、言われた値か、失敗を返す。A は、覚える引数、R は、返す値の型。
// 零値のまま、使える。同時に呼ばれることは、守らない。
type Fake[A, R any] struct {
	// Calls は、呼ばれた引数（呼ばれた順）。
	Calls []A

	set   bool
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

// Call は、生成したコードが呼ぶ: args を Calls に足し、Returns か Throws が言われていれば、そのとおりに返す。言われていなくて、
// impl が nil でなければ、impl（本物）を呼ぶ。nil なら、R の零値を返す（失敗は、ない）。
func (f *Fake[A, R]) Call(args A, impl func() (R, error)) (R, error) {
	f.Calls = append(f.Calls, args)
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
}

// Returns は、これからの呼び出しが、何もせずに、正常に終わるようにする（本物は、呼ばない）。
func (f *FakeVoid[A]) Returns() { f.set, f.err = true, nil }

// Throws は、これからの呼び出しが、err を返すようにする（本物は、呼ばない）。
func (f *FakeVoid[A]) Throws(err error) { f.set, f.err = true, err }

// Call は、生成したコードが呼ぶ: args を Calls に足し、Returns か Throws が言われていれば、そのとおりに返す。言われていなくて、
// impl が nil でなければ、impl（本物）を呼ぶ。nil なら、nil を返す。
func (f *FakeVoid[A]) Call(args A, impl func() error) error {
	f.Calls = append(f.Calls, args)
	if f.set || impl == nil {
		return f.err
	}
	return impl()
}
