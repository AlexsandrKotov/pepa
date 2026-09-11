package repository

// Pagination is normalised in one place: every LIST query in this package takes
// page/perPage straight from a client-controlled query string, so an unbounded
// perPage is a trivial way to force a full-table scan. Call sites keep their
// historical default page size, but the ceiling is shared.
const (
	// maxPerPage is the hard ceiling for a single LIST request.
	maxPerPage = 200
)

// ClampPagination normalises a page/perPage pair. page < 1 becomes 1, a
// non-positive or oversized perPage becomes defaultPerPage and is then capped
// at maxPerPage, so the returned values are always safe to use in
// LIMIT/OFFSET.
func ClampPagination(page, perPage, defaultPerPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}
	return page, perPage
}
