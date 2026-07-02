package main

import (
	"log"
	"os"
)

func main() {
	log.Fatal("fatal error")
	log.Fatalf("fatal: %v", "reason")
	log.Fatalln("fatal line")
	os.Exit(1)
}
