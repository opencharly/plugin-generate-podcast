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
	segments!:  [...#Segment] // the ELEMENT type lives here and survives generation
	_min:      len(segments) >= 3
	_max:      len(segments) <= 5
	pinch!:     {claim!: string & !="", citation!: string} // C3: no consequence without a mechanism
	close!:     string & !="" // points at the receipts, never at the listener's opinion
}

#Coverage: {selected!: int, skipped!: int & >=0, repos_scanned!: int & >=0, repos_used!: int & >=0}
