package podcast

// a complete episode: MUST validate
validEpisode: #Episode & {
	front_matter: {
		episode: "sample"
		topic: "a topic"
		profile: "news-desk"
		cutoff: "2026.274"
		cast: {CONCH: {sid: 1, voice: "af_bella"}, TEMPER: {sid: 6, voice: "am_michael"}}
		sources: 9
		coverage: {selected: 4, skipped: 592, repos_scanned: 243, repos_used: 9}
		gaps: ["what the window could not answer"]
	}
	cold_open: "the topic, in words a listener can hear"
	segments: [
		{
			title: "s1"
			mechanism: "what the sources say about s1"
			receipt: "a verbatim passage from the named source"
			receipt_source: "src-1"
			unlock: "an agent can now do X that it could not do before, because of s1"
			honest_limit: "this does not establish anything about Y"
			comic_beat: "the laugh lives here"
		},
		{
			title: "s2"
			mechanism: "what the sources say about s2"
			receipt: "a verbatim passage from the named source"
			receipt_source: "src-1"
			unlock: "an agent can now do X that it could not do before, because of s2"
			honest_limit: "this does not establish anything about Y"
			comic_beat: "the laugh lives here"
		},
		{
			title: "s3"
			mechanism: "what the sources say about s3"
			receipt: "a verbatim passage from the named source"
			receipt_source: "src-1"
			unlock: "an agent can now do X that it could not do before, because of s3"
			honest_limit: "this does not establish anything about Y"
			comic_beat: "the laugh lives here"
		}
	]
	pinch: {claim: "the one consequence claim", citation: "src-1"}
	close: "point at the receipts"
}
