package connpool

import "errors"

var (
	ErrConnExpired     = errors.New("conn is expired")
	ErrInvalidFactory  = errors.New("invalid factory func settings")
	ErrCloseFactory    = errors.New("invalid close func settings")
	ErrCapacitySetting = errors.New("invalid capacity settings")
	ErrConnNil         = errors.New("connection is nil. rejecting")
	ErrPingFailed      = errors.New("ping to the connection failed")
	ErrPoolClosed      = errors.New("pool is closed")
)
