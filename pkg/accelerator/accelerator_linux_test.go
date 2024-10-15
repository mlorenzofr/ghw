//
// Use and distribution licensed under the Apache license version 2.
//
// See the COPYING file in the root project directory for full text.

package accelerator_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jaypipes/ghw/pkg/accelerator"
	"github.com/jaypipes/ghw/pkg/option"
	"github.com/jaypipes/ghw/pkg/pci"
	"github.com/jaypipes/ghw/pkg/snapshot"

	"github.com/jaypipes/ghw/testdata"
)

func testScenario(t *testing.T, filename string, filters []accelerator.Filter, expectedDevs int) {
	testdataPath, err := testdata.SnapshotsDirectory()
	if err != nil {
		t.Fatalf("Expected nil err, but got %v", err)
	}

	t.Setenv("PCIDB_PATH", testdata.PCIDBChroot())

	workstationSnapshot := filepath.Join(testdataPath, filename)

	tmpRoot := t.TempDir()
	err = snapshot.UnpackInto(workstationSnapshot, tmpRoot)
	if err != nil {
		t.Fatalf("Unable to unpack %q into %q: %v", workstationSnapshot, tmpRoot, err)
	}

	info, err := accelerator.New(
		option.WithChroot(tmpRoot),
		accelerator.WithFilters(filters...),
	)
	if err != nil {
		t.Fatalf("Expected nil err, but got %v", err)
	}
	if info == nil {
		t.Fatalf("Expected non-nil AcceleratorInfo, but got nil")
	}
	if len(info.Devices) != expectedDevs {
		t.Fatalf("Expected %d processing accelerator devices, but found %d.", expectedDevs, len(info.Devices))
	}
}

func TestAcceleratorDefault(t *testing.T) {
	if _, ok := os.LookupEnv("GHW_TESTING_SKIP_ACCELERATOR"); ok {
		t.Skip("Skipping PCI tests.")
	}

	// In this scenario we have 1 processing accelerator device
	testScenario(t, "linux-amd64-accel.tar.gz", nil, 1)

}

func TestAcceleratorNvidia(t *testing.T) {
	if _, ok := os.LookupEnv("GHW_TESTING_SKIP_ACCELERATOR"); ok {
		t.Skip("Skipping PCI tests.")
	}

	// In this scenario we have 1 Nvidia 3D controller device
	testScenario(t, "linux-amd64-accel-nvidia.tar.gz", nil, 1)
}

func TestAcceleratorFilter(t *testing.T) {
	if _, ok := os.LookupEnv("GHW_TESTING_SKIP_ACCELERATOR"); ok {
		t.Skip("Skipping PCI tests.")
	}

	// Set the filter to detect only processing accelerators (PCI class
	// 1200), so the Nvidia 3D controller (0302) is not included
	onlyProcessingAccelerators := func(dev *pci.Device) bool {
		return dev.Class.ID+dev.Subclass.ID == "1200"
	}

	// In this scenario we have 1 Nvidia 3D controller device
	testScenario(t, "linux-amd64-accel-nvidia.tar.gz", []accelerator.Filter{onlyProcessingAccelerators}, 0)
}

// TestAcceleratorFiltersComposition verifies that a device is reported when
// at least one of the user-provided filters accepts it (OR semantics), even
// if other filters reject it.
func TestAcceleratorFiltersComposition(t *testing.T) {
	if _, ok := os.LookupEnv("GHW_TESTING_SKIP_ACCELERATOR"); ok {
		t.Skip("Skipping PCI tests.")
	}

	rejectEverything := func(dev *pci.Device) bool {
		return false
	}
	acceptEverything := func(dev *pci.Device) bool {
		return true
	}

	// In this scenario we have 1 processing accelerator device: the second
	// filter accepts it even though the first one rejects it
	testScenario(t, "linux-amd64-accel.tar.gz", []accelerator.Filter{rejectEverything, acceptEverything}, 1)
}

// TestAcceleratorFilterNil verifies that nil or all-nil filter lists behave
// like no filters at all.
func TestAcceleratorFilterNil(t *testing.T) {
	if _, ok := os.LookupEnv("GHW_TESTING_SKIP_ACCELERATOR"); ok {
		t.Skip("Skipping PCI tests.")
	}

	// In this scenario we have 1 processing accelerator device
	testScenario(t, "linux-amd64-accel.tar.gz", []accelerator.Filter{nil}, 1)
}

// TestAcceleratorFilterBuiltinGateStillApplies verifies that an always-true
// user filter does not bypass the builtin accelerator detection: devices
// which are not accelerator hardware are never reported.
func TestAcceleratorFilterBuiltinGateStillApplies(t *testing.T) {
	if _, ok := os.LookupEnv("GHW_TESTING_SKIP_ACCELERATOR"); ok {
		t.Skip("Skipping PCI tests.")
	}

	acceptEverything := func(dev *pci.Device) bool {
		return true
	}

	// The snapshot contains many PCI devices (bridges, SATA controllers...)
	// but only 1 processing accelerator device
	testScenario(t, "linux-amd64-accel.tar.gz", []accelerator.Filter{acceptEverything}, 1)
}

