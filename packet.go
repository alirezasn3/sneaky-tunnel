package main

type Packet struct {
	Payload []byte // max length : 1024*8 - 1 - 1
	ID      byte   // length : 1
	Flags   byte   // length : 1
	Buffer  []byte // to avoid allocation for each packet
}

func (p *Packet) EncodePacket() []byte {
	if len(p.Payload) > 8190 { // 8192 - 1 - 1
		panic("payload was larger than 8190 bytes")
	}
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
