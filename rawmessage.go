package dbus

// Types
const (
	Byte        = 'y'
	Boolean     = 'b'
	Int16       = 'n'
	UInt16      = 'q'
	Int32       = 'i'
	UInt32      = 'u'
	Int64       = 'x'
	UInt64      = 't'
	Double      = 'd'
	String      = 's'
	ObjectPath  = 'o'
	Signature   = 'g'
	Array       = 'a'
	Variant     = 'v'
	StructBegin = '('
	StructEnd   = ')'
	DictBegin   = '{'
	DictEnd     = '}'
)

// Alignments of above types
var alignment map[byte]int = map[byte]int{
	Byte:        1,
	Boolean:     4,
	Int16:       2,
	UInt16:      2,
	Int32:       4,
	UInt32:      4,
	Int64:       8,
	UInt64:      8,
	Double:      8,
	String:      4,
	ObjectPath:  4,
	Signature:   1,
	Array:       4,
	StructBegin: 8,
	Variant:     1,
	DictBegin:   8,
}

// Offset of individual data of message signature
const (
	rawMsgTypeOffset              = 1
	rawMsgFlagsOffset             = 2
	rawMsgVersionOffset           = 3
	rawMsgBodyLengthOffset        = 4
	rawMsgSerialOffset            = 8
	rawMsgFieldsArrayLenghtOffset = 12
	rawMessageFirstFieldOffset    = 16
)

// Header fields type
const (
	HeaderFieldInvalid = iota
	HeaderFieldPath
	HeaderFieldInterface
	HeaderFieldMember
	HeaderFieldErrorName
	HeaderFieldReplySerial
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
	Fields          []byte
}

func (self *rawHeader) Type() MessageType {
	t, _ := _GetByte(self.HeaderSignature, rawMsgTypeOffset)
	return MessageType(t)
}

func (self *rawHeader) Flags() MessageFlag {
	f, _ := _GetByte(self.HeaderSignature, rawMsgFlagsOffset)
	return MessageFlag(f)
}

func (self *rawHeader) ProtocolVersion() int {
	pv, _ := _GetByte(self.HeaderSignature, rawMsgVersionOffset)
	return int(pv)
}

func (self *rawHeader) BodyLength() int {
	bl, _ := _GetInt32(self.HeaderSignature, rawMsgBodyLengthOffset)
	return int(bl)
}

func (self *rawHeader) Serial() int {
	serial, _ := _GetInt32(self.HeaderSignature, rawMsgSerialOffset)
	return int(serial)
}

func (self *rawHeader) FieldsArrayLength() int {
	length, _ := _GetInt32(self.Fields, 0)
	return int(length)
}

func (self *rawHeader) PaddingBeforeBody() int {
	headerLen := MinimumHeaderSize + self.FieldsArrayLength()
	return (_Align(8, headerLen) - headerLen)
}

type rawMessage struct {
	Header  *rawHeader
	Body    []byte
	MsgSize int
	Msg     []byte
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
	r.Header.HeaderSignature = slice[offset : MinimumHeaderSize-4]

	offset = MinimumHeaderSize - 4
	r.Header.Fields = slice[offset : offset+fieldsArrayLen+4]

	offset = offset + fieldsArrayLen
	r.Body = slice[offset:]

	// copy existing header signature without fields array length
	copy(r.Header.HeaderSignature, sig)

	// copy fields array length to Fields slice
	copy(r.Header.Fields, sig[MinimumHeaderSize-4:])

	return r, nil
}

func (self *rawMessage) FieldsAndBodySlice() []byte {
	return self.Msg[MinimumHeaderSize:]
}
