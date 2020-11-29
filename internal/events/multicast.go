package events

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pkg/errors"
	utils "gitlab.com/mipimipi/go-utils"
	"gitlab.com/mipimipi/yuppie/internal/network"
	"gitlab.com/mipimipi/yuppie/internal/types"
)

// multicast interval in seconds
const multicastInterval time.Duration = 10

// IP address for evnt multicasting
const multicastAddrIPv4 = "239.255.255.246:7900"

var multicastUDPAddr *net.UDPAddr

func init() {
	var err error
	multicastUDPAddr, err = net.ResolveUDPAddr("udp4", multicastAddrIPv4)
	if err != nil {
		log.Panicf("could not resolve %s: %s", multicastAddrIPv4, err)
	}
}

// Multicast implements event mutlicasting
type Multicast struct {
	Listener   chan StateVar
	key        uint32
	changes    []StateVar
	stop       chan struct{}
	mutChanges sync.Mutex
	infs       []net.Interface
	bootID     *types.BootID
}

// NewMulticast creates a Multicast instance. wanted contains the list of network
// interfaces that where configured, booID is a function that returns the current
// BootID. if no interfaces are configured, all interfaces are used
func NewMulticast(wanted []string, bootID *types.BootID) (mul *Multicast, err error) {
	mul = new(Multicast)

	mul.Listener = make(chan StateVar)

	mul.bootID = bootID
	mul.infs, err = network.Interfaces(wanted)
	if err != nil {
		err = errors.Wrap(err, "cannot determine network interfaces for multicast events")
		return
	}

	return
}

// Listen listens to changes for state variables and stores them in me.changes
func (me *Multicast) Listen(ctx context.Context) {
	go func() {
		defer func() {
			close(me.Listener)
			log.Trace("multicast listener stopped")
		}()

		log.Trace("multicast listener started")

		for {
			select {
			case sv := <-me.Listener:
				log.Tracef("received change notification for '%s'", sv.Name())
				// me.changes must only be changed with a lock since it can be
				// changed concurrently in different go functions
				me.mutChanges.Lock()
				me.changes = append(me.changes, sv)
				me.mutChanges.Unlock()

			case <-ctx.Done():
				return
			}
		}
	}()
}

// Run triggers regular multicast event messages
func (me *Multicast) Run() {

	go func() {
		me.stop = make(chan struct{})
		ticker := time.NewTicker(multicastInterval * time.Second)

		defer func() {
			ticker.Stop()
			close(me.stop)
			log.Trace("multicast: stopped")
		}()

		log.Trace("multicast: running")

		for {
			select {
			case <-ticker.C:
				if len(me.changes) > 0 {
					me.send()
				}

			case <-me.stop:
				return
			}
		}
	}()
}

// Stop stops sending regular change events
func (me *Multicast) Stop() {
	me.stop <- struct{}{}
}

// send triggers sending event messages for all changed state variables
// via all interfaces
func (me *Multicast) send() {
	log.Trace("sending multicast events ...")

	// collect state variables to be sent
	svs := make(map[string]StateVar)
	me.mutChanges.Lock()
	for _, sv := range me.changes {
		if _, exists := svs[sv.Name()]; !exists {
			svs[sv.Name()] = sv
		}
	}
	me.changes = nil
	me.mutChanges.Unlock()

	for _, sv := range svs {
		go broadcast(me.key, sv, me.infs, me.bootID.Val())
		me.key++
	}

	log.Trace("multicast events sent")
}

// broadcast sends an event message for one state variable via all interfaces
func broadcast(key uint32, sv StateVar, infs []net.Interface, bootID uint32) {
	log.Tracef("broadcasting state variable '%s' with key %d ...", sv.Name(), key)

	// assemble message body
	body := marshalStatVars([]StateVar{sv})

	// assemble message
	msg := new(bytes.Buffer)
	msg.WriteString("NOTIFY * HTTP/1.1\r\n")
	fmt.Fprintf(msg, "HOST: %s\r\n", multicastAddrIPv4)
	fmt.Fprint(msg, "CONTENT-TYPE: text/xml; charset=\"utf-8\"\r\n")
	fmt.Fprintf(msg, "USN: %s::%s:%s\r\n", sv.DeviceUDN(), sv.ServiceType(), sv.ServiceVersion())
	fmt.Fprintf(msg, "SVCID: %s\r\n", sv.ServiceID())
	fmt.Fprint(msg, "NT: upnp:event\r\n")
	fmt.Fprint(msg, "NTS: upnp:propchange\r\n")
	fmt.Fprintf(msg, "SEQ: %d\r\n", key)
	fmt.Fprint(msg, "LVL: upnp:/info\r\n")
	fmt.Fprintf(msg, "BOOTID.UPNP.ORG: %d\r\n", bootID)
	fmt.Fprintf(msg, "CONTENT-LENGTH: %d\r\n", len(body))
	fmt.Fprint(msg, "\r\n")
	fmt.Fprint(msg, string(body))
	// add empty row at the end as required by the UPnP Device
	// Architecture 2.0
	fmt.Fprint(msg, "\r\n")

	// send event messages multiple times from all interfaces
	for i := 0; i < network.UDPMsgRepetitions; i++ {
		// sleep for a few hundert milliseconds
		if i != 0 {
			utils.RandomNap(500)
		}
		// send event message from all interfaces
		for _, inf := range infs {
			conn, err := network.UDPConn(inf, multicastUDPAddr)
			if err != nil {
				log.Errorf("could not create connection for multicast eventing: %v", err)
				continue
			}
			if err = network.SendUDP(conn, multicastUDPAddr, msg.Bytes()); err != nil {
				log.Errorf("could not send multicast event: %v", err)
				continue
			}
		}
	}

	log.Tracef("broadcasted state variable '%s' with key %d", sv.Name(), key)
}
