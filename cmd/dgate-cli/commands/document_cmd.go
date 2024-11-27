package commands

import (
	"github.com/dgate-io/dgate-api/pkg/dgclient"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/urfave/cli/v2"
)

func DocumentCommand(client dgclient.DGateClient) *cli.Command {
	return &cli.Command{
		Name:      "document",
		Aliases:   []string{"doc"},
		Args:      true,
		ArgsUsage: "<command> <name>",
		Usage:     "document <action> <args>",
		Subcommands: []*cli.Command{
			{
				Name:    "create",
				Aliases: []string{"mk"},
				Usage:   "create a document",
				Action: func(ctx *cli.Context) error {
					doc, err := createMapFromArgs[spec.Document](
						ctx.Args().Slice(), "id",
						"collection", "data",
					)
					if err != nil {
						return err
					}
					err = client.CreateDocument(doc)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(doc)
				},
			},
			{
				Name:    "delete",
				Aliases: []string{"rm"},
				Usage:   "delete a document",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "all",
						Usage: "delete all documents",
					},
				},
				Action: func(ctx *cli.Context) error {
					if ctx.Bool("all") {
						doc, err := createMapFromArgs[spec.Document](
							ctx.Args().Slice())
						if err != nil {
							return err
						}
						err = client.DeleteAllDocument(
							doc.NamespaceName, doc.CollectionName)
						if err != nil {
							return err
						}
						return nil
					} else {
						doc, err := createMapFromArgs[spec.Document](
							ctx.Args().Slice(), "id",
						)
						if err != nil {
							return err
						}
						err = client.DeleteDocument(doc.ID,
							doc.NamespaceName, doc.CollectionName)
						if err != nil {
							return err
						}
					}
					return nil
				},
			},
			{
				Name:    "list",
				Aliases: []string{"ls"},
				Usage:   "list documents",
				Action: func(ctx *cli.Context) error {
					d, err := createMapFromArgs[spec.Document](
						ctx.Args().Slice(), "collection",
					)
					if err != nil {
						return err
					}
					doc, err := client.ListDocument(
						d.NamespaceName, d.CollectionName)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(doc)
				},
			},
			{
				Name:  "get",
				Usage: "get a document",
				Action: func(ctx *cli.Context) error {
					doc, err := createMapFromArgs[spec.Document](
						ctx.Args().Slice(), "id",
					)
					if err != nil {
						return err
					}
					doc, err = client.GetDocument(doc.ID,
						doc.NamespaceName, doc.CollectionName)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(doc)
				},
			},
		},
	}
}
