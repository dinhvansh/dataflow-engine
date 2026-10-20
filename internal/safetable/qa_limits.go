package safetable

import "os"

const defaultTestRowLimit uint64 = 1_000_000
const ownerSevenMillionRowLimit uint64 = 7_000_000

// TestRowLimit keeps the normal TEST safety cap fail-closed. The larger fixed
// ceiling is available only for the explicit owner-operated disposable 7m
// scratch campaign; it is not configurable and cannot enable production use.
func TestRowLimit() uint64 {
	if os.Getenv("DATAFLOW_ALLOW_TEST_LOAD") == "true" &&
		os.Getenv("DATAFLOW_TEST_TARGET_DATABASE") == "scratch" &&
		os.Getenv("DATAFLOW_CORE_FULL_7M") == "YES_OWNED_DISPOSABLE_7M" &&
		os.Getenv("DATAFLOW_QA_LOCAL_OWNER_VERIFIED") == "true" {
		return ownerSevenMillionRowLimit
	}
	return defaultTestRowLimit
}
