package podcast

// A source-free bundle: MUST be REJECTED. §5.4 refuses the source-free run, and the refusal has to be a
// SCHEMA failure rather than a special case in code -- which is what this arm proves. The previous
// `_min: len(entries) >= 1` field did NOT reject it (MEASURED: the comparison yields the boolean value
// false, which CUE accepts), so the refusal had no gate at all.
invalidEmptyBundle: #SourceBundle & {
	cutoff:             "2026.274"
	generated_at:       "2026-10-08T00:00:00Z"
	repos_scanned:      3
	repos_with_entries: 0
	entries:            []
	tag_gaps:           []
}
