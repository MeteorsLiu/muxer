package main

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

var (
	todo = context.TODO()
)

var _ io.ReadWriteCloser = (*MuxerConn)(nil)

type MuxerConn struct {
	leftbyte []byte
	queue    chan []byte

	key    string
	parent Writer

	closeOnce sync.Once
	closed    atomic.Bool
}

func NewMuxerConn(parent Writer, key string) *MuxerConn {
	mc := &MuxerConn{key: key, parent: parent, queue: make(chan []byte, 1024)}
	return mc
}

func (mc *MuxerConn) pushBytes(buf []byte) {
	if !mc.closed.Load() {
		mc.queue <- buf
	}
}

func (mc *MuxerConn) Read(buf []byte) (n int, err error) {
	if mc.leftbyte != nil {
		n = copy(mc.leftbyte, buf)
		if n == len(mc.leftbyte) {
			mc.leftbyte = nil
		} else {
			mc.leftbyte = mc.leftbyte[n:]
		}
		return
	}
	new, ok := <-mc.queue
	if !ok {
		err = io.EOF
		return
	}
	n = copy(buf, new)
	if n != len(new) {
		mc.leftbyte = new[n:]
	}
	return
}

func (mc *MuxerConn) Write(buf []byte) (n int, err error) {
	n, err = mc.parent.WriteData(Relay, mc.key, buf)
	return
}

func (mc *MuxerConn) Free() bool {
	closed := true
	mc.closeOnce.Do(func() {
		closed = false
		mc.closed.Store(true)
		close(mc.queue)
	})

	return closed
}

func (mc *MuxerConn) Close() (err error) {
	if !mc.Free() {
		mc.parent.WriteData(Close, mc.key, nil)
	} else {
		err = net.ErrClosed
	}
	return
}

func (mc *MuxerConn) Key() string {
	return mc.key
}
