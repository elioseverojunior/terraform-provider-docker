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
	_ resource.Resource                = &HubRepositoryTeamPermissionResource{}
	_ resource.ResourceWithImportState = &HubRepositoryTeamPermissionResource{}
)

type HubRepositoryTeamPermissionResource struct {
	hubClient *dockerhub.Client
}

type HubRepositoryTeamPermissionResourceModel struct {
	ID         tftypes.String `tfsdk:"id"`
	Namespace  tftypes.String `tfsdk:"namespace"`
	Repository tftypes.String `tfsdk:"repository"`
	TeamName   tftypes.String `tfsdk:"team_name"`
	Permission tftypes.String `tfsdk:"permission"`
}

func NewHubRepositoryTeamPermissionResource() resource.Resource {
	return &HubRepositoryTeamPermissionResource{}
}

func (r *HubRepositoryTeamPermissionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hub_repository_team_permission"
}

func (r *HubRepositoryTeamPermissionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Docker Hub repository team permissions.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource (namespace/repository/team_name).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"namespace": schema.StringAttribute{
				Description: "The Docker Hub namespace (organization name).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"repository": schema.StringAttribute{
				Description: "The repository name.",
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
			"permission": schema.StringAttribute{
				Description: "The permission level. Valid values: read, write, admin.",
				Required:    true,
			},
		},
	}
}

func (r *HubRepositoryTeamPermissionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *HubRepositoryTeamPermissionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data HubRepositoryTeamPermissionResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to manage repository permissions. Configure hub_username and hub_password in the provider.",
		)
		return
	}

	namespace := data.Namespace.ValueString()
	repository := data.Repository.ValueString()
	teamName := data.TeamName.ValueString()
	permission := data.Permission.ValueString()

	tflog.Debug(ctx, "Setting Docker Hub repository team permission", map[string]interface{}{
		"namespace":  namespace,
		"repository": repository,
		"team":       teamName,
		"permission": permission,
	})

	err := r.hubClient.SetRepositoryTeamPermission(ctx, namespace, repository, teamName, permission)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Hub Repository Team Permission Failed",
			fmt.Sprintf("Failed to set permission for team %s on repo %s/%s: %s", teamName, namespace, repository, err),
		)
		return
	}

	data.ID = tftypes.StringValue(fmt.Sprintf("%s/%s/%s", namespace, repository, teamName))

	tflog.Debug(ctx, "Set Docker Hub repository team permission", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *HubRepositoryTeamPermissionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data HubRepositoryTeamPermissionResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to read repository permissions.",
		)
		return
	}

	namespace := data.Namespace.ValueString()
	repository := data.Repository.ValueString()
	teamName := data.TeamName.ValueString()

	permission, err := r.hubClient.GetRepositoryTeamPermission(ctx, namespace, repository, teamName)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			tflog.Debug(ctx, "Repository team permission not found, removing from state", map[string]interface{}{
				"namespace":  namespace,
				"repository": repository,
				"team":       teamName,
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Docker Hub Repository Team Permission Read Failed",
			fmt.Sprintf("Failed to read permission for team %s on repo %s/%s: %s", teamName, namespace, repository, err),
		)
		return
	}

	data.Permission = tftypes.StringValue(permission.Permission)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *HubRepositoryTeamPermissionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data HubRepositoryTeamPermissionResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to update repository permissions.",
		)
		return
	}

	namespace := data.Namespace.ValueString()
	repository := data.Repository.ValueString()
	teamName := data.TeamName.ValueString()
	permission := data.Permission.ValueString()

	tflog.Debug(ctx, "Updating Docker Hub repository team permission", map[string]interface{}{
		"namespace":  namespace,
		"repository": repository,
		"team":       teamName,
		"permission": permission,
	})

	err := r.hubClient.SetRepositoryTeamPermission(ctx, namespace, repository, teamName, permission)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Hub Repository Team Permission Update Failed",
			fmt.Sprintf("Failed to update permission for team %s on repo %s/%s: %s", teamName, namespace, repository, err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *HubRepositoryTeamPermissionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data HubRepositoryTeamPermissionResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to delete repository permissions.",
		)
		return
	}

	namespace := data.Namespace.ValueString()
	repository := data.Repository.ValueString()
	teamName := data.TeamName.ValueString()

	tflog.Debug(ctx, "Removing Docker Hub repository team permission", map[string]interface{}{
		"namespace":  namespace,
		"repository": repository,
		"team":       teamName,
	})

	err := r.hubClient.RemoveRepositoryTeamPermission(ctx, namespace, repository, teamName)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			tflog.Debug(ctx, "Repository team permission already removed", map[string]interface{}{
				"namespace":  namespace,
				"repository": repository,
				"team":       teamName,
			})
			return
		}
		resp.Diagnostics.AddError(
			"Docker Hub Repository Team Permission Deletion Failed",
			fmt.Sprintf("Failed to remove permission for team %s on repo %s/%s: %s", teamName, namespace, repository, err),
		)
		return
	}

	tflog.Debug(ctx, "Removed Docker Hub repository team permission", map[string]interface{}{
		"namespace":  namespace,
		"repository": repository,
		"team":       teamName,
	})
}

func (r *HubRepositoryTeamPermissionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by namespace/repository/team_name
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected format: namespace/repository/team_name, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("namespace"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("repository"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team_name"), parts[2])...)
}
