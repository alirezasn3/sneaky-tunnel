package main

import "bytes"

// flags:
// 0 -> no flags
// 1 -> dummy
// 2 -> keep-alive
// 3 -> close connection
// 4 -> destination port announcement
// 5 -> keep-alive response
// 6 -> mode announcement: check first byte of Payload: 1 -> tunnel mode, 2 -> vpn mode
type Packet struct {
	Payload []byte        // max length : 1024*8 - 1 - 1 = 8190
	ID      byte          // length : 1
	Flags   byte          // length : 1
	Buffer  *bytes.Buffer // to avoid allocation for each packet
}

func (p *Packet) EncodePacket() {
	p.Buffer.Reset()
	p.Buffer.WriteByte(p.Flags)
	p.Buffer.WriteByte(p.ID)
	p.Buffer.Write(p.Payload)
}

func (p *Packet) DecodePacket() {
	p.Flags = p.Payload[0]
	p.ID = p.Payload[1]
	p.Payload = p.Payload[2:]
}

func ByteSliceToUint16(byteSlice []byte) uint16 {
	return uint16(byteSlice[0]) | uint16(byteSlice[1])<<8
}

func Uint16ToByteSlice(n uint16) []byte {
	temp := []byte{}
	return append(temp, byte(n), byte(n>>8))
}
