package main

import (
	"encoding/binary"
	"errors"
)

const (
	CommonHeaderSize   = 5
	MinRelayHeaderSize = 4
)

var (
	ErrPacket = errors.New("unknown packet")
	ErrOpcode = errors.New("unknown opcode")
)

type Opcode byte

// Specification:
// Common Header:
// byte[0]: Opcode
// byte[1:5] : key size(uint32, 4 bytes)
// byte[5:5+n]: key
//
// New Opcode:
// byte[0:9]: CommonHeader
//
// Relay Opcode:
// byte[0:9]: CommonHeader
// byte[9:13]: Packet Size(uint32, 4 bytes)
// byte[13:n+13]: Packet
//
// Close Opcode:
// byte[0:9]: CommonHeader
const (
	New Opcode = iota
	Relay
	Close
)

type Header []byte

func NewHeader(h Header, op Opcode, key string) Header {
	if h == nil {
		h = make(Header, CommonHeaderSize)
	}
	h[0] = byte(op)
	binary.LittleEndian.PutUint32(h[1:5], uint32(len(key)))
	return h
}

func (h Header) Parse() (op Opcode, n int, err error) {
	if len(h) < CommonHeaderSize {
		err = ErrPacket
		return
	}
	op = Opcode(h[0])
	n = int(binary.LittleEndian.Uint32(h[1:5]))

	return
}
