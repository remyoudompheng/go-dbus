package dbus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
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
		return 0, errors.New("index error")
	}
	return buff[index], nil
}

func _GetInt16(buff []byte, index int) (int16, error) {
	if len(buff) <= index+2-1 {
		return 0, errors.New("index error")
	}
	var n int16
	e := binary.Read(bytes.NewBuffer(buff[index:len(buff)]), binary.LittleEndian, &n)
	if e != nil {
		return 0, e
	}
	return n, nil
}

func _GetUint16(buff []byte, index int) (uint16, error) {
	if len(buff) <= index+2-1 {
		return 0, errors.New("index error")
	}
	var q uint16
	e := binary.Read(bytes.NewBuffer(buff[index:len(buff)]), binary.LittleEndian, &q)
	if e != nil {
		return 0, e
	}
	return q, nil
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

func _GetUint32(buff []byte, index int) (uint32, error) {
	if len(buff) <= index+4-1 {
		return 0, errors.New("index error")
	}
	var u uint32
	e := binary.Read(bytes.NewBuffer(buff[index:len(buff)]), binary.LittleEndian, &u)
	if e != nil {
		return 0, e
	}
	return u, nil
}

func _GetInt64(buff []byte, index int) (int64, error) {
	if len(buff) <= index+8-1 {
		return 0, errors.New("Index error")
	}
	var i int64
	e := binary.Read(bytes.NewBuffer(buff[index:]), binary.LittleEndian, &i)
	if e != nil {
		return 0, e
	}
	return i, nil
}

func _GetUInt64(buff []byte, index int) (uint64, error) {
	if len(buff) <= index+8-1 {
		return 0, errors.New("Index error")
	}
	var u uint64
	e := binary.Read(bytes.NewBuffer(buff[index:]), binary.LittleEndian, &u)
	if e != nil {
		return 0, e
	}
	return u, nil
}

func _GetDouble(buff []byte, index int) (float64, error) {
	if len(buff) <= index+8-1 {
		return 0, errors.New("Index error")
	}
	var d float64
	e := binary.Read(bytes.NewBuffer(buff[index:]), binary.LittleEndian, &d)
	if e != nil {
		return 0, e
	}
	return d, nil
}

func _GetBoolean(buff []byte, index int) (bool, error) {
	if len(buff) <= index+4-1 {
		return false, errors.New("index error")
	}
	var v int32
	e := binary.Read(bytes.NewBuffer(buff[index:len(buff)]), binary.LittleEndian, &v)
	if e != nil {
		return false, e
	}
	return 0 != v, nil
}

func _GetString(buff []byte, index int, size int) (string, error) {
	if len(buff) <= (index + size - 1) {
		return "", errors.New("index error")
	}
	return string(buff[index : index+size]), nil
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

func _GetVariant(buff []byte, index int) (vals []interface{}, retidx int, e error) {
	retidx = index
	sigSize := int(buff[retidx])
	retidx++
	sig := string(buff[retidx : retidx+sigSize])
	vals, retidx, e = Parse(buff, sig, retidx+sigSize+1)
	return
}

func Parse(buff []byte, sig string, index int) (slice []interface{}, bufIdx int, err error) {
	slice = make([]interface{}, 0)
	bufIdx = index
	for sigIdx := 0; sigIdx < len(sig); {
		switch sig[sigIdx] {
		case Boolean: // bool
			bufIdx = _Align(alignment[Boolean], bufIdx)
			b, e := _GetBoolean(buff, bufIdx)
			if e != nil {
				err = e
				return
			}
			slice = append(slice, bool(b))
			bufIdx += 4
			sigIdx++

		case Byte: // byte
			v, e := _GetByte(buff, bufIdx)
			if e != nil {
				err = e
				return
			}
			slice = append(slice, v)
			bufIdx++
			sigIdx++

		case Int16: // int16
			bufIdx = _Align(alignment[Int16], bufIdx)
			n, e := _GetInt16(buff, bufIdx)
			if e != nil {
				err = e
				return
			}
			slice = append(slice, n)
			bufIdx += 2
			sigIdx++

		case UInt16: // uint16
			bufIdx = _Align(alignment[UInt16], bufIdx)
			q, e := _GetUint16(buff, bufIdx)
			if e != nil {
				err = e
				return
			}
			slice = append(slice, q)
			bufIdx += 2
			sigIdx++

		case UInt32: // uint32
			bufIdx = _Align(alignment[UInt32], bufIdx)
			u, e := _GetUint32(buff, bufIdx)
			if e != nil {
				err = e
				return
			}
			slice = append(slice, u)
			bufIdx += 4
			sigIdx++

		case Int64:
			bufIdx = _Align(alignment[Int64], bufIdx)
			n, e := _GetInt64(buff, bufIdx)
			if e != nil {
				err = e
				return
			}
			slice = append(slice, n)
			bufIdx += 8
			sigIdx++

		case UInt64:
			bufIdx = _Align(alignment[UInt64], bufIdx)
			n, e := _GetUInt64(buff, bufIdx)
			if e != nil {
				err = e
				return
			}
			slice = append(slice, n)
			bufIdx += 8
			sigIdx++

		case Double:
			bufIdx = _Align(alignment[Double], bufIdx)
			n, e := _GetDouble(buff, bufIdx)
			if e != nil {
				err = e
				return
			}
			slice = append(slice, n)
			bufIdx += 8
			sigIdx++

		case String, ObjectPath: // string, object
			bufIdx = _Align(alignment[String], bufIdx)

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

		case Signature: // signature
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

		case Array: // array
			startIdx := _Align(alignment[Array], bufIdx)
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

		case StructBegin: // struct
			idx := _Align(alignment[StructBegin], bufIdx)
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

		case DictBegin: // dict
			idx := _Align(alignment[DictBegin], bufIdx)
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

		case Variant: // variant
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
