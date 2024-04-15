package connpool

import "net"

type connFactoryFunc func() (net.Conn, error)
type connCloseFunc func(conn net.Conn) error
type connPingFunc func(conn net.Conn) error

type ConnHandler struct {
	Factory   connFactoryFunc
	CloseConn connCloseFunc
	Ping      connPingFunc
}

func (h *ConnHandler) Validate() error {
	if h.Factory == nil {
		return ErrInvalidFactory
	}
	if h.CloseConn == nil {
		return ErrCloseFactory
	}
	return nil
}

func (h *ConnHandler) create() (net.Conn, error) {
	return h.Factory()
}

func (h *ConnHandler) close(conn net.Conn) error {
	return h.CloseConn(conn)
}

func (h *ConnHandler) ping(conn net.Conn) error {
	if h.Ping == nil {
		return nil
	}
	return h.Ping(conn)
}
