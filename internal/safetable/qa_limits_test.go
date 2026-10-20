package safetable

import "testing"

func TestTestRowLimitRequiresEveryOwnerQAProof(t *testing.T) {
	keys := []string{"DATAFLOW_ALLOW_TEST_LOAD", "DATAFLOW_TEST_TARGET_DATABASE",
		"DATAFLOW_CORE_FULL_7M", "DATAFLOW_QA_LOCAL_OWNER_VERIFIED"}
	values := []string{"true", "scratch", "YES_OWNED_DISPOSABLE_7M", "true"}
	for _, key := range keys { t.Setenv(key, "") }
	if got := TestRowLimit(); got != defaultTestRowLimit { t.Fatalf("default=%d", got) }
	for i, key := range keys {
		for j, setKey := range keys {
			value := values[j]
			if i == j { value = "wrong" }
			t.Setenv(setKey, value)
		}
		if got := TestRowLimit(); got != defaultTestRowLimit {
			t.Fatalf("missing exact %s proof raised cap to %d", key, got)
		}
	}
	for i, key := range keys { t.Setenv(key, values[i]) }
	if got := TestRowLimit(); got != ownerSevenMillionRowLimit { t.Fatalf("owner limit=%d", got) }
}
