package helps

import (
	"github.com/tidwall/gjson"
	"strconv"
)

func observedCacheCounter(node gjson.Result) *int64 {
	if !node.Exists() || node.Type != gjson.Number {
		return nil
	}
	// ParseInt rejects fractional/exponent notation and overflow instead of
	// gjson.Int's truncation/saturation. Absent observation stays partial.
	value, err := strconv.ParseInt(node.Raw, 10, 64)
	if err != nil {
		return nil
	}
	return &value
}
