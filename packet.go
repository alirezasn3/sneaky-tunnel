package main

import "crypto/rand"

// flags:
// 0 -> no flags
// 1 -> dummy
// 2 -> keep-alive
// 3 -> close connection
// 4 -> destination port announcement
// 5 -> keep-alive response
type Packet struct {
	Payload                []byte // max length : 1024*8 - 1 - 1 - 1 - 1 - 16 - 16 = 8156
	ID                     byte   // length : 1
	Flags                  byte   // length : 1
	Buffer                 []byte // to avoid allocation for each packet
	BeginningPaddingLength byte   // length : 1
	EndPaddingLength       byte   // length : 1
	BeginningPaddingBytes  []byte // min length : 8 | max length: 16
	EndPaddingBytes        []byte // min length : 8 | max length: 16
}

func createPacket() Packet {
	var p Packet
	p.ID = 0
	p.Flags = 0
	p.BeginningPaddingLength = 12
	p.EndPaddingLength = 12
	p.BeginningPaddingBytes = make([]byte, 12)
	p.EndPaddingBytes = make([]byte, 12)
	return p
}

func (p *Packet) EncodePacket() []byte {
	rand.Read(p.BeginningPaddingBytes)
	rand.Read(p.EndPaddingBytes)
	p.Buffer = []byte{}
	p.Buffer = append(p.Buffer, p.Flags)
	p.Buffer = append(p.Buffer, p.ID)
	p.Buffer = append(p.Buffer, p.BeginningPaddingLength)
	p.Buffer = append(p.Buffer, p.EndPaddingLength)
	p.Buffer = append(p.Buffer, p.BeginningPaddingBytes...)
	p.Buffer = append(p.Buffer, p.Payload...)
	p.Buffer = append(p.Buffer, p.EndPaddingBytes...)
	return p.Buffer
}

func (p *Packet) DecodePacket(bytes []byte) {
	p.Flags = bytes[0]
	p.ID = bytes[1]
	p.BeginningPaddingLength = bytes[2]
	p.EndPaddingLength = bytes[3]
	p.Payload = bytes[4+p.BeginningPaddingLength : len(bytes)-int(p.EndPaddingLength)]
}

func ByteSliceToUint16(byteSlice []byte) uint16 {
	return uint16(byteSlice[0]) | uint16(byteSlice[1])<<8
}

func Uint16ToByteSlice(n uint16) []byte {
	temp := []byte{}
	return append(temp, byte(n), byte(n>>8))
}
