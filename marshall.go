package dbus

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
)

// Signature parsing.

type signature interface {
	String() string
	istype()
}

type basicSig byte
type structSig []signature
type arraySig struct{ Elem signature }
type dictSig struct {
	Key   basicSig
	Value signature
}

func (sig basicSig) String() string { return string(sig) }

func (sig structSig) String() string {
	buf := make([]byte, 0, len(sig))
	buf = append(buf, '(')
	for _, s := range sig {
		buf = append(buf, s.String()...)
	}
	buf = append(buf, ')')
	return string(buf)
}

func (sig arraySig) String() string { return "a" + sig.Elem.String() }

func (sig dictSig) String() string {
	return "a{" + string(sig.Key) + sig.Value.String() + "}"
}

func (basicSig) istype()  {}
func (arraySig) istype()  {}
func (structSig) istype() {}
func (dictSig) istype()   {}

func parseSignature(s string) (ss []signature, err error) {
	for len(s) > 0 {
		sig, rest, err := parseOneSignature(s)
		if err != nil {
			return nil, err
		}
		ss = append(ss, sig)
		s = rest
	}
	return ss, nil
}

var (
	errMissingCloseParen = errors.New("missing ')' at end of struct signature")
)

func parseOneSignature(s string) (sig signature, rest string, err error) {
	if len(s) == 0 {
		return nil, "", fmt.Errorf("missing type")
	}
	switch s[0] {
	case 'b', 'y',
		'n', 'q', // 16-bit
		'i', 'u', // 32-bit
		'x', 't', // 64-bit
		'd',           // float
		's', 'o', 'g', // string
		'v':
		return basicSig(s[0]), s[1:], nil
	case '(':
		s = s[1:]
		var sigs []signature
		for len(s) > 0 && s[0] != ')' {
			sig, rest, err := parseOneSignature(s)
			if err != nil {
				return nil, s, err
			}
			sigs = append(sigs, sig)
			s = rest
		}
		if len(s) == 0 || s[0] != ')' {
			return nil, "", errMissingCloseParen
		}
		return structSig(sigs), s[1:], nil
	case 'a':
		if len(s) > 1 && s[1] == '{' {
			// Dictionary.
		} else {
			elem, rest, err := parseOneSignature(s[1:])
			if err != nil {
				return nil, "", err
			}
			return arraySig{Elem: elem}, rest, nil
		}
	}
	return nil, "", fmt.Errorf("invalid signature %q", s)
}

func mustParseSig(s string) signature {
	sig, rest, err := parseOneSignature(s)
	if err != nil {
		panic(err)
	}
	if rest != "" {
		panic("trailing characters")
	}
	return sig
}

func mustParseSigs(s string) []signature {
	sigs, err := parseSignature(s)
	if err != nil {
		panic(err)
	}
	return sigs
}

// D-Bus wire format.

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

func appendValue(msg *msgData, sig signature, val interface{}) (err error) {
	var buf [8]byte
	// complex types.
	switch sig := sig.(type) {
	case basicSig:
		break
	case arraySig:
		vals := val.([]interface{})
		appendArray(msg, 1, func(msg *msgData) {
			for _, v := range vals {
				appendValue(msg, sig.Elem, v)
			}
		})
		return nil
	case dictSig:
		vals := val.([]interface{})
		appendArray(msg, 1, func(msg *msgData) {
			for _, v := range vals {
				v := v.([]interface{})
				key, value := v[0], v[1]
				msg.Round(8)
				appendValue(msg, sig.Key, key)
				appendValue(msg, sig.Value, value)
			}
		})
		return nil
	case structSig:
		msg.Round(8)
		vals := val.([]interface{})
		for i, fldsig := range sig {
			appendValue(msg, fldsig, vals[i])
		}
		return nil
	default:
		panic(fmt.Errorf("invalid signature type %T", sig))
	}
	// basic types.
	switch sig := sig.(basicSig); sig {
	case 'y': // byte
		buf[0] = val.(byte)
		msg.Put(buf[:1])

	case 's': // string
		msg.Round(4)
		s := val.(string)
		msg.Endianness.PutUint32(buf[:4], uint32(len(s)))
		msg.Put(buf[:4])
		msg.PutString(s)
		msg.Put(buf[4:5]) // NUL.

	case 'u': // uint32
		msg.Round(4)
		msg.Endianness.PutUint32(buf[:4], val.(uint32))
		msg.Put(buf[:4])

	case 'i': // int32
		msg.Round(4)
		msg.Endianness.PutUint32(buf[:4], uint32(val.(int32)))
		msg.Put(buf[:4])
	default:
		return fmt.Errorf("unsupported type %q", byte(sig))
	}

	return
}

