package app

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newLockCommand(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "lock", Short: "manage tbd remote CAS locks"}
	cmd.AddCommand(lockAcquireCommand(opts), lockReleaseCommand(opts), lockStatusCommand(opts))
	return cmd
}

func lockAcquireCommand(opts *rootOptions) *cobra.Command {
	var ttl string
	var steal bool
	cmd := &cobra.Command{
		Use:   "acquire NAME",
		Short: "acquire a lock",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			if ttl == "" {
				ttl = e.Config.Locks.DefaultTTL
			}
			d, err := time.ParseDuration(ttl)
			if err != nil {
				return err
			}
			if err := e.AcquireLock(args[0], d, steal); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "locked %s for %s\n", args[0], d)
			return nil
		},
	}
	cmd.Flags().StringVar(&ttl, "ttl", "", "lock TTL, defaults to config")
	cmd.Flags().BoolVar(&steal, "steal", false, "steal an expired or currently held lock")
	return cmd
}

func lockReleaseCommand(opts *rootOptions) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "release NAME",
		Short: "release a lock",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			if err := e.ReleaseLock(args[0], force); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "released %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "release a lock held by another owner")
	return cmd
}

func lockStatusCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status NAME",
		Short: "show lock metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(cmd, opts)
			if err != nil {
				return err
			}
			info, ok := e.LockInfo(args[0])
			if !ok {
				fmt.Fprintf(cmd.OutOrStdout(), "%s unlocked\n", args[0])
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s held by %s <%s> until %s\n", args[0], info.Owner, info.Email, info.Expires)
			return nil
		},
	}
}
