package dbus

import "testing"

func TestUnmarshal(t *testing.T) {

	teststr := "l\x01\x00\x01\x00\x00\x00\x00\x01\x00\x00\x00m\x00\x00\x00\x01\x01o\x00\x15\x00\x00\x00/org/freedesktop/DBus\x00\x00\x00\x02\x01s\x00\x14\x00\x00\x00org.freedesktop.DBus\x00\x00\x00\x00\x03\x01s\x00\x05\x00\x00\x00Hello\x00\x00\x00\x06\x01s\x00\x14\x00\x00\x00org.freedesktop.DBus\x00\x00\x00\x00"

	msg, e := unmarshal([]byte(teststr))
	if nil != e {
		t.Error("Unmarshal Failed")
	}
	if TypeMethodCall != msg.Type {
		t.Error("#1 Failed :", msg.Type)
	}
	if "/org/freedesktop/DBus" != msg.Path {
		t.Error("#2 Failed :", msg.Path)
	}
	if "org.freedesktop.DBus" != msg.Dest {
		t.Error("#3 Failed :", msg.Dest)
	}
	if "org.freedesktop.DBus" != msg.Iface {
		t.Error("#4 Failed :", msg.Iface)
	}
	if "Hello" != msg.Member {
		t.Error("#5 Failed :", msg.Member)
	}
}

func TestMarshal(t *testing.T) {
	teststr := "l\x01\x00\x01\x00\x00\x00\x00\x01\x00\x00\x00m\x00\x00\x00\x01\x01o\x00\x15\x00\x00\x00/org/freedesktop/DBus\x00\x00\x00\x02\x01s\x00\x14\x00\x00\x00org.freedesktop.DBus\x00\x00\x00\x00\x03\x01s\x00\x05\x00\x00\x00Hello\x00\x00\x00\x06\x01s\x00\x14\x00\x00\x00org.freedesktop.DBus\x00\x00\x00\x00"

	msg := NewMessage()
	msg.Type = TypeMethodCall
	msg.Flags = MessageFlag(0)
	msg.Path = "/org/freedesktop/DBus"
	msg.Dest = "org.freedesktop.DBus"
	msg.Iface = "org.freedesktop.DBus"
	msg.Member = "Hello"
	msg.serial = 1

	buff, _ := msg._Marshal()
	if teststr != string(buff) {
		t.Errorf("got\n%q\nwant\n%q", buff, teststr)
	}
}

func BenchmarkMessage_Marshal(b *testing.B) {
	msg := NewMessage()
	msg.Type = TypeMethodCall
	msg.Path = "org.freedesktop.systemd1"
	msg.Dest = "/org/freedesktop/systemd1"
	msg.Iface = "org.freedesktop.DBus.Introspectable"
	msg.Member = "Introspect"

	var buf []byte
	var err error
	for i := 0; i < b.N; i++ {
		buf, err = msg._Marshal()
	}

	if err != nil {
		b.Error(err)
	}
	b.SetBytes(int64(len(buf)))
}

