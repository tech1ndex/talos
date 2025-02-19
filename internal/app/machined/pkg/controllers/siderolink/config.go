// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package siderolink

import (
	"context"
	"fmt"

	"github.com/cosi-project/runtime/pkg/controller"
	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/cosi-project/runtime/pkg/state"
	"github.com/siderolabs/go-pointer"
	"github.com/siderolabs/go-procfs/procfs"
	"go.uber.org/zap"

	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/machinery/resources/config"
	"github.com/siderolabs/talos/pkg/machinery/resources/siderolink"
)

// ConfigController interacts with SideroLink API and brings up the SideroLink Wireguard interface.
type ConfigController struct {
	Cmdline *procfs.Cmdline
}

// Name implements controller.Controller interface.
func (ctrl *ConfigController) Name() string {
	return "siderolink.ConfigController"
}

// Inputs implements controller.Controller interface.
func (ctrl *ConfigController) Inputs() []controller.Input {
	return []controller.Input{
		{
			Namespace: config.NamespaceName,
			Type:      config.MachineConfigType,
			ID:        pointer.To(config.V1Alpha1ID),
			Kind:      controller.InputWeak,
		},
	}
}

// Outputs implements controller.Controller interface.
func (ctrl *ConfigController) Outputs() []controller.Output {
	return []controller.Output{
		{
			Type: siderolink.ConfigType,
			Kind: controller.OutputExclusive,
		},
	}
}

// Run implements controller.Controller interface.
func (ctrl *ConfigController) Run(ctx context.Context, r controller.Runtime, _ *zap.Logger) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-r.EventCh():
		}

		cfg, err := safe.ReaderGetByID[*config.MachineConfig](ctx, r, config.V1Alpha1ID)
		if err != nil && !state.IsNotFoundError(err) {
			return err
		}

		if err := ctrl.updateConfig(ctx, r, cfg); err != nil {
			return fmt.Errorf("failed to update config: %w", err)
		}
	}
}

func (ctrl *ConfigController) updateConfig(ctx context.Context, r controller.Runtime, machineConfig *config.MachineConfig) error {
	cfg := siderolink.NewConfig(config.NamespaceName, siderolink.ConfigID)

	endpoint := ctrl.apiEndpoint(machineConfig)
	if endpoint == "" {
		err := r.Destroy(ctx, cfg.Metadata())
		if err != nil && !state.IsNotFoundError(err) {
			return err
		}

		return nil
	}

	return safe.WriterModify(ctx, r, cfg, func(c *siderolink.Config) error {
		c.TypedSpec().APIEndpoint = endpoint

		return nil
	})
}

func (ctrl *ConfigController) apiEndpoint(machineConfig *config.MachineConfig) string {
	if machineConfig != nil && machineConfig.Config().SideroLink() != nil && machineConfig.Config().SideroLink().APIUrl() != nil {
		return machineConfig.Config().SideroLink().APIUrl().String()
	}

	if ctrl.Cmdline == nil || ctrl.Cmdline.Get(constants.KernelParamSideroLink).First() == nil {
		return ""
	}

	return *ctrl.Cmdline.Get(constants.KernelParamSideroLink).First()
}
