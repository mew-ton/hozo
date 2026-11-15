// Package memstore は、名前空間 store の状態の単位の、メモリの上の実装（../../README.md「人が書いたもの」）。
// 単位 gen/storemem（transpilers は基底の go だけ）には、中身を出すトランスパイラがいないので、人が書いて
// store.New の引数に渡す。状態ごとに型が一つで、gen/store で go-sqlite が出す型（store.UsersTable など）と、
// 満たすインターフェース（store.UsersState など）の中身は同じ。
package memstore

import (
	"context"
	"errors"
	"sync"

	store "example.com/shop-server/gen/storemem"
	pkgtype "example.com/shop-server/gen/type"
)

// ErrDuplicate は、@db.key か @db.unique のフィールドが同じ行を足そうとした。
// 仕様はこの失敗を言わない（addUser などに throws はない）。SQLite の制約に合わせて、人が決めたもの。
var ErrDuplicate = errors.New("memstore: duplicate key")

// Store は、メモリの上の四つの状態。op は並行に呼ばれるので、mu で守る。
// 状態の単位の実装は、Users、Sessions、Orders、Notifications が返す。同じ Store のものは、同じ状態を読み書きする。
type Store struct {
	mu            sync.Mutex
	users         []pkgtype.User
	sessions      []pkgtype.Session
	orders        []pkgtype.Order
	notifications []pkgtype.Notification
}

// New は空の Store を返す。Store はいくつ作ってもよく、互いに混ざらない。
func New() *Store { return &Store{} }

// 四つの型は、生成された状態の単位をすべて満たす。メソッドが一つでも足りなければ、ここでコンパイルできない。
var (
	_ store.UsersState         = Users{}
	_ store.SessionsState      = Sessions{}
	_ store.OrdersState        = Orders{}
	_ store.NotificationsState = Notifications{}
)

// Users は状態 store.users の単位（store.UsersState）の実装。
type Users struct{ list[pkgtype.User] }

// Users は m の上の、状態 store.users の単位の実装を返す。
func (m *Store) Users() Users { return Users{listOf(m, &m.users, same)} }

// Sessions は状態 store.sessions の単位（store.SessionsState）の実装。
type Sessions struct{ list[pkgtype.Session] }

// Sessions は m の上の、状態 store.sessions の単位の実装を返す。
func (m *Store) Sessions() Sessions { return Sessions{listOf(m, &m.sessions, same)} }

// Orders は状態 store.orders の単位（store.OrdersState）の実装。
type Orders struct{ list[pkgtype.Order] }

// Orders は m の上の、状態 store.orders の単位の実装を返す。
func (m *Store) Orders() Orders { return Orders{listOf(m, &m.orders, copyOrder)} }

// Notifications は状態 store.notifications の単位（store.NotificationsState）の実装。
type Notifications struct{ list[pkgtype.Notification] }

// Notifications は m の上の、状態 store.notifications の単位の実装を返す。
func (m *Store) Notifications() Notifications {
	return Notifications{listOf(m, &m.notifications, same)}
}

// lock は、ctx が生きていれば mu を取る。中断されているか期限が過ぎていれば、状態に触れずにそのエラーを返す。
func (m *Store) lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	return nil
}

// list は、list の状態（m の rows）の、形で決まる問い合わせ（IsEmpty、Get）。四つの型が埋め込んで、メソッドを受け継ぐ。
// copyRow は、要素一つの写し方。
type list[T any] struct {
	m       *Store
	rows    *[]T
	copyRow func(T) T
}

// listOf は、m の rows の上の list を作る。
func listOf[T any](m *Store, rows *[]T, copyRow func(T) T) list[T] {
	return list[T]{m: m, rows: rows, copyRow: copyRow}
}

// IsEmpty は、形で決まる問い合わせ: 要素が一つもなければ true。
func (l list[T]) IsEmpty(ctx context.Context) (bool, error) {
	if err := l.m.lock(ctx); err != nil {
		return false, err
	}
	defer l.m.mu.Unlock()
	return len(*l.rows) == 0, nil
}

// Get は、形で決まる問い合わせ: すべての要素の写しを、足した順に。
func (l list[T]) Get(ctx context.Context) ([]T, error) {
	if err := l.m.lock(ctx); err != nil {
		return nil, err
	}
	defer l.m.mu.Unlock()
	var out []T
	for _, row := range *l.rows {
		out = append(out, l.copyRow(row))
	}
	return out, nil
}

