package dbus

// Offset of individual data of message signature
const (
	rawMsgTypeOffset = 1
	rawMsgFlagsOffset = 2
	rawMsgVersionOffset = 3
	rawMsgBodyLengthOffset = 4
	rawMsgSerialOffset = 8
	rawMsgFieldsArrayLenghtOffset = 12
	rawMessageFirstFieldOffset = 16
)

// Header fields type
const (
	HeaderFieldInvalid = iota
	HeaderFieldPath
	HeadeFieldInterface
	HeaderFieldMember
	HeaderFieldErrorName
	HeaderFieldReplaySerial
	HeaderFieldDestination
	HeaderFieldSender
	HeaderFieldSignature
	HeaderFieldUnixFDS
)

// Minimum Header Size (which is, of course, invalid)
// It can be used to calculate message size
const MinimumHeaderSize = 16

func headerSigFieldsLen(sig []byte) (int, error) {
	length, err := _GetInt32(sig, rawMsgFieldsArrayLenghtOffset)
	return int(length), err
}

func headerSigBodyLen(sig []byte) (int, error) {
	bl, err := _GetInt32(sig, rawMsgBodyLengthOffset)
	return int(bl), err
}

func headerSigPadLen(sig []byte) (int, error) {
	fieldsLen, err := headerSigFieldsLen(sig)
	if err != nil {
		return 0, err
	}

	headerLen := MinimumHeaderSize + fieldsLen
	return (_Align(8, headerLen) - headerLen), nil
}

func headerSigMsgSize(sig []byte) (int, error) {
	fieldArrLen, err := headerSigFieldsLen(sig)
	if err != nil {
		return 0, err
	}
	
	pad, err := headerSigPadLen(sig)
	if err != nil {
		return 0, err
	}

	bodyLen, err := headerSigBodyLen(sig)
	if err != nil {
		return 0, err
	}

	msgLen := MinimumHeaderSize + fieldArrLen + pad + bodyLen
	return msgLen, nil
}

type rawHeader struct {
	HeaderSignature []byte
	Fields []byte
}
type rawMessage struct {
	Header *rawHeader
	Body []byte
	MsgSize int
	Msg []byte
}

func newRawMessage(sig []byte) (*rawMessage, error) {
	r := &rawMessage{}

	fieldsArrayLen, err := headerSigFieldsLen(sig)
	if err != nil {
		return nil, err
	}

	pad, err := headerSigPadLen(sig)
	if err != nil {
		return nil, err
	}

	bodyLen, err := headerSigBodyLen(sig)
	if err != nil {
		return nil, err
	}
	msgSize := MinimumHeaderSize + fieldsArrayLen + pad + bodyLen
	r.MsgSize = msgSize

	slice := make([]byte, msgSize)
	r.Msg = slice

	r.Header = new(rawHeader)

	offset := 0
	r.Header.HeaderSignature = slice[offset:MinimumHeaderSize-4]

	offset = MinimumHeaderSize-4
	r.Header.Fields = slice[offset:offset+fieldsArrayLen+4]
	
	offset = offset + fieldsArrayLen
	r.Body = slice[offset:]

	// copy existing header signature without fields array length
	copy(r.Header.HeaderSignature, sig)

	// copy fields array length to Fields slice
	copy(r.Header.Fields, sig[MinimumHeaderSize-4:])

	return r, nil
}

func (self *rawMessage) Type() MessageType {
	t,_ := _GetByte(self.Header.HeaderSignature, rawMsgTypeOffset)
	return MessageType(t)
}

func (self *rawMessage) Flags() MessageFlag {
	f,_ := _GetByte(self.Header.HeaderSignature, rawMsgFlagsOffset)
	return MessageFlag(f)
}

func (self *rawMessage) ProtocolVersion() int {
	pv,_ := _GetByte(self.Header.HeaderSignature, rawMsgVersionOffset)
	return int(pv)
}

func (self *rawMessage) BodyLength() int {
	bl,_ := _GetInt32(self.Header.HeaderSignature, rawMsgBodyLengthOffset)
	return int(bl)
}

func (self *rawMessage) Serial() int {
	serial,_ := _GetInt32(self.Header.HeaderSignature, rawMsgSerialOffset)
	return int(serial)
}

func (self *rawMessage) FieldsArrayLength() int {
	length,_ := _GetInt32(self.Header.Fields, 0)
	return int(length)
}

func (self *rawMessage) PaddingBeforeBody() int {
	headerLen := MinimumHeaderSize + self.FieldsArrayLength()
	return (_Align(8, headerLen) - headerLen)
}

func (self *rawMessage) FieldsAndBodySlice() []byte {
	return self.Msg[MinimumHeaderSize:]
}
