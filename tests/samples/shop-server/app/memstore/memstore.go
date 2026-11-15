// Package memstore は、名前空間 store の op の、メモリの上の実装（../../README.md「人が書いたもの」）。
// 単位 gen/storemem（transpilers は基底の go だけ）には、op の中身を出すトランスパイラがいないので、人が書いて
// store.States に入れる。gen/store（go-sqlite が出す）と、op の関数の型は同じ。
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

// Setup は空の Store を作り、その上の op の実装を store.States（gen/storemem）に入れる。
//
// store.States はパッケージの変数なので、一つのプロセスで有効なのは、最後に Setup した Store だけ。
func Setup() *Store {
	m := &Store{}
	// どの op がどの状態の単位に入るかは、生成された states.go の通り（読む先か、効果の宛先の状態）
	store.States.Users.FindUser = m.findUser
	store.States.Users.FindUserById = m.findUserById
	store.States.Users.AddUser = m.addUser
	store.States.Sessions.FindSession = m.findSession
	store.States.Sessions.AddSession = m.addSession
	store.States.Sessions.RemoveSession = m.removeSession
	store.States.Orders.ListOrders = m.listOrders
	store.States.Orders.AddOrder = m.addOrder
	store.States.Notifications.AddNotification = m.addNotification
	return m
}

// Notifications は、足された通知（足した順）。通知を読む op は仕様にないので、テストが使う。
func (m *Store) Notifications() []pkgtype.Notification {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]pkgtype.Notification(nil), m.notifications...)
}

// lock は、ctx が生きていれば mu を取る。中断されているか期限が過ぎていれば、状態に触れずにそのエラーを返す。
func (m *Store) lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	return nil
}

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
