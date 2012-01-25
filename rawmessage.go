package dbus

import "encoding/binary"
import "errors"

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

type ReadError string

func NewReadError(msg string) ReadError {
	var re ReadError
	re = "Read Error: " + ReadError(msg)
	return re
}

func (self ReadError) Error() string {
	return string(self)
}

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

type rawMessage struct {
	Header  []byte
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

	r.Header = slice[:msgSize-bodyLen]

	r.Body = slice[msgSize-bodyLen:]

	// copy existing header signature without fields array length
	copy(r.Header, sig)

	return r, nil
}

func (self *rawMessage) Type() MessageType {
	t, _ := _GetByte(self.Header, rawMsgTypeOffset)
	return MessageType(t)
}

func (self *rawMessage) Flags() MessageFlag {
	f, _ := _GetByte(self.Header, rawMsgFlagsOffset)
	return MessageFlag(f)
}

func (self *rawMessage) ProtocolVersion() int {
	pv, _ := _GetByte(self.Header, rawMsgVersionOffset)
	return int(pv)
}

func (self *rawMessage) BodyLength() int {
	bl, _ := _GetInt32(self.Header, rawMsgBodyLengthOffset)
	return int(bl)
}

func (self *rawMessage) Serial() int {
	serial, _ := _GetInt32(self.Header, rawMsgSerialOffset)
	return int(serial)
}

func (self *rawMessage) FieldsArrayLength() int {
	length, _ := _GetInt32(self.Header, rawMsgFieldsArrayLenghtOffset)
	return int(length)
}

func (self *rawMessage) PaddingBeforeBody() int {
	headerLen := MinimumHeaderSize + self.FieldsArrayLength()
	return (_Align(8, headerLen) - headerLen)
}
func (self *rawMessage) FieldsAndBodySlice() []byte {
	return self.Msg[MinimumHeaderSize:]
}

func (self *rawMessage) HeaderIterReader() *iterReader {
	return newIterReader(self.Header, "yyyyuua(yv)")
}

func (self *rawMessage) BodyIterReader(signature string) *iterReader {
	return newIterReader(self.Body, signature)
}

// EOM error is returned when iter is at the end
type EOM string

func (self EOM) Error() string {
	return "EOM"
}

// iterReader
type iterReader struct {
	data []byte
	signature string
	offset, sigOffset int
	subReader *iterReader
	nextOffset, nextSigOffset int
	currValue interface{}
}

func newIterReader(data []byte, signature string) *iterReader {
	ir := &iterReader{}
	ir.data = data
	ir.signature = signature
	ir.offset = 0
	ir.sigOffset = 0

	if err := ir.Reinit(); err != nil {
		return nil
	}

	return ir
}

func newIterReaderWithOffsets(data []byte, signature string, offset, sigOffset int) *iterReader {
	ir := &iterReader{}
	ir.data = data
	ir.signature = signature
	ir.offset = offset
	ir.sigOffset = sigOffset

	if err := ir.Reinit(); err != nil {
		return nil
	}

	return ir
}

// Resets offset variables
func (self *iterReader) Reset() {
	self.offset = 0
	self.sigOffset = 0
}

func (self *iterReader) GetOffset() int {
	return self.offset
}

func (self *iterReader) GetSigOffset() int {
	return self.sigOffset
}

func (self *iterReader) GetSignature() string {
	return self.signature
}

func (self *iterReader) GetCurrentType() byte {
	return self.signature[self.sigOffset]
}

func (self iterReader) Read(p []byte) (n int, e error) {
	dataLen := len(self.data) - self.offset
	inLen := len(p)
	if dataLen > 0 && dataLen >= inLen {
		n = copy(p, self.data[self.offset:])
		return n, nil
	}

	return 0, NewReadError("Insufficient bytes to read")
}

func (self *iterReader) ReadByte() (byte, error) {
	var val byte
	e := binary.Read(self, binary.LittleEndian, &val)
	return val, e
}

