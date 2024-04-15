package connpool

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type idleConn struct {
	conn     net.Conn
	lastUsed time.Time
}

func (ic *idleConn) isExpired(expire time.Duration) bool {
	return ic.lastUsed.Add(expire).Before(time.Now().UTC())
}

type ConnPool struct {
	*ConnHandler
	*Config
	numActive    int32
	mux          sync.Mutex
	idleConns    chan *idleConn
	waitingQueue []chan *idleConn
	closed       int32 // 1 means closed, 0 means open
}

func NewPool(handler *ConnHandler, options ...Option) (*ConnPool, error) {
	config := LoadConfig(options...)

	if err := config.Validate(); err != nil {
		return nil, err
	}

	if err := handler.Validate(); err != nil {
		return nil, err
	}

	pool := &ConnPool{
		ConnHandler:  handler,
		Config:       config,
		numActive:    0,
		idleConns:    make(chan *idleConn, config.MaxIdle),
		waitingQueue: make([]chan *idleConn, 0),
		mux:          sync.Mutex{},
	}

	if err := pool.initializeConnections(config.InitialCap); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

func (pool *ConnPool) initializeConnections(initCap uint) error {
	for i := 0; i < int(initCap); i++ {
		conn, err := pool.createConn(context.Background())
		if err != nil {
			return err
		}
		pool.idleConns <- &idleConn{conn: conn, lastUsed: time.Now().UTC()}
	}
	return nil
}

func (pool *ConnPool) Get(ctx context.Context) (net.Conn, error) {
	for {
		conn, err := pool.get(ctx)
		if err == nil {
			return conn, nil
		}
		if !errors.Is(err, ErrConnExpired) && !errors.Is(err, ErrPingFailed) {
			return nil, err
		}
	}
}

func (pool *ConnPool) get(ctx context.Context) (net.Conn, error) {
	if pool.IsClosed() {
		return nil, ErrPoolClosed
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:

	}

	select {
	case ic := <-pool.idleConns:
		if err := pool.checkAndCloseExpiredConn(ic); err != nil {
			return nil, err
		}
		return ic.conn, nil
	default:
		if pool.getNumActive() < pool.MaxCap {
			return pool.createConn(ctx)
		}
		return pool.waitForConn(ctx)
	}
}

func (pool *ConnPool) createConn(ctx context.Context) (net.Conn, error) {
	conn, err := pool.create()
	if err != nil {
		return nil, err
	}
	pool.IncrNumActive(1)
	return conn, err
}

func (pool *ConnPool) waitForConn(ctx context.Context) (net.Conn, error) {
	ch := make(chan *idleConn, 1)
	pool.mux.Lock()
	pool.waitingQueue = append(pool.waitingQueue, ch)
	pool.mux.Unlock()

	select {
	case <-ctx.Done():
		pool.removeFromWaitingQueue(ch)
		return nil, ctx.Err()
	case ic := <-ch:
		if ic == nil {
			return nil, ErrPoolClosed
		}
		return ic.conn, nil
	}
}

func (pool *ConnPool) removeFromWaitingQueue(ch chan *idleConn) {
	pool.mux.Lock()
	defer pool.mux.Unlock()
	for i, waitingCh := range pool.waitingQueue {
		if waitingCh == ch {
			pool.waitingQueue = append(pool.waitingQueue[:i], pool.waitingQueue[i+1:]...)
			break
		}
	}
}

func (pool *ConnPool) popWaitingQueue() {
	pool.waitingQueue = pool.waitingQueue[1:]
}

func (pool *ConnPool) checkAndCloseExpiredConn(ic *idleConn) error {
	if ic.isExpired(pool.MaxIdleTime) {
		if err := pool.closeConn(ic.conn); err != nil {
			return fmt.Errorf("%w: %v", ErrConnExpired, err)
		}
		return ErrConnExpired
	}

	if err := pool.ping(ic.conn); err != nil {
		if err := pool.closeConn(ic.conn); err != nil {
			return fmt.Errorf("closing connection failed after ping error: %v, ping error: %w", err, ErrPingFailed)
		}
		return fmt.Errorf("%w: %v", ErrPingFailed, err)
	}

	return nil
}

func (pool *ConnPool) getNumActive() uint {
	return uint(atomic.LoadInt32(&pool.numActive))
}

func (pool *ConnPool) IncrNumActive(delta int32) {
	atomic.AddInt32(&pool.numActive, delta)
}

func (pool *ConnPool) closeConn(conn net.Conn) error {
	pool.IncrNumActive(-1)
	if err := pool.close(conn); err != nil {
		return err
	}
	return nil
}

func (pool *ConnPool) Put(conn net.Conn) error {
	if conn == nil {
		return ErrConnNil
	}

	if pool.IsClosed() {
		return ErrPoolClosed
	}

	ic := &idleConn{
		conn:     conn,
		lastUsed: time.Now().UTC(),
	}

	pool.mux.Lock()
	if len(pool.waitingQueue) > 0 {
		ch := pool.waitingQueue[0]
		ch <- ic
		// 关闭等待队列中的 channel 之后，我们还需要从等待队列中删除它。你的 removeFromWaitingQueue 函数应该是安全的，但是在这里调用它的方式可能会导致误解。更好的做法是创建一个新函数，例如 popWaitingQueue，它既关闭 channel 又从队列中移除它
		pool.popWaitingQueue() // This function closes the channel and removes it from the queue
		pool.mux.Unlock()
		return nil
	}
	pool.mux.Unlock()

	select {
	case pool.idleConns <- ic:
		return nil
	default:
		return pool.closeConn(conn)
	}
}

func (pool *ConnPool) IsClosed() bool {
	return atomic.LoadInt32(&pool.closed) == 1
}

func (pool *ConnPool) Close() {
	if !pool.setClosed() {
		return
	}

	pool.mux.Lock()
	conns := pool.idleConns
	pool.waitingQueue = nil
	pool.numActive = 0
	pool.waitingQueue = nil
	closeFunc := pool.close
	pool.ConnHandler = nil
	pool.mux.Unlock()

	if conns == nil {
		return
	}

	close(conns)
	for ic := range pool.idleConns {
		_ = closeFunc(ic.conn)
	}

}

func (pool *ConnPool) setClosed() bool {
	return atomic.CompareAndSwapInt32(&pool.closed, 0, 1)
}

func (pool *ConnPool) Len() int {
	pool.mux.Lock()
	defer pool.mux.Unlock()
	return len(pool.idleConns)
}
