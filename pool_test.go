package connpool

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
)

var (
	factory  = func() (net.Conn, error) { return new(ConnMock), nil }
	closeFac = func(conn net.Conn) error { return nil }
)

type ConnMock struct {
	mock.Mock
}

// 实现 net.Conn 接口的所有方法
func (c *ConnMock) Read(b []byte) (n int, err error)   { return }
func (c *ConnMock) Write(b []byte) (n int, err error)  { return }
func (c *ConnMock) Close() error                       { return nil }
func (c *ConnMock) LocalAddr() net.Addr                { return nil }
func (c *ConnMock) RemoteAddr() net.Addr               { return nil }
func (c *ConnMock) SetDeadline(t time.Time) error      { return nil }
func (c *ConnMock) SetReadDeadline(t time.Time) error  { return nil }
func (c *ConnMock) SetWriteDeadline(t time.Time) error { return nil }

func TestNewPoolWithMockedConn(t *testing.T) {
	handler := &ConnHandler{Factory: factory, CloseConn: closeFac}
	options := []Option{
		WithInitialCap(10),
		WithMaxCap(20),
		WithMaxIdle(10),
	}
	pool, err := NewPool(handler, options...)
	if err != nil {
		t.Errorf("Failed to create pool: %s", err)
		return
	}
	defer pool.Close()
}

func TestGetConnectionFromPool(t *testing.T) {
	handler := &ConnHandler{Factory: factory, CloseConn: closeFac}
	options := []Option{
		WithInitialCap(5),
		WithMaxCap(10),
		WithMaxIdle(10),
	}
	pool, err := NewPool(handler, options...)
	if err != nil {
		t.Fatal("Failed to create pool:", err)
	}
	defer pool.Close()

	conn, err := pool.Get(context.Background())
	if err != nil || conn == nil {
		t.Fatal("Failed to get connection from pool:", err)
	}
}

func TestPutConnectionBackToPool(t *testing.T) {
	handler := &ConnHandler{Factory: factory, CloseConn: closeFac}
	options := []Option{
		WithInitialCap(5),
		WithMaxCap(10),
		WithMaxIdle(10),
	}
	pool, err := NewPool(handler, options...)
	if err != nil {
		t.Fatal("Failed to create pool:", err)
	}
	defer pool.Close()

	conn, err := pool.Get(context.Background())
	if err != nil || conn == nil {
		t.Fatal("Failed to get connection from pool:", err)
	}

	err = pool.Put(conn)
	if err != nil {
		t.Fatal("Failed to put connection back to pool:", err)
	}
}

func TestPoolIsFull(t *testing.T) {
	handler := &ConnHandler{Factory: factory, CloseConn: closeFac}
	options := []Option{
		WithInitialCap(5),
		WithMaxCap(5),
		WithMaxIdle(5),
	}
	pool, err := NewPool(handler, options...)
	if err != nil {
		t.Fatal("Failed to create pool:", err)
	}
	defer pool.Close()

	for i := 0; i < 5; i++ {
		conn, err := pool.Get(context.Background())
		if err != nil || conn == nil {
			t.Fatal("Failed to get connection from pool:", err)
		}
		go func() {

		}()
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = pool.Get(ctx)
	if err == nil {
		t.Fatal("Expected error when pool is full")
	}
}

func TestPoolClose(t *testing.T) {
	handler := &ConnHandler{Factory: factory, CloseConn: closeFac}
	options := []Option{
		WithInitialCap(5),
		WithMaxCap(10),
		WithMaxIdle(10),
	}
	pool, err := NewPool(handler, options...)
	if err != nil {
		t.Fatal("Failed to create pool:", err)
	}

	pool.Close()

	_, err = pool.Get(context.Background())
	if err == nil {
		t.Fatal("Expected error when getting connection from closed pool")
	}
}

func TestExpiredConnectionFromPool(t *testing.T) {
	handler := &ConnHandler{Factory: factory, CloseConn: closeFac}
	options := []Option{
		WithInitialCap(5),
		WithMaxCap(10),
		WithMaxIdle(10),
		WithMaxIdleTime(time.Millisecond), // 设置极短的空闲超时时间
	}
	pool, err := NewPool(handler, options...)
	if err != nil {
		t.Fatal("Failed to create pool:", err)
	}
	defer pool.Close()

	conn, err := pool.Get(context.Background())
	if err != nil || conn == nil {
		t.Fatal("Failed to get connection from pool:", err)
	}

	time.Sleep(2 * time.Millisecond) // 等待足够的时间让连接过期

	err = pool.Put(conn)
	if err != nil {
		t.Fatal("Failed to put connection back to pool:", err)
	}

	_, err = pool.get(context.Background())
	if err == nil || !errors.Is(err, ErrConnExpired) {
		t.Fatal("Expected ErrConnExpired when getting expired connection")
	}
}

func TestPingFailedForConnectionInPool(t *testing.T) {
	factoryError := func() (net.Conn, error) { return new(ConnMockFailPing), nil }

	handler := &ConnHandler{Factory: factoryError, CloseConn: closeFac, Ping: pingFunc}
	options := []Option{
		WithInitialCap(5),
		WithMaxCap(10),
		WithMaxIdle(10),
	}
	pool, err := NewPool(handler, options...)
	if err != nil {
		t.Fatal("Failed to create pool:", err)
	}
	defer pool.Close()

	conn, err := pool.Get(context.Background())
	if err != nil || conn == nil {
		t.Fatal("Failed to get connection from pool:", err)
	}

	err = pool.Put(conn)
	if err != nil {
		t.Fatal("Failed to put connection back to pool:", err)
	}

	_, err = pool.get(context.Background())
	if err == nil || !errors.Is(err, ErrPingFailed) {
		t.Fatal("Expected ErrPingFailed when getting failed ping connection")
	}
}

type ConnMockFailPing struct{ ConnMock }

func (c *ConnMockFailPing) Read(b []byte) (n int, err error) { return 0, fmt.Errorf("ping error") }

func pingFunc(conn net.Conn) error {
	_, err := conn.Read(make([]byte, 1))
	return err
}

func TestConcurrentGetPut(t *testing.T) {
	handler := &ConnHandler{Factory: factory, CloseConn: closeFac}
	options := []Option{
		WithInitialCap(5),
		WithMaxCap(10),
		WithMaxIdle(10),
	}
	pool, err := NewPool(handler, options...)
	if err != nil {
		t.Fatal("Failed to create pool:", err)
	}
	defer pool.Close()

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := pool.Get(context.Background())
			if err != nil {
				t.Error("Failed to get connection from pool:", err)
			}
			time.Sleep(time.Millisecond) // simulate some work
			err = pool.Put(conn)
			if err != nil {
				t.Error("Failed to put connection back to pool:", err)
			}
		}()
	}

	wg.Wait()

	if pool.Len() != 10 {
		t.Errorf("Expected 10 connections in pool, got %d", pool.Len())
	}
}

