package podcast

#Resolver: "file" | "glob" | "changelog-window" | "git-log" | "issues" | "prs" | "doc" | "doctrine" | "episode"

// #Source — one RESOLVED source. `digest` is mandatory: a source without a digest is not a receipt.
#Source: {
	id!:       string
	resolver!: #Resolver
	ref!:      string
	digest!:   string & =~"^[0-9a-f]{64}$"
	bytes!:    int & >0
	repo?:     string
	calver?:   string & =~"^[0-9]{4}\\.[0-9]{3}\\.[0-9]{4}$"
	path?:     string
	title?:    string
	body?:     string
}

#TagGap: {repo!: string, tag!: string, calver!: string}

// #SourceBundle — recorded with the episode; the bundle is the record, never the query.
#SourceBundle: {
	cutoff!:             string
	generated_at!:       string
	repos_scanned!:      int & >=0
	repos_with_entries!: int & >=0
	// At least one source, or the run is refused (§5.4). MEASURED: this needs the PREFIX-list form.
	// `[...#Source]` alone accepts `[]`; `[...#Source] & [_, ...]` rejects it but INLINES an anonymous
	// struct, so Go callers cannot pass []params.Source; and a hidden `_min: len(entries) >= 1` is a
	// no-op -- the comparison evaluates to the boolean VALUE false, which CUE accepts, so nothing fails.
	// `[#Source, ...#Source]` rejects `[]` AND generates `[]Source`.
	entries!:            [#Source, ...#Source]
	tag_gaps!:           [...#TagGap]
}
