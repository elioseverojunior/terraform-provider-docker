package provider

import (
	"context"
	"fmt"

	"github.com/elioseverojunior/terraform-provider-docker/internal/dockerhub"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &HubRepositoriesDataSource{}

type HubRepositoriesDataSource struct {
	hubClient *dockerhub.Client
}

type HubRepositoriesDataSourceModel struct {
	ID           types.String                   `tfsdk:"id"`
	Namespace    types.String                   `tfsdk:"namespace"`
	Repositories []HubRepositoryDataSourceModel `tfsdk:"repositories"`
}

func NewHubRepositoriesDataSource() datasource.DataSource {
	return &HubRepositoriesDataSource{}
}

func (d *HubRepositoriesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hub_repositories"
}

func (d *HubRepositoriesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists Docker Hub repositories for a namespace.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this data source.",
				Computed:    true,
			},
			"namespace": schema.StringAttribute{
				Description: "The Docker Hub namespace (username or organization name).",
				Required:    true,
			},
			"repositories": schema.ListNestedAttribute{
				Description: "List of repositories in the namespace.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The repository ID.",
							Computed:    true,
						},
						"namespace": schema.StringAttribute{
							Description: "The namespace.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The repository name.",
							Computed:    true,
						},
						"description": schema.StringAttribute{
							Description: "Short description.",
							Computed:    true,
						},
						"full_description": schema.StringAttribute{
							Description: "Full description.",
							Computed:    true,
						},
						"private": schema.BoolAttribute{
							Description: "Whether the repository is private.",
							Computed:    true,
						},
						"pull_count": schema.Int64Attribute{
							Description: "Number of pulls.",
							Computed:    true,
						},
						"star_count": schema.Int64Attribute{
							Description: "Number of stars.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *HubRepositoriesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *HubRepositoriesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data HubRepositoriesDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to list repositories.",
		)
		return
	}

	namespace := data.Namespace.ValueString()

	// Fetch all repositories (paginated)
	var allRepos []dockerhub.Repository
	page := 1
	pageSize := 100

	for {
		repos, count, err := d.hubClient.ListRepositories(ctx, namespace, page, pageSize)
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to List Repositories",
				fmt.Sprintf("Unable to list repositories for %s: %s", namespace, err),
			)
			return
		}

		allRepos = append(allRepos, repos...)

		if len(allRepos) >= count || len(repos) == 0 {
			break
		}
		page++
	}

	data.ID = types.StringValue(namespace)
	data.Repositories = make([]HubRepositoryDataSourceModel, len(allRepos))

	for i, repo := range allRepos {
		data.Repositories[i] = HubRepositoryDataSourceModel{
			ID:              types.StringValue(fmt.Sprintf("%s/%s", repo.Namespace, repo.Name)),
			Namespace:       types.StringValue(repo.Namespace),
			Name:            types.StringValue(repo.Name),
			Description:     types.StringValue(repo.Description),
			FullDescription: types.StringValue(repo.FullDescription),
			Private:         types.BoolValue(repo.IsPrivate),
			PullCount:       types.Int64Value(repo.PullCount),
			StarCount:       types.Int64Value(repo.StarCount),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
