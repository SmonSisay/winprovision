package executor

import (
	"fmt"

	"github.com/SmonSisay/winprovision/internal/utils"
)

// minDestinationHeadroom is the free space required on the destination volume
// on top of the payload itself, so the volume is not left completely full once
// the software copy has finished. 512 MiB.
const minDestinationHeadroom = 512 << 20

// checkDestinationSpace verifies that the destination volume can hold the
// software payload before provisioning starts. On success it returns nil.
//
// The check is advisory in two situations that must not block provisioning:
//   - the payload cannot be measured (missing or unreadable software directory);
//     the copy task reports that failure with better detail
//   - free space cannot be queried (network or virtual volume); the operator is
//     warned instead
//
// When the requirement is genuinely not met, the shortfall is reported on one
// line and a descriptive error is returned for the caller to surface. The check
// stays even though it is terse: it stops a run before a multi-gigabyte copy
// starts, rather than after it has failed part-way.
func checkDestinationSpace(volumeRoot, payloadRoot string) error {
	payload, err := utils.DirSize(payloadRoot)
	if err != nil {
		fmt.Printf("  WARNING: could not measure %s (%v)\n", payloadRoot, err)
		return nil
	}

	free, err := utils.FreeSpaceBytes(volumeRoot)
	if err != nil {
		fmt.Printf("  WARNING: could not determine free space on %s (%v)\n", volumeRoot, err)
		return nil
	}

	available := int64(free)
	return reportDestinationSpace(volumeRoot, payload, available)
}

// reportDestinationSpace compares the measured payload against the available
// space and fails when the requirement is not met. It is separated from
// checkDestinationSpace so the decision can be exercised deterministically,
// without depending on the free space of the host running the tests.
func reportDestinationSpace(volumeRoot string, payload, available int64) error {
	required := payload + minDestinationHeadroom
	if available >= required {
		return nil
	}

	shortBy := required - available

	fmt.Printf("  FATAL: %s cannot hold the software payload — need %s, have %s (short by %s).\n",
		volumeRoot, humanBytes(required), humanBytes(available), humanBytes(shortBy))

	return fmt.Errorf(
		"insufficient free space on %s: need %s, have %s (short by %s)",
		volumeRoot, humanBytes(required), humanBytes(available), humanBytes(shortBy),
	)
}

// humanBytes formats a byte count using binary (IEC) units, so the suffix
// always matches the divisor: 1048576 renders as "1.0 MiB", not "1.0 MB".
// The free-space comparison is a hard gate that can stop a machine from being
// provisioned, so the unit must not be ambiguous.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
