package main

import (
	"fmt"
	"log"
	"os"

	"github.com/alexflint/go-arg"
)

func main() {
	log.SetFlags(log.Flags() | log.Lshortfile)
	err := mainErr()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error in main: %v\n", err)
		os.Exit(1)
	}
}

func mainErr() error {
	var args struct {
		Put *PutCmd `arg:"subcommand"`
		Get *GetCmd `arg:"subcommand"`
	}
	arg.MustParse(&args)
	switch {
	case args.Put != nil:
		return put(args.Put)
	case args.Get != nil:
		return get(args.Get)
	default:
		panic("unimplemented")
	}
}
