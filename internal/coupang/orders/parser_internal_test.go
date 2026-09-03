package orders

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestIntegerValueHonorsNativeIntRange(t *testing.T) {
	raw := map[string]any{"quantity": json.Number("9223372036854775807")}
	value, ok := integerValue(raw, "quantity")
	if strconv.IntSize == 32 {
		if ok {
			t.Fatalf("32-bit int accepted out-of-range value %d", value)
		}
		return
	}
	if !ok || int64(value) != int64(1<<63-1) {
		t.Fatalf("64-bit int result = %d, %t", value, ok)
	}
}
