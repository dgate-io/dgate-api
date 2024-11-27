package commands

import (
	"github.com/dgate-io/dgate-api/pkg/dgclient"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/urfave/cli/v2"
)

func DomainCommand(client dgclient.DGateClient) *cli.Command {
	return &cli.Command{
		Name:      "domain",
		Aliases:   []string{"dom"},
		Args:      true,
		ArgsUsage: "<command> <name>",
		Usage:     "domain <action> <args>",
		Subcommands: []*cli.Command{
			{
				Name:    "create",
				Aliases: []string{"mk"},
				Usage:   "create a domain",
				Action: func(ctx *cli.Context) error {
					dom, err := createMapFromArgs[spec.Domain](
						ctx.Args().Slice(), "name", "patterns",
					)
					if err != nil {
						return err
					}
					err = client.CreateDomain(dom)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(dom)
				},
			},
			{
				Name:    "delete",
				Aliases: []string{"rm"},
				Usage:   "delete a domain",
				Action: func(ctx *cli.Context) error {
					dom, err := createMapFromArgs[spec.Domain](
						ctx.Args().Slice(), "name",
					)
					if err != nil {
						return err
					}
					err = client.DeleteDomain(
						dom.Name, dom.NamespaceName,
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
				Usage:   "list domains",
				Action: func(ctx *cli.Context) error {
					nsp, err := createMapFromArgs[dgclient.NamespacePayload](
						ctx.Args().Slice(),
					)
					if err != nil {
						return err
					}
					dom, err := client.ListDomain(nsp.Namespace)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(dom)
				},
			},
			{
				Name:  "get",
				Usage: "get a domain",
				Action: func(ctx *cli.Context) error {
					dom, err := createMapFromArgs[spec.Domain](
						ctx.Args().Slice(), "name",
					)
					if err != nil {
						return err
					}
					dom, err = client.GetDomain(
						dom.Name, dom.NamespaceName,
					)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(dom)
				},
			},
		},
	}
}
