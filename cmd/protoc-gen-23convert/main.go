package main

import (
	"github.com/mingmxren/protokit"
	"log"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	if err := protokit.RunPlugin(NewPlugin()); err != nil {
		log.Fatal(err)
	}

}