// TestAcceleratorFilterMutationIsolation verifies that filters are handed an
// isolated copy of the candidate device: mutations performed by a filter,
// including on the vendor/product/class metadata and on the parent/children
// links, are discarded and are not visible to the other filters, to the
// discovery process, nor in the returned devices.
func TestAcceleratorFilterMutationIsolation(t *testing.T) {
	if _, ok := os.LookupEnv("GHW_TESTING_SKIP_ACCELERATOR"); ok {
		t.Skip("Skipping PCI tests.")
	}

	const (
		origVendorName = "Red Hat, Inc."
		origClassName  = "Processing accelerators"
	)

	var sawMutatedVendor, sawMutatedClass, sawMutatedParent bool

	mutateEverything := func(dev *pci.Device) bool {
		if dev.Vendor != nil {
			dev.Vendor.Name = "mutated vendor"
			dev.Vendor.ID = "ffff"
		}
		if dev.Class != nil {
			dev.Class.Name = "mutated class"
			dev.Class.ID = "ff"
		}
		if dev.Subclass != nil {
			dev.Subclass.ID = "ff"
		}
		dev.Address = "0000:ff:ff.f"
		if dev.Parent != nil {
			sawMutatedParent = true
			dev.Parent.Address = "0000:ff:ff.f"
			if dev.Parent.Children != nil {
				dev.Parent.Children[0] = nil
			}
		}
		if dev.Children != nil {
			dev.Children = append(dev.Children, &pci.Device{})
		}
		return false
	}

	verifyPristine := func(dev *pci.Device) bool {
		if dev.Vendor != nil {
			if dev.Vendor.Name != origVendorName {
				sawMutatedVendor = true
			}
		}
		if dev.Class != nil && dev.Class.Name != origClassName {
			sawMutatedClass = true
		}
		return true
	}

	testdataPath, err := testdata.SnapshotsDirectory()
	if err != nil {
		t.Fatalf("Expected nil err, but got %v", err)
	}

	t.Setenv("PCIDB_PATH", testdata.PCIDBChroot())

	workstationSnapshot := filepath.Join(testdataPath, "linux-amd64-accel.tar.gz")

	tmpRoot := t.TempDir()
	err = snapshot.UnpackInto(workstationSnapshot, tmpRoot)
	if err != nil {
		t.Fatalf("Unable to unpack %q into %q: %v", workstationSnapshot, tmpRoot, err)
	}

	info, err := accelerator.New(
		option.WithChroot(tmpRoot),
		accelerator.WithFilters(mutateEverything, verifyPristine),
	)
	if err != nil {
		t.Fatalf("Expected nil err, but got %v", err)
	}
	if info == nil {
		t.Fatalf("Expected non-nil AcceleratorInfo, but got nil")
	}
	if len(info.Devices) != 1 {
		t.Fatalf("Expected 1 processing accelerator device, but found %d.", len(info.Devices))
	}

	// the second filter must have seen the pristine metadata
	if sawMutatedVendor {
		t.Fatalf("Expected the vendor metadata to be pristine for the second filter, but it was mutated")
	}
	if sawMutatedClass {
		t.Fatalf("Expected the class metadata to be pristine for the second filter, but it was mutated")
	}

	// the returned device must be pristine as well, including nested metadata
	dev := info.Devices[0].PCIDevice
	if dev == nil {
		t.Fatalf("Expected non-nil PCIDevice for the returned device")
	}
	if dev.Address == "0000:ff:ff.f" {
		t.Fatalf("Expected the returned device address to be pristine, but it was mutated")
	}
	if dev.Vendor == nil {
		t.Fatalf("Expected non-nil vendor for the returned device")
	}
	if dev.Vendor.Name != origVendorName || dev.Vendor.ID == "ffff" {
		t.Fatalf("Expected the returned device vendor metadata to be pristine, but got %q (%s)", dev.Vendor.Name, dev.Vendor.ID)
	}
	if dev.Class == nil {
		t.Fatalf("Expected non-nil class for the returned device")
	}
	if dev.Class.Name != origClassName || dev.Class.ID == "ff" {
		t.Fatalf("Expected the returned device class metadata to be pristine, but got %q (%s)", dev.Class.Name, dev.Class.ID)
	}
	if dev.Parent != nil && dev.Parent.Address == "0000:ff:ff.f" {
		t.Fatalf("Expected the returned device parent to be pristine, but it was mutated")
	}
	if sawMutatedParent {
		t.Logf("Note: the accelerator device had a parent device, which was passed to the filter")
	}
}
