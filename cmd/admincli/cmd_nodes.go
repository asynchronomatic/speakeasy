package main

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v3"
)

func nodeCommand() *cli.Command {
	return &cli.Command{
		Name:   "node",
		Usage:  "Mesh node operations",
		Before: requireToken,
		Commands: []*cli.Command{
			{
				Name:    "list",
				Aliases: []string{"ls", "peers"},
				Usage:   "List registered mesh nodes",
				Action:  nodeList,
			},
			{
				Name:  "register",
				Usage: "Register a named node",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "name", Usage: "node name", Required: true},
					&cli.StringFlag{Name: "id", Usage: "node peer id", Required: true},
				},
				Action: nodeRegister,
			},
			{
				Name:      "unregister",
				Aliases:   []string{"rm"},
				Usage:     "Unregister a node",
				ArgsUsage: "PEER_ID",
				Action:    nodeUnregister,
			},
			{
				Name:    "relay",
				Aliases: []string{"address"},
				Usage:   "Show mesh relay multiaddrs",
				Action:  nodeRelay,
			},
		},
	}
}

func nodeList(_ context.Context, cmd *cli.Command) error {
	mc, err := meshClient(cmd)
	if err != nil {
		return err
	}
	nodes, err := mc.GetPeers()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(out(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tID\tUPDATED")
	for _, n := range nodes {
		updated := ""
		if !n.LastUpdate.IsZero() {
			updated = n.LastUpdate.Format(time.RFC3339)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", n.Name, n.ID, updated)
	}
	return w.Flush()
}

func nodeRegister(_ context.Context, cmd *cli.Command) error {
	mc, err := meshClient(cmd)
	if err != nil {
		return err
	}
	if _, err := mc.Register(cmd.String("name"), cmd.String("id")); err != nil {
		return err
	}
	fmt.Fprintf(out(), "registered %s (%s)\n", cmd.String("name"), cmd.String("id"))
	return nil
}

func nodeUnregister(_ context.Context, cmd *cli.Command) error {
	id, err := requireArg(cmd, "peer id")
	if err != nil {
		return err
	}
	mc, err := meshClient(cmd)
	if err != nil {
		return err
	}
	if err := mc.Unregister(id); err != nil {
		return err
	}
	fmt.Fprintf(out(), "unregistered %s\n", id)
	return nil
}

func nodeRelay(_ context.Context, cmd *cli.Command) error {
	mc, err := meshClient(cmd)
	if err != nil {
		return err
	}
	addrs, updated, logical, err := mc.GetRelay()
	if err != nil {
		return err
	}
	if !updated.IsZero() {
		fmt.Fprintf(out(), "updated: %s\n", updated.Format(time.RFC3339))
	}
	fmt.Fprintf(out(), "logical: %d\n", logical)
	for _, a := range addrs {
		fmt.Fprintln(out(), a)
	}
	return nil
}
