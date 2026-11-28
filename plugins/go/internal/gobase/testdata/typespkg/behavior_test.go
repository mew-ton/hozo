// 入力 typespkg の出力を動かす（plugins/go の TestBehavior が、出力を書いたモジュールのルートに置いて走らせる）。確かめること
// （plugins/go/spec/go.hozo の Requirement.Layout の「型のパッケージ」）:
//
//   - 互いの型を使う二つの名前空間（ship の型が carry の型を使い、carry の関数が ship の型を使う）の出力が、コンパイルできて、動く。
//   - 人が書くコードから見た名前は、変わらない: 入口のパッケージの名前（ship.Parcel、carry.Code）で、型を書ける。
//   - 入口の型、内側の型、型のパッケージの型は、同じ型（別名）。
//   - 述語で決まる、値の分類の、列挙の定数と、判定関数も、入口から、そのまま使える。
package behavior

import (
	"context"
	"testing"

	"example.com/app/gen/carry"
	carryinner "example.com/app/gen/carry/inner"
	carrytypes "example.com/app/gen/carry/types"
	"example.com/app/gen/plain"
	"example.com/app/gen/ship"
	shiptypes "example.com/app/gen/ship/types"
)

// 入口、内側、型のパッケージの型は、同じ型
var (
	_ carrytypes.Code  = carry.Code("")
	_ carryinner.Quote = carry.Quote{}
	_ shiptypes.Label  = ship.Label{}
	_ carrytypes.Tier  = carry.TierCheap
	_ plain.Note       = plain.Note{By: carry.Code("x")}
)

type carryLeaves struct{}

func (carryLeaves) Issue(_ context.Context, parcel ship.Parcel) (ship.Label, error) {
	return ship.Label{Carrier: carry.Code("yamato"), Id: "L-1"}, nil
}

func (carryLeaves) Quote(_ context.Context, parcel ship.Parcel) (carry.Quote, error) {
	return carry.Quote{Code: "yamato", Price: parcel.Weight * 10}, nil
}

type shipLeaves struct{}

func (shipLeaves) Pack(_ context.Context, weight int64) (ship.Parcel, error) {
	return ship.Parcel{Weight: weight}, nil
}

type tiers struct{}

func (tiers) Cheap(_ context.Context, q carry.Quote) (bool, error) { return q.Price < 100, nil }
func (tiers) Dear(_ context.Context, q carry.Quote) (bool, error)  { return q.Price >= 100, nil }

func TestMutualTypes(t *testing.T) {
	ctx := context.Background()
	carrier := carry.New(carryLeaves{})
	got, err := ship.New(shipLeaves{}, carrier).Send(ctx, 3)
	if err != nil || got != (ship.Label{Carrier: "yamato", Id: "L-1"}) {
		t.Fatalf("send = %+v, %v", got, err)
	}
	q, err := carrier.Quote(ctx, ship.Parcel{Weight: 20})
	if err != nil {
		t.Fatal(err)
	}
	tier, err := carry.ClassifyTier(ctx, tiers{}, q)
	if err != nil || tier != carry.TierDear {
		t.Fatalf("ClassifyTier = %v, %v", tier, err)
	}
}
