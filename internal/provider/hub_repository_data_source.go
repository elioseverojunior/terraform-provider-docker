package provider

import (
	"context"
	"fmt"

	"github.com/elioseverojunior/terraform-provider-docker/internal/dockerhub"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &HubRepositoryDataSource{}

type HubRepositoryDataSource struct {
	hubClient *dockerhub.Client
}

type HubRepositoryDataSourceModel struct {
	ID              types.String `tfsdk:"id"`
	Namespace       types.String `tfsdk:"namespace"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	FullDescription types.String `tfsdk:"full_description"`
	Private         types.Bool   `tfsdk:"private"`
	PullCount       types.Int64  `tfsdk:"pull_count"`
	StarCount       types.Int64  `tfsdk:"star_count"`
}

func NewHubRepositoryDataSource() datasource.DataSource {
	return &HubRepositoryDataSource{}
}

func (d *HubRepositoryDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hub_repository"
}

func (d *HubRepositoryDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads information about a Docker Hub repository.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this data source (namespace/name).",
				Computed:    true,
			},
			"namespace": schema.StringAttribute{
				Description: "The Docker Hub namespace (username or organization name).",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The repository name.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "Short description of the repository.",
				Computed:    true,
			},
			"full_description": schema.StringAttribute{
				Description: "Full description of the repository (markdown).",
				Computed:    true,
			},
			"private": schema.BoolAttribute{
				Description: "Whether the repository is private.",
				Computed:    true,
			},
			"pull_count": schema.Int64Attribute{
				Description: "Number of pulls for this repository.",
				Computed:    true,
			},
			"star_count": schema.Int64Attribute{
				Description: "Number of stars for this repository.",
				Computed:    true,
			},
		},
	}
}

func (d *HubRepositoryDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

	d.hubClient = providerData.HubClient
}

func (d *HubRepositoryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data HubRepositoryDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to read repository information.",
		)
		return
	}

	namespace := data.Namespace.ValueString()
	name := data.Name.ValueString()

	result, err := d.hubClient.GetRepository(ctx, namespace, name)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to Read Repository",
			fmt.Sprintf("Unable to read repository %s/%s: %s", namespace, name, err),
		)
		return
	}

	data.ID = types.StringValue(fmt.Sprintf("%s/%s", namespace, name))
	data.Description = types.StringValue(result.Description)
	data.FullDescription = types.StringValue(result.FullDescription)
	data.Private = types.BoolValue(result.IsPrivate)
	data.PullCount = types.Int64Value(result.PullCount)
	data.StarCount = types.Int64Value(result.StarCount)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
