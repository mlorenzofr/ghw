//
// Use and distribution licensed under the Apache license version 2.
//
// See the COPYING file in the root project directory for full text.
//

package accelerator

import (
	"context"

	"github.com/jaypipes/ghw/internal/config"
	"github.com/jaypipes/ghw/pkg/pci"
	"github.com/jaypipes/ghw/pkg/topology"

	"github.com/jaypipes/pcidb"
)

// Filter is a user-provided function used to narrow down the processing
// accelerator devices ghw discovers. Returning true accepts the device
// (the builtin accelerator detection, based on the PCI class, always applies
// as well: a device is only reported if it is an accelerator device *and* at
// least one of the user-provided filters, if any, accepts it).
//
// Filters are handed an isolated copy of the candidate pci.Device: any
// change they make to it, including to the vendor/product/class metadata or
// to the parent/children device links, is discarded and can never affect the
// devices ghw discovered or the data shared with the PCI database. To keep
// the copies cheap and fully isolated, the pcidb relation collections
// (Vendor.Products, Product.Subsystems, Class.Subclasses and
// Subclass.ProgrammingInterfaces) are stripped from the copy; filters should
// match on the device's own attributes (IDs and names) instead.
type Filter func(dev *pci.Device) bool

// filtersKey is the context key under which user-provided accelerator
// discovery filters are stored.
const filtersKey = config.Key("ghw.accelerator.filters")

// WithFilters allows narrowing down the processing accelerator devices ghw
// discovers. Each filter is a function receiving the candidate pci.Device;
// returning true accepts the device (see the Filter type for the exact
// semantics).
//
// The filters are copied into the returned modifier, so the caller may keep
// using or modifying the supplied slice afterwards. Applying the modifier
// more than once to the same context replaces the previously set filters.
// A list made up only of nil filters behaves like no filters at all.
func WithFilters(filters ...Filter) config.Modifier {
	filters = copyFilters(filters)
	return func(ctx context.Context) context.Context {
		return context.WithValue(ctx, filtersKey, filters)
	}
}

// filtersFromContext returns any accelerator discovery filters set in the
// supplied context.
func filtersFromContext(ctx context.Context) []Filter {
	if ctx == nil {
		return nil
	}
	if v := ctx.Value(filtersKey); v != nil {
		return v.([]Filter)
	}
	return nil
}

func copyFilters(filters []Filter) []Filter {
	if len(filters) == 0 {
		return nil
	}
	copied := make([]Filter, 0, len(filters))
	for _, filter := range filters {
		if filter != nil {
			copied = append(copied, filter)
		}
	}
	return copied
}

// matchFilters reports whether the given device is accepted by at least one
// of the supplied filters. When no (non-nil) filter is supplied, the device is
// accepted.
//
// Each filter is handed an isolated copy of the device (see the Filter type
// documentation), so user-provided filters can never mutate the pci.Device
// structs ghw discovered.
func matchFilters(filters []Filter, dev *pci.Device) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		if filter == nil {
			continue
		}
		if filter(copyDeviceForFilter(dev)) {
			return true
		}
	}
	return false
}

// copyDeviceForFilter returns an isolated copy of dev, safe to hand to
// user-provided filters.
//
// The copy strategy is:
//   - plain fields are copied by value;
//   - the pcidb metadata (vendor, product, subsystem, class, subclass,
//     programming interface) is copied by value, with the relation
//     collections that point into the shared PCI database stripped;
//   - the topology node is copied by value, with the core/cache collections
//     stripped;
//   - the parent (resp. children) links are followed and copied, but the
//     copied parent never carries the original children (resp. the copied
//     children never carry a parent pointer), so no copied device ever links
//     back to a device ghw discovered.
func copyDeviceForFilter(dev *pci.Device) *pci.Device {
	if dev == nil {
		return nil
	}
	return copyDevice(dev, true, true)
}

// copyDevice copies dev, optionally following the parent and children links,
// severing the links that would lead back to original (shared) devices.
func copyDevice(dev *pci.Device, withParent bool, withChildren bool) *pci.Device {
	if dev == nil {
		return nil
	}
	copied := *dev
	copied.Vendor = copyVendor(dev.Vendor)
	copied.Product = copyProduct(dev.Product)
	copied.Subsystem = copyProduct(dev.Subsystem)
	copied.Class = copyClass(dev.Class)
	copied.Subclass = copySubclass(dev.Subclass)
	copied.ProgrammingInterface = copyProgrammingInterface(dev.ProgrammingInterface)
	copied.Node = copyNode(dev.Node)
	copied.Parent = nil
	if withParent {
		// follow the parent chain, but never copy the parent's children:
		// they point back to the original devices
		copied.Parent = copyDevice(dev.Parent, true, false)
	}
	copied.Children = nil
	if withChildren {
		copied.Children = make([]*pci.Device, 0, len(dev.Children))
		for _, child := range dev.Children {
			// follow the children, but never give them a parent pointer
			// back to the copied or original devices
			copied.Children = append(copied.Children, copyDevice(child, false, true))
		}
	}
	return &copied
}

func copyVendor(v *pcidb.Vendor) *pcidb.Vendor {
	if v == nil {
		return nil
	}
	copied := *v
	copied.Products = nil
	return &copied
}

func copyProduct(p *pcidb.Product) *pcidb.Product {
	if p == nil {
		return nil
	}
	copied := *p
	copied.Subsystems = nil
	return &copied
}

func copyClass(c *pcidb.Class) *pcidb.Class {
	if c == nil {
		return nil
	}
	copied := *c
	copied.Subclasses = nil
	return &copied
}

func copySubclass(s *pcidb.Subclass) *pcidb.Subclass {
	if s == nil {
		return nil
	}
	copied := *s
	copied.ProgrammingInterfaces = nil
	return &copied
}

func copyProgrammingInterface(pi *pcidb.ProgrammingInterface) *pcidb.ProgrammingInterface {
	if pi == nil {
		return nil
	}
	copied := *pi
	return &copied
}

func copyNode(n *topology.Node) *topology.Node {
	if n == nil {
		return nil
	}
	copied := *n
	copied.Cores = nil
	copied.Caches = nil
	if n.Distances != nil {
		copied.Distances = make([]int, len(n.Distances))
		copy(copied.Distances, n.Distances)
	}
	if n.Memory != nil {
		memory := *n.Memory
		memory.SupportedPageSizes = nil
		memory.HugePageAmountsBySize = nil
		copied.Memory = &memory
	}
	return &copied
}