func TestConcurrentExhaustion(t *testing.T) {
	handler := &ConnHandler{Factory: factory, CloseConn: closeFac}
	options := []Option{
		WithInitialCap(2),
		WithMaxCap(2), // very small pool
		WithMaxIdle(2),
	}
	pool, err := NewPool(handler, options...)
	if err != nil {
		t.Fatal("Failed to create pool:", err)
	}
	defer pool.Close()

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ { // more goroutines than pool size
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := pool.Get(context.Background())
			if err != nil {
				t.Error("Failed to get connection from pool:", err)
				return
			}
			time.Sleep(10 * time.Millisecond) // simulate some work
			err = pool.Put(conn)
			if err != nil {
				t.Error("Failed to put connection back to pool:", err)
			}
		}()
	}

	wg.Wait()

	if pool.Len() != 2 {
		t.Errorf("Expected 2 connections in pool, got %d", pool.Len())
	}
}

func TestGetTimeout(t *testing.T) {
	handler := &ConnHandler{Factory: factory, CloseConn: closeFac}
	options := []Option{
		WithInitialCap(2),
		WithMaxCap(2), // very small pool
		WithMaxIdle(2),
	}
	pool, err := NewPool(handler, options...)
	if err != nil {
		t.Fatal("Failed to create pool:", err)
	}
	defer pool.Close()

	// Deplete the pool
	conn1, err := pool.Get(context.Background())
	if err != nil {
		t.Fatal("Failed to get connection from pool:", err)
	}

	conn2, err := pool.Get(context.Background())
	if err != nil {
		t.Fatal("Failed to get connection from pool:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = pool.Get(ctx)
	elapsed := time.Since(start)

	// Check if timeout error
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("Expected context.DeadlineExceeded error when getting connection with timeout")
	}

	// Check if it waited for about the right duration before timing out
	if elapsed < 100*time.Millisecond {
		t.Fatal("Get should have blocked until the context deadline")
	}

	err = pool.Put(conn1)
	if err != nil {
		t.Fatal("Failed to put connection back to pool:", err)
	}

	err = pool.Put(conn2)
	if err != nil {
		t.Fatal("Failed to put connection back to pool:", err)
	}
}

// Test for inserting a nil connection
func TestPutNilConn(t *testing.T) {
	handler := &ConnHandler{Factory: factory, CloseConn: closeFac}
	pool, _ := NewPool(handler)

	err := pool.Put(nil)
	if err != ErrConnNil {
		t.Error("Expected ErrConnNil error")
	}
}

// Test for inserting into a closed pool
func TestPutIntoClosedPool(t *testing.T) {
	handler := &ConnHandler{Factory: factory, CloseConn: closeFac}
	pool, _ := NewPool(handler)
	conn, _ := pool.Get(context.Background())
	pool.Close()

	err := pool.Put(conn)
	if err != ErrPoolClosed {
		t.Error("Expected ErrPoolClosed error")
	}
}

// Test for inserting when there are goroutines waiting for a connection
func TestPutWithWaitingQueue(t *testing.T) {
	handler := &ConnHandler{Factory: factory, CloseConn: closeFac}
	pool, _ := NewPool(handler, WithMaxCap(1), WithInitialCap(1), WithMaxIdle(1))

	// Deplete the pool
	conn1, _ := pool.Get(context.Background())

	go func() {
		_, err := pool.Get(context.Background())
		if err != nil {
			t.Error("Failed to get connection from pool:", err)
		}
	}()

	time.Sleep(time.Millisecond) // give Get time to start waiting

	// Now we insert a connection while there is a waiting goroutine
	err := pool.Put(conn1)
	if err != nil {
		t.Error("Failed to put connection back to pool:", err)
	}
}

// Test for inserting a connection into an idle pool
func TestPutIntoIdlePool(t *testing.T) {
	handler := &ConnHandler{Factory: factory, CloseConn: closeFac}
	pool, _ := NewPool(handler)

	conn, _ := pool.Get(context.Background())
	err := pool.Put(conn)
	if err != nil {
		t.Error("Failed to put connection back to pool:", err)
	}
}
