// Package main generates the Go types that implement the UPnP types i4, ui2,
// string, fixed.14.4 etc. It generates the file yuppie/types_gen.go
// based on the mapping configuration at the beginning of the main function.
// To (re-)generate the Go types, go into the directory yuppie/gen and
// execute ./gen on the command line.
// Note: If a new mapping is introduced, corresponding functions for marshalling
// and unmarshalling must be available in ../conversion.go.
package main

import (
	"fmt"
	"os"
	"strings"
)

// upnp2go is the structure for the mapping between UPnP and Go types
type upnp2Go struct {
	UPnPType string
	TypeName string
	GoType   string
}

func (me upnp2Go) TypeNameTitle() string {
	return strings.Title(me.TypeName)
}

// Data contains the type mapping data to be passed to the template engine
type Data struct {
	TypeMapping []upnp2Go
}

func main() {
	// mapping between SOAP and Go types
	data := Data{
		[]upnp2Go{
			{"ui1", "upnpUI1", "uint8"},
			{"ui2", "upnpUI2", "uint16"},
			{"ui4", "upnpUI4", "uint32"},
			{"ui8", "upnpUI8", "uint64"},
			{"i1", "upnpI1", "int8"},
			{"i2", "upnpI2", "int16"},
			{"i4", "upnpI4", "int32"},
			{"int", "upnpInt", "int64"},
			{"r4", "upnpR4", "float32"},
			{"r8", "upnpR8", "float64"},
			{"number", "upnpNumber", "float64"},
			{"fixed.14.4", "upnpFixed14_4", "float64"},
			{"float", "upnpFloat", "float64"},
			{"char", "upnpChar", "rune"},
			{"string", "upnpString", "string"},
			{"date", "upnpDate", "time.Time"},
			{"dateTime", "upnpDateTime", "time.Time"},
			{"dateTime.tz", "upnpDateTimeTz", "time.Time"},
			{"time", "upnpTimeOfDay", "timeOfDay"},
			{"time.tz", "upnpTimeOfDayTz", "timeOfDay"},
			{"boolean", "upnpBoolean", "bool"},
			{"bin.base64", "upnpBinBase64", "[]byte"},
			{"bin.hex", "upnpBinHex", "[]byte"},
			{"uri", "upnpURI", "*url.URL"},
		},
	}

	// create output file
	out, err := os.Create("../types_gen.go")
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	defer out.Close()

	// generate content of output file based on the template and the
	// type mapping as defined in soap2Go
	if err = typesTmpl.Execute(out, data); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
