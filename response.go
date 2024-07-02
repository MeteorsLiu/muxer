package main

import (
	"context"
	"net"
)

type response struct {
	n   int
	err error
}

type writeRequest struct {
	key string
	buf []byte
	op  Opcode
	res chan *response
	ctx context.Context
}

func newWriteResponse(ctx context.Context, op Opcode, key string, buf []byte) *writeRequest {
	return &writeRequest{ctx: ctx, op: op, key: key, buf: buf, res: make(chan *response, 1)}
}

func (wr *writeRequest) writeResult(n int, err error) {
	select {
	case wr.res <- &response{n, err}:
	case <-wr.ctx.Done():
	}
}

func (wr *writeRequest) result() (n int, err error) {
	select {
	case res := <-wr.res:
		n, err = res.n, res.err
	case <-wr.ctx.Done():
		err = net.ErrClosed
	}
	return
}
