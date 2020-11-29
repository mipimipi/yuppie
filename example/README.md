# Example: Simple UPnP Server

In this example a simple UPnP server is implemented serving just two songs. The songs are taken from [Small Colins](https://smallcolin.bandcamp.com/) album [Entry](https://cctrax.com/small-colin/entry), licensed under [CC-BY-4.0](https://creativecommons.org/licenses/by/4.0/).

The server only implements the [ContentDirectory service](http://www.upnp.org/specs/av/UPnP-av-ContentDirectory-Service.pdf) and only its required actions. The defintions of the server device and the ContentDirectory service are taken from XML files ([devicedesc.xml](./devicedesc.xml) and [contentdirectory.xml](./contentdirectory.xml)).

To run the server, just clone this repository, navigate into the folder `example`. Build the server:

    go build

And start it:

    ./example

Now, it should appear in your UPnP client as "go-upnp test server". You can stop the server by pressing the ENTER key.

The server consists of a [single file](./main.go) only. Most of it contains boilerplate code, for example code to provide handlers for the SOAP actions that are required by the UPnP standard. The interesting parts are ...

* the browse function since it implements the browse action of the ContentDirectory service and
* the HTTP handler functions since they implement a general HTML page for the server and the transfer of the music files.