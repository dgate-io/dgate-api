package commands

import (
	"github.com/dgate-io/dgate-api/pkg/dgclient"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/urfave/cli/v2"
)

func NamespaceCommand(client dgclient.DGateClient) *cli.Command {
	return &cli.Command{
		Name:      "namespace",
		Args:      true,
		Aliases:   []string{"ns"},
		ArgsUsage: "<command> <name>",
		Usage:     "namespace <action> <args>",
		Subcommands: []*cli.Command{
			{
				Name:    "create",
				Aliases: []string{"mk"},
				Usage:   "create a namespace",
				Action: func(ctx *cli.Context) error {
					ns, err := createMapFromArgs[spec.Namespace](
						ctx.Args().Slice(), "name",
					)
					if err != nil {
						return err
					}
					err = client.CreateNamespace(ns)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(ns)
				},
			},
			{
				Name:    "delete",
				Aliases: []string{"rm"},
				Usage:   "delete a namespace",
				Action: func(ctx *cli.Context) error {
					if ctx.NArg() != 1 || ctx.Args().First() == "" {
						return cli.ShowSubcommandHelp(ctx)
					}
					err := client.DeleteNamespace(ctx.Args().First())
					if err != nil {
						return err
					}
					return nil
				},
			},
			{
				Name:    "list",
				Aliases: []string{"ls"},
				Usage:   "list namespaces",
				Action: func(ctx *cli.Context) error {
					ns, err := client.ListNamespace()
					if err != nil {
						return err
					}
					return jsonPrettyPrint(ns)
				},
			},
			{
				Name:  "get",
				Usage: "get a namespace",
				Action: func(ctx *cli.Context) error {
					ns, err := createMapFromArgs[spec.Namespace](
						ctx.Args().Slice(), "name",
					)
					if err != nil {
						return err
					}
					ns, err = client.GetNamespace(ns.Name)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(ns)
				},
			},
		},
	}
}
