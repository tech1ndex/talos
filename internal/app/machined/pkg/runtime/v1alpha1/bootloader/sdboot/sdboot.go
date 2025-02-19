// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package sdboot provides the interface to the Systemd-Boot bootloader: config management, installation, etc.
package sdboot

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ecks/uefi/efi/efivario"
	"github.com/siderolabs/go-blockdevice/blockdevice"

	"github.com/siderolabs/talos/internal/app/machined/pkg/runtime/v1alpha1/bootloader/assets"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/version"
)

// Config describe sd-boot state.
type Config struct {
	Default  string
	Fallback string
}

func isUEFIBoot() bool {
	// https://renenyffenegger.ch/notes/Linux/fhs/sys/firmware/efi/index
	_, err := os.Stat("/sys/firmware/efi")

	return err == nil
}

// IsBootedUsingSDBoot returns true if the system is booted using sd-boot.
func IsBootedUsingSDBoot() bool {
	// https://www.freedesktop.org/software/systemd/man/systemd-stub.html#EFI%20Variables
	// https://www.freedesktop.org/software/systemd/man/systemd-stub.html#StubInfo
	_, err := os.Stat(fmt.Sprintf("/sys/firmware/efi/efivars/StubInfo-%s", SystemdBootGUIDString))

	return err == nil
}

// New creates a new sdboot bootloader config.
func New() *Config {
	return &Config{}
}

// Probe for existing sd-boot bootloader.
func Probe(disk string) (*Config, error) {
	// if not UEFI boot, nothing to do
	if !isUEFIBoot() {
		return nil, nil
	}

	if !IsBootedUsingSDBoot() {
		return nil, nil
	}

	// TODO: read /boot/EFI and find if sd-boot is already being used

	// here we need to read the EFI vars to see if we have any defaults
	// and populate config accordingly
	// https://www.freedesktop.org/software/systemd/man/systemd-boot.html#LoaderEntryDefault
	// this should be set on install/upgrades

	efiCtx := efivario.NewDefaultContext()

	bootedEntry, err := ReadVariable(efiCtx, LoaderEntrySelectedName)
	if err != nil {
		return nil, err
	}

	log.Printf("booted entry: %q", bootedEntry)

	// TODO: verify that bootedEntry is in the EFI partition

	return &Config{
		Default: bootedEntry,
	}, nil
}

// UEFIBoot returns true if bootloader is UEFI-only.
func (c *Config) UEFIBoot() bool {
	return true
}

// Install the bootloader.
//
// Assumes that EFI partition is already mounted.
// Writes down the UKI and updates the EFI variables.
//
//nolint:gocyclo
func (c *Config) Install(bootDisk, arch, cmdline string) error {
	var sdbootFilename string

	switch arch {
	case "amd64":
		sdbootFilename = "BOOTX64.efi"
	case "arm64":
		sdbootFilename = "BOOTAA64.efi"
	default:
		return fmt.Errorf("unsupported architecture: %s", arch)
	}

	// list existing UKIs, and clean up all but the current one (used to boot)
	files, err := filepath.Glob(filepath.Join(constants.EFIMountPoint, "EFI", "Linux", "Talos-*.efi"))
	if err != nil {
		return err
	}

	// writing UKI by version-based filename here
	ukiPath := fmt.Sprintf("%s-%s.efi", "Talos", version.Tag)

	for _, file := range files {
		if strings.EqualFold(filepath.Base(file), c.Default) {
			if !strings.EqualFold(c.Default, ukiPath) {
				// set fallback to the current default unless it matches the new install
				c.Fallback = c.Default
			}

			continue
		}

		log.Printf("removing old UKI: %s", file)

		if err = os.Remove(file); err != nil {
			return err
		}
	}

	assets := assets.Assets{
		{
			Source:      fmt.Sprintf(constants.UKIAssetPath, arch),
			Destination: filepath.Join(constants.EFIMountPoint, "EFI", "Linux", ukiPath),
		},
		{
			Source:      fmt.Sprintf(constants.SDBootAssetPath, arch),
			Destination: filepath.Join(constants.EFIMountPoint, "EFI", "boot", sdbootFilename),
		},
	}
	if err = assets.Install(); err != nil {
		return err
	}

	blk, err := getBlockDeviceName(bootDisk)
	if err != nil {
		return err
	}

	loopDevice := strings.HasPrefix(blk, "/dev/loop")

	// don't update EFI variables if we're installing to a loop device
	if !loopDevice {
		efiCtx := efivario.NewDefaultContext()

		// set the new entry as a default one
		if err := WriteVariable(efiCtx, LoaderEntryDefaultName, ukiPath); err != nil {
			return err
		}

		// set default 5 second boot timeout
		if err := WriteVariable(efiCtx, LoaderConfigTimeoutName, "5"); err != nil {
			return err
		}
	}

	return nil
}

// PreviousLabel returns the label of the previous bootloader version.
func (c *Config) PreviousLabel() string {
	return c.Fallback
}

// Revert the bootloader to the previous version.
func (c *Config) Revert() error {
	// TODO: use c.Default as the current entry, list other UKIs, find the one which is not c.Default, and update EFI var
	panic("not implemented")
}

func getBlockDeviceName(bootDisk string) (string, error) {
	dev, err := blockdevice.Open(bootDisk, blockdevice.WithMode(blockdevice.ReadonlyMode))
	if err != nil {
		return "", err
	}

	//nolint:errcheck
	defer dev.Close()

	// verify that BootDisk has EFI partition
	_, err = dev.GetPartition(constants.EFIPartitionLabel)
	if err != nil {
		return "", err
	}

	blk := dev.Device().Name()

	return blk, nil
}
