package events

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"gitlab.com/mipimipi/yuppie/internal/network"
)

const (
	// minimal subscription timeout in seconds as defined in UPnP Device
	// Architecture 2.0
	minSubTimeout time.Duration = 1800

	// event interval in seconds
	eventInterval time.Duration = 10
)

var (
	reURLs    = regexp.MustCompile(`^(<.+>)+$`)
	reTimeOut = regexp.MustCompile(`Second-\d+`)
)

// Subscription represents the subscription of one recipient to all evented
// state variables
type Subscription struct {
	sid       uuid.UUID
	timer     *time.Timer
	urls      []*url.URL
	stateVars []StateVar
	sequence  uint32
	stop      chan struct{}
}

// Subscriptions maps subscription ID's to the corresponding subscription
type Subscriptions map[uuid.UUID]*Subscription

// NewSubscriptionMap does what the name suggests
func NewSubscriptionMap() Subscriptions {
	return make(Subscriptions)
}

// Add adds a new subscription
func (me Subscriptions) Add(dur time.Duration, urls []*url.URL, svs []StateVar) (sid uuid.UUID) {
	// get new subscription id
	sid = uuid.New()

	// create new subscription
	me[sid] = &Subscription{
		sid: sid,
		// after timeout (dur) is exceeded, the subscription is removed
		timer: time.AfterFunc(
			dur,
			func() {
				if err := me.Remove(sid); err != nil {
					log.Errorf("could not remove subscription: %v", err)
				}
				log.Tracef("removed subscription %s due to timeout", sid.String())
			},
		),
		urls:      urls,
		stateVars: svs,
		stop:      make(chan struct{}),
	}

	// send events periodically
	me[sid].Run()

	log.Tracef("added subscription %s of %s", sid.String(), urls[0].String())

	return
}

// Remove removes the subscription with the ID sid. In case there's no
// subscription with that ID, en error is returned
func (me Subscriptions) Remove(sid uuid.UUID) (err error) {
	_, ok := me[sid]
	if !ok {
		err = fmt.Errorf("no subscription with uuid %s found: cannot unsubscribe", sid)
		log.Error(err)
		return
	}

	// stop sending events periodically
	me[sid].Stop()

	log.Tracef("removed subscription %s of %s", sid.String(), me[sid].urls[0].String())

	delete(me, sid)

	return
}

// RemoveAll remove all subscriptions
func (me Subscriptions) RemoveAll() {
	for sid, sub := range me {
		sub.Stop()
		delete(me, sid)
	}
	log.Trace("all subscriptions removed")
}

// Renew renews the subscription with ID sid. In case there's no subscription
// with that ID, en error is returned
func (me Subscriptions) Renew(sid uuid.UUID, dur time.Duration) (err error) {
	// check if subscription with sid exists
	sub, ok := me[sid]
	if !ok {
		err = fmt.Errorf("no subscription with uuid:%s found: cannot renew subscription", sid.String())
		log.Error(err)
		return
	}

	sub.timer.Reset(dur)

	log.Tracef("subscription %s of %s renewed", sid.String(), sub.urls[0].String())

	return
}

// Run implements the main eventing loop: Each eventInterval seconds an event
// for all state variables is sent until a stop request is received
func (me Subscription) Run() {
	me.sendEvent()

	go func() {
		ticker := time.NewTicker(eventInterval * time.Second)
		defer ticker.Stop()

		log.Tracef("subscription %s of %s started", me.sid.String(), me.urls[0].String())

		for {
			select {
			case <-ticker.C:
				me.sendEvent()
			case <-me.stop:
				log.Tracef("subscription %s of %s stopped", me.sid.String(), me.urls[0].String())
				return
			}
		}
	}()
}

// Stop stops the main event loop for the subscription
func (me Subscription) Stop() {
	close(me.stop)
}

