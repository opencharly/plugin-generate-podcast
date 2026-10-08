package podcast

// #DeskConfig — desk.yml. Closed: an unknown key is an error, not a shrug.
#DeskConfig: {
	profile!: string
	topic!:   string
	cutoff!:  string & =~"^[0-9]{4}\\.[0-9]{3}$" // CalVer floor, e.g. "2026.274"
	select?:  int & >=3 & <=5 | *4
	minutes?: int & >0 | *12
	audio?:   bool | *true
	out!:     string
	cast!: [NAME=string]: string // speaker NAME -> voice name ("af_bella") or numeric sid ("1")
}
