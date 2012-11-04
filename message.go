package dbus

import (
	"encoding/binary"
	"sync/atomic"
)

// See the D-Bus tutorial for information about message types.
//		http://dbus.freedesktop.org/doc/dbus-tutorial.html#messages
type MessageType uint8

const (
	TypeInvalid MessageType = iota
	TypeMethodCall
	TypeMethodReturn
	TypeError
	TypeSignal
)

var messageTypeString = map[MessageType]string{
	TypeInvalid:      "invalid",
	TypeMethodCall:   "method_call",
	TypeMethodReturn: "method_return",
	TypeSignal:       "signal",
	TypeError:        "error",
}

func (t MessageType) String() string { return messageTypeString[t] }

type MessageFlag uint8

const (
	FlagNoReplyExpected MessageFlag = 1 << iota
	FlagNoAutoStart
)

type Message struct {
	Type        MessageType
	Flags       MessageFlag
	Protocol    int
	bodyLength  int
	Path        string
	Dest        string
	Iface       string
	Member      string
	Sig         string
	serial      uint32
	replySerial uint32
	ErrorName   string
	//	Sender;

	byteOrder binary.ByteOrder // Raw data byte order.
	raw       []byte           // Raw data.
	Params    []interface{}    // Unmarshaled contents.
}

var messageSerial = uint32(0)

func generateSerial() uint32 {
	return atomic.AddUint32(&messageSerial, 1)
}

// Create a new message with Flags == 0 and Protocol == 1.
func NewMessage() *Message {
	msg := new(Message)

	msg.serial = generateSerial()
	msg.replySerial = 0
	msg.Flags = 0
	msg.Protocol = 1

	msg.Params = make([]interface{}, 0)

	return msg
}

func newRawMessage(data []byte) (*Message, error) {
	msg := &msgData{Data: data, Idx: 0}
	switch data[0] {
	case 'l':
		msg.ByteOrder = binary.LittleEndian
	case 'B':
		msg.ByteOrder = binary.BigEndian
	}
	hdr, flds, err := msg.scanHeader()
	if err != nil {
		return nil, err
	}

	p := &Message{
		byteOrder:  msg.ByteOrder,
		Type:       MessageType(hdr.Type),
		Flags:      MessageFlag(hdr.Flags),
		Protocol:   int(hdr.Protocol),
		bodyLength: int(hdr.BodyLength),
		serial:     hdr.Serial,

		Path:        string(flds.Path),
		Iface:       flds.Interface,
		Member:      flds.Member,
		ErrorName:   flds.ErrorName,
		replySerial: flds.ReplySerial,
		Dest:        flds.Destination,
		// FIXME:  flds.Sender
		Sig: string(flds.Signature),
		// FIXME:  flds.NumFDs
	}

	msg.Round(8)
	p.raw = data[msg.Idx:]
	return p, nil
}

func (p *Message) parseParams() (err error) {
	if p.bodyLength > 0 {
		p.Params, _, err = Parse(p.raw, p.Sig, 0)
	}
	return
}

func unmarshal(buff []byte) (*Message, error) {
	msg, err := newRawMessage(buff)
	if err != nil {
		return msg, err
	}
	err = msg.parseParams()
	return msg, err
}

func (p *Message) _Marshal() ([]byte, error) {
	b := make([]byte, 0, 8+len(p.Dest)+len(p.Path)+len(p.Iface)+len(p.Member))
	hdr := msgHeader{
		ByteOrder: 'l',
		Type:      byte(p.Type),
		Flags:     byte(p.Flags),
		Protocol:  byte(p.Protocol),
		// Bodylength to fill later in buf[4:8]
		Serial: uint32(p.serial),
	}
	flds := msgHeaderFields{
		Path:        ObjectPath(p.Path),
		Interface:   p.Iface,
		Member:      p.Member,
		ErrorName:   p.ErrorName,
		ReplySerial: p.replySerial,
		Destination: p.Dest,
		Signature:   p.Sig,
		// Sender, NumFDs
	}

	msg := &msgData{
		ByteOrder: binary.LittleEndian,
		Data:      b, Idx: 0}
	err := msg.putHeader(hdr, flds)
	if err != nil {
		return nil, err
	}

	submsg := &msgData{ByteOrder: binary.LittleEndian}
	appendParamsData(submsg, p.Sig, p.Params)
	msg.ByteOrder.PutUint32(msg.Data[4:8], uint32(len(submsg.Data)))

	msg.Round(8)
	msg.Put(submsg.Data)

	return msg.Data, nil
}
