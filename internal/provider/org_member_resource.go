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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &OrgMemberResource{}
	_ resource.ResourceWithImportState = &OrgMemberResource{}
)

type OrgMemberResource struct {
	hubClient *dockerhub.Client
}

type OrgMemberResourceModel struct {
	ID       tftypes.String `tfsdk:"id"`
	OrgName  tftypes.String `tfsdk:"org_name"`
	Username tftypes.String `tfsdk:"username"`
	Role     tftypes.String `tfsdk:"role"`
}

func NewOrgMemberResource() resource.Resource {
	return &OrgMemberResource{}
}

func (r *OrgMemberResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_member"
}

func (r *OrgMemberResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Docker Hub organization members.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource (org_name/username).",
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
			"username": schema.StringAttribute{
				Description: "The username to add to the organization.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Description: "The role of the member in the organization. Valid values: member, owner. Default: member.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("member"),
			},
		},
	}
}

func (r *OrgMemberResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *OrgMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data OrgMemberResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to manage organization members. Configure hub_username and hub_password in the provider.",
		)
		return
	}

	orgName := data.OrgName.ValueString()
	username := data.Username.ValueString()
	role := data.Role.ValueString()

	tflog.Debug(ctx, "Adding member to Docker Hub organization", map[string]interface{}{
		"org":      orgName,
		"username": username,
		"role":     role,
	})

	err := r.hubClient.AddOrgMember(ctx, orgName, username, role)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Hub Organization Member Add Failed",
			fmt.Sprintf("Failed to add member %s to org %s: %s", username, orgName, err),
		)
		return
	}

	data.ID = tftypes.StringValue(fmt.Sprintf("%s/%s", orgName, username))

	tflog.Debug(ctx, "Added member to Docker Hub organization", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrgMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data OrgMemberResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to read organization members.",
		)
		return
	}

	orgName := data.OrgName.ValueString()
	username := data.Username.ValueString()

	result, err := r.hubClient.GetOrgMember(ctx, orgName, username)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			tflog.Debug(ctx, "Organization member not found, removing from state", map[string]interface{}{
				"org":      orgName,
				"username": username,
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Docker Hub Organization Member Read Failed",
			fmt.Sprintf("Failed to read member %s in org %s: %s", username, orgName, err),
		)
		return
	}

	data.Role = tftypes.StringValue(result.Role)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrgMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data OrgMemberResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to update organization members.",
		)
		return
	}

	orgName := data.OrgName.ValueString()
	username := data.Username.ValueString()
	role := data.Role.ValueString()

	tflog.Debug(ctx, "Updating Docker Hub organization member", map[string]interface{}{
		"org":      orgName,
		"username": username,
		"role":     role,
	})

	err := r.hubClient.UpdateOrgMember(ctx, orgName, username, role)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Hub Organization Member Update Failed",
			fmt.Sprintf("Failed to update member %s in org %s: %s", username, orgName, err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OrgMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data OrgMemberResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to delete organization members.",
		)
		return
	}

	orgName := data.OrgName.ValueString()
	username := data.Username.ValueString()

	tflog.Debug(ctx, "Removing member from Docker Hub organization", map[string]interface{}{
		"org":      orgName,
		"username": username,
	})

	err := r.hubClient.RemoveOrgMember(ctx, orgName, username)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			tflog.Debug(ctx, "Organization member already removed", map[string]interface{}{
				"org":      orgName,
				"username": username,
			})
			return
		}
		resp.Diagnostics.AddError(
			"Docker Hub Organization Member Deletion Failed",
			fmt.Sprintf("Failed to remove member %s from org %s: %s", username, orgName, err),
		)
		return
	}

	tflog.Debug(ctx, "Removed member from Docker Hub organization", map[string]interface{}{
		"org":      orgName,
		"username": username,
	})
}

func (r *OrgMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by org_name/username
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected format: org_name/username, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("org_name"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("username"), parts[1])...)
}
