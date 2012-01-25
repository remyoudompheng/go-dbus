package dbus

import "testing"

func TestRead(t *testing.T) {

	slice := []byte("l\x01\x00\x01\x00\x00\x00\x00\x01\x00\x00\x00m\x00\x00\x00\x01\x01o\x00\x15\x00\x00\x00/org/freedesktop/DBus\x00\x00\x00\x02\x01s\x00\x14\x00\x00\x00org.freedesktop.DBus\x00\x00\x00\x00\x03\x01s\x00\x05\x00\x00\x00Hello\x00\x00\x00\x06\x01s\x00\x14\x00\x00\x00org.freedesktop.DBus\x00\x00\x00\x00")

	ir := newIterReader(slice, "yyyyuua(yv)")

	if ir == nil {
		t.Fatal("Iterator not initialized")
	}

	var e error

	for {
		ir.Value()

		e = ir.Next()
		if _, ok := e.(EOM); ok {
			break
		}

		if e != nil {
			t.Error("Read error: " + e.Error())
		}
	}
}
