package provider

import (
	"context"
	"fmt"

	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &RegistryImageDataSource{}

type RegistryImageDataSource struct {
	client *docker.Client
}

type RegistryImageDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	SHA256Digest types.String `tfsdk:"sha256_digest"`
}

func NewRegistryImageDataSource() datasource.DataSource {
	return &RegistryImageDataSource{}
}

func (d *RegistryImageDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_registry_image"
}

func (d *RegistryImageDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads the digest of a Docker image from a registry.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this data source.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the Docker image, including any tags or digest (e.g., 'nginx:latest', 'nginx@sha256:...').",
				Required:    true,
			},
			"sha256_digest": schema.StringAttribute{
				Description: "The sha256 digest of the image.",
				Computed:    true,
			},
		},
	}
}

func (d *RegistryImageDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *ProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = providerData.DockerClient
}

func (d *RegistryImageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RegistryImageDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	imageName := data.Name.ValueString()

	// Inspect the local image to get its digest
	inspect, _, err := d.client.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to Inspect Image",
			fmt.Sprintf("Unable to inspect image %s: %s. Make sure the image exists locally.", imageName, err),
		)
		return
	}

	data.ID = types.StringValue(inspect.ID)

	// Get digest from RepoDigests if available
	if len(inspect.RepoDigests) > 0 {
		// RepoDigests are in format "repo@sha256:..."
		data.SHA256Digest = types.StringValue(inspect.RepoDigests[0])
	} else {
		// Fall back to image ID
		data.SHA256Digest = types.StringValue(inspect.ID)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
