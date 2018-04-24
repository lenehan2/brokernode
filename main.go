package main

import (
	"github.com/gobuffalo/pop"
	"github.com/oysterprotocol/brokernode/actions"
	"log"
	"math/rand"
	"time"
)

func main() {
	pop.Debug = false
	// Setup rand. See https://til.hashrocket.com/posts/355f31f19c-seeding-golangs-rand
	rand.Seed(time.Now().Unix())
	app := actions.App()
	if err := app.Serve(); err != nil {
		log.Fatal(err)
	}
}
