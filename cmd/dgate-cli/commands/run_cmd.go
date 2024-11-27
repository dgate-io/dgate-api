package commands

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/dgate-io/dgate-api/pkg/dgclient"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

func Run(client dgclient.DGateClient, version string) error {
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		bv := buildInfo.Main.Version
		if bv != "" && bv != "(devel)" {
			version = buildInfo.Main.Version
		}
	}

	app := &cli.App{
		Name:                   "dgate-cli",
		Usage:                  "a command line interface for DGate (API Gateway) Admin API",
		Version:                version,
		UseShortOptionHandling: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "admin",
				Value:   "http://localhost:9080",
				EnvVars: []string{"DGATE_ADMIN_API"},
				Usage:   "the url for the file client",
			},
			// only basic auth support for now
			&cli.StringFlag{
				Name:    "auth",
				Aliases: []string{"a"},
				EnvVars: []string{"DGATE_ADMIN_AUTH"},
				Usage:   "basic auth username:password; or just username for password prompt",
			},
			&cli.BoolFlag{
				Name:        "follow",
				DefaultText: "false",
				Aliases:     []string{"f"},
				EnvVars:     []string{"DGATE_FOLLOW_REDIRECTS"},
				Usage:       "follows redirects, useful for raft leader changes",
			},
			&cli.BoolFlag{
				Name:        "verbose",
				DefaultText: "false",
				Aliases:     []string{"V"},
				Usage:       "enable verbose logging",
			},
		},
		Before: func(ctx *cli.Context) (err error) {
			var authOption dgclient.Options = func(dc dgclient.DGateClient) {}
			if auth := ctx.String("auth"); strings.Contains(auth, ":") {
				pair := strings.SplitN(ctx.String("auth"), ":", 2)
				username := pair[0]
				password := ""
				if len(pair) > 1 {
					password = pair[1]
				}
				authOption = dgclient.WithBasicAuth(
					username, password,
				)
			} else if auth != "" {
				fmt.Printf("password for %s:", auth)
				password, err := term.ReadPassword(0)
				if err != nil {
					return err
				}
				fmt.Print("\n")
				authOption = dgclient.WithBasicAuth(
					auth, string(password),
				)
			}
			return client.Init(ctx.String("admin"),
				http.DefaultClient,
				dgclient.WithFollowRedirect(
					ctx.Bool("follow"),
				),
				dgclient.WithUserAgent(
					"DGate CLI "+version+
						";os="+runtime.GOOS+
						";arch="+runtime.GOARCH,
				),
				dgclient.WithVerboseLogging(
					ctx.Bool("verbose"),
				),
				authOption,
			)
		},
		Action: func(ctx *cli.Context) error {
			return ctx.App.Command("help").Run(ctx)
		},
		Commands: []*cli.Command{
			NamespaceCommand(client),
			ServiceCommand(client),
			ModuleCommand(client),
			RouteCommand(client),
			DomainCommand(client),
			CollectionCommand(client),
			DocumentCommand(client),
			SecretCommand(client),
		},
	}

	return app.Run(os.Args)
}
