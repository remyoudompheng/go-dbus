package dbus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
)

func _Align(length int, index int) int {
	switch length {
	case 1:
		return index
	case 2, 4, 8:
		bit := length - 1
		return ^bit & (index + bit)
	}
	// default
	return -1
}

func _AppendAlign(length int, buff *bytes.Buffer) {
	padno := _Align(length, buff.Len()) - buff.Len()
	for i := 0; i < padno; i++ {
		buff.WriteByte(0)
	}
}

func _AppendString(buff *bytes.Buffer, str string) {
	_AppendAlign(4, buff)
	binary.Write(buff, binary.LittleEndian, int32(len(str)))
	buff.Write([]byte(str))
	buff.WriteByte(0)
}

func _AppendSignature(buff *bytes.Buffer, sig string) {
	_AppendByte(buff, byte(len(sig)))
	buff.Write([]byte(sig))
	buff.WriteByte(0)
}

func _AppendByte(buff *bytes.Buffer, b byte) { binary.Write(buff, binary.LittleEndian, b) }

func _AppendBoolean(buff *bytes.Buffer, b bool) {
	var val uint32 = 0
	if b {
		val = 1
	}
	_AppendUInt32(buff, val)
}

func _AppendUInt32(buff *bytes.Buffer, ui uint32) {
	_AppendAlign(4, buff)
	binary.Write(buff, binary.LittleEndian, ui)
}

func _AppendInt32(buff *bytes.Buffer, i int32) {
	_AppendAlign(4, buff)
	binary.Write(buff, binary.LittleEndian, i)
}

func _AppendInt64(buff *bytes.Buffer, i int64) {
	_AppendAlign(8, buff)
	binary.Write(buff, binary.LittleEndian, i)
}

func _AppendUInt64(buff *bytes.Buffer, u uint64) {
	_AppendAlign(8, buff)
	binary.Write(buff, binary.LittleEndian, u)
}

func _AppendDouble(buff *bytes.Buffer, f float64) {
	_AppendAlign(8, buff)
	binary.Write(buff, binary.LittleEndian, f)
}

func _AppendArray(buff *bytes.Buffer, align int, proc func(b *bytes.Buffer)) {
	_AppendAlign(4, buff)
	_AppendAlign(align, buff)
	b := bytes.NewBuffer(buff.Bytes())
	b.Write([]byte("ABCD")) // "ABCD" will be replaced with array-size.
	pos1 := b.Len()
	proc(b)
	pos2 := b.Len()
	binary.Write(buff, binary.LittleEndian, int32(pos2-pos1))
	buff.Write(b.Bytes()[pos1:pos2])
}

func _AppendValue(buff *bytes.Buffer, sig string, val interface{}) (sigOffset int, e error) {
	if len(sig) == 0 {
		return 0, errors.New("Invalid Signature")
	}

	e = nil

	switch sig[0] {
	case Byte:
		_AppendByte(buff, val.(byte))
		sigOffset = 1

	case String:
		_AppendString(buff, val.(string))
		sigOffset = 1

	case Boolean:
		_AppendBoolean(buff, val.(bool))
		sigOffset = 1

	case UInt32:
		_AppendUInt32(buff, val.(uint32))
		sigOffset = 1

	case Int32:
		_AppendInt32(buff, val.(int32))
		sigOffset = 1

	case Int64:
		_AppendInt64(buff, val.(int64))
		sigOffset = 1

	case UInt64:
		_AppendUInt64(buff, val.(uint64))
		sigOffset = 1

	case Double:
		_AppendDouble(buff, val.(float64))
		sigOffset = 1

	case Array: // ary
		sigBlock, _ := _GetSigBlock(sig, 1)
		_AppendArray(buff, 1, func(b *bytes.Buffer) {
			if slice, ok := val.([]interface{}); ok && slice != nil {
				for _, v := range slice {
					_AppendValue(b, sigBlock, v)
				}
			}
		})
		sigOffset = 1 + len(sigBlock)

	case StructBegin: // struct FIXME: nested struct not support
		_AppendAlign(8, buff)
		structSig, _ := _GetStructSig(sig, 0)
		for i, s := range structSig {
			_AppendValue(buff, string(s), val.([]interface{})[i])
		}
		sigOffset = 2 + len(structSig)

	case DictBegin:
		_AppendAlign(8, buff)
		dictSig, _ := _GetDictSig(sig, 0)
		for i, s := range dictSig {
			_AppendValue(buff, string(s), val.([]interface{})[i])
		}
		sigOffset = 2 + len(dictSig)
	}

	return
}

