package dbus

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

func parseSig(s string) signature {
      sig, rest, err := parseOneSignature(s)
      if err != nil {
            panic(err)
      }
      if rest != "" {
            panic("trailing characters")
      }
      return sig
}

func TestAppendArray(t *testing.T) {
	teststr := "\x01\x02\x03\x04\x05\x00\x00\x00\x05\x00\x00\x00\x00\x00\x00\x00\x02"

	msg := &msgData{
		Endianness: binary.LittleEndian,
		Data:       []byte{1, 2, 3, 4, 5},
		Idx:        5,
	}

	appendArray(msg, 1,
		func(msg *msgData) {
			t.Log(msg.Data)
			msg.Round(8)
			t.Log(msg.Data)
			msg.Put([]byte{2})
			t.Log(msg.Data)
		})

	if teststr != string(msg.Data) {
		t.Errorf("got\n%q\nwant\n%q", msg.Data, teststr)
	}
}

func TestAppendValue(t *testing.T) {
	buff := new(msgData)
	buff.Endianness = binary.LittleEndian

	appendValue(buff, parseSig("s"), "string")
	appendValue(buff, parseSig("s"), "test2")
	ref := []byte("\x06\x00\x00\x00string\x00\x00\x05\x00\x00\x00test2\x00")
	if !bytes.Equal(ref, buff.Data) {
		t.Errorf("got\n%q\nwant\n%q", buff.Data, ref)
	}
	buff = new(msgData)
	buff.Endianness = binary.LittleEndian
	slice := make([]interface{}, 0)
	slice = append(slice, []interface{}{"test1", uint32(1)})
	slice = append(slice, []interface{}{"test2", uint32(2)})
	slice = append(slice, []interface{}{"test3", uint32(3)})
	appendValue(buff, parseSig("a(su)"), slice)
	ref2 := []byte("\x34\x00\x00\x00\x00\x00\x00\x00\x05\x00\x00\x00test1\x00\x00\x00\x01\x00\x00\x00\x05\x00\x00\x00test2\x00\x00\x00\x02\x00\x00\x00\x05\x00\x00\x00test3\x00\x00\x00\x03\x00\x00\x00")
	if !bytes.Equal(ref2, buff.Data) {
		t.Errorf("got\n%q\nwant\n%q", buff.Data, ref2)
	}
}

// sliceRef([1,2,3], 1) => 2
// sliceRef([[1,2],3], 0, 1) => 2
func sliceRef(s []interface{}, arg1 int, args ...int) interface{} {
	ret := s[arg1]

	if len(args) > 0 {
		ret1 := ret.([]interface{})
		i := 0
		for ; i < len(args)-1; i++ {
			ret1 = ret1[args[i]].([]interface{})
		}
		return ret1[args[i]]
	}
	return ret
}

