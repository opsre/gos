package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	metrics "gos/internal/domain/observability"
)

const maxBusinessResponseBytes = 1 << 20

func (e *Exporter) collectBusinessSource(ctx context.Context, source metrics.BusinessSource) ([]float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return nil, errors.New("invalid business source request")
	}
	req.Header.Set("Accept", "application/json")
	if source.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+source.BearerToken)
	}
	response, err := e.client.Do(req)
	if err != nil {
		return nil, errors.New("business source request failed or timed out")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("business source returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBusinessResponseBytes+1))
	if err != nil || len(body) > maxBusinessResponseBytes {
		return nil, errors.New("business source body is unreadable or exceeds 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, errors.New("business source returned invalid JSON")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("business source must return one JSON document")
	}
	values := make([]float64, len(source.Metrics))
	for i, m := range source.Metrics {
		value, err := jsonPointerNumber(document, m.Path)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || (m.Type == "counter" && value < 0) {
			return nil, fmt.Errorf("metric %q requires a finite numeric value (nonnegative for counters)", m.Name)
		}
		values[i] = value
	}
	return values, nil
}

func jsonPointerNumber(document any, pointer string) (float64, error) {
	current := document
	if pointer != "" {
		for _, part := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
			// RFC 6901 escape order matters: ~01 denotes a literal ~1.
			part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
			switch v := current.(type) {
			case map[string]any:
				var exists bool
				current, exists = v[part]
				if !exists {
					return 0, errors.New("JSON pointer does not exist")
				}
			case []any:
				index, err := strconv.Atoi(part)
				if err != nil || index < 0 || index >= len(v) {
					return 0, errors.New("invalid JSON array index")
				}
				current = v[index]
			default:
				return 0, errors.New("JSON pointer does not address a number")
			}
		}
	}
	number, ok := current.(json.Number)
	if !ok {
		return 0, errors.New("JSON value is not numeric")
	}
	return number.Float64()
}
