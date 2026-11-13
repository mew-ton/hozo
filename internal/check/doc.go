// Package check は検査器（docs/check.md）。規則の ID は docs/requirements.md「検査の規則の一覧」のもので、
// 規則を実装する関数とテストはその ID を書く。hozoc と言語サーバーの両方がこのパッケージを呼び、結果は同じになる。
//
// 入口は三つ。
//
//   - Compute は docs/check.md「計算」を行い、Op.Signature（Sig(O, ns(O))）と Call.Bound を埋める。
//   - Check は計算の上で docs/check.md「診断」の規則（と docs/config.md「診断」のうちプログラムが要るもの）を検査する。
//     Check は計算を含むので、先に Compute を呼ばなくてよい。CFG-6 はプラグインのマニフェストが要るので、
//     CheckUnsupported に、単位の in に合ったファイルと unsupported の一覧を渡して別に呼ぶ。
//   - Views は、単位と、その単位が出力する名前空間の組ごとの View（docs/ir.md「その名前空間から見た IR」）を作る。
//
// 診断の位置は IR の Span（ファイルのパスと行・列）から作る。診断の Span はバイトのオフセットなので、
// Checker.Locate（FileSetLocator）で直す。Locate がなければ Span は nil で、File に IR のファイルのパスを置く。
//
// 規則ごとに一つの関数で、ファイルは節ごと: throws.go（THROWS-1、REL-*）、effects.go（STATE-2、EFF-2）、
// decorator.go（DEC-*）、wiring.go（WIRE-6、WIRE-7）、bind.go（BIND-*）、namespace.go（DOC-1）、cfg.go（CFG-*）。
// 計算の途中でしか分からない規則（THROWS-1、REL-4、DEC-5、DEC-16、WIRE-7）は compute.go から呼ばれる。
package check