func appendParamsData(msg *msgData, sig string, params []interface{}) {
	sigs, err := parseSignature(sig)
	if err != nil {
		panic(err)
	}
	for i, sigelem := range sigs {
		err = appendValue(msg, sigelem, params[i])
		if err != nil {
			panic(err)
		}
	}
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
	sigs, err := parseSignature(sig)
	if err != nil {
		return
	}
	slice, err = parseVariants(msg, sigs)
	return slice, msg.Idx, err
}

func parseVariants(msg *msgData, sigs []signature) (slice []interface{}, err error) {
	slice = make([]interface{}, 0, len(sigs))
	for _, sig := range sigs {
		switch sig := sig.(type) {
		case basicSig:
			break
		case arraySig:
			msg.Round(4)
			// length in bytes.
			l := msg.Endianness.Uint32(msg.Next(4))
			end := msg.Idx + int(l)
			tmpSlice := make([]interface{}, 0)
			var arrValues []interface{}
			elemsig := []signature{sig.Elem}
			for msg.Idx < end {
				arrValues, err = parseVariants(msg, elemsig)
				if err != nil {
					return
				}
				tmpSlice = append(tmpSlice, arrValues...)
			}
			slice = append(slice, tmpSlice)
			continue
		case structSig:
			msg.Round(8)
			var structVals []interface{}
			structVals, err = parseVariants(msg, sig)
			if err != nil {
				return
			}
			slice = append(slice, structVals)
			continue
		case dictSig:
			msg.Round(4)
			// length in bytes.
			l := msg.Endianness.Uint32(msg.Next(4))
			end := msg.Idx + int(l)
			var dictVals []interface{}
			elemsig := []signature{sig.Key, sig.Value}
			for msg.Idx < end {
				msg.Round(8)
				kv, err := parseVariants(msg, elemsig)
				if err != nil {
					return nil, err
				}
				dictVals = append(dictVals, kv)
			}
			slice = append(slice, dictVals)
			continue
		default:
			panic(fmt.Errorf("invalid signature type %T", sig))
		}
		switch sig := sig.(basicSig); sig {
		case 'b': // bool
			msg.Round(4)
			x := msg.Endianness.Uint32(msg.Next(4))
			slice = append(slice, bool(x != 0))

		case 'y': // byte
			slice = append(slice, msg.Data[msg.Idx])
			msg.Idx++

		case 'n': // int16
			msg.Round(2)
			x := msg.Endianness.Uint16(msg.Next(2))
			slice = append(slice, int16(x))

		case 'q': // uint16
			msg.Round(2)
			x := msg.Endianness.Uint16(msg.Next(2))
			slice = append(slice, uint16(x))

		case 'u': // uint32
			msg.Round(4)
			x := msg.Endianness.Uint32(msg.Next(4))
			slice = append(slice, uint32(x))

		case 's', 'o': // string, object
			msg.Round(4)
			l := msg.Endianness.Uint32(msg.Next(4))
			s := msg.Next(int(l) + 1)
			slice = append(slice, string(s[:l]))

		case 'g': // signature
			l := msg.Next(1)[0]
			s := msg.Next(int(l) + 1)
			slice = append(slice, string(s[:l]))

		case 'v': // variant
			vals, idx, e := _GetVariant(msg.Data, msg.Idx)
			msg.Idx = idx
			if e != nil {
				err = e
				return
			}
			slice = append(slice, vals...)

		default:
			fmt.Println(sig)
			return nil, errors.New("unknown type")
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

type msgHeaderFields struct {
	Path        ObjectPath // field 1
	Interface   string
	Member      string
	ErrorName   string
	ReplySerial uint32
	Destination string
	Sender      string
	Signature   string
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
		if b > 9 {
			err = fmt.Errorf("invalid header field ID: %d", b)
			return
		}
		// A variant is a signature and value.
		var fldSig string
		msg.scan("g", &fldSig)
		msg.scan(fldSig, fldVal.Field(int(b)-1).Addr().Interface())
	}
	return
}

// the Dbus signatures for msgHeader and msgHeaderFields.
var hdrSigs = mustParseSig("(yyyyuu)")
var fldSigs = mustParseSigs("osssussgu")

func (msg *msgData) putHeader(hdr msgHeader, flds msgHeaderFields) (err error) {
	defer catchPanicErr(&err)
	var buf [8]byte
	msg.putValue(hdrSigs, reflect.ValueOf(hdr))
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
		fldSig := fldSigs[i]
		msg.put("g", fldSig.String())
		msg.putValue(fldSig, elem)
	}
	length := msg.Idx - fldStart
	msg.Endianness.PutUint32(msg.Data[fldStart-4:fldStart], uint32(length))
	return nil
}

// scan is a helper taking a signature string.
func (msg *msgData) scan(sigstr string, val interface{}) (err error) {
	sig, rest, err := parseOneSignature(sigstr)
	if rest != "" {
		return fmt.Errorf("trailing signature %q", rest)
	}
	if err != nil {
		return
	}
	return msg.scanValue(sig, reflect.ValueOf(val).Elem())
}

func (msg *msgData) put(sigstr string, val interface{}) (err error) {
	sig, rest, err := parseOneSignature(sigstr)
	if rest != "" {
		return fmt.Errorf("trailing signature %q", rest)
	}
	if err != nil {
		return
	}
	if _, ok := val.(reflect.Value); ok {
		panic("put called with reflect.Value argument")
	}
	return msg.putValue(sig, reflect.ValueOf(val))
}

func (msg *msgData) scanMany(s string, val ...reflect.Value) (err error) {
	sigs, err := parseSignature(s)
	if err != nil {
		return err
	}
	for i, sig := range sigs {
		msg.scanValue(sig, val[i])
	}
	return
}

// scan reads data from buf according to the first item in signature sig and fills val.
// It returns the number of bytes consumed.
// http://dbus.freedesktop.org/doc/dbus-specification.html#type-system
func (msg *msgData) scanValue(sig signature, val reflect.Value) (err error) {
	defer catchPanicErr(&err)
	switch sig := sig.(type) {
	case basicSig:
		break
	case arraySig:
		msg.Round(4)
		// length in bytes.
		l := msg.Endianness.Uint32(msg.Next(4))
		end := msg.Idx + int(l)
		for msg.Idx < end {
			elemval := reflect.New(val.Type().Elem()).Elem()
			_ = msg.scanValue(sig.Elem, elemval)
			v := reflect.Append(val, elemval)
			val.Set(v)
		}
	case structSig:
		msg.Round(8)
		for i, fldsig := range sig {
			msg.scanValue(fldsig, val.Field(i))
		}
	case dictSig:
		panic("unsupported dictionaries")
	default:
		panic("impossible signature type")
	}
	switch sig.(basicSig) {
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

	default:
		panic("unsupported")
		//case '{': // dict
		//case 'h': // file descriptor
	}
	return nil
}

func (msg *msgData) putValue(sig signature, val reflect.Value) (err error) {
	defer catchPanicErr(&err)
	var buf [8]byte

	switch sig := sig.(type) {
	case basicSig:
		break
	case arraySig:
		msg.Round(4)
		// length in bytes.
		idx := msg.Idx
		msg.Put(buf[:4])
		begin := msg.Idx
		for i, imax := 0, val.Len(); i < imax; i++ {
			elem := val.Index(i)
			msg.putValue(sig.Elem, elem)
		}
		length := msg.Idx - begin
		msg.Endianness.PutUint32(msg.Data[idx:idx+4], uint32(length))

	case structSig:
		msg.Round(8)
		for i, fldsig := range sig {
			fld := val.Field(i)
			msg.putValue(fldsig, fld)
		}
	case dictSig:
		panic("dictionaries are unsupported")
	default:
		panic("impossible signature type")
	}
	switch sig.(basicSig) {
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
		var s string
		if v, ok := val.Interface().(signature); ok {
			s = v.String()
		} else {
			s = val.String()
		}
		buf[0] = byte(len(s))
		msg.Put(buf[:1])
		msg.PutString(s)
		msg.Put(buf[1:2]) // NUL

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