func (self *iterReader) ReadInt16() (int16, error) {
	var val int16
	e := binary.Read(self, binary.LittleEndian, &val)
	return val, e
}

func (self *iterReader) ReadUInt16() (uint16, error) {
	var val uint16
	e := binary.Read(self, binary.LittleEndian, &val)
	return val, e
}

func (self *iterReader) ReadInt32() (int32, error) {
	var val int32
	e := binary.Read(self, binary.LittleEndian, &val)
	return val, e
}

func (self *iterReader) ReadUInt32() (uint32, error) {
	var val uint32
	e := binary.Read(self, binary.LittleEndian, &val)
	return val, e
}

func (self *iterReader) ReadInt64() (int64, error) {
	var val int64
	e := binary.Read(self, binary.LittleEndian, &val)
	return val, e
}

func (self *iterReader) ReadUInt64() (uint64, error) {
	var val uint64
	e := binary.Read(self, binary.LittleEndian, &val)
	return val, e
}

func (self *iterReader) ReadDouble() (float64, error) {
	var val float64
	e := binary.Read(self, binary.LittleEndian, &val)
	return val, e
}

func (self *iterReader) ReadBool() (bool, error) {
	var val uint32
	e := binary.Read(self, binary.LittleEndian, &val)
	return val != 0, e
}

func (self *iterReader) ReadString() (string, int, error) {
	strLen, err := self.ReadUInt32()
	if err != nil {
		return "", 0, err
	}

	offsetEnd := self.offset + 4 + int(strLen)
	if offsetEnd >= len(self.data) {
		return "", 0, ReadError("Insufficient bytes to read")
	}
    return string(self.data[4+self.offset:offsetEnd]), offsetEnd+1, nil
}

func (self *iterReader) ReadSignature() (string, int, error) {
	sigLen, err := self.ReadByte()
	if err != nil {
		return "", 0, err
	}

	offsetEnd := self.offset + 1 + int(sigLen)
	if offsetEnd >= len(self.data) {
		return "", 0, ReadError("Insufficient bytes to read")
	}
	return string(self.data[1+self.offset:offsetEnd]), offsetEnd+1, nil
}

func (self *iterReader) ReadVariant() (interface{}, int, error) {
	sig, newOffset, err := self.ReadSignature()
	if err != nil {
		return nil, 0, err
	}

	self.subReader = newIterReaderWithOffsets(self.data, sig, newOffset, 0)

	return self.subReader.currValue, self.subReader.nextOffset, nil
}

func (self *iterReader) getStructSig() (string, int, error) {
	if self.signature[self.sigOffset] != StructBegin {
		return "", 0, errors.New("Parse error: current signature not struct")
	}

	i := self.sigOffset + 1
	sigLen := len(self.signature)
	for depth := 0; i < sigLen; i++ {
		switch self.signature[i] {
		case StructEnd:
			if depth == 0 {
				return self.signature[self.sigOffset+1:i], i+1, nil
			}
			depth--

		case StructBegin:
			depth++
		}
	}

	return "", 0, errors.New("Parse error: No struct end byte in signature")
}

// ReadStruct reads dbus struct into slice of values
// It returns slice, new offset, new signature offset and possibly error
func (self *iterReader) ReadStruct() ([]interface{}, int, int, error) {
	sig, newSigOffset, err := self.getStructSig()
	if err != nil {
		return nil, 0, 0, err
	}

	self.subReader = newIterReaderWithOffsets(self.data, sig, self.offset, 0)

	slice := []interface{}{}

	for {
		slice = append(slice, self.subReader.Value())

		err = self.subReader.Next()
		if _, ok := err.(EOM); ok {
			break
		}

		if err != nil {
			return nil, 0, 0, err
		}
	}

	return slice, self.subReader.nextOffset, newSigOffset, nil
}

