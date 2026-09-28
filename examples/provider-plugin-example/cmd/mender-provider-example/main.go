package main

import (
	"log"

	providerplugin "github.com/orz-i/Mender/sdk/providerplugin/go"
)

func main() {
	if err := providerplugin.Serve(newEchoPlugin()); err != nil {
		log.Fatal(err)
	}
}
