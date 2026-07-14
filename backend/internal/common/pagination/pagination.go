// Package pagination normalises page/pageSize query params shared by every
// module that lists owner-scoped rows (wallet accounts, transfers).
package pagination

const (
	defaultPageSize = 20
	maxPageSize     = 100
	// maxOffset bounds the computed offset to the int32 range sqlc/Postgres
	// accept, so an absurdly large page cannot overflow into a negative offset
	// (which Postgres rejects, surfacing as a 500).
	maxOffset = 1<<31 - 1
)

// Clamp normalises page/pageSize and returns the effective page (echoed back
// to the client so the response matches the data served) plus the
// limit/offset for sqlc.
func Clamp(page, pageSize int) (effectivePage int, limit, offset int32) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	off := int64(page-1) * int64(pageSize)
	if off > maxOffset {
		off = maxOffset
	}
	return page, int32(pageSize), int32(off)
}