func (self *iterReader) getDictSig() (string, int, error) {
	if self.signature[self.sigOffset] != DictBegin {
		return "", 0, errors.New("Parse error: current signature not dictionary")
	}

	i := self.sigOffset + 1
	sigLen := len(self.signature)
	for depth := 0; i < sigLen; i++ {
		switch self.signature[i] {
		case DictEnd:
			if depth == 0 {
				return self.signature[self.sigOffset+1:i], i+1, nil
			}
			depth--

		case DictBegin:
			depth++
		}
	}

	return "", 0, errors.New("Parse error: No Dict ending in signature")
}

// ReadDict reads dbus dictionary into slice of values
// It returns slice, new offset, new signature offset and possibly error
func (self *iterReader) ReadDict() ([]interface{}, int, int, error) {
	sig, newSigOffset, err := self.getDictSig()
	if err != nil {
		return nil, 0, 0, err
	}

	self.subReader = newIterReaderWithOffsets(self.data, sig, self.offset, 0)

	slice := []interface{}{}

	for {
		slice = append(slice, self.subReader.Value())

		err = self.subReader.Next()
		if _, ok := err.(EOM); ok {
			break
		}

		if err != nil {
			return nil, 0, 0, err
		}
	}

	return slice, self.subReader.nextOffset, newSigOffset, nil
}

func (self *iterReader) getArraySig() (string, int, error) {
	if self.signature[self.sigOffset] != Array {
		return "", 0, errors.New("Parse error: current signature not array")
	}

	self.sigOffset += 1

	switch self.signature[self.sigOffset] {
	case StructBegin:
		sig, newSigOffset, err := self.getStructSig()
		if err != nil {
			return "", 0, err
		}
		return string(StructBegin)+sig+string(StructEnd), newSigOffset, nil
	}

	return self.signature[self.sigOffset:self.sigOffset+1], self.sigOffset+1, nil
}

func (self *iterReader) ReadArray() ([]interface{}, int, int, error) {
	arrayLen, err := self.ReadUInt32()
	if err != nil {
		return nil, 0, 0, err
	}

	arraySig, newSigOffset, err := self.getArraySig()
	if err != nil {
		return nil, 0, 0, err
	}

	self.subReader = newIterReaderWithOffsets(self.data, arraySig, self.offset+4, 0)

	slice := []interface{}{}
	endOffset := self.offset + 4 + int(arrayLen)

	for  {
		slice = append(slice, self.subReader.Value())

		self.subReader.nextSigOffset = 0

		if self.subReader.nextOffset < endOffset {
			err = self.subReader.Next()
			if err != nil {
				return nil, 0, 0, err
			}
			continue
		}
		break
	}

	return slice, endOffset, newSigOffset, nil
}

func (self *iterReader) RecalculateOffset() error {
	currType := self.signature[self.sigOffset]

	if align, ok := alignment[currType]; ok {
		newOffset := _Align(align, self.offset)

		if newOffset >= len(self.data) {
			return ReadError("Not enough bytes to read")
		}

		if newOffset >= 0 {
			self.offset = newOffset
			return nil
		}
	}

	return errors.New("Unknown type" + string(currType))
}

func (self *iterReader) Reinit() (err error) {
	if err = self.RecalculateOffset(); err != nil {
		return err
	}

	return self.fillValue()
}

// Next goes to the next value in signature
// It returns EOM (end of message) error at the end
func (self *iterReader) Next() error {

	if self.nextSigOffset >= len(self.signature) {
		return EOM("")
	}
	self.sigOffset = self.nextSigOffset


	if self.nextOffset >= len(self.data) {
		return ReadError("Signature longer than data length")
	}

	self.offset = self.nextOffset
	if err := self.RecalculateOffset(); err != nil {
		return err
	}

	// Reads new value according to signature
	if err := self.fillValue(); err != nil {
		return err
	}

	return nil
}