func TestParse(t *testing.T) {
	ret, _, _ := Parse([]byte("\x01\x02"), "y", 0)
	if !reflect.DeepEqual([]interface{}{byte(1)}, ret) {
		t.Error("#1 Failed:", ret)
	}

	ret, _, _ = Parse([]byte("\x03\x00\x00\x00\x04\x00\x00\x00test\x00\x04"), "ysy", 0)
	if !reflect.DeepEqual([]interface{}{byte(3), "test", byte(4)}, ret) {
		t.Error("#1 Failed:", ret)
	}

	ret, _, _ = Parse([]byte("\x22\x00\x00\x00\x04\x00\x00\x00test\x00\x00\x00\x00\x05\x00\x00\x00test2\x00\x00\x00\x05\x00\x00\x00test3\x00\x01"), "asy", 0)
	//	if "test" != ret.At(0).(*vector.Vector).At(0).(string) { t.Error("#3-1 Failed:")}
	if "test" != sliceRef(ret, 0, 0).(string) {
		t.Error("#3-1 Failed:")
	}
	if "test2" != sliceRef(ret, 0, 1).(string) {
		t.Error("#3-2 Failed:")
	}
	if "test3" != sliceRef(ret, 0, 2).(string) {
		t.Error("#3-3 Failed:")
	}
	if byte(1) != sliceRef(ret, 1).(byte) {
		t.Error("#3-4 Failed:")
	}

	ret, _, e := Parse([]byte("\x22\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x04\x00\x00\x00true\x00\x00\x00\x00\x00\x00\x00\x00\x05\x00\x00\x00false\x00"), "a(bs)", 0)
	if e != nil {
		t.Error(e.Error())
	}
	if true != sliceRef(ret, 0, 0, 0).(bool) {
		t.Error("#4-1 Failed:")
	}
	if "true" != sliceRef(ret, 0, 0, 1).(string) {
		t.Error("#4-2 Failed:", sliceRef(ret, 0, 0, 1).(string))
	}
	if false != sliceRef(ret, 0, 1, 0).(bool) {
		t.Error("#4-3 Failed:")
	}
	if "false" != sliceRef(ret, 0, 1, 1).(string) {
		t.Error("#4-4 Failed:")
	}

	ret, _, _ = Parse([]byte("l\x00\x00\x00\x00\x01\x00\x00test"), "yu", 0)
	if 'l' != sliceRef(ret, 0).(byte) {
		t.Error("#5-1 Failed:")
	}
	if 0x100 != sliceRef(ret, 1).(uint32) {
		t.Error("#5-2 Failed:")
	}
}

func TestGetVariant(t *testing.T) {
	val, index, _ := _GetVariant([]byte("\x00\x00\x01s\x00\x00\x00\x00\x04\x00\x00\x00test\x00"), 2)
	str, ok := val[0].(string)
	if !ok {
		t.Error("#1-1 Failed")
	}
	if "test" != str {
		t.Error("#1-2 Failed", str)
	}
	if 17 != index {
		t.Error("#1-3 Failed")
	}
}

func TestParseVariant(t *testing.T) {
	vec, _, e := Parse([]byte("\x01s\x00\x00\x04\x00\x00\x00test\x00\x01y\x00\x03\x01u\x00\x04\x00\x00\x00"), "vvv", 0)
	if nil != e {
		t.Error("#1 Failed")
	}
	if "test" != vec[0].(string) {
		t.Error("#2 Failed")
	}
	if 3 != vec[1].(byte) {
		t.Error("#3 Failed")
	}
	if 4 != vec[2].(uint32) {
		t.Error("#4 Failed", vec[2].(uint32))
	}
}

func TestParseNumber(t *testing.T) {
	vec, _, e := Parse([]byte("\x04\x00\x00\x00"), "u", 0)
	if nil != e {
		t.Error("#1 Failed")
	}
	if uint32(4) != sliceRef(vec, 0).(uint32) {
		t.Error("#1 Failed", sliceRef(vec, 0).(uint32))
	}
}

