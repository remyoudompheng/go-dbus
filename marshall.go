package dbus

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
)

var errIndex = errors.New("index error")

type errOutOfRange struct{ Offset, Length int }

func (err *errOutOfRange) Error() string {
	return fmt.Sprintf("message index out of range (%d/%d)", err.Offset+1, err.Length)
}

func appendArray(msg *msgData, align int, proc func(*msgData)) {
	var buf [4]byte
	msg.Round(4)
	msg.Round(align)
	msg.Put(buf[:4])
	start := msg.Idx
	proc(msg)
	length := msg.Idx - start
	msg.Endianness.PutUint32(msg.Data[start-4:start], uint32(length))
}

func appendValue(msg *msgData, sig string, val interface{}) (sigOffset int, e error) {
	if len(sig) == 0 {
		return 0, errors.New("Invalid Signature")
	}

	e = nil
	var buf [8]byte

	switch sig[0] {
	case 'y': // byte
		buf[0] = val.(byte)
		msg.Put(buf[:1])
		sigOffset = 1

	case 's': // string
		msg.Round(4)
		s := val.(string)
		msg.Endianness.PutUint32(buf[:4], uint32(len(s)))
		msg.Put(buf[:4])
		msg.PutString(s)
		msg.Put(buf[4:5]) // NUL.
		sigOffset = 1

	case 'u': // uint32
		msg.Round(4)
		msg.Endianness.PutUint32(buf[:4], val.(uint32))
		msg.Put(buf[:4])
		sigOffset = 1

	case 'i': // int32
		msg.Round(4)
		msg.Endianness.PutUint32(buf[:4], uint32(val.(int32)))
		msg.Put(buf[:4])
		sigOffset = 1

	case 'a': // ary
		sigBlock, _ := _GetSigBlock(sig, 1)
		appendArray(msg, 1, func(msg *msgData) {
			if slice, ok := val.([]interface{}); ok && slice != nil {
				for _, v := range slice {
					appendValue(msg, sigBlock, v)
				}
			}
		})
		sigOffset = 1 + len(sigBlock)

	case '(': // struct FIXME: nested struct not support
		msg.Round(8)
		structSig, _ := _GetStructSig(sig, 0)
		for i, s := range structSig {
			appendValue(msg, string(s), val.([]interface{})[i])
		}
		sigOffset = 2 + len(structSig)

	case '{':
		msg.Round(8)
		dictSig, _ := _GetDictSig(sig, 0)
		for i, s := range dictSig {
			appendValue(msg, string(s), val.([]interface{})[i])
		}
		sigOffset = 2 + len(dictSig)
	}

	return
}

func appendParamsData(msg *msgData, sig string, params []interface{}) {
	sigOffset := 0
	prmsOffset := 0
	sigLen := len(sig)
	for ; sigOffset < sigLen; prmsOffset++ {
		offset, _ := appendValue(msg, sig[sigOffset:len(sig)], params[prmsOffset])
		sigOffset += offset
	}
}

