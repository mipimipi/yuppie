// Package main implements a simple test server
package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"

	log "github.com/sirupsen/logrus"
	"gitlab.com/mipimipi/go-utils"
	"gitlab.com/mipimipi/yuppie"
	"gitlab.com/mipimipi/yuppie/desc"
)

var (
	rootMeta     string
	rootChildren string
	song1Meta    string
	song2Meta    string
)

func init() {
	// determine external IP address
	addr, err := utils.IPaddr()
	if err != nil {
		log.Panicf("could not determine own IP addr: %v", err)
		panic(fmt.Sprintf("could not determine own IP addr: %v", err))
	}

	// assemble URL for music dir
	musicURL := url.URL{
		Scheme: "http",
		Host:   addr.String() + ":8008",
		Path:   "/music/",
	}

	// DIDL-Lite content for responses of Browse action
	song1Meta = `<item id="1" parentID="0" restricted="1">
						<dc:title>Air Shores</dc:title>
						<upnp:class>object.item.audioItem.musicTrack</upnp:class>
						<res protocolInfo="http-get:*:audio/mpeg:*">` + musicURL.String() + `1.mp3</res>
					</item>`
	song2Meta = `<item id="2" parentID="0" restricted="1">
					<dc:title>Much Moves</dc:title>
					<upnp:class>object.item.audioItem.musicTrack</upnp:class>
					<res protocolInfo="http-get:*:audio/mpeg:*">` + musicURL.String() + `2.mp3</res>
				</item>`
	rootMeta = `<container id="0" parentID="-1" restricted="1" searchable="0" childCount="2">
					<dc:title>root</dc:title>
					<upnp:class>object.container</upnp:class>
				</container>`
	rootChildren = song1Meta + song2Meta
}

// set HTTP handler functions
func setHTTPHandlers(srv *yuppie.Server) {
	// set HTTP handler for server root dir
	srv.HTTPHandleFunc("/",
		func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "Hello, here I am - the yuppie test server :)\n\n%s", srv.ServerString())
			fmt.Fprintf(w, "%s\n\n", srv.ServerString())
		},
	)
	// set HTTP handler for music dir
	srv.HTTPHandleFunc("/music/",
		func(w http.ResponseWriter, r *http.Request) {
			path, _ := url.QueryUnescape(r.URL.String())
			http.ServeFile(w, r, path[1:])
		},
	)
}

// setSOAPHandlers defines handler functions for the required actions of the
// ContentDirectory service
func setSOAPHandlers(srv *yuppie.Server) {
	srv.SOAPHandleFunc("ContentDirectory", "GetSearchCapabilities",
		func(reqArgs yuppie.StateVars) (yuppie.SOAPRespArgs, yuppie.SOAPError) {
			return yuppie.SOAPRespArgs{"SearchCaps": ""}, yuppie.SOAPError{}
		})
	srv.SOAPHandleFunc("ContentDirectory", "GetSortCapabilities",
		func(reqArgs yuppie.StateVars) (yuppie.SOAPRespArgs, yuppie.SOAPError) {
			return yuppie.SOAPRespArgs{"SortCaps": ""}, yuppie.SOAPError{}
		})
	srv.SOAPHandleFunc("ContentDirectory", "GetFeatureList",
		func(reqArgs yuppie.StateVars) (yuppie.SOAPRespArgs, yuppie.SOAPError) {
			return yuppie.SOAPRespArgs{"FeatureList": ""}, yuppie.SOAPError{}
		})
	srv.SOAPHandleFunc("ContentDirectory", "GetSystemUpdateID",
		func(reqArgs yuppie.StateVars) (yuppie.SOAPRespArgs, yuppie.SOAPError) {
			return yuppie.SOAPRespArgs{"Id": "1"}, yuppie.SOAPError{}
		})
	srv.SOAPHandleFunc("ContentDirectory", "GetServiceResetToken",
		func(reqArgs yuppie.StateVars) (yuppie.SOAPRespArgs, yuppie.SOAPError) {
			return yuppie.SOAPRespArgs{"ResetToken": "1"}, yuppie.SOAPError{}
		})
	srv.SOAPHandleFunc("ContentDirectory", "Browse",
		func(reqArgs yuppie.StateVars) (yuppie.SOAPRespArgs, yuppie.SOAPError) {
			return browse(reqArgs)
		})
}

