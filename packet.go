package main

type Packet struct {
	Payload []byte // max length : 1024*8 - 1 - 1
	ID      byte   // max length : 1
	Flags   byte   // max length : 1
}

func (p *Packet) EncodePacket() []byte {
	if len(p.Payload) > 8190 { // 8192 - 1 - 1
		panic("payload was larger than 8190 bytes")
	}
	var bytes []byte
	bytes = append(bytes, p.Flags)
	bytes = append(bytes, p.ID)
	bytes = append(bytes, p.Payload...)
	return bytes
}

func (p *Packet) DecodePacket(bytes []byte) {
	p.Flags = bytes[0]
	p.ID = bytes[1]
	p.Payload = bytes[2:]
}
