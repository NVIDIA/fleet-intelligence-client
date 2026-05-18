package main

import (
	"context"
	"os"
)

func main() {
	if err := execute(context.Background(), os.Args[1:]); err != nil {
		os.Exit(1)
	}
}
