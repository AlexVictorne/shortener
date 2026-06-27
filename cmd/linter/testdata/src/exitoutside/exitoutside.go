package main

import (
	"log"
	"os"
)

func helper() {
	log.Fatal("boom")   // want `log\.Fatal called outside of main\.main`
	log.Fatalf("oops")  // want `log\.Fatalf called outside of main\.main`
	log.Fatalln("bye")  // want `log\.Fatalln called outside of main\.main`
	os.Exit(2)          // want `os\.Exit called outside of main\.main`
}

func main() {
	helper()
}
