package dbus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
)

var errIndex = errors.New("index error")

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
	_AppendInt32(buff, int32(len(str)))
	buff.WriteString(str)
	buff.WriteByte(0)
}

func _AppendSignature(buff *bytes.Buffer, sig string) {
	buff.WriteByte(byte(len(sig)))
	buff.WriteString(sig)
	buff.WriteByte(0)
}

func _AppendUint32(buff *bytes.Buffer, ui uint32) {
	_AppendAlign(4, buff)
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], ui)
	buff.Write(b[:])
}

func _AppendInt32(buff *bytes.Buffer, i int32) {
	_AppendAlign(4, buff)
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(i))
	buff.Write(b[:])
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
	case 'y': // byte
		buff.WriteByte(val.(byte))
		sigOffset = 1

	case 's': // string
		_AppendString(buff, val.(string))
		sigOffset = 1

	case 'u': // uint32
		_AppendUint32(buff, val.(uint32))
		sigOffset = 1

	case 'i': // int32
		_AppendInt32(buff, val.(int32))
		sigOffset = 1

	case 'a': // ary
		sigBlock, _ := _GetSigBlock(sig, 1)
		_AppendArray(buff, 1, func(b *bytes.Buffer) {
			if slice, ok := val.([]interface{}); ok && slice != nil {
				for _, v := range slice {
					_AppendValue(b, sigBlock, v)
				}
			}
		})
		sigOffset = 1 + len(sigBlock)

	case '(': // struct FIXME: nested struct not support
		_AppendAlign(8, buff)
		structSig, _ := _GetStructSig(sig, 0)
		for i, s := range structSig {
			_AppendValue(buff, string(s), val.([]interface{})[i])
		}
		sigOffset = 2 + len(structSig)

	case '{':
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
		return 0, errIndex
	}
	return buff[index], nil
}

func _GetInt16(buff []byte, index int) (int16, error) {
	if len(buff) < index+2 {
		return 0, errIndex
	}
	return int16(binary.LittleEndian.Uint16(buff[index:])), nil
}

func _GetUint16(buff []byte, index int) (uint16, error) {
	if len(buff) < index+2 {
		return 0, errIndex
	}
	return binary.LittleEndian.Uint16(buff[index:]), nil
}

func _GetInt32(buff []byte, index int) (int32, error) {
	if len(buff) < index+4 {
		return 0, errIndex
	}
	return int32(binary.LittleEndian.Uint32(buff[index:])), nil
}

func _GetUint32(buff []byte, index int) (uint32, error) {
	if len(buff) < index+4 {
		return 0, errIndex
	}
	return binary.LittleEndian.Uint32(buff[index:]), nil
}

func _GetBoolean(buff []byte, index int) (bool, error) {
	if len(buff) < index+4 {
		return false, errIndex
	}
	v := binary.LittleEndian.Uint32(buff[index:])
	return v != 0, nil
}

func _GetString(buff []byte, index int, size int) (string, error) {
	if len(buff) < index+size {
		return "", errIndex
	}
	return string(buff[index : index+size]), nil
}

