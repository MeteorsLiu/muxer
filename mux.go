package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"log"
	"net"
	"sync"
)

type Muxer struct {
	cn  net.Conn
	buf []byte

	recvCh  chan *MuxerConn
	writeCh chan *writeRequest
	connMu  sync.Mutex
	connMap map[string]*MuxerConn

	stop   context.Context
	doStop context.CancelFunc
}

type Writer interface {
	WriteData(op Opcode, key string, buf []byte) (n int, err error)
}

func newMuxer(cn net.Conn) *Muxer {
	m := &Muxer{cn: cn,
		connMap: map[string]*MuxerConn{},
		recvCh:  make(chan *MuxerConn, 1),
		writeCh: make(chan *writeRequest, 1024),
		buf:     make([]byte, 4),
	}
	m.stop, m.doStop = context.WithCancel(todo)
	go m.readLoop()
	go m.writeLoop()
	return m
}

func (n *Muxer) Send(key string) (writer io.WriteCloser, err error) {
	mc := NewMuxerConn(n, key)
	n.connMu.Lock()
	n.connMap[key] = mc
	n.connMu.Unlock()

	_, err = n.WriteData(New, key, nil)
	if err == nil {
		writer = mc
	}
	return
}

func (n *Muxer) Receive() (key string, reader io.Reader, err error) {
	var cn *MuxerConn
	select {
	case cn = <-n.recvCh:
	case <-n.stop.Done():
		err = net.ErrClosed
		return
	}

	key, reader, err = cn.Key(), cn, nil
	return
}

func (nx *Muxer) WriteData(op Opcode, key string, buf []byte) (n int, err error) {
	res := newWriteResponse(nx.stop, op, key, buf)
	select {
	case nx.writeCh <- res:
	case <-nx.stop.Done():
		err = net.ErrClosed
		return
	}

	n, err = res.result()
	return
}

func (n *Muxer) Close() {
	n.doStop()
	n.cn.Close()
}

func (n *Muxer) writeLoop() {
	h := make(Header, CommonHeaderSize)
	relayBuf := make([]byte, MinRelayHeaderSize)
	defer n.Close()

	for {
		select {
		case wr := <-n.writeCh:
			_, err := n.cn.Write(NewHeader(h, wr.op, wr.key))
			if err != nil {
				log.Println(err)
				return
			}
			// second, write key
			_, err = n.cn.Write([]byte(wr.key))
			if err != nil {
				log.Println(err)
				return
			}
			if wr.op == Relay && len(wr.buf) > 0 {
				// third, write RelayHeader
				binary.LittleEndian.PutUint32(relayBuf, uint32(len(wr.buf)))
				_, err := n.cn.Write(relayBuf)
				if err != nil {
					log.Println(err)
					return
				}
				nc, err := n.cn.Write(wr.buf)
				wr.writeResult(nc, err)
				if err != nil {
					log.Println(err)
					return
				}
			} else {
				wr.writeResult(0, nil)
			}
		case <-n.stop.Done():
			return
		}
	}
}

func (n *Muxer) handleNew(key string) {
	mc := NewMuxerConn(n, key)
	n.connMu.Lock()
	n.connMap[key] = mc
	n.connMu.Unlock()

	select {
	case n.recvCh <- mc:
	case <-n.stop.Done():
	}
}

func (n *Muxer) handleClose(key string) {
	n.connMu.Lock()
	mc := n.connMap[key]
	delete(n.connMap, key)
	n.connMu.Unlock()

	if mc != nil {
		mc.Close()
	}
}

func (nx *Muxer) handleRelay(br *bufio.Reader, key string) error {
	_, err := io.ReadFull(br, nx.buf)
	if err != nil {
		return err
	}
	n := binary.LittleEndian.Uint32(nx.buf)

	nx.connMu.Lock()
	mc := nx.connMap[key]
	nx.connMu.Unlock()

	buf := make([]byte, n)

	io.ReadFull(br, buf)

	if mc != nil {
		mc.pushBytes(buf)
	}
	return nil
}

func (n *Muxer) readLoop() {
	keybuf := make([]byte, 256)
	h := make(Header, CommonHeaderSize)
	br := bufio.NewReader(n.cn)
	defer n.Close()

	for {
		_, err := io.ReadFull(br, h)
		if err != nil {
			log.Println("Read Header Fail: ", err)
			break
		}
		op, keySize, err := h.Parse()
		if err != nil {
			log.Println("Parse Header Fail: ", err)
			continue
		}

		_, err = io.ReadFull(br, keybuf[0:keySize])
		if err != nil {
			log.Println("Read Key Fail: ", err)
			break
		}

		key := string(keybuf[0:keySize])

		switch op {
		case New:
			n.handleNew(key)
		case Relay:
			if err := n.handleRelay(br, key); err != nil {
				log.Println(err)
				return
			}
		case Close:
			n.handleClose(key)
		}
	}
}
