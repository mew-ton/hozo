// Package memstore は、名前空間 store の op と、状態の形で決まる問い合わせの、メモリの上の実装
// （../../README.md「人が書いたもの」）。単位 gen/storemem（transpilers は基底の go だけ）には、中身を出す
// トランスパイラがいないので、人が書いて store.States に入れる。gen/store（go-sqlite が出す）と、関数の型は同じ。
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

// Store は、メモリの上の状態。op は並行に呼ばれるので、mu で守る。
type Store struct {
	mu            sync.Mutex
	users         []pkgtype.User
	sessions      []pkgtype.Session
	orders        []pkgtype.Order
	notifications []pkgtype.Notification
}

// Setup は空の Store を作り、その上の実装を store.States（gen/storemem）に入れる。
//
// store.States はパッケージの変数なので、一つのプロセスで有効なのは、最後に Setup した Store だけ。
func Setup() {
	m := &Store{}
	// 口の並びは、生成された states.go の通り: 状態の単位ごとに、形で決まる問い合わせ（IsEmpty、Get）、
	// その状態を読む op、その状態を宛先にする効果を持つ op
	store.States.Users.IsEmpty = isEmpty(m, &m.users)
	store.States.Users.Get = get(m, &m.users, same)
	store.States.Users.FindUser = m.findUser
	store.States.Users.FindUserById = m.findUserById
	store.States.Users.AddUser = m.addUser
	store.States.Sessions.IsEmpty = isEmpty(m, &m.sessions)
	store.States.Sessions.Get = get(m, &m.sessions, same)
	store.States.Sessions.FindSession = m.findSession
	store.States.Sessions.AddSession = m.addSession
	store.States.Sessions.RemoveSession = m.removeSession
	store.States.Orders.IsEmpty = isEmpty(m, &m.orders)
	store.States.Orders.Get = get(m, &m.orders, copyOrder)
	store.States.Orders.ListOrders = m.listOrders
	store.States.Orders.AddOrder = m.addOrder
	store.States.Notifications.IsEmpty = isEmpty(m, &m.notifications)
	store.States.Notifications.Get = get(m, &m.notifications, same)
	store.States.Notifications.AddNotification = m.addNotification
}

// lock は、ctx が生きていれば mu を取る。中断されているか期限が過ぎていれば、状態に触れずにそのエラーを返す。
func (m *Store) lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	return nil
}

// isEmpty は、状態（m の rows）の、形で決まる問い合わせ IsEmpty を作る: 要素が一つもなければ true。
func isEmpty[T any](m *Store, rows *[]T) func(context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		if err := m.lock(ctx); err != nil {
			return false, err
		}
		defer m.mu.Unlock()
		return len(*rows) == 0, nil
	}
}

// get は、状態（m の rows）の、形で決まる問い合わせ Get を作る: すべての要素の写しを、足した順に。
// copyRow は、要素一つの写し方。
func get[T any](m *Store, rows *[]T, copyRow func(T) T) func(context.Context) ([]T, error) {
	return func(ctx context.Context) ([]T, error) {
		if err := m.lock(ctx); err != nil {
			return nil, err
		}
		defer m.mu.Unlock()
		var out []T
		for _, row := range *rows {
			out = append(out, copyRow(row))
		}
		return out, nil
	}
}

// same は、スライスを持たない要素の写し（値そのまま）。
func same[T any](row T) T { return row }

func (m *Store) findUser(ctx context.Context, email pkgtype.Email) (*pkgtype.User, error) {
	if err := m.lock(ctx); err != nil {
		return nil, err
	}
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Email == email {
			return &u, nil
		}
	}
	return nil, nil
}

func (m *Store) findUserById(ctx context.Context, id pkgtype.UserId) (*pkgtype.User, error) {
	if err := m.lock(ctx); err != nil {
		return nil, err
	}
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Id == id {
			return &u, nil
		}
	}
	return nil, nil
}

func (m *Store) addUser(ctx context.Context, user pkgtype.User) error {
	if err := m.lock(ctx); err != nil {
		return err
	}
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Id == user.Id || u.Email == user.Email {
			return ErrDuplicate
		}
	}
	m.users = append(m.users, user)
	return nil
}

func (m *Store) findSession(ctx context.Context, token *pkgtype.Token) (*pkgtype.Session, error) {
	if err := m.lock(ctx); err != nil {
		return nil, err
	}
	defer m.mu.Unlock()
	if token == nil {
		return nil, nil
	}
	for _, s := range m.sessions {
		if s.Token == *token {
			return &s, nil
		}
	}
	return nil, nil
}

func (m *Store) addSession(ctx context.Context, session pkgtype.Session) error {
	if err := m.lock(ctx); err != nil {
		return err
	}
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if s.Token == session.Token {
			return ErrDuplicate
		}
	}
	m.sessions = append(m.sessions, session)
	return nil
}

func (m *Store) removeSession(ctx context.Context, token pkgtype.Token) error {
	if err := m.lock(ctx); err != nil {
		return err
	}
	defer m.mu.Unlock()
	for i, s := range m.sessions {
		if s.Token == token {
			m.sessions = append(m.sessions[:i], m.sessions[i+1:]...)
			break
		}
	}
	return nil
}

func (m *Store) listOrders(ctx context.Context, userId pkgtype.UserId) ([]pkgtype.Order, error) {
	if err := m.lock(ctx); err != nil {
		return nil, err
	}
	defer m.mu.Unlock()
	var out []pkgtype.Order
	for _, o := range m.orders {
		if o.UserId == userId {
			out = append(out, copyOrder(o))
		}
	}
	return out, nil
}

func (m *Store) addOrder(ctx context.Context, order pkgtype.Order) error {
	if err := m.lock(ctx); err != nil {
		return err
	}
	defer m.mu.Unlock()
	for _, o := range m.orders {
		if o.Id == order.Id {
			return ErrDuplicate
		}
	}
	m.orders = append(m.orders, copyOrder(order))
	return nil
}

func (m *Store) addNotification(ctx context.Context, notification pkgtype.Notification) error {
	if err := m.lock(ctx); err != nil {
		return err
	}
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, notification)
	return nil
}

// copyOrder は、行のスライスを共有しない注文の写し（呼ぶ側が後から書き換えても、状態が変わらないように）。
func copyOrder(o pkgtype.Order) pkgtype.Order {
	o.Lines = append([]pkgtype.OrderLine{}, o.Lines...)
	return o
}
