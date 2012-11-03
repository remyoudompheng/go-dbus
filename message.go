package dbus

import (
	"bytes"
	"encoding/binary"
	"sync"
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
	Params      []interface{}
	serial      int
	replySerial uint32
	ErrorName   string
	//	Sender;
}

var serialMutex sync.Mutex
var messageSerial = int(0)

func _GetNewSerial() int {
	serialMutex.Lock()
	messageSerial++
	serial := messageSerial
	serialMutex.Unlock()
	return serial
}

// Create a new message with Flags == 0 and Protocol == 1.
func NewMessage() *Message {
	msg := new(Message)

	msg.serial = _GetNewSerial()
	msg.replySerial = 0
	msg.Flags = 0
	msg.Protocol = 1

	msg.Params = make([]interface{}, 0)

	return msg
}

func (p *Message) _BufferToMessage(buff []byte) (int, error) {
	msg := &msgData{Data: buff, Idx: 0}
	switch buff[0] {
	case 'l':
		msg.Endianness = binary.LittleEndian
	case 'B':
		msg.Endianness = binary.BigEndian
	}
	hdr, flds, err := msg.scanHeader()
	if err != nil {
		return 0, err
	}

	p.Type = MessageType(hdr.Type)
	p.Flags = MessageFlag(hdr.Flags)
	p.Protocol = int(hdr.Protocol)
	p.bodyLength = int(hdr.BodyLength)
	p.serial = int(hdr.Serial)

	p.Path = string(flds.Path)
	p.Iface = flds.Interface
	p.Member = flds.Member
	p.ErrorName = flds.ErrorName
	p.replySerial = flds.ReplySerial
	p.Dest = flds.Destination
	// FIXME = flds.Sender
	p.Sig = string(flds.Signature)
	// FIXME = flds.NumFDs

	msg.Round(8)
	idx := msg.Idx
	if 0 < p.bodyLength {
		p.Params, idx, _ = Parse(buff, p.Sig, idx)
	}
	return idx, nil
}

func _Unmarshal(buff []byte) (*Message, int, error) {
	msg := NewMessage()
	idx, e := msg._BufferToMessage(buff)
	if e != nil {
		return nil, 0, e
	}
	return msg, idx, nil
}

func (p *Message) _Marshal() ([]byte, error) {
	b := make([]byte, 0, 8+len(p.Dest)+len(p.Path)+len(p.Iface)+len(p.Member))
	buff := bytes.NewBuffer(b)
	buff.WriteByte(byte('l')) // little Endian
	buff.WriteByte(byte(p.Type))
	buff.WriteByte(byte(p.Flags))
	buff.WriteByte(byte(p.Protocol))

	tmpBuff := new(bytes.Buffer)
	_AppendParamsData(tmpBuff, p.Sig, p.Params)
	_AppendUint32(buff, uint32(len(tmpBuff.Bytes())))
	_AppendUint32(buff, uint32(p.serial))

	_AppendArray(buff, 1,
		func(b *bytes.Buffer) {
			if p.Path != "" {
				_AppendAlign(8, b)
				b.WriteByte(1) // path
				b.WriteByte(1) // signature size
				b.WriteByte('o')
				b.WriteByte(0)
				_AppendString(b, p.Path)
			}

			if p.Iface != "" {
				_AppendAlign(8, b)
				b.WriteByte(2) // interface
				b.WriteByte(1) // signature size
				b.WriteByte('s')
				b.WriteByte(0)
				_AppendString(b, p.Iface)
			}

			if p.Member != "" {
				_AppendAlign(8, b)
				b.WriteByte(3) // member
				b.WriteByte(1) // signature size
				b.WriteByte('s')
				b.WriteByte(0)
				_AppendString(b, p.Member)
			}

			if p.replySerial != 0 {
				_AppendAlign(8, b)
				b.WriteByte(5) // reply serial
				b.WriteByte(1) // signature size
				b.WriteByte('u')
				b.WriteByte(0)
				_AppendUint32(b, uint32(p.replySerial))
			}

			if p.Dest != "" {
				_AppendAlign(8, b)
				b.WriteByte(6) // destination
				b.WriteByte(1) // signature size
				b.WriteByte('s')
				b.WriteByte(0)
				_AppendString(b, p.Dest)
			}

			if p.Sig != "" {
				_AppendAlign(8, b)
				b.WriteByte(8) // signature
				b.WriteByte(1) // signature size
				b.WriteByte('g')
				b.WriteByte(0)
				_AppendSignature(b, p.Sig)
			}
		})

	_AppendAlign(8, buff)
	_AppendParamsData(buff, p.Sig, p.Params)

	return buff.Bytes(), nil
}