// fillValue read current value into currValue member
// and sets offsets for the next value
func (self *iterReader) fillValue() error {
	var val interface{}
	var err error = nil

	switch self.signature[self.sigOffset] {
	case Boolean:
		if val, err = self.ReadBool(); err == nil {
			self.currValue = val
			self.nextOffset = self.offset + alignment[Boolean]
			self.nextSigOffset = self.sigOffset + 1
		}

	case Byte:
		if val, err = self.ReadByte(); err == nil {
			self.currValue = val
			self.nextOffset = self.offset + alignment[Byte]
			self.nextSigOffset = self.sigOffset + 1
		}

	case Int16:
		if val, err = self.ReadInt16(); err == nil {
			self.currValue = val
			self.nextOffset = self.offset + alignment[Int16]
			self.nextSigOffset = self.sigOffset + 1
		}

	case UInt16:
		if val, err = self.ReadUInt16(); err == nil {
			self.currValue = val
			self.nextOffset = self.offset + alignment[UInt16]
			self.nextSigOffset = self.sigOffset + 1
		}

	case Int32:
		if val, err = self.ReadInt32(); err == nil {
			self.currValue = val
			self.nextOffset = self.offset + alignment[Int32]
			self.nextSigOffset = self.sigOffset + 1
		}

	case UInt32:
		if val, err = self.ReadUInt32(); err == nil {
			self.currValue = val
			self.nextOffset = self.offset + alignment[UInt32]
			self.nextSigOffset = self.sigOffset + 1
		}

	case Int64:
		if val, err = self.ReadInt64(); err == nil {
			self.currValue = val
			self.nextOffset = self.offset + alignment[Int64]
			self.nextSigOffset = self.sigOffset + 1
		}

	case UInt64:
		if val, err = self.ReadUInt64(); err == nil {
			self.currValue = val
			self.nextOffset = self.offset + alignment[UInt64]
			self.nextSigOffset = self.sigOffset + 1
		}

	case Double:
		if val, err = self.ReadDouble(); err == nil {
			self.currValue = val
			self.nextOffset = self.offset + alignment[Double]
			self.nextSigOffset = self.sigOffset + 1
		}

	case String, ObjectPath:
		var newOffset int
		if val, newOffset, err = self.ReadString(); err == nil {
			self.currValue = val
			self.nextOffset = newOffset
			self.nextSigOffset = self.sigOffset + 1
		}

	case Signature:
		var newOffset int
		if val, newOffset, err = self.ReadSignature(); err == nil {
			self.currValue = val
			self.nextOffset = newOffset
			self.nextSigOffset = self.sigOffset + 1
		}

	case Variant:
		var newOffset int
		if val, newOffset, err = self.ReadVariant(); err == nil {
			self.currValue = val
			self.nextOffset = newOffset
			self.nextSigOffset = self.sigOffset + 1
		}

	case StructBegin:
		var newOffset, newSigOffset int
		if val, newOffset, newSigOffset, err = self.ReadStruct(); err == nil {
			self.currValue = val
			self.nextOffset = newOffset
			self.nextSigOffset = newSigOffset
		}

	case DictBegin:
		var newOffset, newSigOffset int
		if val, newOffset, newSigOffset, err = self.ReadDict(); err == nil {
			self.currValue = val
			self.nextOffset = newOffset
			self.nextSigOffset = newSigOffset
		}

	case Array:
		var newOffset, newSigOffset int
		if val, newOffset, newSigOffset, err = self.ReadArray(); err == nil {
			self.currValue = val
			self.nextOffset = newOffset
			self.nextSigOffset = newSigOffset
		}

	default:
		return ReadError("Value type unknown: " + string(self.signature[self.sigOffset]))
	}

	return err
}

// Value returns value at current offset (according to signature)
func (self *iterReader) Value() interface{} {
	return self.currValue
}

// NextValue moves iter to the next value and returns it
// This is identical to Next + Value
func (self *iterReader) NextValue() (val interface{}, e error) {
	e = self.Next()
	if e != nil {
		return nil, e
	}

	val = self.Value()

	return
}
