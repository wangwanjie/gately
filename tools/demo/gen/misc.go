package gen

import (
	"encoding/json"
	"strconv"
)

func itoa(n int) string { return strconv.Itoa(n) }

func jsonLine(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
