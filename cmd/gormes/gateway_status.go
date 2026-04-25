package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/gateway"
)

func init() {
	gatewayCmd.AddCommand(gatewayStatusCmd)
}

var gatewayStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Inspect configured gateway channels and persisted runtime state",
	RunE:  runGatewayStatus,
}

func runGatewayStatus(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(nil)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	pairingStatus, err := gateway.NewXDGPairingStore().ReadPairingStatus(ctx)
	if err != nil {
		return fmt.Errorf("pairing status: %w", err)
	}

	runtimeSnapshot, err := gateway.NewRuntimeStatusStore(config.GatewayRuntimeStatusPath()).ReadRuntimeStatusSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("runtime status: %w", err)
	}
	runtimeStatus := runtimeSnapshot.Status
	if runtimeSnapshot.Missing {
		runtimeStatus = gateway.RuntimeStatus{}
	}

	_, err = fmt.Fprint(cmd.OutOrStdout(), gateway.RenderStatusSummary(gateway.StatusSummary{
		Channels: configuredGatewayStatusChannels(cfg),
		Pairing:  pairingStatus,
		Runtime:  runtimeStatus,
	}))
	return err
}

func configuredGatewayStatusChannels(cfg config.Config) []gateway.StatusChannel {
	channels := []gateway.StatusChannel{}
	if cfg.Telegram.BotToken != "" {
		detail := "first_run_discovery=" + strconv.FormatBool(cfg.Telegram.FirstRunDiscovery)
		if cfg.Telegram.AllowedChatID != 0 {
			detail = "allowed_chat_id=" + strconv.FormatInt(cfg.Telegram.AllowedChatID, 10)
		}
		channels = append(channels, gateway.StatusChannel{
			Name:   "telegram",
			Detail: detail,
		})
	}
	if cfg.Discord.Enabled() {
		detail := "first_run_discovery=" + strconv.FormatBool(cfg.Discord.FirstRunDiscovery)
		if cfg.Discord.AllowedChannelID != "" {
			detail = "allowed_channel_id=" + cfg.Discord.AllowedChannelID
		}
		channels = append(channels, gateway.StatusChannel{
			Name:   "discord",
			Detail: detail,
		})
	}
	return channels
}
