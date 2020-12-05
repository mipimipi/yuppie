// Package events implements eventing for state variables. It covers multicast
// and subscription- based eventing
package events

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	l "github.com/sirupsen/logrus"
	"gitlab.com/mipimipi/yuppie/internal/network"
	"gitlab.com/mipimipi/yuppie/internal/types"
)

var log *l.Entry

// event interval in milli seconds
const eventInterval time.Duration = 200

// minimal subscription timeout in seconds as defined in UPnP Device
// Architecture 2.0
const minSubTimeout time.Duration = 1800

// IP address for evnt multicasting
const multicastAddrIPv4 = "239.255.255.246:7900"

var multicastUDPAddr *net.UDPAddr

func init() {
	// set events indicatorg
	log = l.WithFields(l.Fields{"srv": "upnp:events"})

	var err error
	multicastUDPAddr, err = net.ResolveUDPAddr("udp4", multicastAddrIPv4)
	if err != nil {
		log.Panicf("could not resolve %s: %s", multicastAddrIPv4, err)
	}
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
	ToBeEvented() bool
	ToBeMulticasted() bool
}

// Subscription represents the subscription of one recipient to all evented
// state variables
type Subscription struct {
	sid       uuid.UUID
	timer     *time.Timer
	urls      []*url.URL
	stateVars []StateVar
	sequence  uint32
}

// Eventing implements multicast and subscription based eventing as specified
// in the UPnP device architecture 2.0
type Eventing struct {
	Listener   chan StateVar
	key        uint32
	changes    []StateVar
	subs       map[uuid.UUID]*Subscription
	stop       chan struct{}
	mutChanges *sync.Mutex
	mutSubs    *sync.Mutex
	infs       []net.Interface
	bootID     *types.BootID
}

// NewEventing creates an Eventing instance. wanted contains the list of network
// interfaces that where configured, booID is a function that returns the current
// BootID. if no interfaces are configured, all interfaces are used
func NewEventing(wanted []string, bootID *types.BootID) (evt *Eventing, err error) {
	evt = new(Eventing)

	evt.Listener = make(chan StateVar)
	evt.mutChanges = new(sync.Mutex)

	evt.subs = make(map[uuid.UUID]*Subscription)
	evt.mutSubs = new(sync.Mutex)

	evt.bootID = bootID
	evt.infs, err = network.Interfaces(wanted)
	if err != nil {
		err = errors.Wrap(err, "cannot determine network interfaces for eventing")
		return
	}

	return
}

// Listen listens to changes for state variables and stores them in me.changes
func (me *Eventing) Listen(ctx context.Context) {
	go func() {
		defer func() {
			close(me.Listener)
			log.Trace("event listener stopped")
		}()

		log.Trace("event listener started")

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

// Run implemente the main eventing loop and triggers event sending (if
// necessary - i.e. if state variable were changed)
func (me *Eventing) Run() {

	go func() {
		me.stop = make(chan struct{})
		ticker := time.NewTicker(eventInterval * time.Millisecond)

		defer func() {
			ticker.Stop()
			close(me.stop)
			log.Trace("eventing stopped")
		}()

		log.Trace("eventing running")

		for {
			select {
			case <-ticker.C:
				// extract to be multicasted and to be evented state variables
				// from changes array. Clear the array afterwards.
				me.mutChanges.Lock()
				var toBeMulticasted, toBeEvented []StateVar
				for _, sv := range me.changes {
					if sv.ToBeMulticasted() {
						toBeMulticasted = append(toBeMulticasted, sv)
					}
					if sv.ToBeEvented() {
						toBeEvented = append(toBeEvented, sv)
					}
				}
				me.changes = []StateVar{}
				me.mutChanges.Unlock()

				// send multicast events
				me.sendMulticast(toBeMulticasted)

				// send sunscription events
				if len(toBeEvented) > 0 {
					for _, sub := range me.subs {
						sub.sendEvent()
					}
				}

			case <-me.stop:
				return
			}
		}
	}()
}

// Stop stops sending regular change events
func (me *Eventing) Stop() {
	me.stop <- struct{}{}
}

// AddSub adds a new subscription
func (me *Eventing) AddSub(dur time.Duration, urls []*url.URL, svs []StateVar) (sid uuid.UUID) {
	// get new subscription id
	sid = uuid.New()

	// create new subscription
	sub := Subscription{
		sid: sid,
		// after timeout (dur) is exceeded, the subscription is removed
		timer: time.AfterFunc(
			dur,
			func() {
				if err := me.RemoveSub(sid); err != nil {
					log.Errorf("could not remove subscription: %v", err)
				}
				log.Tracef("removed subscription %s due to timeout", sid.String())
			},
		),
		urls:      urls,
		stateVars: svs,
	}
	me.mutSubs.Lock()
	me.subs[sid] = &sub
	me.mutSubs.Unlock()

	// send initial event
	sub.sendEvent()

	log.Tracef("added subscription %s of %s", sid.String(), urls[0].String())

	return
}

// RemoveSub removes the subscription with the ID sid. In case there's no
// subscription with that ID, an error is returned
func (me *Eventing) RemoveSub(sid uuid.UUID) (err error) {
	me.mutSubs.Lock()
	defer me.mutSubs.Unlock()

	_, ok := me.subs[sid]
	if !ok {
		err = fmt.Errorf("no subscription with uuid %s found: cannot unsubscribe", sid)
		log.Error(err)
		return
	}

	log.Tracef("removed subscription %s of %s", sid.String(), me.subs[sid].urls[0].String())

	delete(me.subs, sid)

	return
}

// RemoveAllSubs remove all subscriptions
func (me *Eventing) RemoveAllSubs() {
	me.mutSubs.Lock()
	defer me.mutSubs.Unlock()
	for sid := range me.subs {
		delete(me.subs, sid)
	}
	log.Trace("all subscriptions removed")
}

// RenewSub renews the subscription with ID sid. In case there's no subscription
// with that ID, an error is returned
func (me *Eventing) RenewSub(sid uuid.UUID, dur time.Duration) (err error) {
	me.mutSubs.Lock()
	defer me.mutSubs.Unlock()

	// check if subscription with sid exists
	sub, ok := me.subs[sid]
	if !ok {
		err = fmt.Errorf("no subscription with uuid:%s found: cannot renew subscription", sid.String())
		log.Error(err)
		return
	}

	sub.timer.Reset(dur)

	log.Tracef("subscription %s of %s renewed", sid.String(), sub.urls[0].String())
	return
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
