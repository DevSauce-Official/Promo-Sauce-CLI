package main

import (
	"fmt"
	"os"

	"github.com/github/gh/command"
)

func main() {
	if err := command.RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
