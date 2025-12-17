package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ImageDataSource{}

type ImageDataSource struct {
	client *docker.Client
}

type ImageDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	ImageID     types.String `tfsdk:"image_id"`
	RepoDigests types.List   `tfsdk:"repo_digests"`
	RepoTags    types.List   `tfsdk:"repo_tags"`
}

func NewImageDataSource() datasource.DataSource {
	return &ImageDataSource{}
}

func (d *ImageDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_image"
}

func (d *ImageDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to get information about a Docker image.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the Docker image, including any tags or SHA256 repo digests.",
				Required:    true,
			},
			"image_id": schema.StringAttribute{
				Description: "The ID of the image.",
				Computed:    true,
			},
			"repo_digests": schema.ListAttribute{
				Description: "The image repo digests.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"repo_tags": schema.ListAttribute{
				Description: "The image repo tags.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *ImageDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ImageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ImageDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	imageName := data.Name.ValueString()

	imageInspect, _, err := d.client.ImageInspectWithRaw(ctx, imageName)
	if err != nil {
		if strings.Contains(err.Error(), "No such image") {
			resp.Diagnostics.AddError("Image Not Found", fmt.Sprintf("Image %s not found", imageName))
			return
		}
		resp.Diagnostics.AddError("Image Read Error", fmt.Sprintf("Unable to read image %s: %s", imageName, err))
		return
	}

	data.ID = types.StringValue(imageInspect.ID)
	data.ImageID = types.StringValue(imageInspect.ID)

	// Repo digests
	repoDigests, diags := types.ListValueFrom(ctx, types.StringType, imageInspect.RepoDigests)
	resp.Diagnostics.Append(diags...)
	data.RepoDigests = repoDigests

	// Repo tags
	repoTags, diags := types.ListValueFrom(ctx, types.StringType, imageInspect.RepoTags)
	resp.Diagnostics.Append(diags...)
	data.RepoTags = repoTags

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