func _AppendParamsData(buff *bytes.Buffer, sig string, params []interface{}) {
	sigOffset := 0
	prmsOffset := 0
	sigLen := len(sig)
	for ; sigOffset < sigLen; prmsOffset++ {
		offset, _ := _AppendValue(buff, sig[sigOffset:len(sig)], params[prmsOffset])
		sigOffset += offset
	}
}
func _GetByte(buff []byte, index int) (byte, error) {
	if len(buff) <= index {
		return 0, errors.New("index error")
	}
	return buff[index], nil
}
func _GetInt32(buff []byte, index int) (int32, error) {
	if len(buff) <= index+4-1 {
		return 0, errors.New("index error")
	}
	var l int32
	e := binary.Read(bytes.NewBuffer(buff[index:len(buff)]), binary.LittleEndian, &l)
	if e != nil {
		return 0, e
	}
	return l, nil
}
func _GetStructSig(sig string, startIdx int) (string, error) {
	if len(sig) <= startIdx || '(' != sig[startIdx] {
		return "<nil>", errors.New("index error")
	}
	sigIdx := startIdx + 1
	for depth := 0; sigIdx < len(sig); sigIdx++ {
		switch sig[sigIdx] {
		case ')':
			if depth == 0 {
				return sig[startIdx+1 : sigIdx], nil
			}
			depth--
		case '(':
			depth++
		}
	}

	return "<nil>", errors.New("parse error")
}

func _GetDictSig(sig string, startIdx int) (string, error) {
	if len(sig) <= startIdx || '{' != sig[startIdx] {
		return "<nil>", errors.New("index error")
	}
	sigIdx := startIdx + 1
	for depth := 0; sigIdx < len(sig); sigIdx++ {
		switch sig[sigIdx] {
		case '}':
			if depth == 0 {
				return sig[startIdx+1 : sigIdx], nil
			}
			depth--
		case '{':
			depth++
		}
	}

	return "<nil>", errors.New("parse error")
}

func _GetSigBlock(sig string, index int) (string, error) {
	switch sig[index] {
	case '(':
		str, e := _GetStructSig(sig, index)
		if e != nil {
			return "", e
		}
		return strings.Join([]string{"(", str, ")"}, ""), nil

	case '{':
		str, e := _GetDictSig(sig, index)
		if e != nil {
			return "", e
		}
		return strings.Join([]string{"{", str, "}"}, ""), nil

	}

	// default
	return sig[index : index+1], nil
}

// EOM error is returned when iter is at the end
type EOM string

func (self EOM) Error() string {
	return "EOM"
}

// iterReader
type iterReader struct {
	data                      []byte
	signature                 string
	offset, sigOffset         int
	subReader                 *iterReader
	nextOffset, nextSigOffset int
	currValue                 interface{}
}

func newIterReader(data []byte, signature string) *iterReader {
	return newIterReaderWithOffsets(data, signature, 0, 0)
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
	return string(self.data[4+self.offset : offsetEnd]), offsetEnd + 1, nil
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
	return string(self.data[1+self.offset : offsetEnd]), offsetEnd + 1, nil
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
				return self.signature[self.sigOffset+1 : i], i + 1, nil
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
				return self.signature[self.sigOffset+1 : i], i + 1, nil
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
		return string(StructBegin) + sig + string(StructEnd), newSigOffset, nil

	case DictBegin:
		sig, newSigOffset, err := self.getDictSig()
		if err != nil {
			return "", 0, err
		}
		return string(DictBegin) + sig + string(DictEnd), newSigOffset, nil
	}

	return self.signature[self.sigOffset : self.sigOffset+1], self.sigOffset + 1, nil
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

	for {
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
func Parse(buff []byte, sig string) (slice []interface{}, err error) {
	slice = make([]interface{}, 0)
	iter := newIterReader(buff, sig)

	for {
		slice = append(slice, iter.Value())

		e := iter.Next()
		if _, ok := e.(EOM); ok {
			break
		}

		if e != nil {
			return nil, e
		}

	}

	return slice, nil
}
