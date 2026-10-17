package main

import (
	"fmt"

	"github.com/godbus/dbus/v5"
	"reflect"
)

func main() {
	// Minimal repro of the schema-vs-godbus store contract:
	// real message: one out param of type 'ao'
	var results, locked []dbus.ObjectPath
	src := []interface{}{[]dbus.ObjectPath{}}
	err := dbus.Store(src, &results, &locked)
	fmt.Printf("two dests vs 1 src param: %v (muster's bug)\n", err)

	var single []dbus.ObjectPath
	err = dbus.Store(src, &single)
	fmt.Printf("one dest vs 1 src:       %v\n", err)

	// the compatible-shape case that DOES work: reply carrying 2 out params
	src2 := []interface{}{[]dbus.ObjectPath{}, []dbus.ObjectPath{}}
	err = dbus.Store(src2, &results, &locked)
	fmt.Printf("two dests vs 2 src dois: %v\n", err)

	_ = reflect.TypeOf("")
}
