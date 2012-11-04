package dbus

import (
	"fmt"
	"log"
	"testing"
)

type callTest struct {
	dest, path, iface, method string
	args                      []interface{}
	validate                  func([]interface{}) error
}

var callTests = []callTest{
	{"org.freedesktop.DBus", "/org/freedesktop/DBus",
		"org.freedesktop.DBus", "ListNames",
		[]interface{}{
			"go-dbus", uint32(0),
			"info", "testing go-dbus", "test_body",
			[]string{}, map[uint32]interface{}{},
			int32(2000)},
		func([]interface{}) error {
			return nil
		}},
}

func testCall(c *Connection, t *testing.T, test callTest) {
	method, err := c.Object(test.dest, test.path).
		Interface(test.iface).
		Method(test.method)
	if err != nil {
		t.Error(err)
	}
	out, err := c.Call(method, test.args...)
	if err != nil {
		t.Errorf("failed Method.Call: %v", err)
		return
	}
	t.Log(out)
	if err = test.validate(out); err != nil {
		t.Errorf("failed validation: %v", err)
	}
}

func TestDBus(t *testing.T) {
	con, err := Connect(SessionBus)
	if err != nil {
		t.Fatal(err.Error())
	}

	if err = con.Authenticate(); err != nil {
		t.Fatal("Failed Connection.Authenticate:", err.Error())
	}

	for i, test := range callTests {
		t.Logf("call test %d", i)
		testCall(con, t, test)
	}
}

func ExampleConnection_Call(t *testing.T) {
	conn, err := Connect(SystemBus)
	if err != nil {
		log.Fatal(err)
	}
	conn.Authenticate()
	method, err := conn.
		Object("org.freedesktop.DBus", "/org/freedesktop/DBus").
		//Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1").
		Interface("org.freedesktop.DBus.Introspectable").
		Method("Introspect")
	if err != nil {
		log.Fatal(err)
	}

	reply, err := conn.call(method, nil, false)
	if err != nil {
		log.Fatal(err)
	}
	var data string
	reply.Unmarshal(&data)
	fmt.Println(data)
}
