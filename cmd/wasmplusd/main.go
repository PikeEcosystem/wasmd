package main

import (
	"os"

	"github.com/PikeEcosystem/cosmos-sdk/server"
	svrcmd "github.com/PikeEcosystem/cosmos-sdk/server/cmd"

	app "github.com/PikeEcosystem/wasmd/appplus"
)

func main() {
	rootCmd, _ := NewRootCmd()

	if err := svrcmd.Execute(rootCmd, app.DefaultNodeHome); err != nil {
		switch e := err.(type) {
		case server.ErrorCode:
			os.Exit(e.Code)

		default:
			os.Exit(1)
		}
	}
}
