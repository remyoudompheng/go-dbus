package dbus

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
)

func init() {
	// Set up logging for exceptional errors.
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

type StandardBus int

const (
	SessionBus StandardBus = iota
	SystemBus
)

const dbusXMLIntro = `
<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN"
"http://www.freedesktop.org/standards/dbus/1.0/introspect.dtd">
<node>
  <interface name="org.freedesktop.DBus.Introspectable">
    <method name="Introspect">
      <arg name="data" direction="out" type="s"/>
    </method>
  </interface>
  <interface name="org.freedesktop.DBus">
    <method name="RequestName">
      <arg direction="in" type="s"/>
      <arg direction="in" type="u"/>
      <arg direction="out" type="u"/>
    </method>
    <method name="ReleaseName">
      <arg direction="in" type="s"/>
      <arg direction="out" type="u"/>
    </method>
    <method name="StartServiceByName">
      <arg direction="in" type="s"/>
      <arg direction="in" type="u"/>
      <arg direction="out" type="u"/>
    </method>
    <method name="Hello">
      <arg direction="out" type="s"/>
    </method>
    <method name="NameHasOwner">
      <arg direction="in" type="s"/>
      <arg direction="out" type="b"/>
    </method>
    <method name="ListNames">
      <arg direction="out" type="as"/>
    </method>
    <method name="ListActivatableNames">
      <arg direction="out" type="as"/>
    </method>
    <method name="AddMatch">
      <arg direction="in" type="s"/>
    </method>
    <method name="RemoveMatch">
      <arg direction="in" type="s"/>
    </method>
    <method name="GetNameOwner">
      <arg direction="in" type="s"/>
      <arg direction="out" type="s"/>
    </method>
    <method name="ListQueuedOwners">
      <arg direction="in" type="s"/>
      <arg direction="out" type="as"/>
    </method>
    <method name="GetConnectionUnixUser">
      <arg direction="in" type="s"/>
      <arg direction="out" type="u"/>
    </method>
    <method name="GetConnectionUnixProcessID">
      <arg direction="in" type="s"/>
      <arg direction="out" type="u"/>
    </method>
    <method name="GetConnectionSELinuxSecurityContext">
      <arg direction="in" type="s"/>
      <arg direction="out" type="ay"/>
    </method>
    <method name="ReloadConfig">
    </method>
    <signal name="NameOwnerChanged">
      <arg type="s"/>
      <arg type="s"/>
      <arg type="s"/>
    </signal>
    <signal name="NameLost">
      <arg type="s"/>
    </signal>
    <signal name="NameAcquired">
      <arg type="s"/>
    </signal>
  </interface>
</node>`

type signalHandler struct {
	mr   MatchRule
	proc func(*Message)
}

type Connection struct {
	addressMap       map[string]string
	uniqName         string
	signalMatchRules []signalHandler
	conn             net.Conn
	proxy            *Interface
	// reply channels.
	replyChans map[uint32]chan<- []byte
}

type Object struct {
	dest  string
	path  string
	intro Introspect
}

type Interface struct {
	obj   *Object
	name  string
	intro InterfaceData
}

type Method struct {
	iface *Interface
	data  MethodData
}

type Signal struct {
	iface *Interface
	data  SignalData
}

// Retrieve a method by name.
func (iface *Interface) Method(name string) (*Method, error) {
	method := iface.intro.GetMethodData(name)
	if nil == method {
		return nil, errors.New("Invalid Method")
	}
	return &Method{iface, method}, nil
}

// Retrieve a signal by name.
func (iface *Interface) Signal(name string) (*Signal, error) {
	signal := iface.intro.GetSignalData(name)
	if nil == signal {
		return nil, errors.New("Invalid Signalx")
	}
	return &Signal{iface, signal}, nil
}

func Connect(busType StandardBus) (*Connection, error) {
	var address string

	switch busType {
	case SessionBus:
		address = os.Getenv("DBUS_SESSION_BUS_ADDRESS")

	case SystemBus:
		if address = os.Getenv("DBUS_SYSTEM_BUS_ADDRESS"); len(address) == 0 {
			address = "unix:path=/var/run/dbus/system_bus_socket"
		}

	default:
		return nil, errors.New("Unknown bus")
	}

	if len(address) == 0 {
		return nil, errors.New("Unknown bus address")
	}
	transport := address[:strings.Index(address, ":")]

	bus := new(Connection)
	bus.addressMap = make(map[string]string)
	for _, pair := range strings.Split(address[len(transport)+1:], ",") {
		pair := strings.Split(pair, "=")
		bus.addressMap[pair[0]] = pair[1]
	}

	var ok bool
	if address, ok = bus.addressMap["path"]; ok {
	} else if address, ok = bus.addressMap["abstract"]; ok {
		address = "@" + address
	} else {
		return nil, errors.New("Unknown address key")
	}

	var err error
	if bus.conn, err = net.Dial(transport, address); err != nil {
		return nil, err
	}

	if _, err = bus.conn.Write([]byte{0}); err != nil {
		return nil, err
	}

	bus.replyChans = make(map[uint32]chan<- []byte)
	bus.signalMatchRules = make([]signalHandler, 0)
	bus.proxy = bus._GetProxy()
	return bus, nil
}

func (p *Connection) Authenticate() error {
	err := p.authenticate(new(AuthDbusCookieSha1))
	if err != nil {
		err = p.authenticate(new(AuthExternal))
	}
	if err != nil {
		return err
	}
	go p.handleReplies()
	p._SendHello()
	return nil
}

type errMalformedEndianness byte

func (e errMalformedEndianness) Error() string {
	return fmt.Sprintf("unexpected endianness tag %q", byte(e))
}

type errIncompleteMessage struct{ E error }

func (e errIncompleteMessage) Error() string {
	return fmt.Sprintf("incomplete message data: %s", e.E)
}

// handleReplies reads messages from the connection and dispatches
// them to the client goroutines.
func (p *Connection) handleReplies() error {
	r := bufio.NewReader(p.conn)
	for {
		// Get message.
		msg, replyTo, err := popMessage(r)
		if err != nil {
			return err
		}
		// Dispatch.
		err = p.dispatch(replyTo, msg)
		if err != nil {
			log.Print(err)
		}
	}
	panic("unreachable")
}

// constants for handmade header parsing.
const (
	msgOffsetType       = 1
	msgOffsetBodySize   = 4
	msgOffsetSerial     = 8
	msgOffsetFieldsSize = 12
)

func popMessage(r *bufio.Reader) (msg []byte, serial uint32, err error) {
	// Read message header.
	header, err := r.Peek(16)
	if err != nil {
		return
	}
	order := binary.ByteOrder(nil)
	switch header[0] {
	case 'l':
		order = binary.LittleEndian
	case 'B':
		order = binary.BigEndian
	default:
		err = errMalformedEndianness(header[0])
		return
	}

	// Determine length
	bodySize := order.Uint32(header[msgOffsetBodySize : msgOffsetBodySize+4])
	serial = order.Uint32(header[msgOffsetSerial : msgOffsetSerial+4])
	fldSize := order.Uint32(header[msgOffsetFieldsSize : msgOffsetFieldsSize+4])
	fldSize = (fldSize + 7) &^ 7 // pad.

	// Read entire message.
	msg = make([]byte, 16+fldSize+bodySize)
	_, err = io.ReadFull(r, msg)
	if err != nil {
		err = errIncompleteMessage{err}
		return
	}

	// Find reply serial.
	decoder := &msgData{Endianness: order, Data: msg}
	_, flds, _ := decoder.scanHeader()
	return msg, flds.ReplySerial, nil
}

type errUnknownSerial uint32

func (e errUnknownSerial) Error() string {
	return fmt.Sprintf("message for unknown serial number %d", uint32(e))
}

// dispatch sends a raw message to the appropriate goroutine.
func (p *Connection) dispatch(serial uint32, rawmsg []byte) error {
	if serial == 0 {
		return nil
	}
	ch := p.replyChans[serial]
	delete(p.replyChans, serial)
	if ch == nil {
		return errUnknownSerial(serial)
	}
	ch <- rawmsg
	return nil
}

// sendSync sends a message and synchronously waits fro the reply.
func (p *Connection) sendSync(msg *Message, callback func(*Message)) error {
	rawmsg, err := msg._Marshal()
	if err != nil {
		return err
	}

	// Prepare response channel.
	seri := uint32(msg.serial)
	replyChan := make(chan []byte, 1)
	p.replyChans[seri] = replyChan
	_, err = p.conn.Write(rawmsg)
	if err != nil {
		// kill connection.
		p.conn.Close()
		return err
	}

	// Receive reply.
	rawreply := <-replyChan
	reply, _, err := _Unmarshal(rawreply)
	if err != nil {
		return err
	}
	switch reply.Type {
	case TypeMethodReturn:
		callback(reply)
	case TypeSignal:
		for _, handler := range p.signalMatchRules {
			if handler.mr._Match(reply) {
				handler.proc(reply)
			}
		}
	case TypeError:
		// TODO: actually handle error messages.
		callback(reply)
	}
	return nil
}

func (p *Connection) _SendHello() error {
	if method, err := p.proxy.Method("Hello"); err == nil {
		p.Call(method)
	}
	return nil
}

func (p *Connection) _GetIntrospect(dest string, path string) Introspect {
	msg := NewMessage()
	msg.Type = TypeMethodCall
	msg.Path = path
	msg.Dest = dest
	msg.Iface = "org.freedesktop.DBus.Introspectable"
	msg.Member = "Introspect"

	var intro Introspect

	p.sendSync(msg, func(reply *Message) {
		if v, ok := reply.Params[0].(string); ok {
			if i, err := NewIntrospect(v); err == nil {
				intro = i
			}
		}
	})

	return intro
}

// Retrieve an interface by name.
func (obj *Object) Interface(name string) *Interface {
	if obj == nil || obj.intro == nil {
		return nil
	}

	iface := new(Interface)
	iface.obj = obj
	iface.name = name

	data := obj.intro.GetInterfaceData(name)
	if nil == data {
		return nil
	}

	iface.intro = data

	return iface
}

func (p *Connection) _GetProxy() *Interface {
	obj := new(Object)
	obj.path = "/org/freedesktop/DBus"
	obj.dest = "org.freedesktop.DBus"
	obj.intro, _ = NewIntrospect(dbusXMLIntro)

	iface := new(Interface)
	iface.obj = obj
	iface.name = "org.freedesktop.DBus"
	iface.intro = obj.intro.GetInterfaceData("org.freedesktop.DBus")

	return iface
}

// Call a method with the given arguments.
func (p *Connection) Call(method *Method, args ...interface{}) ([]interface{}, error) {
	iface := method.iface
	msg := NewMessage()

	msg.Type = TypeMethodCall
	msg.Path = iface.obj.path
	msg.Iface = iface.name
	msg.Dest = iface.obj.dest
	msg.Member = method.data.GetName()
	msg.Sig = method.data.GetInSignature()
	if len(args) > 0 {
		msg.Params = args[:]
	}

	var ret []interface{}
	p.sendSync(msg, func(reply *Message) {
		ret = reply.Params
	})

	return ret, nil
}

// Emit a signal with the given arguments.
func (p *Connection) Emit(signal *Signal, args ...interface{}) error {
	iface := signal.iface

	msg := NewMessage()

	msg.Type = TypeSignal
	msg.Path = iface.obj.path
	msg.Iface = iface.name
	msg.Dest = iface.obj.dest
	msg.Member = signal.data.GetName()
	msg.Sig = signal.data.GetSignature()
	msg.Params = args[:]

	buff, _ := msg._Marshal()
	_, err := p.conn.Write(buff)

	return err
}

// Retrieve a specified object.
func (p *Connection) Object(dest string, path string) *Object {

	obj := new(Object)
	obj.path = path
	obj.dest = dest
	obj.intro = p._GetIntrospect(dest, path)

	return obj
}

// Handle received signals.
func (p *Connection) Handle(rule *MatchRule, handler func(*Message)) {
	p.signalMatchRules = append(p.signalMatchRules, signalHandler{*rule, handler})
	if method, err := p.proxy.Method("AddMatch"); err == nil {
		p.Call(method, rule.String())
	}
}