func _GetStructSig(sig string, startIdx int) (string, error) {
	if len(sig) <= startIdx || '(' != sig[startIdx] {
		return "<nil>", errIndex
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
		return "<nil>", errIndex
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

func _GetVariant(buff []byte, index int) (vals []interface{}, retidx int, e error) {
	retidx = index
	sigSize := int(buff[retidx])
	retidx++
	sig := string(buff[retidx : retidx+sigSize])
	vals, retidx, e = Parse(buff, sig, retidx+sigSize+1)
	return
}

func Parse(buff []byte, sig string, index int) (slice []interface{}, bufIdx int, err error) {
	slice = make([]interface{}, 0, len(sig))
	bufIdx = index
	for sigIdx := 0; sigIdx < len(sig); {
		switch sig[sigIdx] {
		case 'b': // bool
			bufIdx = _Align(4, bufIdx)
			b, e := _GetBoolean(buff, bufIdx)
			if e != nil {
				err = e
				return
			}
			slice = append(slice, bool(b))
			bufIdx += 4
			sigIdx++

		case 'y': // byte
			v, e := _GetByte(buff, bufIdx)
			if e != nil {
				err = e
				return
			}
			slice = append(slice, v)
			bufIdx++
			sigIdx++

		case 'n': // int16
			bufIdx = _Align(2, bufIdx)
			n, e := _GetInt16(buff, bufIdx)
			if e != nil {
				err = e
				return
			}
			slice = append(slice, n)
			bufIdx += 2
			sigIdx++

		case 'q': // uint16
			bufIdx = _Align(2, bufIdx)
			q, e := _GetUint16(buff, bufIdx)
			if e != nil {
				err = e
				return
			}
			slice = append(slice, q)
			bufIdx += 2
			sigIdx++

		case 'u': // uint32
			bufIdx = _Align(4, bufIdx)
			u, e := _GetUint32(buff, bufIdx)
			if e != nil {
				err = e
				return
			}
			slice = append(slice, u)
			bufIdx += 4
			sigIdx++

		case 's', 'o': // string, object
			bufIdx = _Align(4, bufIdx)

			size, e := _GetInt32(buff, bufIdx)
			if e != nil {
				err = e
				return
			}

			str, e := _GetString(buff, bufIdx+4, int(size))
			if e != nil {
				err = e
				return
			}
			slice = append(slice, str)
			bufIdx += (4 + int(size) + 1)
			sigIdx++

		case 'g': // signature
			size, e := _GetByte(buff, bufIdx)
			if e != nil {
				err = e
				return
			}

			str, e := _GetString(buff, bufIdx+1, int(size))
			if e != nil {
				err = e
				return
			}
			slice = append(slice, str)
			bufIdx += (1 + int(size) + 1)
			sigIdx++

		case 'a': // array
			startIdx := _Align(4, bufIdx)
			arySize, e := _GetInt32(buff, startIdx)
			if e != nil {
				err = e
				return
			}

			sigBlock, e := _GetSigBlock(sig, sigIdx+1)
			if e != nil {
				err = e
				return
			}

			aryIdx := startIdx + 4
			tmpSlice := make([]interface{}, 0)
			for aryIdx < (startIdx+4)+int(arySize) {
				retSlice, retidx, e := Parse(buff, sigBlock, aryIdx)
				if e != nil {
					err = e
					return
				}
				tmpSlice = append(tmpSlice, retSlice...)
				aryIdx = retidx
			}
			bufIdx = aryIdx
			sigIdx += (1 + len(sigBlock))
			slice = append(slice, tmpSlice)

		case '(': // struct
			idx := _Align(8, bufIdx)
			stSig, e := _GetStructSig(sig, sigIdx)
			if e != nil {
				err = e
				return
			}

			retSlice, retidx, e := Parse(buff, stSig, idx)
			if e != nil {
				err = e
				return
			}

			bufIdx = retidx
			sigIdx += (len(stSig) + 2)
			slice = append(slice, retSlice)

		case '{': // dict
			idx := _Align(8, bufIdx)
			stSig, e := _GetDictSig(sig, sigIdx)
			if e != nil {
				err = e
				return
			}

			retSlice, retidx, e := Parse(buff, stSig, idx)
			if e != nil {
				err = e
				return
			}

			bufIdx = retidx
			sigIdx += (len(stSig) + 2)
			slice = append(slice, retSlice)

		case 'v': // variant
			vals, idx, e := _GetVariant(buff, bufIdx)
			if e != nil {
				err = e
				return
			}

			bufIdx = idx
			sigIdx++
			slice = append(slice, vals...)

		default:
			fmt.Println(sig[sigIdx])
			return nil, index, errors.New("unknown type")
		}
	}
	return
}

// The D-Bus message header. A message consists of this and an array
// of byte and variants.
type msgHeader struct {
	Endianness byte
	Type       byte
	Flags      byte
	Protocol   byte
	BodyLength uint32
	Serial     uint32
}

type ObjectPath string
type Signature string

type msgHeaderFields struct {
	Path        ObjectPath // field 1
	Interface   string
	Member      string
	ErrorName   string
	ReplySerial uint32
	Destination string
	Sender      string
	Signature   Signature
	NumFD       uint32 // field 9
}

type msgData struct {
	Endianness binary.ByteOrder

	Data []byte
	Idx  int
}

func (msg *msgData) Round(rnd int) {
	switch rnd {
	case 1:
		// nothing.
	case 2, 4, 8:
		bit := rnd - 1
		msg.Idx = ^bit & (msg.Idx + bit)
	default:
		panic("invalid Round argument")
	}
}

func (msg *msgData) Next(n int) []byte {
	s := msg.Data[msg.Idx:]
	msg.Idx += n
	return s
}

func (msg *msgData) scanHeader() (hdr msgHeader, flds msgHeaderFields, err error) {
	// The fixed header.
	msg.scan("(yyyyuu)", &hdr)
	// Now an array of byte and variant.
	fldVal := reflect.ValueOf(&flds).Elem()
	msg.Round(4)
	fldLen := msg.Endianness.Uint32(msg.Next(4))
	fldEnd := msg.Idx + int(fldLen)
	for msg.Idx < fldEnd {
		// A field is a struct byte + variant, hence aligned on 8 bytes.
		msg.Round(8)
		b := msg.Next(1)[0]
		// A variant is a signature and value.
		var fldSig Signature
		msg.scan("g", &fldSig)
		msg.scanValue(fldSig, fldVal.Field(int(b)-1))
	}
	return
}

func (msg *msgData) scan(sig Signature, val interface{}) (err error) {
	return msg.scanValue(sig, reflect.ValueOf(val).Elem())
}

func (msg *msgData) scanMany(sig Signature, val ...reflect.Value) (err error) {
	i := 0
	for len(sig) > 0 {
		item, _ := _GetSigBlock(string(sig), 0)
		sig = sig[len(item):]
		msg.scanValue(Signature(item), val[i])
		i++
	}
	return
}

// scan reads data from buf according to the first item in signature sig and fills val.
// It returns the number of bytes consumed.
// http://dbus.freedesktop.org/doc/dbus-specification.html#type-system
func (msg *msgData) scanValue(sig Signature, val reflect.Value) (err error) {
	switch sig[0] {
	case 'y': // byte
		val.SetUint(uint64(msg.Data[msg.Idx]))
		msg.Idx++

	case 'b': // bool
		msg.Round(4)
		x := msg.Endianness.Uint32(msg.Next(4))
		val.SetBool(x != 0)

	case 'n': // int16
		msg.Round(2)
		x := msg.Endianness.Uint16(msg.Next(2))
		val.SetInt(int64(x))
	case 'q': // uint16
		msg.Round(2)
		x := msg.Endianness.Uint16(msg.Next(2))
		val.SetUint(uint64(x))

	case 'i': // int32
		msg.Round(4)
		x := msg.Endianness.Uint32(msg.Next(4))
		val.SetInt(int64(x))
	case 'u': // uint32
		msg.Round(4)
		x := msg.Endianness.Uint32(msg.Next(4))
		val.SetUint(uint64(x))

	case 'x': // int64
		msg.Round(8)
		x := msg.Endianness.Uint64(msg.Next(8))
		val.SetInt(int64(x))
	case 't': // uint64
		msg.Round(4)
		x := msg.Endianness.Uint64(msg.Next(8))
		val.SetUint(x)
	case 'd': // double
		msg.Round(8)
		x := msg.Endianness.Uint64(msg.Next(8))
		val.SetFloat(math.Float64frombits(x))

	case 's', 'o': // string, object name
		msg.Round(4)
		l := msg.Endianness.Uint32(msg.Next(4))
		s := msg.Next(int(l) + 1)
		val.SetString(string(s[:l]))

	case 'g': // signature string
		l := msg.Next(1)[0]
		s := msg.Next(int(l) + 1)
		val.SetString(string(s[:l]))

	case 'a': // array
		elemsig := sig[1:]
		msg.Round(4)
		// length in bytes.
		l := msg.Endianness.Uint32(msg.Next(4))
		end := msg.Idx + int(l)
		for msg.Idx < end {
			elemval := reflect.New(val.Type().Elem()).Elem()
			_ = msg.scanValue(elemsig, elemval)
			v := reflect.Append(val, elemval)
			val.Set(v)
		}

	case '(': // struct
		msg.Round(8)
		structSig, _ := _GetSigBlock(string(sig), 0)
		fields := make([]reflect.Value, val.NumField())
		for i := range fields {
			fields[i] = val.Field(i)
		}
		msg.scanMany(sig[1:len(structSig)-1], fields...)

	default:
		panic("unsupported")
		//case '(': // struct
		//case '{': // dict
		//case 'h': // file descriptor
	}
	return nil
}