// sendEvent sends an event to the recipient of this subscription. As the UPnP
// Device Architecture 2.0 requires, it tries to send the message to all urls
// of that recipient subsequently as long as one these transmissions could be
// done without an error. Note: It's important that the very first event that
// is sent to a subscription has the the sequence id (SEQ) 0.
func (me *Subscription) sendEvent() {
	// assemble event message body
	body := marshalStatVars(me.stateVars)

	success := false
	for _, u := range me.urls {
		var err error

		// assemble message
		msg := new(bytes.Buffer)
		fmt.Fprintf(msg, "NOTIFY %s HTTP/1.1\r\n", u.Path)
		fmt.Fprintf(msg, "HOST: %s:%s\r\n", u.Hostname(), u.Port())
		fmt.Fprint(msg, "CONTENT-TYPE: text/xml; charset=\"utf-8\"\r\n")
		fmt.Fprintf(msg, "CONTENT-LENGTH: %d\r\n", len(body))
		fmt.Fprint(msg, "NT: upnp:event\r\n")
		fmt.Fprint(msg, "NTS: upnp:propchange\r\n")
		fmt.Fprintf(msg, "SID: uuid:%s\r\n", me.sid.String())
		fmt.Fprintf(msg, "SEQ: %d\r\n", me.sequence)
		fmt.Fprint(msg, "\r\n")
		fmt.Fprint(msg, string(body))
		fmt.Fprint(msg, "\r\n")

		// create TCP connection
		var conn net.Conn
		if conn, err = network.TCPConn(u.Hostname() + ":" + u.Port()); err != nil {
			log.Errorf("subscriptions: cannot create TCP connection to %s", u.Hostname()+":"+u.Port())
			continue
		}

		// send event message
		if err = network.SendTCP(conn, msg.Bytes()); err != nil {
			log.Infof("cannot send subscription event to %s", u.Hostname()+":"+u.Port())
			conn.Close()
			continue
		}
		conn.Close()

		log.Tracef("sent subscription event to %s, seq=%d ", u.String(), me.sequence)
		success = true
		break
	}
	if success {
		me.sequence++
	}
}

// ParseURLs parses the callback string that the recipient submitted as part of
// the subscription request. If the string is not according to the required
// format an error is returned. As defined in UPnP Device Architecture 2.0, the
// required format is <url_1><url_2>...<url_n>, where url_x must be a valid
// url for x=1, ..., n
func ParseURLs(callback string) (urls []*url.URL, err error) {
	// callback must be of the form <url_1><url_2>...<url_n>
	if !reURLs.MatchString(callback) {
		err = fmt.Errorf("callback malformatted: %s", callback)
		log.Error(err)
		return
	}

	// extract urls from callback
	callback = callback[1 : len(callback)-1]
	a := strings.Split(callback, "><")
	for _, s := range a {
		u, err := url.ParseRequestURI(s)
		if err != nil {
			err = fmt.Errorf("callback malformatted: %s", s)
			log.Error(err)
			return nil, err
		}
		urls = append(urls, u)
	}

	// at least one url required
	if len(urls) == 0 {
		err = fmt.Errorf("callback malformatted: %s", callback)
		log.Error(err)
		return nil, err
	}

	return
}

// ParseTimeout parses the timeout string that the recipient submitted as part
// of the subscription request. If the string is not according to the required
// format an error is returned. As defined in UPnP Device Architecture 2.0, the
// required format is Second-<number>, where <number> is requested timeout in
// seconds
func ParseTimeout(t string) (dur time.Duration, err error) {
	if t == "" {
		return
	}

	if !reTimeOut.MatchString(t) {
		err = errors.Wrapf(err, "timeout malformatted: %s", t)
		log.Error(err)
		return
	}

	f, err := strconv.ParseFloat(t[7:], 64)
	if err != nil {
		err = errors.Wrapf(err, "timeout malformatted: %s", t)
		log.Error(err)
		return
	}

	// make sure that minimum timeout is kept
	if time.Duration(f) < minSubTimeout*time.Second {
		dur = minSubTimeout * time.Second
	} else {
		dur = time.Duration(f)
	}

	return
}
