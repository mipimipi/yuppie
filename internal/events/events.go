// Package events implements eventing for state variables. It covers multicast
// and subscription- based eventing
package events

import (
	"bytes"
	"fmt"

	l "github.com/sirupsen/logrus"
)

var log *l.Entry

func init() {
	// set events indicatorg
	log = l.WithFields(l.Fields{"srv": "upnp:events"})
}

// StateVar represents a state variable for eventing (i.e. the functions that
// are required for eventing)
type StateVar interface {
	Name() string
	String() string
	ServiceType() string
	ServiceVersion() string
	DeviceUDN() string
	ServiceID() string
}

// marshalStatVars marshals an array of state variable into XML for event
// messages
func marshalStatVars(svs []StateVar) []byte {
	xml := new(bytes.Buffer)
	fmt.Fprint(xml, "<?xml version=\"1.0\"?>")
	fmt.Fprint(xml, "<e:propertyset xmlns:e=\"urn:schemas-upnp-org:event-1-0\">")
	for _, sv := range svs {
		fmt.Fprint(xml, "<e:property>")
		fmt.Fprintf(xml, "<%s>%s</%s>", sv.Name(), sv.String(), sv.Name())
		fmt.Fprint(xml, "</e:property>")
	}
	fmt.Fprint(xml, "</e:propertyset>\r\n")

	return xml.Bytes()
}
