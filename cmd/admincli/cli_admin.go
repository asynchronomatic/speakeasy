package main

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/asynchronomatic/speakeasy/api"
)

func adminCommand() *cli.Command {
	return &cli.Command{
		Name:   "admin",
		Usage:  "Admin operations",
		Before: requireToken,
		Commands: []*cli.Command{
			{
				Name:  "invite",
				Usage: "Create a mesh invite link",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "name", Usage: "name attached to invited nodes"},
					&cli.DurationFlag{Name: "lifetime", Usage: "invite lifetime (default 24h; ignored with --forever)", Value: 24 * time.Hour},
					&cli.BoolFlag{Name: "reusable", Usage: "expire after first redeem (default true)", Value: false},
				},
				Action: adminInvite,
			},
			{
				Name:      "delete-invite",
				Usage:     "Delete an invite by id",
				ArgsUsage: "INVITE_ID",
				Action:    adminDeleteInvite,
			},
			{
				Name:      "kick",
				Usage:     "Kick a peer from the mesh",
				ArgsUsage: "PEER_ID",
				Action:    adminKick,
			},
		},
	}
}

func adminInvite(_ context.Context, cmd *cli.Command) error {
	lifetime := cmd.Duration("lifetime")

	resp, err := newAPI(cmd).Admin().CreateInvite(api.CreateInviteRequest{
		MeshId:      cmd.String("mesh"),
		Name:        cmd.String("name"),
		Reusable:    cmd.Bool("reusable"),
		LifetimeSec: uint64(lifetime.Seconds()),
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out(), "invite id:   %s\n", resp.InviteId)
	fmt.Fprintf(out(), "invite link: %s\n", resp.InviteLink)
	if resp.Reusable {
		fmt.Fprintf(out(), "uses:        1 remaining\n")
	} else {
		fmt.Fprintf(out(), "uses:        unlimited\n")
	}
	if resp.Expires == 0 {
		fmt.Fprintf(out(), "expires:     never (until revoked)\n")
	} else {
		fmt.Fprintf(out(), "expires:     %s\n", time.Unix(resp.Expires, 0).UTC().Format(time.RFC3339))
	}
	return nil
}

func adminDeleteInvite(_ context.Context, cmd *cli.Command) error {
	id, err := requireArg(cmd, "invite id")
	if err != nil {
		return err
	}
	if err := newAPI(cmd).Admin().DeleteInvite(id); err != nil {
		return err
	}
	fmt.Fprintf(out(), "deleted invite %s\n", id)
	return nil
}

func adminKick(_ context.Context, cmd *cli.Command) error {
	id, err := requireArg(cmd, "peer id")
	if err != nil {
		return err
	}
	if err := newAPI(cmd).Admin().KickPeer(id); err != nil {
		return err
	}
	fmt.Fprintf(out(), "kicked %s\n", id)
	return nil
}

func listMeshes(_ context.Context, cmd *cli.Command) error {
	meshes := newAPI(cmd).MeshList()
	w := tabwriter.NewWriter(out(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tDESCRIPTION")
	for _, m := range meshes {
		fmt.Fprintf(w, "%s\t%s\n", m.ID, m.Description)
	}
	return w.Flush()
}

func redeemInvite(_ context.Context, cmd *cli.Command) error {
	link, err := requireArg(cmd, "invite url")
	if err != nil {
		return err
	}
	resp, err := api.RedeemInvite(link, api.Node{
		ID:   cmd.String("id"),
		Name: cmd.String("name"),
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out(), "mesh id:     %s\n", resp.MeshId)
	fmt.Fprintf(out(), "mesh secret: %s\n", resp.MeshSecret)
	fmt.Fprintf(out(), "server:      %s\n", resp.MeshServer)

	return nil
}
