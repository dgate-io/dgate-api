package commands

import (
	"github.com/dgate-io/dgate-api/pkg/dgclient"
	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/urfave/cli/v2"
)

func SecretCommand(client dgclient.DGateClient) *cli.Command {
	return &cli.Command{
		Name:      "secret",
		Aliases:   []string{"sec"},
		Args:      true,
		ArgsUsage: "<command> <name>",
		Usage:     "secret <action> <args>",
		Subcommands: []*cli.Command{
			{
				Name:    "create",
				Aliases: []string{"mk"},
				Usage:   "create a secret",
				Action: func(ctx *cli.Context) error {
					sec, err := createMapFromArgs[spec.Secret](
						ctx.Args().Slice(), "name", "data",
					)
					if err != nil {
						return err
					}
					err = client.CreateSecret(sec)
					if err != nil {
						return err
					}
					// redact the data field
					sec.Data = "**redacted**"
					return jsonPrettyPrint(sec)
				},
			},
			{
				Name:    "delete",
				Aliases: []string{"rm"},
				Usage:   "delete a secret",
				Action: func(ctx *cli.Context) error {
					sec, err := createMapFromArgs[spec.Secret](
						ctx.Args().Slice(), "name",
					)
					if err != nil {
						return err
					}
					err = client.DeleteSecret(
						sec.Name, sec.NamespaceName)
					if err != nil {
						return err
					}
					return nil
				},
			},
			{
				Name:    "list",
				Aliases: []string{"ls"},
				Usage:   "list services",
				Action: func(ctx *cli.Context) error {
					nsp, err := createMapFromArgs[dgclient.NamespacePayload](
						ctx.Args().Slice(),
					)
					if err != nil {
						return err
					}
					sec, err := client.ListSecret(nsp.Namespace)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(sec)
				},
			},
			{
				Name:  "get",
				Usage: "get a secret",
				Action: func(ctx *cli.Context) error {
					s, err := createMapFromArgs[spec.Secret](
						ctx.Args().Slice(), "name",
					)
					if err != nil {
						return err
					}
					sec, err := client.GetSecret(
						s.Name, s.NamespaceName,
					)
					if err != nil {
						return err
					}
					return jsonPrettyPrint(sec)
				},
			},
		},
	}
}
