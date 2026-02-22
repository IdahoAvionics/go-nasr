package main

import (
	"fmt"
	"os"

	nasr "github.com/IdahoAvionics/go-nasr"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: %s <nasr-subscription.zip> <output.db>\n", os.Args[0])
		os.Exit(1)
	}
	if err := nasr.Extract(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
