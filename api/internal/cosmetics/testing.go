package cosmetics

// WithCatalogForTest swaps the package catalog (and its indexes) for tests in
// other packages; call the returned func to restore the originals. Not safe
// for parallel tests.
func WithCatalogForTest(entries []CatalogEntry) (restore func()) {
	oc, oi, os := catalog, byID, bySKU
	catalog = entries
	byID, bySKU = buildIndexes(entries)
	return func() { catalog, byID, bySKU = oc, oi, os }
}
