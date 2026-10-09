package helps

import "github.com/tidwall/gjson"

func observedCacheCounter(node gjson.Result) *int64 {
	if !node.Exists() || node.Type != gjson.Number {
		return nil
	}
	value := node.Int()
	return &value
}
