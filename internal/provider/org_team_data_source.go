package provider

import (
	"context"
	"fmt"

	"github.com/elioseverojunior/terraform-provider-docker/internal/dockerhub"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &OrgTeamDataSource{}

type OrgTeamDataSource struct {
	hubClient *dockerhub.Client
}

type OrgTeamDataSourceModel struct {
	ID              types.String `tfsdk:"id"`
	OrgName         types.String `tfsdk:"org_name"`
	TeamName        types.String `tfsdk:"team_name"`
	TeamDescription types.String `tfsdk:"team_description"`
	MemberCount     types.Int64  `tfsdk:"member_count"`
}

func NewOrgTeamDataSource() datasource.DataSource {
	return &OrgTeamDataSource{}
}

func (d *OrgTeamDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_team"
}

func (d *OrgTeamDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads information about a Docker Hub organization team.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this data source (org_name/team_name).",
				Computed:    true,
			},
			"org_name": schema.StringAttribute{
				Description: "The organization name.",
				Required:    true,
			},
			"team_name": schema.StringAttribute{
				Description: "The team name.",
				Required:    true,
			},
			"team_description": schema.StringAttribute{
				Description: "The team description.",
				Computed:    true,
			},
			"member_count": schema.Int64Attribute{
				Description: "Number of members in the team.",
				Computed:    true,
			},
		},
	}
}

func (d *OrgTeamDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *OrgTeamDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data OrgTeamDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to read team information.",
		)
		return
	}

	orgName := data.OrgName.ValueString()
	teamName := data.TeamName.ValueString()

	result, err := d.hubClient.GetTeam(ctx, orgName, teamName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to Read Team",
			fmt.Sprintf("Unable to read team %s in org %s: %s", teamName, orgName, err),
		)
		return
	}

	data.ID = types.StringValue(fmt.Sprintf("%s/%s", orgName, teamName))
	data.TeamDescription = types.StringValue(result.Description)
	data.MemberCount = types.Int64Value(int64(result.MemberCount))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