// initialize state variable ServiceResetToken and SystemUpdateID
func initStateVariables(srv *yuppie.Server) {
	// set state variable "ServiceResetToken"
	sv, exists := srv.StateVariable("ContentDirectory", "ServiceResetToken")
	if !exists {
		panic("state variable 'ServiceResetToken' not found: cannot initialize")
	}
	sv.Lock()
	if sv.Get().(string) == "" {
		if err := sv.Init("0"); err != nil {
			panic(err)
		}
	}
	sv.Unlock()

	// set state variable "SystemUpdateID"
	sv, exists = srv.StateVariable("ContentDirectory", "SystemUpdateID")
	if !exists {
		panic("state variable 'SystemUpdateID' not found: cannot initialize")
	}
	sv.Lock()
	if sv.Get().(uint32) == 0 {
		if err := sv.Init(uint32(0)); err != nil {
			panic(err)
		}
	}
	sv.Unlock()
}

// browse implements the Browse action of the ContentDirectory service
func browse(reqArgs yuppie.StateVars) (respArgs yuppie.SOAPRespArgs, soapErr yuppie.SOAPError) {
	// retrieve and check input arguments
	objID, exists := reqArgs["ObjectID"]
	if !exists || (objID.String() != "0" && objID.String() != "1" && objID.String() != "2") {
		fmt.Printf("invalid ObjectID argument in browse action: '%s'", objID)
		soapErr = yuppie.SOAPError{
			Code: yuppie.UPnPErrorInvalidArgs,
			Desc: fmt.Sprintf("invalid ObjectID argument in browse action: '%s'", objID),
		}
		return
	}
	mode, exists := reqArgs["BrowseFlag"]
	if !exists || (mode.String() != "BrowseDirectChildren" && mode.String() != "BrowseMetadata") {
		fmt.Printf("invalid BrowseFlag argument in browse action: '%s'", objID)
		soapErr = yuppie.SOAPError{
			Code: yuppie.UPnPErrorInvalidArgs,
			Desc: fmt.Sprintf("invalid BrowseFlag argument in browse action: '%s'", objID),
		}
		return
	}

	// assemble result
	buf := new(bytes.Buffer)
	number := 0
	fmt.Fprint(buf, `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">`)
	switch objID.String() {
	case "0":
		if mode.String() == "BrowseMeta" {
			_, _ = buf.WriteString(rootMeta)
			number = 1
		} else {
			_, _ = buf.WriteString(rootChildren)
			number = 2
		}
	case "1":
		_, _ = buf.WriteString(song1Meta)
		number = 1
	case "2":
		_, _ = buf.WriteString(song2Meta)
		number = 1
	}
	fmt.Fprint(buf, `</DIDL-Lite>`)
	respArgs = yuppie.SOAPRespArgs{
		"Result":         buf.String(),
		"NumberReturned": fmt.Sprintf("%d", number),
		"TotalMatches":   fmt.Sprintf("%d", number),
		"UpdateID":       "1",
	}

	return
}

func main() {
	// set up logging: no log entries possible before this statement!
	log.SetLevel(log.ErrorLevel)

	// load device description from file
	root, err := desc.LoadRootDevice("./device.xml")
	if err != nil {
		fmt.Printf("cannot load device description file: %v", err)
		return
	}

	// make services: There's only the ContentDirectory service. It's
	// definition is loaded from file. The map key "ContentDirectory" must
	// correspond to the service id in the device description.
	svcs := make(desc.ServiceMap)
	svc, err := desc.LoadService("./contentdirectory.xml")
	if err != nil {
		fmt.Printf("cannot load description file for content directory service: %v", err)
		return
	}
	svcs["ContentDirectory"] = svc

	// create server
	srv, err := yuppie.New(yuppie.Config{}, root, svcs)
	if err != nil {
		fmt.Printf("cannot create server: %v", err)
		return
	}

	// set HTTP handlers for server root dir and for music dir
	setHTTPHandlers(srv)

	// set handlers for content directory service
	setSOAPHandlers(srv)

	// initialize state variables
	initStateVariables(srv)

	// start server
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go srv.Run(ctx, &wg)
	fmt.Println("server started")

	// wait for keystroke
	fmt.Println("press <ENTER> to stop the server")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')

	// stop server
	fmt.Println("stopping ...")
	cancel()
	wg.Wait()
	fmt.Println("server stopped")
	fmt.Println("bye")
}
