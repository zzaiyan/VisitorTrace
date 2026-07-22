package maprender

import "testing"

func TestOptionsCacheKeyIsCanonicalAndCollisionResistant(t *testing.T) {
	left := DefaultOptions()
	left.Title = "a,b"
	left.PVLabel = "c"
	left.Show = map[string]bool{"uv": true, "title": true, "pv": true}

	equivalent := left
	equivalent.Show = map[string]bool{"pv": true, "uv": true, "title": true}
	if left.CacheKey() != equivalent.CacheKey() {
		t.Fatal("equivalent Options produced different cache keys")
	}

	right := left
	right.Title = "a"
	right.PVLabel = "b,c"
	if left.CacheKey() == right.CacheKey() {
		t.Fatal("distinct labels produced a cache-key collision")
	}
}
