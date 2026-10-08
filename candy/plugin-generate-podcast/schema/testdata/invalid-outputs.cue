package podcast

// a writing-stage output with NO receipts: MUST be REJECTED. §5.4 refuses a source-free run, and the
// refusal is a schema failure (`_min: len(sources) >= 1`) rather than a special case in Go. Every other
// block is complete, so the ONLY reason this file can fail is the empty `sources` list.
invalidOutputs: #TypedOutputs & {
	script: "# the episode markdown"
	sources: []
	coverage: {selected: 0, skipped: 0, repos_scanned: 0, repos_used: 0}
	gaps: []
}
