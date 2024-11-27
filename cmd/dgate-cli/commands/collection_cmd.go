package commands

import (
	"github.com/dgate-io/dgate-api/pkg/dgclient"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/urfave/cli/v2"
)

func CollectionCommand(client dgclient.DGateClient) *cli.Command {
	return &cli.Command{
		Name:      "collection",
		Aliases:   []string{"col"},
		Args:      true,
		ArgsUsage: "<command> <name>",
		Usage:     "collection <action> <args>",
		Subcommands: []*cli.Command{
			{
				Name:    "create",
				Aliases: []string{"mk"},
				Usage:   "create a collection",
				Action: func(ctx *cli.Context) error {
					col, err := createMapFromArgs[spec.Collection](
						ctx.Args().Slice(), "name", "schema",
					)
					if err != nil {
						return err
					}
					err = client.CreateCollection(col)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(col)
				},
			},
			{
				Name:    "delete",
				Aliases: []string{"rm"},
				Usage:   "delete a collection",
				Action: func(ctx *cli.Context) error {
					col, err := createMapFromArgs[spec.Collection](
						ctx.Args().Slice(), "name",
					)
					if err != nil {
						return err
					}
					err = client.DeleteCollection(
						col.Name, col.NamespaceName,
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
				Usage:   "list collections",
				Action: func(ctx *cli.Context) error {
					nsp, err := createMapFromArgs[dgclient.NamespacePayload](
						ctx.Args().Slice(),
					)
					if err != nil {
						return err
					}
					col, err := client.ListCollection(nsp.Namespace)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(col)
				},
			},
			{
				Name:  "get",
				Usage: "get a collection",
				Action: func(ctx *cli.Context) error {
					col, err := createMapFromArgs[spec.Collection](
						ctx.Args().Slice(), "name",
					)
					if err != nil {
						return err
					}
					col, err = client.GetCollection(
						col.Name, col.NamespaceName,
					)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(col)
				},
			},
		},
	}
}
