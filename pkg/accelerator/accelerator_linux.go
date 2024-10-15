// Use and distribution licensed under the Apache license version 2.
//
// See the COPYING file in the root project directory for full text.
//

package accelerator

import (
	"context"
	"slices"

	"github.com/jaypipes/ghw/internal/log"
	"github.com/jaypipes/ghw/pkg/pci"
)

// PCI IDs list available at https://admin.pci-ids.ucw.cz/read/PD
const (
	pciClassProcessingAccelerator    = "12"
	pciSubclassProcessingAccelerator = "00"
	pciClassController               = "03"
	pciSubclass3DController          = "02"
	pciSubclassDisplayController     = "80"
)

var (
	acceleratorPCIClasses = map[string][]string{
		pciClassProcessingAccelerator: []string{
			pciSubclassProcessingAccelerator,
		},
		pciClassController: []string{
			pciSubclass3DController,
			pciSubclassDisplayController,
		},
	}
)

func (i *Info) load(ctx context.Context) error {
	accelDevices := make([]*AcceleratorDevice, 0)

	// get PCI devices
	pciInfo, err := pci.New(ctx)
	if err != nil {
		log.Warn(ctx, "error loading PCI information: %s", err)
		return nil
	}

	// Prepare hardware filter based in the PCI Class + Subclass
	isAccelerator := func(dev *pci.Device) bool {
		class := dev.Class.ID
		subclass := dev.Subclass.ID
		if subclasses, ok := acceleratorPCIClasses[class]; ok {
			if slices.Contains(subclasses, subclass) {
				return true
			}
		}
		return false
	}

	// This loop iterates over the list of PCI devices and filters them based
	// on discovery criteria: the builtin accelerator detection plus, if any
	// were provided, the user-supplied filters.
	filters := filtersFromContext(ctx)
	for _, device := range pciInfo.Devices {
		if !isAccelerator(device) {
			continue
		}
		if !matchFilters(filters, device) {
			continue
		}
		accelDev := &AcceleratorDevice{
			Address:   device.Address,
			PCIDevice: device,
		}
		accelDevices = append(accelDevices, accelDev)
	}

	i.Devices = accelDevices
	return nil
}