// same は、スライスを持たない要素の写し（値そのまま）。
func same[T any](row T) T { return row }

// FindUser は store.findUser の本体。
func (s Users) FindUser(ctx context.Context, email pkgtype.Email) (*pkgtype.User, error) {
	if err := s.m.lock(ctx); err != nil {
		return nil, err
	}
	defer s.m.mu.Unlock()
	for _, u := range s.m.users {
		if u.Email == email {
			return &u, nil
		}
	}
	return nil, nil
}

// FindUserById は store.findUserById の本体。
func (s Users) FindUserById(ctx context.Context, id pkgtype.UserId) (*pkgtype.User, error) {
	if err := s.m.lock(ctx); err != nil {
		return nil, err
	}
	defer s.m.mu.Unlock()
	for _, u := range s.m.users {
		if u.Id == id {
			return &u, nil
		}
	}
	return nil, nil
}

// AddUser は store.addUser の本体。id（@db.key）か email（@db.unique）が同じユーザーがいれば ErrDuplicate。
func (s Users) AddUser(ctx context.Context, user pkgtype.User) error {
	if err := s.m.lock(ctx); err != nil {
		return err
	}
	defer s.m.mu.Unlock()
	for _, u := range s.m.users {
		if u.Id == user.Id || u.Email == user.Email {
			return ErrDuplicate
		}
	}
	s.m.users = append(s.m.users, user)
	return nil
}

// FindSession は store.findSession の本体。トークンがなければ、ない。
func (s Sessions) FindSession(ctx context.Context, token *pkgtype.Token) (*pkgtype.Session, error) {
	if err := s.m.lock(ctx); err != nil {
		return nil, err
	}
	defer s.m.mu.Unlock()
	if token == nil {
		return nil, nil
	}
	for _, x := range s.m.sessions {
		if x.Token == *token {
			return &x, nil
		}
	}
	return nil, nil
}

// AddSession は store.addSession の本体。token（@db.key）が同じセッションがあれば ErrDuplicate。
func (s Sessions) AddSession(ctx context.Context, session pkgtype.Session) error {
	if err := s.m.lock(ctx); err != nil {
		return err
	}
	defer s.m.mu.Unlock()
	for _, x := range s.m.sessions {
		if x.Token == session.Token {
			return ErrDuplicate
		}
	}
	s.m.sessions = append(s.m.sessions, session)
	return nil
}

// RemoveSession は store.removeSession の本体。そのトークンのセッションがなければ、何もしない。
func (s Sessions) RemoveSession(ctx context.Context, token pkgtype.Token) error {
	if err := s.m.lock(ctx); err != nil {
		return err
	}
	defer s.m.mu.Unlock()
	for i, x := range s.m.sessions {
		if x.Token == token {
			s.m.sessions = append(s.m.sessions[:i], s.m.sessions[i+1:]...)
			break
		}
	}
	return nil
}

// ListOrders は store.listOrders の本体。そのユーザーの注文の写しを、足した順に。
func (s Orders) ListOrders(ctx context.Context, userId pkgtype.UserId) ([]pkgtype.Order, error) {
	if err := s.m.lock(ctx); err != nil {
		return nil, err
	}
	defer s.m.mu.Unlock()
	var out []pkgtype.Order
	for _, o := range s.m.orders {
		if o.UserId == userId {
			out = append(out, copyOrder(o))
		}
	}
	return out, nil
}

// AddOrder は store.addOrder の本体。id（@db.key）が同じ注文があれば ErrDuplicate。
func (s Orders) AddOrder(ctx context.Context, order pkgtype.Order) error {
	if err := s.m.lock(ctx); err != nil {
		return err
	}
	defer s.m.mu.Unlock()
	for _, o := range s.m.orders {
		if o.Id == order.Id {
			return ErrDuplicate
		}
	}
	s.m.orders = append(s.m.orders, copyOrder(order))
	return nil
}

// AddNotification は store.addNotification の本体。
func (s Notifications) AddNotification(ctx context.Context, notification pkgtype.Notification) error {
	if err := s.m.lock(ctx); err != nil {
		return err
	}
	defer s.m.mu.Unlock()
	s.m.notifications = append(s.m.notifications, notification)
	return nil
}

// copyOrder は、行のスライスを共有しない注文の写し（呼ぶ側が後から書き換えても、状態が変わらないように）。
func copyOrder(o pkgtype.Order) pkgtype.Order {
	o.Lines = append([]pkgtype.OrderLine{}, o.Lines...)
	return o
}