func TestReflect(t *testing.T) {
	// A simple list of strings.
	const test_as = "\xa9\x02\x00\x00\x14\x00\x00\x00org.freedesktop.DBus\x00\x00\x00\x00\x05\x00\x00\x00:1.92\x00\x00\x00\x04\x00\x00\x00:1.7\x00\x00\x00\x00\x0e\x00\x00\x00org.xfce.Panel\x00\x00\x04\x00\x00\x00:1.8\x00\x00\x00\x00\x04\x00\x00\x00:1.9\x00\x00\x00\x00\x0f\x00\x00\x00org.xfce.Thunar\x00\x12\x00\x00\x00org.xfce.Appfinder\x00\x00\x0f\x00\x00\x00org.gnome.GConf\x00$\x00\x00\x00org.gtk.Private.UDisks2VolumeMonitor\x00\x00\x00\x00\f\x00\x00\x00org.a11y.Bus\x00\x00\x00\x00\x05\x00\x00\x00:1.10\x00\x00\x00\x0f\x00\x00\x00org.xfce.Xfconf\x00\x05\x00\x00\x00:1.55\x00\x00\x00\x05\x00\x00\x00:1.11\x00\x00\x00\x05\x00\x00\x00:1.12\x00\x00\x00\x05\x00\x00\x00:1.24\x00\x00\x00\x04\x00\x00\x00:1.0\x00\x00\x00\x00\x12\x00\x00\x00org.gtk.vfs.Daemon\x00\x00\x06\x00\x00\x00:1.133\x00\x00\x06\x00\x00\x00:1.122\x00\x00\x05\x00\x00\x00:1.25\x00\x00\x00\x05\x00\x00\x00:1.14\x00\x00\x00\x04\x00\x00\x00:1.1\x00\x00\x00\x00\x06\x00\x00\x00:1.123\x00\x00\x05\x00\x00\x00:1.37\x00\x00\x00\x05\x00\x00\x00:1.15\x00\x00\x00\x17\x00\x00\x00org.xfce.SettingsDaemon\x00\x17\x00\x00\x00org.xfce.SessionManager\x00\x04\x00\x00\x00:1.2\x00\x00\x00\x00\x06\x00\x00\x00:1.124\x00\x00\x05\x00\x00\x00:1.16\x00\x00\x00\x04\x00\x00\x00:1.3\x00\x00\x00\x00\x05\x00\x00\x00:1.28\x00\x00\x00\x05\x00\x00\x00:1.17\x00\x00\x00\x14\x00\x00\x00org.xfce.FileManager\x00\x00\x00\x00\x04\x00\x00\x00:1.4\x00\x00\x00\x00\x0e\x00\x00\x00ca.desrt.dconf\x00\x00\x05\x00\x00\x00:1.18\x00\x00\x00\x04\x00\x00\x00:1.5\x00\x00\x00\x00\x05\x00\x00\x00:1.91\x00\x00\x00\x05\x00\x00\x00:1.19\x00\x00\x00\x04\x00\x00\x00:1.6\x00"

	msg := &msgData{Endianness: binary.LittleEndian, Data: []byte(test_as)}
	var l []string
	msg.scan("as", &l)
	t.Log(l)

	// A full message.
	const testMsg = "l\x02\x01\x01\xad\x02\x00\x00\x04\x00\x00\x00=\x00\x00\x00" +
		"\x06\x01s\x00\x06\x00\x00\x00:1.138\x00\x00\x05\x01u\x00\x06\x00\x00\x00\x08\x01g\x00\x02as\x00\x07\x01s\x00\x14\x00\x00\x00org.freedesktop.DBus\x00\x00\x00\x00" +
		test_as

	msg = &msgData{Endianness: binary.LittleEndian, Data: []byte(testMsg)}
	hdr, flds, _ := msg.scanHeader()
	t.Logf("%+v", hdr)
	t.Logf("%+v", flds)
}

type sigTest struct {
	s   string
	sig signature
}

const isig = basicSig('i')

var sigTests = []sigTest{
	{"i", isig},
	{"(ii)", structSig{isig, isig}},
	{"(i(ii))", structSig{isig, structSig{isig, isig}}},
	{"ai", arraySig{Elem: isig}},
	{"a(ii)", arraySig{Elem: structSig{isig, isig}}},
	{"aai", arraySig{Elem: arraySig{Elem: isig}}},
	// Incomplete
	{"aa", nil},
	{"(ii", nil},
}

func TestParseOneSig(t *testing.T) {
	for _, test := range sigTests {
		sig, rest, err := parseOneSignature(test.s)
		if err != nil {
			t.Logf("parsing %q gives %s", test.s, err)
			sig, rest = nil, ""
		}
		if rest != "" {
			t.Errorf("parsing of %q did not consume full input", test.s)
		}
		if !reflect.DeepEqual(test.sig, sig) {
			t.Errorf("got %#v, expected %#v", test.sig, sig)
		}
	}
}

func TestSignatureRoundTrip(t *testing.T) {
	for _, test := range sigTests {
		if test.sig == nil {
			continue
		}
		sig, _, _ := parseOneSignature(test.s)
		if test.s != sig.String() {
			t.Errorf("expected %s, got %s", test.s, sig)
		}
	}
}
