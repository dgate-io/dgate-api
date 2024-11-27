package commands

import (
	"github.com/dgate-io/dgate-api/pkg/dgclient"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/urfave/cli/v2"
)

func ModuleCommand(client dgclient.DGateClient) *cli.Command {
	return &cli.Command{
		Name:      "module",
		Aliases:   []string{"mod"},
		Args:      true,
		ArgsUsage: "<command> <name>",
		Usage:     "module <action> <args>",
		Subcommands: []*cli.Command{
			{
				Name:    "create",
				Aliases: []string{"mk"},
				Usage:   "create a module",
				Action: func(ctx *cli.Context) error {
					mod, err := createMapFromArgs[spec.Module](
						ctx.Args().Slice(), "name", "payload",
					)
					if err != nil {
						return err
					}
					err = client.CreateModule(mod)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(mod)
				},
			},
			{
				Name:    "delete",
				Aliases: []string{"rm"},
				Usage:   "delete a module",
				Action: func(ctx *cli.Context) error {
					mod, err := createMapFromArgs[spec.Module](
						ctx.Args().Slice(), "name",
					)
					if err != nil {
						return err
					}
					err = client.DeleteModule(
						mod.Name, mod.NamespaceName,
					)
					if err != nil {
						return err
					}
					return nil
				},
			},
			{
				Name:    "list",
				Aliases: []string{"ls"},
				Usage:   "list modules",
				Action: func(ctx *cli.Context) error {
					nsp, err := createMapFromArgs[dgclient.NamespacePayload](
						ctx.Args().Slice(),
					)
					if err != nil {
						return err
					}
					mod, err := client.ListModule(nsp.Namespace)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(mod)
				},
			},
			{
				Name:  "get",
				Usage: "get a module",
				Action: func(ctx *cli.Context) error {
					mod, err := createMapFromArgs[spec.Module](
						ctx.Args().Slice(), "name",
					)
					if err != nil {
						return err
					}
					mod, err = client.GetModule(
						mod.Name, mod.NamespaceName,
					)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(mod)
				},
			},
		},
	}
}
