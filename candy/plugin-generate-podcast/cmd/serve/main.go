package main

import (
	plugingenpodcast "github.com/opencharly/plugin-generate-podcast/candy/plugin-generate-podcast"
	"github.com/opencharly/sdk"
)

func main() {
	sdk.Main(plugingenpodcast.NewProvider(), plugingenpodcast.NewMeta(), plugingenpodcast.CliMain)
}