func BenchmarkMessage_Unmarshal(b *testing.B) {
	const teststr = "l\x01\x00\x01\x00\x00\x00\x00\x01\x00\x00\x00m\x00\x00\x00\x01\x01o\x00\x15\x00\x00\x00/org/freedesktop/DBus\x00\x00\x00\x02\x01s\x00\x14\x00\x00\x00org.freedesktop.DBus\x00\x00\x00\x00\x03\x01s\x00\x05\x00\x00\x00Hello\x00\x00\x00\x06\x01s\x00\x14\x00\x00\x00org.freedesktop.DBus\x00\x00\x00\x00"

	input := []byte(teststr)
	for i := 0; i < b.N; i++ {
		msg, err := unmarshal(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = msg
	}
	b.SetBytes(int64(len(teststr)))
}

const test_as = "\xa9\x02\x00\x00\x14\x00\x00\x00org.freedesktop.DBus\x00\x00\x00\x00\x05\x00\x00\x00:1.92\x00\x00\x00\x04\x00\x00\x00:1.7\x00\x00\x00\x00\x0e\x00\x00\x00org.xfce.Panel\x00\x00\x04\x00\x00\x00:1.8\x00\x00\x00\x00\x04\x00\x00\x00:1.9\x00\x00\x00\x00\x0f\x00\x00\x00org.xfce.Thunar\x00\x12\x00\x00\x00org.xfce.Appfinder\x00\x00\x0f\x00\x00\x00org.gnome.GConf\x00$\x00\x00\x00org.gtk.Private.UDisks2VolumeMonitor\x00\x00\x00\x00\f\x00\x00\x00org.a11y.Bus\x00\x00\x00\x00\x05\x00\x00\x00:1.10\x00\x00\x00\x0f\x00\x00\x00org.xfce.Xfconf\x00\x05\x00\x00\x00:1.55\x00\x00\x00\x05\x00\x00\x00:1.11\x00\x00\x00\x05\x00\x00\x00:1.12\x00\x00\x00\x05\x00\x00\x00:1.24\x00\x00\x00\x04\x00\x00\x00:1.0\x00\x00\x00\x00\x12\x00\x00\x00org.gtk.vfs.Daemon\x00\x00\x06\x00\x00\x00:1.133\x00\x00\x06\x00\x00\x00:1.122\x00\x00\x05\x00\x00\x00:1.25\x00\x00\x00\x05\x00\x00\x00:1.14\x00\x00\x00\x04\x00\x00\x00:1.1\x00\x00\x00\x00\x06\x00\x00\x00:1.123\x00\x00\x05\x00\x00\x00:1.37\x00\x00\x00\x05\x00\x00\x00:1.15\x00\x00\x00\x17\x00\x00\x00org.xfce.SettingsDaemon\x00\x17\x00\x00\x00org.xfce.SessionManager\x00\x04\x00\x00\x00:1.2\x00\x00\x00\x00\x06\x00\x00\x00:1.124\x00\x00\x05\x00\x00\x00:1.16\x00\x00\x00\x04\x00\x00\x00:1.3\x00\x00\x00\x00\x05\x00\x00\x00:1.28\x00\x00\x00\x05\x00\x00\x00:1.17\x00\x00\x00\x14\x00\x00\x00org.xfce.FileManager\x00\x00\x00\x00\x04\x00\x00\x00:1.4\x00\x00\x00\x00\x0e\x00\x00\x00ca.desrt.dconf\x00\x00\x05\x00\x00\x00:1.18\x00\x00\x00\x04\x00\x00\x00:1.5\x00\x00\x00\x00\x05\x00\x00\x00:1.91\x00\x00\x00\x05\x00\x00\x00:1.19\x00\x00\x00\x04\x00\x00\x00:1.6\x00"
const testMsg2 = "l\x02\x01\x01\xad\x02\x00\x00\x04\x00\x00\x00=\x00\x00\x00" +
	"\x06\x01s\x00\x06\x00\x00\x00:1.138\x00\x00\x05\x01u\x00\x06\x00\x00\x00\x08\x01g\x00\x02as\x00\x07\x01s\x00\x14\x00\x00\x00org.freedesktop.DBus\x00\x00\x00\x00" +
	test_as

func BenchmarkMessage_Unmarshal2(b *testing.B) {
	input := []byte(testMsg2)
	for i := 0; i < b.N; i++ {
		msg, err := unmarshal(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = msg
	}
	b.SetBytes(int64(len(testMsg2)))
}

func BenchmarkMessage_UnmarshalReflect1(b *testing.B) {
	input := []byte(testMsg2)
	var data []string
	for i := 0; i < b.N; i++ {
		msg, err := newRawMessage(input)
		if err != nil {
			b.Fatal(err)
		}
		msg.unmarshalReflect(&data)
	}
	b.SetBytes(int64(len(testMsg2)))
}
