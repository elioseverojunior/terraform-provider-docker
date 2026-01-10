package provider

import (
	"context"
	"fmt"

	"github.com/elioseverojunior/terraform-provider-docker/internal/dockerhub"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &HubRepositoryTagsDataSource{}

type HubRepositoryTagsDataSource struct {
	hubClient *dockerhub.Client
}

type HubRepositoryTagsDataSourceModel struct {
	ID        types.String            `tfsdk:"id"`
	Namespace types.String            `tfsdk:"namespace"`
	Name      types.String            `tfsdk:"name"`
	Tags      []HubRepositoryTagModel `tfsdk:"tags"`
}

type HubRepositoryTagModel struct {
	Name        types.String `tfsdk:"name"`
	FullSize    types.Int64  `tfsdk:"full_size"`
	LastUpdated types.String `tfsdk:"last_updated"`
	Digest      types.String `tfsdk:"digest"`
}

func NewHubRepositoryTagsDataSource() datasource.DataSource {
	return &HubRepositoryTagsDataSource{}
}

func (d *HubRepositoryTagsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hub_repository_tags"
}

func (d *HubRepositoryTagsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists tags for a Docker Hub repository.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this data source.",
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
			"tags": schema.ListNestedAttribute{
				Description: "List of tags in the repository.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The tag name.",
							Computed:    true,
						},
						"full_size": schema.Int64Attribute{
							Description: "The full size of the image in bytes.",
							Computed:    true,
						},
						"last_updated": schema.StringAttribute{
							Description: "When the tag was last updated.",
							Computed:    true,
						},
						"digest": schema.StringAttribute{
							Description: "The digest of the image.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *HubRepositoryTagsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *HubRepositoryTagsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data HubRepositoryTagsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to list repository tags.",
		)
		return
	}

	namespace := data.Namespace.ValueString()
	name := data.Name.ValueString()

	// Fetch all tags (paginated)
	var allTags []dockerhub.RepositoryTag
	page := 1
	pageSize := 100

	for {
		tags, count, err := d.hubClient.ListRepositoryTags(ctx, namespace, name, page, pageSize)
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to List Repository Tags",
				fmt.Sprintf("Unable to list tags for %s/%s: %s", namespace, name, err),
			)
			return
		}

		allTags = append(allTags, tags...)

		if len(allTags) >= count || len(tags) == 0 {
			break
		}
		page++
	}

	data.ID = types.StringValue(fmt.Sprintf("%s/%s", namespace, name))
	data.Tags = make([]HubRepositoryTagModel, len(allTags))

	for i, tag := range allTags {
		data.Tags[i] = HubRepositoryTagModel{
			Name:        types.StringValue(tag.Name),
			FullSize:    types.Int64Value(tag.FullSize),
			LastUpdated: types.StringValue(tag.LastUpdated.Format("2006-01-02T15:04:05Z")),
			Digest:      types.StringValue(tag.Digest),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
