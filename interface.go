package connpool

import (
	"context"
	"net"
)

type Pool interface {
	Get(ctx context.Context) (net.Conn, error)
	Put(conn net.Conn) error
	Len() int
}
