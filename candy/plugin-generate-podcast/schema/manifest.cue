package podcast

#RenderManifest: {
	renderer!:        "podcast-render"
	source!:          string
	output_wav!:      string
	output_m4a!:      string
	voices!:          [NAME=string]: {sid!: int & >=0, voice!: string}
	total_duration_s!: number & >0
	segments!: [...{
		index!:      int & >0
		kind!:       "speech" | "pause"
		sid?:        int & >=0 // required when kind == "speech"
		voice?:      string
		duration_s!: number & >0
		text?:       string
	}]
}
