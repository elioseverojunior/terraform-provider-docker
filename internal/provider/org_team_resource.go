package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/elioseverojunior/terraform-provider-docker/internal/dockerhub"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &OrgTeamResource{}
	_ resource.ResourceWithImportState = &OrgTeamResource{}
)

type OrgTeamResource struct {
	hubClient *dockerhub.Client
}

type OrgTeamResourceModel struct {
	ID              tftypes.String `tfsdk:"id"`
	OrgName         tftypes.String `tfsdk:"org_name"`
	TeamName        tftypes.String `tfsdk:"team_name"`
	TeamDescription tftypes.String `tfsdk:"team_description"`
	MemberCount     tftypes.Int64  `tfsdk:"member_count"`
}

func NewOrgTeamResource() resource.Resource {
	return &OrgTeamResource{}
}

func (r *OrgTeamResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_team"
}

func (r *OrgTeamResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Docker Hub organization teams.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource (org_name/team_name).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_name": schema.StringAttribute{
				Description: "The Docker Hub organization name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"team_name": schema.StringAttribute{
				Description: "The team name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"team_description": schema.StringAttribute{
				Description: "Description of the team.",
				Optional:    true,
			},
			"member_count": schema.Int64Attribute{
				Description: "Number of members in the team.",
				Computed:    true,
			},
		},
	}
}

func (r *OrgTeamResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *ProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.hubClient = providerData.HubClient
}

func (r *OrgTeamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OrgTeamResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to manage organization teams. Configure hub_username and hub_password in the provider.",
		)
		return
	}

	team := &dockerhub.Team{
		Name:        data.TeamName.ValueString(),
		Description: data.TeamDescription.ValueString(),
	}

	tflog.Debug(ctx, "Creating Docker Hub organization team", map[string]interface{}{
		"org":  data.OrgName.ValueString(),
		"team": team.Name,
	})

	result, err := r.hubClient.CreateTeam(ctx, data.OrgName.ValueString(), team)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Hub Team Creation Failed",
			fmt.Sprintf("Failed to create team %s in org %s: %s", team.Name, data.OrgName.ValueString(), err),
		)
		return
	}

	data.ID = tftypes.StringValue(fmt.Sprintf("%s/%s", data.OrgName.ValueString(), result.Name))
	data.MemberCount = tftypes.Int64Value(int64(result.MemberCount))

	tflog.Debug(ctx, "Created Docker Hub organization team", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrgTeamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OrgTeamResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to read organization teams.",
		)
		return
	}

	orgName := data.OrgName.ValueString()
	teamName := data.TeamName.ValueString()

	result, err := r.hubClient.GetTeam(ctx, orgName, teamName)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			tflog.Debug(ctx, "Team not found, removing from state", map[string]interface{}{
				"org":  orgName,
				"team": teamName,
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Docker Hub Team Read Failed",
			fmt.Sprintf("Failed to read team %s in org %s: %s", teamName, orgName, err),
		)
		return
	}

	data.TeamDescription = tftypes.StringValue(result.Description)
	data.MemberCount = tftypes.Int64Value(int64(result.MemberCount))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrgTeamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data OrgTeamResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to update organization teams.",
		)
		return
	}

	team := &dockerhub.Team{
		Name:        data.TeamName.ValueString(),
		Description: data.TeamDescription.ValueString(),
	}

	tflog.Debug(ctx, "Updating Docker Hub organization team", map[string]interface{}{
		"org":  data.OrgName.ValueString(),
		"team": team.Name,
	})

	result, err := r.hubClient.UpdateTeam(ctx, data.OrgName.ValueString(), team)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Hub Team Update Failed",
			fmt.Sprintf("Failed to update team %s in org %s: %s", team.Name, data.OrgName.ValueString(), err),
		)
		return
	}

	data.MemberCount = tftypes.Int64Value(int64(result.MemberCount))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrgTeamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OrgTeamResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to delete organization teams.",
		)
		return
	}

	orgName := data.OrgName.ValueString()
	teamName := data.TeamName.ValueString()

	tflog.Debug(ctx, "Deleting Docker Hub organization team", map[string]interface{}{
		"org":  orgName,
		"team": teamName,
	})

	err := r.hubClient.DeleteTeam(ctx, orgName, teamName)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			tflog.Debug(ctx, "Team already deleted", map[string]interface{}{
				"org":  orgName,
				"team": teamName,
			})
			return
		}
		resp.Diagnostics.AddError(
			"Docker Hub Team Deletion Failed",
			fmt.Sprintf("Failed to delete team %s in org %s: %s", teamName, orgName, err),
		)
		return
	}

	tflog.Debug(ctx, "Deleted Docker Hub organization team", map[string]interface{}{
		"org":  orgName,
		"team": teamName,
	})
}

func (r *OrgTeamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by org_name/team_name
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected format: org_name/team_name, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("org_name"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team_name"), parts[1])...)
}
