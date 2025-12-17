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
	_ resource.Resource                = &OrgTeamMemberResource{}
	_ resource.ResourceWithImportState = &OrgTeamMemberResource{}
)

type OrgTeamMemberResource struct {
	hubClient *dockerhub.Client
}

type OrgTeamMemberResourceModel struct {
	ID       tftypes.String `tfsdk:"id"`
	OrgName  tftypes.String `tfsdk:"org_name"`
	TeamName tftypes.String `tfsdk:"team_name"`
	Username tftypes.String `tfsdk:"username"`
}

func NewOrgTeamMemberResource() resource.Resource {
	return &OrgTeamMemberResource{}
}

func (r *OrgTeamMemberResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_team_member"
}

func (r *OrgTeamMemberResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Docker Hub organization team members.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource (org_name/team_name/username).",
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
			"username": schema.StringAttribute{
				Description: "The username to add to the team.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *OrgTeamMemberResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrgTeamMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OrgTeamMemberResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to manage team members. Configure hub_username and hub_password in the provider.",
		)
		return
	}

	orgName := data.OrgName.ValueString()
	teamName := data.TeamName.ValueString()
	username := data.Username.ValueString()

	tflog.Debug(ctx, "Adding member to Docker Hub team", map[string]interface{}{
		"org":      orgName,
		"team":     teamName,
		"username": username,
	})

	err := r.hubClient.AddTeamMember(ctx, orgName, teamName, username)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Hub Team Member Add Failed",
			fmt.Sprintf("Failed to add member %s to team %s in org %s: %s", username, teamName, orgName, err),
		)
		return
	}

	data.ID = tftypes.StringValue(fmt.Sprintf("%s/%s/%s", orgName, teamName, username))

	tflog.Debug(ctx, "Added member to Docker Hub team", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrgTeamMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OrgTeamMemberResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to read team members.",
		)
		return
	}

	orgName := data.OrgName.ValueString()
	teamName := data.TeamName.ValueString()
	username := data.Username.ValueString()

	isMember, err := r.hubClient.IsTeamMember(ctx, orgName, teamName, username)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Hub Team Member Read Failed",
			fmt.Sprintf("Failed to check membership of %s in team %s org %s: %s", username, teamName, orgName, err),
		)
		return
	}

	if !isMember {
		tflog.Debug(ctx, "Team member not found, removing from state", map[string]interface{}{
			"org":      orgName,
			"team":     teamName,
			"username": username,
		})
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrgTeamMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Team membership is immutable - any change requires replacement
	// This should not be called due to RequiresReplace plan modifiers
	var data OrgTeamMemberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrgTeamMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OrgTeamMemberResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to delete team members.",
		)
		return
	}

	orgName := data.OrgName.ValueString()
	teamName := data.TeamName.ValueString()
	username := data.Username.ValueString()

	tflog.Debug(ctx, "Removing member from Docker Hub team", map[string]interface{}{
		"org":      orgName,
		"team":     teamName,
		"username": username,
	})

	err := r.hubClient.RemoveTeamMember(ctx, orgName, teamName, username)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			tflog.Debug(ctx, "Team member already removed", map[string]interface{}{
				"org":      orgName,
				"team":     teamName,
				"username": username,
			})
			return
		}
		resp.Diagnostics.AddError(
			"Docker Hub Team Member Deletion Failed",
			fmt.Sprintf("Failed to remove member %s from team %s in org %s: %s", username, teamName, orgName, err),
		)
		return
	}

	tflog.Debug(ctx, "Removed member from Docker Hub team", map[string]interface{}{
		"org":      orgName,
		"team":     teamName,
		"username": username,
	})
}

func (r *OrgTeamMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by org_name/team_name/username
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected format: org_name/team_name/username, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("org_name"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team_name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("username"), parts[2])...)
}
