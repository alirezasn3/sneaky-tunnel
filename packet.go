package main

// flags:
// 0 -> no flags
// 1 -> dummy
// 2 -> keep-alive
// 3 -> close connection
type Packet struct {
	Payload         []byte // max length : 1024*8 - 1 - 1 - 2 = 8188
	ID              byte   // length : 1
	Flags           byte   // length : 1
	DestinationPort uint16 // length : 2
	Buffer          []byte // to avoid allocation for each packet
}

func (p *Packet) EncodePacket() []byte {
	if len(p.Payload) > 8188 {
		panic("payload was larger than 8190 bytes")
	}
	p.Buffer = []byte{}
	p.Buffer = append(p.Buffer, p.Flags)
	p.Buffer = append(p.Buffer, p.ID)
	p.Buffer = append(p.Buffer, byte(p.DestinationPort), byte(p.DestinationPort>>8))
	p.Buffer = append(p.Buffer, p.Payload...)
	return p.Buffer
}

func (p *Packet) DecodePacket(bytes []byte) {
	p.Flags = bytes[0]
	p.ID = bytes[1]
	p.DestinationPort = uint16(bytes[2]) | uint16(bytes[3])<<8
	p.Payload = bytes[4:]
}
