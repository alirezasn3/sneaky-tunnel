package main

// flags:
// 0 -> no flags
// 1 -> dummy
// 2 -> keep-alive
// 3 -> close connection
// 4 -> destination port announcement
type Packet struct {
	Payload []byte // max length : 1024*8 - 1 - 1 = 8190
	ID      byte   // length : 1
	Flags   byte   // length : 1
	Buffer  []byte // to avoid allocation for each packet
}

func (p *Packet) EncodePacket() []byte {
	p.Buffer = []byte{}
	p.Buffer = append(p.Buffer, p.Flags)
	p.Buffer = append(p.Buffer, p.ID)
	p.Buffer = append(p.Buffer, p.Payload...)
	return p.Buffer
}

func (p *Packet) DecodePacket(bytes []byte) {
	p.Flags = bytes[0]
	p.ID = bytes[1]
	p.Payload = bytes[2:]
}

func ByteSliceToUint16(byteSlice []byte) uint16 {
	return uint16(byteSlice[2]) | uint16(byteSlice[3])<<8
}

func Uint16ToByteSlice(n uint16) []byte {
	temp := make([]byte, 2)
	return append(temp, byte(n), byte(n>>8))
}
