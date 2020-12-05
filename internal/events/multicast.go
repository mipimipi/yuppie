package events

import (
	"bytes"
	"fmt"
	"net"

	utils "gitlab.com/mipimipi/go-utils"
	"gitlab.com/mipimipi/yuppie/internal/network"
)

// send triggers sending multicast event messages for all changed state
// variables via all interfaces
func (me *Eventing) sendMulticast(svs []StateVar) {
	// nothing to do if state variables array is empty
	if len(svs) == 0 {
		return
	}

	log.Trace("sending multicast events ...")

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
