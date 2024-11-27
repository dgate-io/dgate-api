package main

import (
	"os"

	"github.com/dgate-io/dgate-api/cmd/dgate-cli/commands"
	"github.com/dgate-io/dgate-api/pkg/dgclient"
)

var version string = "dev"

func main() {
	client := dgclient.NewDGateClient()
	err := commands.Run(client, version)
	if err != nil {
		os.Stderr.WriteString(err.Error())
		os.Stderr.WriteString("\n")
		os.Exit(1)
		return
	}
	os.Stdout.WriteString("\n")
}
