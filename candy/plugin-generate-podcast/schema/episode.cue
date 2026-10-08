package podcast

#Beat:      string & !="" // the quoted passage, verbatim in the named source
#Unlock:    string & !="" // must name a mechanism AND an actor (C3)
#HonestLimit: string & !="" // what this segment does NOT establish (C5)

#Segment: {
	title!:        string & !=""
	mechanism!:    string & !="" // what the sources actually say
	receipt!:      #Beat
	receipt_source!: string     // a #Source.id — the citation
	unlock!:       #Unlock
	honest_limit!: #HonestLimit
	comic_beat!:   string & !="" // C1: where the laugh is, named
}

#Episode: {
	front_matter!: {
		episode!:  string
		topic!:    string
		profile!:  string
		cutoff!:   string
		cast!:     [NAME=string]: {sid!: int & >=0, voice!: string}
		sources!:  int & >0
		coverage!: #Coverage
		gaps!:     [...string] // may be empty ONLY if the bundle is fully covered
	}
	cold_open!: string & !=""
	// The floor is THREE segments, in the prefix-list form (see source.cue's MEASURED note: a hidden
	// `_min: len(segments) >= 3` is inert, and `[...#Segment] & [_, _, _]` destroys the Go element type).
	// The CEILING of five cannot be expressed without an inline element type, so it is enforced in Go
	// (Emit) -- a type-inexpressible remainder, guarded where the episode is actually consumed.
	segments!:  [#Segment, #Segment, #Segment, ...#Segment]
	pinch!:     {claim!: string & !="", citation!: string} // C3: no consequence without a mechanism
	close!:     string & !="" // points at the receipts, never at the listener's opinion
}

#Coverage: {selected!: int, skipped!: int & >=0, repos_scanned!: int & >=0, repos_used!: int & >=0}

// #Claim — one sentence of the script and the receipt under it. `quote` must appear VERBATIM in the
// source named by `source_id`; that cross-reference is the one check a type cannot express, so the Go
// gate in validate.go owns it (C4: the anti-fabrication gate).
#Claim: {
	claim_id!:  string & !=""
	source_id!: string & !="" // a #Source.id — the citation
	quote!:     string & !="" // verbatim in that source
}

// #TypedOutputs — the writing stage's output as ONE unit (§13.8): all four blocks are required, so a
// missing block is a validation failure rather than a silent omission. `script` is emitted from
// #Episode; `sources`/`coverage`/`gaps` are the provenance the episode publishes.
#TypedOutputs: {
	script!:   string & !=""
	sources!:  [#Claim, ...#Claim] // at least one receipt; the prefix form keeps Go's []Claim intact
	coverage!: #Coverage
	gaps!:     [...string] // may be empty ONLY when the bundle leaves nothing open
}
