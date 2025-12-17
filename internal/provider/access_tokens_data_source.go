package provider

import (
	"context"
	"fmt"

	"github.com/elioseverojunior/terraform-provider-docker/internal/dockerhub"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &AccessTokensDataSource{}

type AccessTokensDataSource struct {
	hubClient *dockerhub.Client
}

type AccessTokensDataSourceModel struct {
	ID     types.String            `tfsdk:"id"`
	Tokens []AccessTokenModel      `tfsdk:"tokens"`
}

type AccessTokenModel struct {
	UUID        types.String `tfsdk:"uuid"`
	Label       types.String `tfsdk:"label"`
	IsActive    types.Bool   `tfsdk:"is_active"`
	CreatedAt   types.String `tfsdk:"created_at"`
	LastUsedAt  types.String `tfsdk:"last_used_at"`
	GeneratedBy types.String `tfsdk:"generated_by"`
}

func NewAccessTokensDataSource() datasource.DataSource {
	return &AccessTokensDataSource{}
}

func (d *AccessTokensDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_tokens"
}

func (d *AccessTokensDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists Docker Hub Personal Access Tokens.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this data source.",
				Computed:    true,
			},
			"tokens": schema.ListNestedAttribute{
				Description: "List of access tokens.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"uuid": schema.StringAttribute{
							Description: "The UUID of the token.",
							Computed:    true,
						},
						"label": schema.StringAttribute{
							Description: "The label/description of the token.",
							Computed:    true,
						},
						"is_active": schema.BoolAttribute{
							Description: "Whether the token is active.",
							Computed:    true,
						},
						"created_at": schema.StringAttribute{
							Description: "When the token was created.",
							Computed:    true,
						},
						"last_used_at": schema.StringAttribute{
							Description: "When the token was last used.",
							Computed:    true,
						},
						"generated_by": schema.StringAttribute{
							Description: "How the token was generated.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *AccessTokensDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AccessTokensDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data AccessTokensDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to list access tokens.",
		)
		return
	}

	// Fetch all tokens (paginated)
	var allTokens []dockerhub.AccessToken
	page := 1
	pageSize := 100

	for {
		tokens, count, err := d.hubClient.ListAccessTokens(ctx, page, pageSize)
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to List Access Tokens",
				fmt.Sprintf("Unable to list access tokens: %s", err),
			)
			return
		}

		allTokens = append(allTokens, tokens...)

		if len(allTokens) >= count || len(tokens) == 0 {
			break
		}
		page++
	}

	data.ID = types.StringValue("access_tokens")
	data.Tokens = make([]AccessTokenModel, len(allTokens))

	for i, token := range allTokens {
		createdAt := ""
		if !token.CreatedAt.IsZero() {
			createdAt = token.CreatedAt.Format("2006-01-02T15:04:05Z")
		}
		lastUsedAt := ""
		if !token.LastUsedAt.IsZero() {
			lastUsedAt = token.LastUsedAt.Format("2006-01-02T15:04:05Z")
		}

		data.Tokens[i] = AccessTokenModel{
			UUID:        types.StringValue(token.UUID),
			Label:       types.StringValue(token.Label),
			IsActive:    types.BoolValue(token.IsActive),
			CreatedAt:   types.StringValue(createdAt),
			LastUsedAt:  types.StringValue(lastUsedAt),
			GeneratedBy: types.StringValue(token.GeneratedBy),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