func _GetInt32(buff []byte, index int) (int32, error) {
	if len(buff) < index+4 {
		return 0, errIndex
	}
	return int32(binary.LittleEndian.Uint32(buff[index:])), nil
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
	msg := &msgData{Endianness: binary.LittleEndian, Data: buff, Idx: index}
	defer func() { bufIdx = msg.Idx }()
	slice = make([]interface{}, 0, len(sig))
	for sigIdx := 0; sigIdx < len(sig); {
		bufIdx = msg.Idx
		switch sig[sigIdx] {
		case 'b': // bool
			msg.Round(4)
			x := msg.Endianness.Uint32(msg.Next(4))
			slice = append(slice, bool(x != 0))
			sigIdx++

		case 'y': // byte
			slice = append(slice, msg.Data[msg.Idx])
			msg.Idx++
			sigIdx++

		case 'n': // int16
			msg.Round(2)
			x := msg.Endianness.Uint16(msg.Next(2))
			slice = append(slice, int16(x))
			sigIdx++

		case 'q': // uint16
			msg.Round(2)
			x := msg.Endianness.Uint16(msg.Next(2))
			slice = append(slice, uint16(x))
			sigIdx++

		case 'u': // uint32
			msg.Round(4)
			x := msg.Endianness.Uint32(msg.Next(4))
			slice = append(slice, uint32(x))
			sigIdx++

		case 's', 'o': // string, object
			msg.Round(4)
			l := msg.Endianness.Uint32(msg.Next(4))
			s := msg.Next(int(l) + 1)
			slice = append(slice, string(s[:l]))
			sigIdx++

		case 'g': // signature
			l := msg.Next(1)[0]
			s := msg.Next(int(l) + 1)
			slice = append(slice, string(s[:l]))
			sigIdx++

		case 'a': // array
			sigBlock, e := _GetSigBlock(sig, sigIdx+1)
			if e != nil {
				err = e
				return
			}

			msg.Round(4)
			// length in bytes.
			l := msg.Endianness.Uint32(msg.Next(4))
			end := msg.Idx + int(l)
			tmpSlice := make([]interface{}, 0)
			var arrValues []interface{}
			for msg.Idx < end {
				arrValues, msg.Idx, err = Parse(msg.Data, sigBlock, msg.Idx)
				if err != nil {
					return
				}
				tmpSlice = append(tmpSlice, arrValues...)
			}
			msg.Idx = end
			sigIdx += (1 + len(sigBlock))
			slice = append(slice, tmpSlice)

		case '(': // struct
			msg.Round(8)
			stSig, e := _GetStructSig(sig, sigIdx)
			if e != nil {
				err = e
				return
			}

			var structVals []interface{}
			structVals, msg.Idx, err = Parse(msg.Data, stSig, msg.Idx)
			if err != nil {
				return
			}

			sigIdx += (len(stSig) + 2)
			slice = append(slice, structVals)

		case '{': // dict
			msg.Round(8)
			stSig, e := _GetDictSig(sig, sigIdx)
			if e != nil {
				err = e
				return
			}

			var dictVals []interface{}
			dictVals, msg.Idx, err = Parse(msg.Data, stSig, msg.Idx)
			if err != nil {
				return
			}

			sigIdx += (len(stSig) + 2)
			slice = append(slice, dictVals)

		case 'v': // variant
			vals, idx, e := _GetVariant(buff, bufIdx)
			if e != nil {
				err = e
				return
			}

			bufIdx = idx
			sigIdx++
			slice = append(slice, vals...)
			msg.Idx = bufIdx

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
	end := msg.Idx + n
	if end > len(msg.Data) {
		err := &errOutOfRange{Offset: end, Length: len(msg.Data)}
		panic(error(err))
	}
	s := msg.Data[msg.Idx:end]
	msg.Idx = end
	return s
}

func (msg *msgData) Put(s []byte) {
	if msg.Idx >= cap(msg.Data) {
		newdata := make([]byte, len(msg.Data), msg.Idx+len(msg.Data)/4)
		copy(newdata, msg.Data)
		msg.Data = newdata
	}
	msg.Data = append(msg.Data[:msg.Idx], s...)
	msg.Idx += len(s)
}

func (msg *msgData) PutString(s string) {
	if msg.Idx >= cap(msg.Data) {
		newdata := make([]byte, len(msg.Data), msg.Idx+len(msg.Data)/4)
		copy(newdata, msg.Data)
		msg.Data = newdata
	}
	msg.Data = append(msg.Data[:msg.Idx], s...)
	msg.Idx += len(s)
}

func (msg *msgData) scanHeader() (hdr msgHeader, flds msgHeaderFields, err error) {
	defer catchPanicErr(&err)
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

var fldSigs = []Signature{
	1: "o",
	2: "s",
	3: "s",
	4: "s",
	5: "u",
	6: "s",
	7: "s",
	8: "g",
	9: "u",
}

func (msg *msgData) putHeader(hdr msgHeader, flds msgHeaderFields) error {
	defer catchPanicErr(&err)
	var buf [8]byte
	msg.put("(yyyyuu)", hdr)
	// Now an array of byte and variant.
	msg.Put(buf[:4])
	fldStart := msg.Idx
	fldVal := reflect.ValueOf(flds)
	for i, imax := 0, fldVal.NumField(); i < imax; i++ {
		elem := fldVal.Field(i)
		if elem.Interface() == reflect.Zero(elem.Type()).Interface() {
			continue
		}
		msg.Round(8)
		// field type byte.
		buf[0] = byte(i + 1)
		msg.Put(buf[:1])
		// field value.
		fldSig := fldSigs[i+1]
		msg.put("g", fldSig)
		msg.putValue(fldSig, elem)
	}
	length := msg.Idx - fldStart
	msg.Endianness.PutUint32(msg.Data[fldStart-4:fldStart], uint32(length))
	return nil
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
	defer catchPanicErr(&err)
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
		//case '{': // dict
		//case 'h': // file descriptor
	}
	return nil
}

func (msg *msgData) put(sig Signature, val interface{}) (err error) {
	return msg.putValue(sig, reflect.ValueOf(val))
}

func (msg *msgData) putValue(sig Signature, val reflect.Value) (err error) {
	defer catchPanicErr(&err)
	var buf [8]byte
	switch sig[0] {
	case 'y': // byte
		buf[0] = byte(val.Uint())
		msg.Put(buf[:1])

	case 'b': // bool
		if val.Bool() {
			buf[0] = 1
		}
		msg.Round(4)
		msg.Put(buf[:4])

	case 'n': // int16
		msg.Round(2)
		msg.Endianness.PutUint16(buf[:], uint16(val.Int()))
		msg.Put(buf[:2])
	case 'q': // uint16
		msg.Round(2)
		msg.Endianness.PutUint16(buf[:], uint16(val.Uint()))
		msg.Put(buf[:2])

	case 'i': // int32
		msg.Round(4)
		msg.Endianness.PutUint32(buf[:], uint32(val.Int()))
		msg.Put(buf[:4])
	case 'u': // uint32
		msg.Round(4)
		msg.Endianness.PutUint32(buf[:], uint32(val.Uint()))
		msg.Put(buf[:4])

	case 'x': // int64
		msg.Round(8)
		msg.Endianness.PutUint64(buf[:], uint64(val.Int()))
		msg.Put(buf[:8])
	case 't': // uint64
		msg.Round(8)
		msg.Endianness.PutUint64(buf[:], val.Uint())
		msg.Put(buf[:8])
	case 'd': // double
		msg.Round(8)
		msg.Endianness.PutUint64(buf[:], math.Float64bits(val.Float()))
		msg.Put(buf[:8])

	case 's', 'o': // string, object name
		s := val.String()
		msg.Round(4)
		msg.Endianness.PutUint32(buf[:], uint32(len(s)))
		msg.Put(buf[:4])
		msg.PutString(s)
		msg.Put(buf[5:6]) // Nul byte.

	case 'g': // signature string
		s := val.String()
		buf[0] = byte(len(s))
		msg.Put(buf[:1])
		msg.PutString(s)
		msg.Put(buf[1:2]) // NUL

	case 'a': // array
		elemsig := sig[1:]
		msg.Round(4)
		// length in bytes.
		idx := msg.Idx
		msg.Put(buf[:4])
		begin := msg.Idx
		for i, imax := 0, val.Len(); i < imax; i++ {
			elem := val.Index(i)
			msg.putValue(elemsig, elem)
		}
		length := msg.Idx - begin
		msg.Endianness.PutUint32(msg.Data[idx:idx+4], uint32(length))

	case '(': // struct
		msg.Round(8)
		structSig, _ := _GetSigBlock(string(sig), 0)
		structSig = structSig[1 : len(structSig)-1]
		for i, imax := 0, val.NumField(); i < imax; i++ {
			fldSig, _ := _GetSigBlock(structSig, 0)
			structSig = structSig[len(fldSig):]
			fld := val.Field(i)
			msg.putValue(Signature(fldSig), fld)
		}

	default:
		panic("unsupported")
		//case '{': // dict
		//case 'h': // file descriptor
	}
	return nil
}

func catchPanicErr(err *error) {
	switch p := recover(); e := p.(type) {
	case nil:
		// OK.
	case error:
		*err = e
	default:
		// re-panic.
		panic(p)
	}
}
