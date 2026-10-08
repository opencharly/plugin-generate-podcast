package generatepodcast

import (
	"context"
	"embed"
	"fmt"

	"github.com/opencharly/sdk"
	pb "github.com/opencharly/spec/proto"
)

//go:embed schema/*.cue
var schemaFS embed.FS

const calver = "2026.281.0000"

// NewProvider returns the gRPC provider. Capability dispatch lands in Invoke below.
func NewProvider() pb.ProviderServer { return &provider{} }

// NewMeta declares the ONE capability this plugin serves. `plugin.providers:` in the candy manifest is
// this same claim in authored form, and both render into the plugin's published docs page -- so the word
// and the code that answers it ship together or not at all.
func NewMeta() pb.PluginMetaServer {
	return sdk.NewMeta(calver, []sdk.ProvidedCapability{
		{Class: "command", Word: "generate-podcast"},
	}, schemaFS)
}

type provider struct{ pb.UnimplementedProviderServer }

// Invoke is the single-shot dispatch surface. The desk's own work is CLI-shaped (`charly generate-podcast
// emit|resolve|…`), so Invoke answers the ops the host can drive without a CLI: currently none -- a call
// here returns an explicit error rather than a silent empty reply.
func (p *provider) Invoke(_ context.Context, req *pb.InvokeRequest) (*pb.InvokeReply, error) {
	return nil, fmt.Errorf("generate-podcast: op %q is not served over Invoke; use the command face (`charly generate-podcast`)", req.GetOp())
}
