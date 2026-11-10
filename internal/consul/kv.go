package consul

import (
	"context"
	"fmt"
	"strings"
)

// Permite testear la precedencia de configuración.
type KVReader interface {
	KVList(context.Context, string) ([]KVEntry, error)
}

func FetchKVConfig(ctx context.Context, reader KVReader, prefix string) (map[string]string, error) {
	entries, err := reader.KVList(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("leer configuración desde Consul KV: %w", err)
	}

	base := strings.TrimSuffix(prefix, "/") + "/"
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		key := strings.TrimPrefix(entry.Key, base)
		if key == entry.Key || key == "" || strings.Contains(key, "/") {
			continue
		}
		values[key] = entry.Value
	}
	return values, nil
}
